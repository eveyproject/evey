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

func fastRetry() eventbus.RetryConfig {
	return eventbus.RetryConfig{
		MaxRetries: 3,
		Backoffs:   []time.Duration{time.Millisecond, 2 * time.Millisecond, 4 * time.Millisecond},
	}
}

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
	bus := eventbus.NewWithRetry(fastRetry())
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
	bus := eventbus.NewWithRetry(fastRetry())
	var calls atomic.Int32

	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		n := calls.Add(1)
		if n <= 2 {
			panic("transient failure")
		}
	})

	_ = bus.Publish(context.Background(), testEvent{msg: "retry"})
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, int32(3), calls.Load(), "handler should be called 3 times (2 panics + 1 success)")
}

func TestAllRetriesExhausted(t *testing.T) {
	t.Parallel()
	bus := eventbus.NewWithRetry(fastRetry())
	var calls atomic.Int32

	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		calls.Add(1)
		panic("always fails")
	})

	_ = bus.Publish(context.Background(), testEvent{msg: "exhaust"})
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, int32(3), calls.Load(), "handler called MaxRetries times")
}

func TestContextCancellationStopsRetry(t *testing.T) {
	t.Parallel()
	bus := eventbus.NewWithRetry(eventbus.RetryConfig{
		MaxRetries: 5,
		Backoffs:   []time.Duration{50 * time.Millisecond, 50 * time.Millisecond},
	})
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
	bus := eventbus.NewWithRetry(fastRetry())
	var calls atomic.Int32

	bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		calls.Add(1)
	})

	_ = bus.Publish(context.Background(), testEvent{msg: "ok"})
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, int32(1), calls.Load(), "successful handler called exactly once")
}
