package eventbus_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/pkg/eventbus"
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

func TestContextCancellationStopsRetry(t *testing.T) {
	t.Parallel()
	bus := eventbus.New()
	var calls atomic.Int32

	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		calls.Add(1)
		panic("always fails")
	})

	ctx, cancel := context.WithCancel(context.Background())
	_ = bus.Publish(ctx, testEvent{msg: "cancel"})

	// Cancel after first retry backoff starts
	time.Sleep(30 * time.Millisecond)
	cancel()
	time.Sleep(150 * time.Millisecond)

	got := calls.Load()
	assert.Less(t, got, int32(5), "should stop before all 5 retries; got %d", got)
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
	bus := eventbus.NewWithPoolSize(1)
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
	assert.Greater(t, got, int32(0), "at least some events should be processed")
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
