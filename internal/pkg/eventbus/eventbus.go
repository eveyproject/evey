package eventbus

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/gravity-zero/evey/internal/domain/shared"
)

type subscription struct {
	id      string
	handler func(ctx context.Context, event shared.Event)
}

type InMemoryEventBus struct {
	mu      sync.RWMutex
	subs    map[string][]subscription
	nextID  atomic.Int64
}

func New() *InMemoryEventBus {
	return &InMemoryEventBus{
		subs: make(map[string][]subscription),
	}
}

func (b *InMemoryEventBus) Publish(ctx context.Context, event shared.Event) error {
	b.mu.RLock()
	specific := append([]subscription{}, b.subs[event.EventName()]...)
	wildcards := append([]subscription{}, b.subs["*"]...)
	b.mu.RUnlock()

	for _, s := range specific {
		go func(sub subscription) {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("eventbus: handler panic",
						slog.String("event", event.EventName()),
						slog.String("sub", sub.id),
						slog.Any("panic", r))
				}
			}()
			sub.handler(ctx, event)
		}(s)
	}
	for _, s := range wildcards {
		go func(sub subscription) {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("eventbus: handler panic",
						slog.String("event", event.EventName()),
						slog.String("sub", sub.id),
						slog.Any("panic", r))
				}
			}()
			sub.handler(ctx, event)
		}(s)
	}
	return nil
}

func (b *InMemoryEventBus) Subscribe(eventName string, handler func(ctx context.Context, event shared.Event)) string {
	id := fmt.Sprintf("sub-%d", b.nextID.Add(1))
	b.mu.Lock()
	b.subs[eventName] = append(b.subs[eventName], subscription{id: id, handler: handler})
	b.mu.Unlock()
	return id
}

func (b *InMemoryEventBus) Unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for name, subs := range b.subs {
		for i, s := range subs {
			if s.id == id {
				b.subs[name] = append(subs[:i], subs[i+1:]...)
				return
			}
		}
	}
}
