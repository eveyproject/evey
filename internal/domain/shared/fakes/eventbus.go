package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/shared"
)

type FakeEventBus struct {
	mu       sync.RWMutex
	events   []shared.Event
	handlers map[string][]func(ctx context.Context, event shared.Event)
	nextID   int
	idMap    map[string]struct{ name string; idx int }
}

func NewFakeEventBus() *FakeEventBus {
	return &FakeEventBus{
		handlers: make(map[string][]func(ctx context.Context, event shared.Event)),
		idMap:    make(map[string]struct{ name string; idx int }),
	}
}

func (b *FakeEventBus) Publish(_ context.Context, event shared.Event) error {
	b.mu.Lock()
	b.events = append(b.events, event)
	b.mu.Unlock()
	return nil
}

func (b *FakeEventBus) Subscribe(eventName string, handler func(ctx context.Context, event shared.Event)) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	id := eventName + "-" + string(rune(b.nextID+'0'))
	b.handlers[eventName] = append(b.handlers[eventName], handler)
	return id
}

func (b *FakeEventBus) Unsubscribe(_ string) {}

func (b *FakeEventBus) PublishedEvents() []shared.Event {
	b.mu.RLock()
	defer b.mu.RUnlock()
	cp := make([]shared.Event, len(b.events))
	copy(cp, b.events)
	return cp
}
