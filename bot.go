package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"sync"

	meshcore "github.com/meshcore-go/meshcore-go"
	"github.com/meshcore-go/meshcore-go/node"
)

type triggerEntry struct {
	trigger Trigger
	config  TriggerConfig
}

type Bot struct {
	name      string
	node      *node.Node
	radio     node.MuxRadio
	sender    Sender
	templater *Templater
	triggers  []triggerEntry

	mu     sync.Mutex
	cancel context.CancelFunc
}

func NewBot(cfg BotConfig, mux *node.RadioMux, sf SenderFactory) (*Bot, error) {
	if cfg.Name == nil || *cfg.Name == "" {
		return nil, fmt.Errorf("bot name is required")
	}
	botName := *cfg.Name

	identity := identityFromName(botName)
	radio := mux.NewRadio()
	n := node.New(identity, radio,
		node.WithErrorHandler(func(err error) {
			slog.Error("node error", "bot", botName, "error", err)
		}),
	)

	sender := sf(n)

	b := &Bot{
		name:      botName,
		node:      n,
		radio:     radio,
		sender:    sender,
		templater: NewTemplater(),
	}

	channelIdx := 0
	for _, trigCfg := range cfg.Triggers {
		entry, err := b.buildTrigger(trigCfg, channelIdx)
		if err != nil {
			return nil, fmt.Errorf("bot %q trigger %q: %w", botName, trigCfg.Type, err)
		}

		if trigCfg.Channels != nil {
			for _, chName := range *trigCfg.Channels {
				sender.RegisterChannel(channelIdx, chName)
				channelIdx++
			}
		}

		b.triggers = append(b.triggers, *entry)
	}

	return b, nil
}

func (b *Bot) buildTrigger(cfg TriggerConfig, startIdx int) (*triggerEntry, error) {
	var t Trigger
	var err error

	switch cfg.Type {
	case "group":
		t, err = NewGroupTrigger(b.name, cfg, b.node, startIdx)
	case "cron":
		t, err = NewCronTrigger(b.name, cfg)
	default:
		return nil, fmt.Errorf("unknown trigger type %q", cfg.Type)
	}
	if err != nil {
		return nil, err
	}

	return &triggerEntry{
		trigger: t,
		config:  cfg,
	}, nil
}

func (b *Bot) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	b.mu.Lock()
	b.cancel = cancel
	b.mu.Unlock()

	for i, entry := range b.triggers {
		e := entry
		if err := e.trigger.Start(ctx, b.makeCallback(ctx, e)); err != nil {
			cancel()
			return fmt.Errorf("starting trigger %d (%s): %w", i, e.config.Type, err)
		}
	}

	return nil
}

func (b *Bot) Stop() error {
	b.mu.Lock()
	if b.cancel != nil {
		b.cancel()
	}
	b.mu.Unlock()

	for _, entry := range b.triggers {
		entry.trigger.Stop()
	}
	b.node.Stop()
	return nil
}

func (b *Bot) makeCallback(ctx context.Context, entry triggerEntry) TriggerCallback {
	return func(evt TriggerEvent) {
		rendered, err := b.templater.Render(&evt, entry.config.Template)
		if err != nil {
			slog.Error("template error", "bot", b.name, "error", err)
			return
		}

		slog.Log(ctx, LevelTrace, "template rendered",
			"bot", b.name, "trigger", evt.Type, "output", rendered)

		switch evt.Type {
		case "group":
			chName, _ := evt.Data["Channel"].(string)
			slog.Debug("sending group txt", "bot", b.name, "channel", chName)
			if err := b.sender.SendGroupText(ctx, chName, b.name, rendered); err != nil {
				slog.Error("send error", "bot", b.name, "error", err)
			}
		case "cron":
			if entry.config.Channels != nil {
				for _, ch := range *entry.config.Channels {
					slog.Debug("sending group txt", "bot", b.name, "channel", ch)
					if err := b.sender.SendGroupText(ctx, ch, b.name, rendered); err != nil {
						slog.Error("send error", "bot", b.name, "error", err)
					}
				}
			}
		}
	}
}

func identityFromName(name string) meshcore.LocalIdentity {
	hash := sha256.Sum256([]byte(name))
	return meshcore.NewLocalIdentityFromSeed(hash)
}
