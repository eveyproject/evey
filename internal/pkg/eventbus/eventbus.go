package eventbus

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
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

type dispatchJob struct {
	ctx   context.Context
	sub   subscription
	event shared.Event
}

type InMemoryEventBus struct {
	mu     sync.RWMutex
	subs   map[string][]subscription
	nextID atomic.Int64
	retry  RetryConfig
	queue  chan dispatchJob
	wg     sync.WaitGroup
}

type Option func(*options)

type options struct {
	queueSize int
}

func WithQueueSize(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.queueSize = n
		}
	}
}

func New(opts ...Option) *InMemoryEventBus {
	return NewWithPoolSize(runtime.NumCPU()*2, opts...)
}

func NewWithPoolSize(workers int, opts ...Option) *InMemoryEventBus {
	if workers <= 0 {
		workers = 1
	}
	o := options{queueSize: workers * 256}
	for _, fn := range opts {
		fn(&o)
	}
	b := &InMemoryEventBus{
		subs:  make(map[string][]subscription),
		retry: DefaultRetryConfig,
		queue: make(chan dispatchJob, o.queueSize),
	}
	for range workers {
		b.wg.Add(1)
		go b.worker()
	}
	return b
}

func (b *InMemoryEventBus) worker() {
	defer b.wg.Done()
	for job := range b.queue {
		b.dispatch(job.ctx, job.sub, job.event)
	}
}

func (b *InMemoryEventBus) Stop() {
	close(b.queue)
	b.wg.Wait()
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
		b.enqueue(ctx, s, event)
	}
	for _, s := range wildcards {
		b.enqueue(ctx, s, event)
	}
	return nil
}

func (b *InMemoryEventBus) enqueue(ctx context.Context, s subscription, event shared.Event) {
	select {
	case b.queue <- dispatchJob{ctx: ctx, sub: s, event: event}:
	default:
		slog.Warn("eventbus: queue full, dropping event",
			slog.String("event", event.EventName()),
			slog.String("sub", s.id))
	}
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
