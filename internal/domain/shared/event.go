package shared

import "context"

type Event interface {
	EventName() string
}

type EventBus interface {
	Publish(ctx context.Context, event Event) error
	Subscribe(eventName string, handler func(ctx context.Context, event Event)) string
	Unsubscribe(id string)
}
