package fakes

import (
	"context"
	"fmt"
	"sync"

	"github.com/eveyproject/evey/internal/domain/shared"
)

type FakeEventBus struct {
	mu         sync.RWMutex
	events     []shared.Event
	publishCtx []context.Context
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
	b.publishCtx = append(b.publishCtx, ctx)
	var entries []subEntry
	entries = append(entries, b.handlers[event.EventName()]...)
	entries = append(entries, b.handlers["*"]...)
	b.mu.Unlock()
	for _, e := range entries {
		e.handler(ctx, event)
	}
	return nil
}

// PublishContexts returns a snapshot of the contexts captured at each
// Publish call. Tests assert on these to verify the production code
// detaches publication from caller cancellation when needed.
func (b *FakeEventBus) PublishContexts() []context.Context {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]context.Context, len(b.publishCtx))
	copy(out, b.publishCtx)
	return out
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
