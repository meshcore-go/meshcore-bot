package main

import "context"

// TriggerEvent is emitted by a trigger and passed to the template engine.
type TriggerEvent struct {
	Cfg     TriggerConfig
	Type    string // group, dm, cron, cap, etc
	BotName string
	Data    map[string]any // Trigger-specific data available to templates
}

// TriggerCallback is called when a trigger fires.
type TriggerCallback func(TriggerEvent)

// Trigger is the interface all trigger types implement.
type Trigger interface {
	// Start begins listening/polling. The callback is invoked for each event.
	// The context controls the trigger's lifetime.
	Start(ctx context.Context, callback TriggerCallback) error

	// Stop shuts down the trigger and releases resources.
	Stop() error
}
