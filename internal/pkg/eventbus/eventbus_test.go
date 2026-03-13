package eventbus_test

import (
	"context"
	"sync"
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
	bus := eventbus.New()
	var mu sync.Mutex
	var received []shared.Event

	bus.Subscribe("test.event", func(_ context.Context, event shared.Event) {
		mu.Lock()
		received = append(received, event)
		mu.Unlock()
	})

	bus.Publish(context.Background(), testEvent{msg: "hello"})
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, received, 1)
	assert.Equal(t, "hello", received[0].(testEvent).msg)
}

func TestMultipleSubscribers(t *testing.T) {
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

	bus.Publish(context.Background(), testEvent{msg: "multi"})
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 3, count)
}

func TestUnsubscribe(t *testing.T) {
	bus := eventbus.New()
	var mu sync.Mutex
	count := 0

	id := bus.Subscribe("test.event", func(_ context.Context, _ shared.Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	bus.Unsubscribe(id)
	bus.Publish(context.Background(), testEvent{msg: "gone"})
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 0, count)
}

func TestPanickingHandlerDoesNotCrash(t *testing.T) {
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
		bus.Publish(context.Background(), testEvent{msg: "panic test"})
	})
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.True(t, normalCalled)
}
