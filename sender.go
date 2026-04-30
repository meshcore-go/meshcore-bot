package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	meshcore "github.com/meshcore-go/meshcore-go"
	companionClient "github.com/meshcore-go/meshcore-go/companion/client"
	"github.com/meshcore-go/meshcore-go/node"
)

type Sender interface {
	SendGroupText(ctx context.Context, channel *meshcore.ChannelEntry, senderName string, text string, pathHashSize uint8) error
	RegisterChannel(idx int, channel *meshcore.ChannelEntry)
}

type SenderFactory func(n *node.Node) Sender

type NodeSender struct {
	node *node.Node
}

func NewNodeSender(n *node.Node) *NodeSender {
	return &NodeSender{node: n}
}

func (s *NodeSender) RegisterChannel(_ int, _ *meshcore.ChannelEntry) {}

func (s *NodeSender) SendGroupText(_ context.Context, channel *meshcore.ChannelEntry, senderName string, text string, pathHashSize uint8) error {
	reply := &meshcore.GroupTextPayload{
		Timestamp: uint32(time.Now().Unix()),
		Sender:    senderName,
		Text:      text,
	}

	gt, err := reply.Encrypt(channel.Hash, channel.PSK[:])
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}

	payload, err := gt.ToBytes()
	if err != nil {
		return fmt.Errorf("serialize: %w", err)
	}

	if pathHashSize < 1 {
		pathHashSize = 1
	}
	if pathHashSize > 4 {
		pathHashSize = 4
	}

	pkt := &meshcore.Packet{
		Header:     meshcore.MakeHeader(meshcore.RouteTypeFlood, meshcore.PayloadTypeGrpTxt, 0),
		PathLength: (pathHashSize - 1) << 6,
		Payload:    payload,
	}

	return s.node.SendPacket(pkt)
}

const maxDeviceChannels = 16

type CompanionSender struct {
	ctx    context.Context
	client *companionClient.Client

	mu       sync.RWMutex
	channels map[string]byte // normalized name -> device slot index
}

func NewCompanionSender(ctx context.Context, c *companionClient.Client) (*CompanionSender, error) {
	s := &CompanionSender{
		ctx:      ctx,
		client:   c,
		channels: make(map[string]byte),
	}

	if err := s.loadDeviceChannels(); err != nil {
		return nil, fmt.Errorf("loading device channels: %w", err)
	}

	return s, nil
}

func (s *CompanionSender) loadDeviceChannels() error {
	for i := range byte(maxDeviceChannels) {
		info, err := s.client.GetChannel(s.ctx, i)
		if err != nil {
			break
		}
		if info.Name == "" {
			continue
		}
		name := normalizeChannelName(info.Name)
		s.channels[name] = i
		slog.Debug("found device channel", "idx", i, "name", name)
	}
	return nil
}

func (s *CompanionSender) RegisterChannel(_ int, channel *meshcore.ChannelEntry) {
	name := normalizeChannelName(channel.Name)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.channels[name]; ok {
		return
	}

	idx, err := s.findEmptySlot()
	if err != nil {
		slog.Error("no empty channel slot", "channel", name, "error", err)
		return
	}

	if err := s.client.SetChannel(s.ctx, idx, name, channel.PSK); err != nil {
		slog.Error("failed to set channel on device", "channel", name, "idx", idx, "error", err)
		return
	}

	s.channels[name] = idx
	slog.Info("configured channel on device", "channel", name, "idx", idx)
}

func (s *CompanionSender) findEmptySlot() (byte, error) {
	used := make(map[byte]bool, len(s.channels))
	for _, idx := range s.channels {
		used[idx] = true
	}
	for i := range byte(maxDeviceChannels) {
		if !used[i] {
			return i, nil
		}
	}
	return 0, fmt.Errorf("all %d channel slots occupied", maxDeviceChannels)
}

func (s *CompanionSender) SendGroupText(ctx context.Context, channel *meshcore.ChannelEntry, _ string, text string, _ uint8) error {
	name := normalizeChannelName(channel.Name)

	s.mu.RLock()
	idx, ok := s.channels[name]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("channel %q not registered on companion device", channel.Name)
	}

	_, err := s.client.SendChannelTextMessage(ctx, idx, text, 0)
	return err
}

func normalizeChannelName(name string) string {
	if strings.EqualFold(name, "Public") {
		return "Public"
	}
	return meshcore.NormalizeHashtag(name)
}

var _ Sender = (*NodeSender)(nil)
var _ Sender = (*CompanionSender)(nil)
