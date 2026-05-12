package shared

import (
	"context"
	"time"
)

type Event interface {
	EventName() string
}

type EventBus interface {
	Publish(ctx context.Context, event Event) error
	Subscribe(eventName string, handler func(ctx context.Context, event Event)) string
	Unsubscribe(id string)
}

// ServerConfigChanged is emitted whenever an admin (or boot bootstrap)
// writes a key to the generic server_config store. Subscribers that hold
// runtime state derived from a config key (auto-download toggle, intro
// providers enabled, etc.) listen on the matching key to apply the new
// value without forcing a server restart. Empty Value means the key was
// deleted (handlers may treat that as "fall back to default").
type ServerConfigChanged struct {
	Key        string
	Value      string
	Deleted    bool
	OccurredAt time.Time
}

func (e ServerConfigChanged) EventName() string { return "server.config.changed" }
