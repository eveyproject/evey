package eventbus_test

import (
	"context"
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


func TestPublishSubscribe(t *testing.T) {
	t.Parallel()
	bus := eventbus.New()
	var mu sync.Mutex
	var received []shared.Event

	bus.Subscribe("test.event", func(_ context.Context, event shared.Event) {
		mu.Lock()
		received = append(received, event)
		mu.Unlock()
	})

	_ = bus.Publish(context.Background(), testEvent{msg: "hello"})
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, received, 1)
	assert.Equal(t, "hello", received[0].(testEvent).msg)
}

func TestMultipleSubscribers(t *testing.T) {
	t.Parallel()
	bus := eventbus.New()
	var mu sync.Mutex
	count := 0

	for range 3 {
		bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
			mu.Lock()
			count++
			mu.Unlock()
		})
	}

	_ = bus.Publish(context.Background(), testEvent{msg: "multi"})
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 3, count)
}

func TestUnsubscribe(t *testing.T) {
	t.Parallel()
	bus := eventbus.New()
	var mu sync.Mutex
	count := 0

	id := bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	bus.Unsubscribe(id)
	_ = bus.Publish(context.Background(), testEvent{msg: "gone"})
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 0, count)
}

func TestPanickingHandlerDoesNotCrash(t *testing.T) {
	t.Parallel()
	bus := eventbus.New()
	var mu sync.Mutex
	normalCalled := false

	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		panic("deliberate panic")
	})
	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		mu.Lock()
		normalCalled = true
		mu.Unlock()
	})

	assert.NotPanics(t, func() {
		_ = bus.Publish(context.Background(), testEvent{msg: "panic test"})
	})
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.True(t, normalCalled)
}

func TestRetryOnPanicThenSuccess(t *testing.T) {
	t.Parallel()
	bus := eventbus.New()
	var calls atomic.Int32

	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		n := calls.Add(1)
		if n <= 2 {
			panic("transient failure")
		}
	})

	_ = bus.Publish(context.Background(), testEvent{msg: "retry"})
	time.Sleep(3 * time.Second)

	assert.Equal(t, int32(3), calls.Load(), "handler should be called 3 times (2 panics + 1 success)")
}

func TestAllRetriesExhausted(t *testing.T) {
	t.Parallel()
	bus := eventbus.New()
	var calls atomic.Int32

	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		calls.Add(1)
		panic("always fails")
	})

	_ = bus.Publish(context.Background(), testEvent{msg: "exhaust"})
	time.Sleep(3 * time.Second)

	assert.Equal(t, int32(3), calls.Load(), "handler called MaxRetries times")
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
	case <-time.After(500 * time.Millisecond):
		t.Fatal("handler was not invoked after publisher ctx cancel")
	}
}

func TestBusStopCancelsWorkerRetries(t *testing.T) {
	t.Parallel()
	bus := eventbus.NewWithPoolSize(1)
	var calls atomic.Int32

	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		calls.Add(1)
		panic("always fails")
	})

	_ = bus.Publish(context.Background(), testEvent{msg: "busstop"})
	// Let the first call happen then stop the bus during first backoff (100ms).
	time.Sleep(30 * time.Millisecond)
	bus.Stop()
	time.Sleep(200 * time.Millisecond)

	got := calls.Load()
	assert.LessOrEqual(t, got, int32(2), "bus stop should abort retry loop; got %d", got)
}

func TestSuccessfulDeliveryNoRetry(t *testing.T) {
	t.Parallel()
	bus := eventbus.New()
	var calls atomic.Int32

	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		calls.Add(1)
	})

	_ = bus.Publish(context.Background(), testEvent{msg: "ok"})
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, int32(1), calls.Load(), "successful handler called exactly once")
}

func TestWorkerPoolBoundedConcurrency(t *testing.T) {
	t.Parallel()
	bus := eventbus.NewWithPoolSize(2)
	defer bus.Stop()

	var maxConcurrent atomic.Int32
	var current atomic.Int32
	var done sync.WaitGroup

	done.Add(20)
	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		defer done.Done()
		c := current.Add(1)
		for {
			old := maxConcurrent.Load()
			if c <= old || maxConcurrent.CompareAndSwap(old, c) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		current.Add(-1)
	})

	for range 20 {
		_ = bus.Publish(context.Background(), testEvent{msg: "pool"})
	}
	done.Wait()

	assert.LessOrEqual(t, maxConcurrent.Load(), int32(2), "concurrency should not exceed pool size")
}

func TestWorkerPoolQueueFullDropsEvent(t *testing.T) {
	t.Parallel()
	bus := eventbus.NewWithPoolSize(1, eventbus.WithQueueSize(64))
	defer bus.Stop()

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
	time.Sleep(200 * time.Millisecond)

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
	time.Sleep(100 * time.Millisecond)

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
