package shared

import (
	"context"
	"encoding/json"
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
	// Secret: Value is a credential. In-process subscribers read it as is;
	// every JSON copy (a webhook on "*") carries a placeholder instead.
	Secret bool `json:"-"`
}

// MarshalJSON keeps the historical wire shape and redacts a secret Value.
func (e ServerConfigChanged) MarshalJSON() ([]byte, error) {
	type wire ServerConfigChanged
	w := wire(e)
	if e.Secret {
		w.Value = "[redacted]"
	}
	return json.Marshal(w)
}

func (e ServerConfigChanged) EventName() string { return "server.config.changed" }
