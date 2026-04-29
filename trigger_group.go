package main

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
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

func NewGroupTrigger(botName string, cfg TriggerConfig, n *node.Node, channels []*meshcore.ChannelEntry) (*GroupTrigger, error) {
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

	var channelFilter map[string]bool
	if len(channels) > 0 {
		channelFilter = make(map[string]bool, len(channels))
		for _, ch := range channels {
			channelFilter[ch.Name] = true
		}
	}

	return &GroupTrigger{
		cfg:      cfg,
		botName:  botName,
		node:     n,
		patterns: patterns,
		channels: channelFilter,
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
		slog.Log(context.Background(), LevelTrace, "group decrypt failed",
			"bot", t.botName, "error", err)
		return
	}

	slog.Log(context.Background(), LevelTrace, "group message received",
		"bot", t.botName, "channel", ch.Name, "sender", msg.Sender,
		"text", msg.Text, "snr", pkt.SNR, "rssi", pkt.RSSI)

	if t.channels != nil && !t.channels[ch.Name] {
		slog.Log(context.Background(), LevelTrace, "channel not matched, skipping",
			"bot", t.botName, "channel", ch.Name)
		return
	}

	captures := t.matchesAny(msg.Text)
	if captures == nil {
		slog.Log(context.Background(), LevelTrace, "no pattern matched",
			"bot", t.botName, "text", msg.Text)
		return
	}

	slog.Log(context.Background(), LevelTrace, "trigger matched",
		"bot", t.botName, "captures", captures)

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
			"ChannelEntry": ch,
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
		slog.Log(context.Background(), LevelTrace, "regex check",
			"bot", t.botName, "pattern", re.String(),
			"text", text, "matched", m != nil)
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
