package config

import (
	"context"
	"errors"
	"testing"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_Set_and_List(t *testing.T) {
	repo := fakes.NewFakeServerConfigRepository()
	svc := NewService(repo)
	ctx := context.Background()

	require.NoError(t, svc.Set(ctx, "theme", "dark"))
	require.NoError(t, svc.Set(ctx, "lang", "fr"))

	entries, err := svc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, entries, 2)

	keys := map[string]string{}
	for _, e := range entries {
		keys[e.Key] = e.Value
	}
	assert.Equal(t, "dark", keys["theme"])
	assert.Equal(t, "fr", keys["lang"])
}

func TestService_Set_overwrite(t *testing.T) {
	repo := fakes.NewFakeServerConfigRepository()
	svc := NewService(repo)
	ctx := context.Background()

	require.NoError(t, svc.Set(ctx, "k", "v1"))
	require.NoError(t, svc.Set(ctx, "k", "v2"))

	entries, err := svc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "v2", entries[0].Value)
}

func TestService_Delete(t *testing.T) {
	repo := fakes.NewFakeServerConfigRepository()
	svc := NewService(repo)
	ctx := context.Background()

	require.NoError(t, svc.Set(ctx, "k", "v"))
	require.NoError(t, svc.Delete(ctx, "k"))

	entries, err := svc.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestService_Delete_nonexistent(t *testing.T) {
	repo := fakes.NewFakeServerConfigRepository()
	svc := NewService(repo)

	err := svc.Delete(context.Background(), "nope")
	assert.NoError(t, err)
}

func TestService_List_empty(t *testing.T) {
	repo := fakes.NewFakeServerConfigRepository()
	svc := NewService(repo)

	entries, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestService_repo_error(t *testing.T) {
	repoErr := errors.New("db down")
	repo := fakes.NewFakeServerConfigRepository()
	repo.Err = repoErr
	svc := NewService(repo)
	ctx := context.Background()

	t.Run("Set", func(t *testing.T) {
		assert.ErrorIs(t, svc.Set(ctx, "k", "v"), repoErr)
	})
	t.Run("Delete", func(t *testing.T) {
		assert.ErrorIs(t, svc.Delete(ctx, "k"), repoErr)
	})
	t.Run("List", func(t *testing.T) {
		_, err := svc.List(ctx)
		assert.ErrorIs(t, err, repoErr)
	})
	t.Run("Get", func(t *testing.T) {
		_, err := svc.Get(ctx, "k")
		assert.ErrorIs(t, err, repoErr)
	})
}

// Live config reload: any Set/Delete must publish a ServerConfigChanged
// event when the bus is wired. Subscribers (auto-download toggle, etc.)
// rely on this to refresh cached state without a server restart.
func TestService_Set_publishesEvent(t *testing.T) {
	repo := fakes.NewFakeServerConfigRepository()
	bus := fakes.NewFakeEventBus()
	svc := NewService(repo, WithEventBus(bus))

	require.NoError(t, svc.Set(context.Background(), "artwork.auto_download", "false"))

	events := bus.PublishedEvents()
	require.Len(t, events, 1)
	ev, ok := events[0].(shared.ServerConfigChanged)
	require.True(t, ok, "expected ServerConfigChanged, got %T", events[0])
	assert.Equal(t, "artwork.auto_download", ev.Key)
	assert.Equal(t, "false", ev.Value)
	assert.False(t, ev.Deleted)
	assert.False(t, ev.OccurredAt.IsZero())
}

func TestService_Delete_publishesEventWithDeletedFlag(t *testing.T) {
	repo := fakes.NewFakeServerConfigRepository()
	bus := fakes.NewFakeEventBus()
	svc := NewService(repo, WithEventBus(bus))

	require.NoError(t, svc.Set(context.Background(), "k", "v"))
	require.NoError(t, svc.Delete(context.Background(), "k"))

	events := bus.PublishedEvents()
	require.Len(t, events, 2)
	delEv, ok := events[1].(shared.ServerConfigChanged)
	require.True(t, ok)
	assert.Equal(t, "k", delEv.Key)
	assert.True(t, delEv.Deleted, "Delete must set Deleted=true so subscribers fall back to default")
	assert.Empty(t, delEv.Value)
}

// Subscriber roundtrip: published event reaches a handler subscribed via
// the same bus instance. Guards against name-mismatch regressions in the
// event-name constant.
func TestService_Set_eventReachesSubscriber(t *testing.T) {
	repo := fakes.NewFakeServerConfigRepository()
	bus := fakes.NewFakeEventBus()
	svc := NewService(repo, WithEventBus(bus))

	var received shared.ServerConfigChanged
	bus.Subscribe(shared.ServerConfigChanged{}.EventName(), func(_ context.Context, e shared.Event) {
		ev, ok := e.(shared.ServerConfigChanged)
		if ok {
			received = ev
		}
	})

	require.NoError(t, svc.Set(context.Background(), "lang", "fr"))
	assert.Equal(t, "lang", received.Key)
	assert.Equal(t, "fr", received.Value)
}

// Without WithEventBus, the service stays a pure CRUD facade - no panic,
// no nil-deref, no surprise. Default for callers that don't need reload.
func TestService_Set_noEventBus_noop(t *testing.T) {
	repo := fakes.NewFakeServerConfigRepository()
	svc := NewService(repo)

	require.NoError(t, svc.Set(context.Background(), "k", "v"))
	require.NoError(t, svc.Delete(context.Background(), "k"))
}

func TestService_Get(t *testing.T) {
	repo := fakes.NewFakeServerConfigRepository()
	svc := NewService(repo)
	ctx := context.Background()

	require.NoError(t, svc.Set(ctx, "theme", "dark"))
	entry, err := svc.Get(ctx, "theme")
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.Equal(t, "theme", entry.Key)
	assert.Equal(t, "dark", entry.Value)
}
