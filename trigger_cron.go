package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

type CronTrigger struct {
	cfg      TriggerConfig
	botName  string
	cron     *cron.Cron
	mu       sync.Mutex
	callback TriggerCallback
}

func NewCronTrigger(botName string, cfg TriggerConfig) (*CronTrigger, error) {
	if cfg.Schedule == "" {
		return nil, fmt.Errorf("cron trigger requires a schedule")
	}
	return &CronTrigger{
		cfg:     cfg,
		botName: botName,
	}, nil
}

func (t *CronTrigger) Start(ctx context.Context, callback TriggerCallback) error {
	t.mu.Lock()
	t.callback = callback
	t.cron = cron.New()
	t.mu.Unlock()

	_, err := t.cron.AddFunc(t.cfg.Schedule, func() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		slog.Log(ctx, LevelTrace, "cron fired",
			"bot", t.botName, "schedule", t.cfg.Schedule)

		t.mu.Lock()
		cb := t.callback
		t.mu.Unlock()
		if cb == nil {
			return
		}

		cb(TriggerEvent{
			Type:    "cron",
			BotName: t.botName,
			Data: map[string]any{
				"Time":     time.Now(),
				"Schedule": t.cfg.Schedule,
			},
		})
	})
	if err != nil {
		return fmt.Errorf("invalid cron schedule %q: %w", t.cfg.Schedule, err)
	}

	t.cron.Start()

	go func() {
		<-ctx.Done()
		t.Stop()
	}()

	return nil
}

func (t *CronTrigger) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cron != nil {
		t.cron.Stop()
		t.cron = nil
	}
	return nil
}

var _ Trigger = (*CronTrigger)(nil)
