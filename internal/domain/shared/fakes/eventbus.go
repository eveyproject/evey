package fakes

import (
	"context"
	"fmt"
	"sync"

	"github.com/evey-project/evey/internal/domain/shared"
)

type FakeEventBus struct {
	mu         sync.RWMutex
	events     []shared.Event
	handlers   map[string][]subEntry
	nextID     int
	PublishErr error
}

type subEntry struct {
	id      string
	handler func(ctx context.Context, event shared.Event)
}

func NewFakeEventBus() *FakeEventBus {
	return &FakeEventBus{
		handlers: make(map[string][]subEntry),
	}
}

func (b *FakeEventBus) Publish(ctx context.Context, event shared.Event) error {
	if b.PublishErr != nil {
		return b.PublishErr
	}
	b.mu.Lock()
	b.events = append(b.events, event)
	var entries []subEntry
	entries = append(entries, b.handlers[event.EventName()]...)
	entries = append(entries, b.handlers["*"]...)
	b.mu.Unlock()
	for _, e := range entries {
		e.handler(ctx, event)
	}
	return nil
}

func (b *FakeEventBus) Subscribe(eventName string, handler func(ctx context.Context, event shared.Event)) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	id := fmt.Sprintf("%s-%d", eventName, b.nextID)
	b.handlers[eventName] = append(b.handlers[eventName], subEntry{id: id, handler: handler})
	return id
}

func (b *FakeEventBus) Unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for name, entries := range b.handlers {
		for i, e := range entries {
			if e.id == id {
				b.handlers[name] = append(entries[:i], entries[i+1:]...)
				return
			}
		}
	}
}

func (b *FakeEventBus) PublishedEvents() []shared.Event {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cp := make([]shared.Event, len(b.events))
	copy(cp, b.events)
	return cp
}
