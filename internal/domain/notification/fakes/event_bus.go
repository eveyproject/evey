package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/notification"
)

type FakeEventBus struct {
	mu            sync.RWMutex
	subscriptions map[string]notification.Subscription
	Published     []notification.Event
}

func NewFakeEventBus() *FakeEventBus {
	return &FakeEventBus{
		subscriptions: make(map[string]notification.Subscription),
	}
}

func (b *FakeEventBus) Publish(_ context.Context, event notification.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Published = append(b.Published, event)
	for _, sub := range b.subscriptions {
		if sub.EventType == event.Type || sub.EventType == "*" {
			sub.Callback(event)
		}
	}
	return nil
}

func (b *FakeEventBus) Subscribe(_ context.Context, sub notification.Subscription) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscriptions[sub.ID] = sub
	return nil
}

func (b *FakeEventBus) Unsubscribe(_ context.Context, subscriptionID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subscriptions, subscriptionID)
	return nil
}

func (b *FakeEventBus) PublishedEvents() []notification.Event {
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make([]notification.Event, len(b.Published))
	copy(result, b.Published)
	return result
}

func (b *FakeEventBus) PublishedByType(eventType string) []notification.Event {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var result []notification.Event
	for _, e := range b.Published {
		if e.Type == eventType {
			result = append(result, e)
		}
	}
	return result
}
