package eventbus

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
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

// dispatchJob holds the data needed by a worker to deliver an event to one
// subscriber. It intentionally does NOT carry the publisher's ctx: once the
// event is accepted into the queue the worker owns its lifecycle and uses the
// bus-scoped ctx for retries so that a cancelling publisher cannot cause a
// pending event to be dropped mid-retry.
type dispatchJob struct {
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
	ctx    context.Context
	cancel context.CancelFunc

	// stopped + sendMu make close(queue) safe against a concurrent send.
	//
	// The ctx.Done() guard in enqueue and the send onto queue are two separate
	// selects, so a publisher could pass the guard, lose the scheduler, and
	// come back to send on a channel Stop had since closed - an unrecovered
	// `send on closed channel` that takes the whole process down. Reproduced
	// under -race.
	//
	// It is reachable in production, not just in a test: Stop() runs BEFORE the
	// gRPC server stops accepting requests, background workers are still
	// publishing (they only check ctx cooperatively), and the admin panel's
	// restart button makes the process SIGTERM itself - so clicking restart
	// could crash the server instead of stopping it cleanly.
	//
	// Same shape as the fix already in app/qoe/sink.go: a lock-free fast path,
	// then a re-check under the read lock, with close() taken under the write
	// lock so no send can be in flight when the channel closes.
	stopped atomic.Bool
	sendMu  sync.RWMutex
}

type Option func(*options)

type options struct {
	queueSize int
	retry     *RetryConfig
}

func WithQueueSize(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.queueSize = n
		}
	}
}

// WithRetry overrides the bus retry policy. Primarily intended for tests that
// need short, deterministic backoffs; production code should rely on
// DefaultRetryConfig.
func WithRetry(cfg RetryConfig) Option {
	return func(o *options) {
		o.retry = &cfg
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
	retry := DefaultRetryConfig
	if o.retry != nil {
		retry = *o.retry
	}
	ctx, cancel := context.WithCancel(context.Background())
	b := &InMemoryEventBus{
		subs:   make(map[string][]subscription),
		retry:  retry,
		queue:  make(chan dispatchJob, o.queueSize),
		ctx:    ctx,
		cancel: cancel,
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
		queueDepthGauge.Set(float64(len(b.queue)))
		b.dispatch(b.ctx, job.sub, job.event)
	}
}

func (b *InMemoryEventBus) Stop() {
	// Idempotent: a second Stop must not close an already-closed channel.
	if !b.stopped.CompareAndSwap(false, true) {
		return
	}
	b.cancel()
	// Take the write lock so any in-flight enqueue send completes first.
	// stopped is already true, so no new send can start past the re-check.
	b.sendMu.Lock()
	close(b.queue)
	b.sendMu.Unlock()
	b.wg.Wait()
	queueDepthGauge.Set(0)
}

func (b *InMemoryEventBus) dispatch(ctx context.Context, sub subscription, event shared.Event) {
	for attempt := range b.retry.MaxRetries {
		panicVal := callHandler(ctx, sub.handler, event)
		if panicVal == nil {
			return
		}

		slog.WarnContext(ctx, "eventbus: handler panicked, retrying",
			slog.String("event", event.EventName()),
			slog.String("sub", sub.id),
			slog.Int("attempt", attempt+1),
			slog.Int("max", b.retry.MaxRetries),
			slog.Any("panic", panicVal))

		backoff := b.backoffDuration(attempt)
		select {
		case <-ctx.Done():
			slog.ErrorContext(ctx, "eventbus: ctx cancelled, aborting retry",
				slog.String("event", event.EventName()),
				slog.String("sub", sub.id),
				slog.Any("ctx_err", ctx.Err()))
			eventsDroppedTotal.WithLabelValues(event.EventName()).Inc()
			return
		case <-time.After(backoff):
		}
	}

	slog.ErrorContext(ctx, "eventbus: all retries exhausted",
		slog.String("event", event.EventName()),
		slog.String("sub", sub.id),
		slog.Int("retries", b.retry.MaxRetries))
	eventsDroppedTotal.WithLabelValues(event.EventName()).Inc()
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

// Publish enqueues the event for asynchronous delivery to every matching
// subscriber. The supplied ctx bounds ONLY the synchronous enqueue step
// (respecting caller deadlines while pushing into the queue). Once the event
// is accepted the worker takes ownership and uses the bus-scoped ctx, so a
// cancelling publisher cannot abort in-flight retries.
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
	case <-b.ctx.Done():
		slog.Warn("eventbus: bus stopped, dropping event",
			slog.String("event", event.EventName()),
			slog.String("sub", s.id))
		eventsDroppedTotal.WithLabelValues(event.EventName()).Inc()
		return
	case <-ctx.Done():
		slog.Warn("eventbus: publisher ctx cancelled before enqueue, dropping event",
			slog.String("event", event.EventName()),
			slog.String("sub", s.id),
			slog.Any("ctx_err", ctx.Err()))
		eventsDroppedTotal.WithLabelValues(event.EventName()).Inc()
		return
	default:
	}

	if b.stopped.Load() {
		eventsDroppedTotal.WithLabelValues(event.EventName()).Inc()
		return
	}
	b.sendMu.RLock()
	defer b.sendMu.RUnlock()
	if b.stopped.Load() {
		// Stop ran between the lock-free check and acquiring the RLock: the
		// queue is closed or about to be. Drop, don't send.
		eventsDroppedTotal.WithLabelValues(event.EventName()).Inc()
		return
	}

	select {
	case b.queue <- dispatchJob{sub: s, event: event}:
		queueDepthGauge.Set(float64(len(b.queue)))
	default:
		slog.Warn("eventbus: queue full, dropping event",
			slog.String("event", event.EventName()),
			slog.String("sub", s.id))
		eventsDroppedTotal.WithLabelValues(event.EventName()).Inc()
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
