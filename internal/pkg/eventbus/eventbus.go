package eventbus

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gravity-zero/evey/internal/domain/shared"
)

type subscription struct {
	id      string
	handler func(ctx context.Context, event shared.Event)
}

type RetryConfig struct {
	MaxRetries int
	Backoffs   []time.Duration
}

var DefaultRetryConfig = RetryConfig{
	MaxRetries: 3,
	Backoffs:   []time.Duration{100 * time.Millisecond, 500 * time.Millisecond, 2 * time.Second},
}

type InMemoryEventBus struct {
	mu     sync.RWMutex
	subs   map[string][]subscription
	nextID atomic.Int64
	retry  RetryConfig
}

func New() *InMemoryEventBus {
	return &InMemoryEventBus{
		subs:  make(map[string][]subscription),
		retry: DefaultRetryConfig,
	}
}

func NewWithRetry(cfg RetryConfig) *InMemoryEventBus {
	return &InMemoryEventBus{
		subs:  make(map[string][]subscription),
		retry: cfg,
	}
}

func (b *InMemoryEventBus) dispatch(ctx context.Context, sub subscription, event shared.Event) {
	for attempt := range b.retry.MaxRetries {
		panicVal := callHandler(ctx, sub.handler, event)
		if panicVal == nil {
			return
		}

		slog.Warn("eventbus: handler panicked, retrying",
			slog.String("event", event.EventName()),
			slog.String("sub", sub.id),
			slog.Int("attempt", attempt+1),
			slog.Int("max", b.retry.MaxRetries),
			slog.Any("panic", panicVal))

		backoff := b.backoffDuration(attempt)
		select {
		case <-ctx.Done():
			slog.Error("eventbus: ctx cancelled, aborting retry",
				slog.String("event", event.EventName()),
				slog.String("sub", sub.id),
				slog.Any("ctx_err", ctx.Err()))
			return
		case <-time.After(backoff):
		}
	}

	slog.Error("eventbus: all retries exhausted",
		slog.String("event", event.EventName()),
		slog.String("sub", sub.id),
		slog.Int("retries", b.retry.MaxRetries))
}

func (b *InMemoryEventBus) backoffDuration(attempt int) time.Duration {
	if attempt < len(b.retry.Backoffs) {
		return b.retry.Backoffs[attempt]
	}
	return b.retry.Backoffs[len(b.retry.Backoffs)-1]
}

func callHandler(ctx context.Context, handler func(context.Context, shared.Event), event shared.Event) (panicVal any) {
	defer func() {
		panicVal = recover()
	}()
	handler(ctx, event)
	return nil
}

func (b *InMemoryEventBus) Publish(ctx context.Context, event shared.Event) error {
	b.mu.RLock()
	specific := append([]subscription{}, b.subs[event.EventName()]...)
	wildcards := append([]subscription{}, b.subs["*"]...)
	b.mu.RUnlock()

	for _, s := range specific {
		go b.dispatch(ctx, s, event)
	}
	for _, s := range wildcards {
		go b.dispatch(ctx, s, event)
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
