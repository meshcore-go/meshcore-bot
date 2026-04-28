package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	meshcore "github.com/meshcore-go/meshcore-go"
	"github.com/meshcore-go/meshcore-go/node"
)

type GroupTrigger struct {
	cfg      TriggerConfig
	botName  string
	node     *node.Node
	patterns []*regexp.Regexp
	channels map[string]bool // channel names this trigger listens on; nil = all

	mu       sync.Mutex
	callback TriggerCallback
	cancel   context.CancelFunc
}

func NewGroupTrigger(botName string, cfg TriggerConfig, n *node.Node, startChannelIdx int) (*GroupTrigger, error) {
	var patterns []*regexp.Regexp
	if cfg.Match != nil {
		patterns = make([]*regexp.Regexp, 0, len(*cfg.Match))
		for _, m := range *cfg.Match {
			re, err := regexp.Compile(m)
			if err != nil {
				return nil, fmt.Errorf("invalid match pattern %q: %w", m, err)
			}
			patterns = append(patterns, re)
		}
	}

	var channels map[string]bool
	if cfg.Channels != nil && len(*cfg.Channels) > 0 {
		channels = make(map[string]bool, len(*cfg.Channels))
		for i, ch := range *cfg.Channels {
			idx := startChannelIdx + i
			if strings.EqualFold(ch, "Public") {
				channels["Public"] = true
				pub, _ := meshcore.NewChannelFromBase64("Public", "izOH6cXN6mrJ5e26oRXNcg==")
				n.SetChannel(idx, pub)
			} else {
				nCh := meshcore.NormalizeHashtag(ch)
				channels[nCh] = true
				chEntry := meshcore.NewChannelFromHashtag(nCh)
				n.SetChannel(idx, chEntry)
			}
		}
	}

	return &GroupTrigger{
		cfg:      cfg,
		botName:  botName,
		node:     n,
		patterns: patterns,
		channels: channels,
	}, nil
}

func (t *GroupTrigger) Start(ctx context.Context, callback TriggerCallback) error {
	ctx, cancel := context.WithCancel(ctx)
	t.mu.Lock()
	t.callback = callback
	t.cancel = cancel
	t.mu.Unlock()

	t.node.OnPacket(meshcore.PayloadTypeGrpTxt, func(pkt *meshcore.Packet) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		t.handlePacket(pkt)
	})

	return nil
}

func (t *GroupTrigger) Stop() error {
	t.mu.Lock()
	if t.cancel != nil {
		t.cancel()
	}
	t.mu.Unlock()
	return nil
}

func (t *GroupTrigger) handlePacket(pkt *meshcore.Packet) {
	msg, ch, err := t.node.DecryptGroupText(pkt)
	if err != nil {
		return
	}

	if t.channels != nil && !t.channels[ch.Name] {
		return
	}

	captures := t.matchesAny(msg.Text)
	if captures == nil {
		return
	}

	t.mu.Lock()
	cb := t.callback
	t.mu.Unlock()
	if cb == nil {
		return
	}

	cb(TriggerEvent{
		Type:    "group",
		BotName: t.botName,
		Data: map[string]any{
			"Sender":       msg.Sender,
			"Channel":      ch.Name,
			"Message":      msg.Text,
			"Match":        captures,
			"Timestamp":    msg.Timestamp,
			"SNR":          pkt.SNR,
			"RSSI":         pkt.RSSI,
			"Hops":         pkt.PathHashCount(),
			"PathHashes":   pkt.PathHashes(),
			"PathHashSize": pkt.PathHashSize(),
		},
	})
}

// matchesAny returns the first matching pattern's named capture groups, or nil
// if no pattern matches. When there are no patterns, it returns an empty
// (non-nil) map to indicate a match-all.
func (t *GroupTrigger) matchesAny(text string) map[string]string {
	if len(t.patterns) == 0 {
		return map[string]string{} // no patterns = match everything
	}
	for _, re := range t.patterns {
		m := re.FindStringSubmatch(text)
		if m == nil {
			continue
		}
		captures := make(map[string]string)
		for i, name := range re.SubexpNames() {
			if i == 0 || name == "" {
				continue
			}
			captures[name] = m[i]
		}
		return captures
	}
	return nil
}

var _ Trigger = (*GroupTrigger)(nil)
