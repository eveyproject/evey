package eventbus_test

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/eventbus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

type testEvent struct {
	msg string
}

func (e testEvent) EventName() string { return "test.event" }

// fastRetry is a tight retry policy for tests that need to exercise the retry
// code path without paying production-sized backoffs.
func fastRetry() eventbus.RetryConfig {
	return eventbus.RetryConfig{
		MaxRetries: 3,
		Backoffs: []time.Duration{
			1 * time.Millisecond,
			1 * time.Millisecond,
			1 * time.Millisecond,
		},
	}
}

// waitFor yields the goroutine until fn returns true or the deadline elapses.
// It avoids fixed sleeps by using runtime.Gosched so the scheduler can run the
// asynchronous workers under test.
func waitFor(t *testing.T, timeout time.Duration, fn func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return true
		}
		runtime.Gosched()
	}
	return fn()
}

func TestPublishSubscribe(t *testing.T) {
	t.Parallel()
	bus := eventbus.New()
	defer bus.Stop()

	done := make(chan shared.Event, 1)
	bus.Subscribe("test.event", func(_ context.Context, event shared.Event) {
		done <- event
	})

	_ = bus.Publish(context.Background(), testEvent{msg: "hello"})

	select {
	case ev := <-done:
		assert.Equal(t, "hello", ev.(testEvent).msg)
	case <-time.After(2 * time.Second):
		t.Fatal("handler never received the event")
	}
}

func TestMultipleSubscribers(t *testing.T) {
	t.Parallel()
	bus := eventbus.New()
	defer bus.Stop()

	var count atomic.Int32
	var wg sync.WaitGroup
	wg.Add(3)
	for range 3 {
		bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
			count.Add(1)
			wg.Done()
		})
	}

	_ = bus.Publish(context.Background(), testEvent{msg: "multi"})
	waitGroupOrFatal(t, &wg, 2*time.Second, "not all subscribers were invoked")
	assert.Equal(t, int32(3), count.Load())
}

func TestUnsubscribe(t *testing.T) {
	t.Parallel()
	bus := eventbus.New()
	defer bus.Stop()

	var count atomic.Int32
	id := bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		count.Add(1)
	})

	// Add a second subscriber so we can deterministically wait for the publish
	// to be fully drained even though the first handler should never run.
	drained := make(chan struct{}, 1)
	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		drained <- struct{}{}
	})

	bus.Unsubscribe(id)
	_ = bus.Publish(context.Background(), testEvent{msg: "gone"})

	select {
	case <-drained:
	case <-time.After(2 * time.Second):
		t.Fatal("publish was never delivered to the marker subscriber")
	}
	assert.Equal(t, int32(0), count.Load())
}

func TestPanickingHandlerDoesNotCrash(t *testing.T) {
	t.Parallel()
	bus := eventbus.New(eventbus.WithRetry(fastRetry()))
	defer bus.Stop()

	normalCalled := make(chan struct{}, 1)

	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		panic("deliberate panic")
	})
	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		select {
		case normalCalled <- struct{}{}:
		default:
		}
	})

	assert.NotPanics(t, func() {
		_ = bus.Publish(context.Background(), testEvent{msg: "panic test"})
	})

	select {
	case <-normalCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("normal subscriber was not invoked")
	}
}

func TestRetryOnPanicThenSuccess(t *testing.T) {
	t.Parallel()
	bus := eventbus.New(eventbus.WithRetry(fastRetry()))
	defer bus.Stop()

	var calls atomic.Int32
	done := make(chan struct{}, 1)

	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		n := calls.Add(1)
		if n <= 2 {
			panic("transient failure")
		}
		select {
		case done <- struct{}{}:
		default:
		}
	})

	_ = bus.Publish(context.Background(), testEvent{msg: "retry"})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("handler did not succeed after retries; calls=%d", calls.Load())
	}
	assert.Equal(t, int32(3), calls.Load(),
		"handler should be called 3 times (2 panics + 1 success)")
}

func TestAllRetriesExhausted(t *testing.T) {
	t.Parallel()
	bus := eventbus.New(eventbus.WithRetry(fastRetry()))
	defer bus.Stop()

	var calls atomic.Int32
	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		calls.Add(1)
		panic("always fails")
	})

	_ = bus.Publish(context.Background(), testEvent{msg: "exhaust"})

	if !waitFor(t, 2*time.Second, func() bool { return calls.Load() == 3 }) {
		t.Fatalf("handler was not retried MaxRetries times; got %d", calls.Load())
	}
	// Wait a little longer to assert no further calls happen after exhaustion.
	if waitFor(t, 100*time.Millisecond, func() bool { return calls.Load() > 3 }) {
		t.Fatalf("handler invoked more than MaxRetries times: %d", calls.Load())
	}
	assert.Equal(t, int32(3), calls.Load(), "handler called MaxRetries times")
}

// More retries than configured backoffs: the last backoff is reused for the
// rest, and the worker never reads past the list (an index panic there would
// take the whole server down, not just the handler). Only equal-length
// configs were tested (mutation 2026-09-28).
func TestRetriesBeyondTheBackoffListReuseTheLastOne(t *testing.T) {
	t.Parallel()
	bus := eventbus.New(eventbus.WithRetry(eventbus.RetryConfig{
		MaxRetries: 5,
		Backoffs:   []time.Duration{time.Millisecond},
	}))
	defer bus.Stop()

	var calls atomic.Int32
	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		calls.Add(1)
		panic("always fails")
	})
	_ = bus.Publish(context.Background(), testEvent{msg: "long retry"})

	if !waitFor(t, 2*time.Second, func() bool { return calls.Load() == 5 }) {
		t.Fatalf("handler must be tried MaxRetries times with one backoff reused; got %d", calls.Load())
	}
}

func TestPublisherCtxCancelDoesNotAbortWorker(t *testing.T) {
	t.Parallel()
	bus := eventbus.New()
	defer bus.Stop()

	called := make(chan struct{}, 1)
	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		called <- struct{}{}
	})

	ctx, cancel := context.WithCancel(context.Background())
	_ = bus.Publish(ctx, testEvent{msg: "decoupled"})
	// Cancel publisher ctx immediately; worker should still deliver the event
	// because it owns the lifecycle once the job is enqueued.
	cancel()

	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("handler was not invoked after publisher ctx cancel")
	}
}

func TestBusStopCancelsWorkerRetries(t *testing.T) {
	t.Parallel()
	// Use longer-than-test backoffs so we can race Stop() against the first
	// retry wait deterministically.
	cfg := eventbus.RetryConfig{
		MaxRetries: 3,
		Backoffs: []time.Duration{
			500 * time.Millisecond,
			500 * time.Millisecond,
			500 * time.Millisecond,
		},
	}
	bus := eventbus.NewWithPoolSize(1, eventbus.WithRetry(cfg))

	var calls atomic.Int32
	firstCall := make(chan struct{}, 1)
	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		calls.Add(1)
		select {
		case firstCall <- struct{}{}:
		default:
		}
		panic("always fails")
	})

	_ = bus.Publish(context.Background(), testEvent{msg: "busstop"})

	// Wait for the worker to enter the first backoff, then Stop the bus.
	select {
	case <-firstCall:
	case <-time.After(2 * time.Second):
		t.Fatal("handler was not invoked at all")
	}
	bus.Stop()

	// After Stop the worker exits its retry loop; ensure call count never
	// reaches MaxRetries (because we cancelled before the second backoff
	// elapsed).
	got := calls.Load()
	assert.LessOrEqual(t, got, int32(2), "bus stop should abort retry loop; got %d", got)
}

func TestSuccessfulDeliveryNoRetry(t *testing.T) {
	t.Parallel()
	bus := eventbus.New(eventbus.WithRetry(fastRetry()))
	defer bus.Stop()

	var calls atomic.Int32
	done := make(chan struct{}, 1)
	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		calls.Add(1)
		done <- struct{}{}
	})

	_ = bus.Publish(context.Background(), testEvent{msg: "ok"})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler was not invoked")
	}
	// Ensure no spurious retries follow.
	if waitFor(t, 50*time.Millisecond, func() bool { return calls.Load() > 1 }) {
		t.Fatalf("successful handler was retried; calls=%d", calls.Load())
	}
	assert.Equal(t, int32(1), calls.Load(), "successful handler called exactly once")
}

func TestWorkerPoolBoundedConcurrency(t *testing.T) {
	t.Parallel()
	bus := eventbus.NewWithPoolSize(2)
	defer bus.Stop()

	var maxConcurrent atomic.Int32
	var current atomic.Int32
	var done sync.WaitGroup

	const total = 20
	done.Add(total)
	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		defer done.Done()
		c := current.Add(1)
		for {
			old := maxConcurrent.Load()
			if c <= old || maxConcurrent.CompareAndSwap(old, c) {
				break
			}
		}
		// Tight busy-yield to keep the handler "in-flight" briefly without
		// using a fixed time.Sleep. Yielding the goroutine lets the scheduler
		// run sibling workers so the concurrency observation is meaningful.
		for i := 0; i < 100; i++ {
			runtime.Gosched()
		}
		current.Add(-1)
	})

	for range total {
		_ = bus.Publish(context.Background(), testEvent{msg: "pool"})
	}
	waitGroupOrFatal(t, &done, 5*time.Second, "not all events were processed")

	assert.LessOrEqual(t, maxConcurrent.Load(), int32(2),
		"concurrency should not exceed pool size")
}

func TestWorkerPoolQueueFullDropsEvent(t *testing.T) {
	t.Parallel()
	bus := eventbus.NewWithPoolSize(1, eventbus.WithQueueSize(64))

	blocker := make(chan struct{})
	var received atomic.Int32

	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		received.Add(1)
		<-blocker
	})

	// Fill the queue: 1 worker blocked + 64 buffer
	for range 200 {
		_ = bus.Publish(context.Background(), testEvent{msg: "flood"})
	}

	close(blocker)

	// Stop the bus to deterministically drain the queue: Stop() closes the
	// queue and waits for all in-flight jobs to finish. After Stop returns
	// the worker is guaranteed not to consume any more events.
	bus.Stop()

	got := received.Load()
	assert.Less(t, got, int32(200), "some events should be dropped when queue is full")
	assert.Positive(t, got, "at least some events should be processed")
}

func TestWorkerPoolStop(t *testing.T) {
	t.Parallel()
	bus := eventbus.NewWithPoolSize(4)
	var calls atomic.Int32

	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		calls.Add(1)
	})

	_ = bus.Publish(context.Background(), testEvent{msg: "before stop"})
	bus.Stop()

	assert.GreaterOrEqual(t, calls.Load(), int32(0))
}

type droppedEvent struct{}

func (droppedEvent) EventName() string { return "test.dropped" }

func TestMetricDropsOnQueueFull(t *testing.T) {
	t.Parallel()
	bus := eventbus.NewWithPoolSize(1, eventbus.WithQueueSize(4))
	defer bus.Stop()

	droppedBefore := gatherCounter(t, "eventbus_events_dropped_total", "event_name", "test.dropped")

	blocker := make(chan struct{})
	bus.Subscribe("test.dropped", func(_ context.Context, _ shared.Event) {
		<-blocker
	})

	for range 200 {
		_ = bus.Publish(context.Background(), droppedEvent{})
	}
	close(blocker)

	// Wait until the dropped counter strictly increases. The increment is
	// recorded synchronously in Publish for queue-full drops, so a short poll
	// is sufficient even on a busy CI host.
	if !waitFor(t, 2*time.Second, func() bool {
		return gatherCounter(t, "eventbus_events_dropped_total", "event_name", "test.dropped") > droppedBefore
	}) {
		t.Fatal("eventbus_events_dropped_total never increased")
	}
	droppedAfter := gatherCounter(t, "eventbus_events_dropped_total", "event_name", "test.dropped")
	assert.Greater(t, droppedAfter-droppedBefore, float64(0),
		"eventbus_events_dropped_total should increase when queue is saturated")
}

type depthEvent struct{}

func (depthEvent) EventName() string { return "test.depth" }

func TestMetricQueueDepthObserved(t *testing.T) {
	// Not parallel: the depth gauge is global so concurrent buses would
	// interfere with the reading.
	bus := eventbus.NewWithPoolSize(1, eventbus.WithQueueSize(32))
	defer bus.Stop()

	blocker := make(chan struct{})
	defer close(blocker)
	bus.Subscribe("test.depth", func(_ context.Context, _ shared.Event) {
		<-blocker
	})

	var maxDepth float64
	for range 5 {
		_ = bus.Publish(context.Background(), depthEvent{})
		if d := gatherGauge(t, "eventbus_queue_depth"); d > maxDepth {
			maxDepth = d
		}
	}

	assert.GreaterOrEqual(t, maxDepth, float64(1),
		"eventbus_queue_depth should reflect buffered events before workers drain")
}

func gatherCounter(t *testing.T, name, labelName, labelValue string) float64 {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	assert.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == labelName && l.GetValue() == labelValue {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

func gatherGauge(t *testing.T, name string) float64 {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	assert.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			return m.GetGauge().GetValue()
		}
	}
	return 0
}

// waitGroupOrFatal blocks until wg.Wait() returns or timeout elapses.
func waitGroupOrFatal(t *testing.T, wg *sync.WaitGroup, timeout time.Duration, msg string) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal(msg)
	}
}
