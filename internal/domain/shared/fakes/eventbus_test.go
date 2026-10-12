package fakes

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testEvent struct{ name string }

func (e testEvent) EventName() string { return e.name }

func TestFakeEventBus_Publish_and_Subscribe(t *testing.T) {
	bus := NewFakeEventBus()
	ctx := context.Background()

	var received []shared.Event
	bus.Subscribe("item.created", func(_ context.Context, e shared.Event) {
		received = append(received, e)
	})

	require.NoError(t, bus.Publish(ctx, testEvent{"item.created"}))
	require.NoError(t, bus.Publish(ctx, testEvent{"item.deleted"}))

	assert.Len(t, received, 1)
	assert.Equal(t, "item.created", received[0].EventName())
}

func TestFakeEventBus_WildcardSubscriber(t *testing.T) {
	bus := NewFakeEventBus()

	var received []shared.Event
	bus.Subscribe("*", func(_ context.Context, e shared.Event) {
		received = append(received, e)
	})

	require.NoError(t, bus.Publish(context.Background(), testEvent{"a"}))
	require.NoError(t, bus.Publish(context.Background(), testEvent{"b"}))

	assert.Len(t, received, 2)
}

func TestFakeEventBus_Unsubscribe(t *testing.T) {
	bus := NewFakeEventBus()
	called := 0
	id := bus.Subscribe("ev", func(_ context.Context, _ shared.Event) {
		called++
	})

	require.NoError(t, bus.Publish(context.Background(), testEvent{"ev"}))
	assert.Equal(t, 1, called)

	bus.Unsubscribe(id)
	require.NoError(t, bus.Publish(context.Background(), testEvent{"ev"}))
	assert.Equal(t, 1, called)
}

func TestFakeEventBus_PublishErr(t *testing.T) {
	bus := NewFakeEventBus()
	bus.PublishErr = errors.New("fail")

	err := bus.Publish(context.Background(), testEvent{"ev"})
	assert.ErrorIs(t, err, bus.PublishErr)
	assert.Empty(t, bus.PublishedEvents())
}

func TestFakeEventBus_PublishedEvents(t *testing.T) {
	bus := NewFakeEventBus()

	require.NoError(t, bus.Publish(context.Background(), testEvent{"a"}))
	require.NoError(t, bus.Publish(context.Background(), testEvent{"b"}))

	events := bus.PublishedEvents()
	assert.Len(t, events, 2)
	assert.Equal(t, "a", events[0].EventName())
	assert.Equal(t, "b", events[1].EventName())
}

func TestFakeEventBus_ConcurrentPublish(t *testing.T) {
	bus := NewFakeEventBus()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = bus.Publish(context.Background(), testEvent{"ev"})
		}()
	}
	wg.Wait()
	assert.Len(t, bus.PublishedEvents(), 50)
}
