package notification

import "context"

type EventBus interface {
	Publish(ctx context.Context, event Event) error
	Subscribe(ctx context.Context, sub Subscription) error
	Unsubscribe(ctx context.Context, subscriptionID string) error
}
