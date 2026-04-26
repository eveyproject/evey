package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/playback"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustWPDuration(t *testing.T, ms int64) shared.Duration {
	t.Helper()
	d, err := shared.NewDurationMs(ms)
	if err != nil {
		t.Fatalf("NewDurationMs(%d): %v", ms, err)
	}
	return d
}


func TestWatchProgressRepo_Upsert_GetByProfileAndMedia(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	wpRepo := postgres.NewWatchProgressRepository(db)
	ctx := context.Background()

	u := newTestUser("wpu1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("wpp1", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("wplib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("wpmi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	wp := &playback.WatchProgress{
		ID:           "wp1",
		ProfileID:    p.ID,
		MediaItemID:  item.ID,
		Position:     mustWPDuration(t, 30000),
		Completed:    false,
		PlayCount:    1,
		LastPlayedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, wpRepo.Upsert(ctx, wp))

	got, err := wpRepo.GetByProfileAndMedia(ctx, string(p.ID), string(item.ID))
	require.NoError(t, err)
	assert.Equal(t, int64(30000), got.Position.Milliseconds())
	assert.Equal(t, 1, got.PlayCount)
	assert.False(t, got.Completed)
}

func TestWatchProgressRepo_Upsert_UpdatesOnConflict(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	wpRepo := postgres.NewWatchProgressRepository(db)
	ctx := context.Background()

	u := newTestUser("wpu2")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("wpp2", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("wplib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("wpmi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	wp := &playback.WatchProgress{
		ID: "wp2", ProfileID: p.ID, MediaItemID: item.ID,
		Position: mustWPDuration(t, 10000), PlayCount: 1, LastPlayedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, wpRepo.Upsert(ctx, wp))

	wp.Position = mustWPDuration(t, 90000)
	wp.Completed = true
	wp.PlayCount = 2
	require.NoError(t, wpRepo.Upsert(ctx, wp))

	got, err := wpRepo.GetByProfileAndMedia(ctx, string(p.ID), string(item.ID))
	require.NoError(t, err)
	assert.Equal(t, int64(90000), got.Position.Milliseconds())
	assert.True(t, got.Completed)
	assert.Equal(t, 2, got.PlayCount)
}

func TestWatchProgressRepo_GetByProfile(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	wpRepo := postgres.NewWatchProgressRepository(db)
	ctx := context.Background()

	u := newTestUser("wpu3")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("wpp3", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("wplib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	item1 := newTestMediaItem("wpmi3a", lib.ID)
	item2 := newTestMediaItem("wpmi3b", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item1))
	require.NoError(t, itemRepo.Create(ctx, item2))

	require.NoError(t, wpRepo.Upsert(ctx, &playback.WatchProgress{
		ID: "wp3a", ProfileID: p.ID, MediaItemID: item1.ID,
		Position: mustWPDuration(t, 1000), PlayCount: 1, LastPlayedAt: time.Now().UTC().Truncate(time.Millisecond),
	}))
	require.NoError(t, wpRepo.Upsert(ctx, &playback.WatchProgress{
		ID: "wp3b", ProfileID: p.ID, MediaItemID: item2.ID,
		Position: mustWPDuration(t, 2000), PlayCount: 1, LastPlayedAt: time.Now().UTC().Truncate(time.Millisecond),
	}))

	list, err := wpRepo.GetByProfile(ctx, string(p.ID), 10, 0)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}
