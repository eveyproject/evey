package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/gravity-zero/evey/internal/domain/playback"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)


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
	p := newTestProfile("wpp1", string(u.ID))
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("wplib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("wpmi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	wp := &playback.WatchProgress{
		ID:           "wp1",
		ProfileID:    string(p.ID),
		MediaItemID:  string(item.ID),
		PositionMs:   30000,
		Completed:    false,
		PlayCount:    1,
		LastPlayedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, wpRepo.Upsert(ctx, wp))

	got, err := wpRepo.GetByProfileAndMedia(ctx, string(p.ID), string(item.ID))
	require.NoError(t, err)
	assert.Equal(t, int64(30000), got.PositionMs)
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
	p := newTestProfile("wpp2", string(u.ID))
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("wplib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("wpmi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	wp := &playback.WatchProgress{
		ID: "wp2", ProfileID: string(p.ID), MediaItemID: string(item.ID),
		PositionMs: 10000, PlayCount: 1, LastPlayedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, wpRepo.Upsert(ctx, wp))

	wp.PositionMs = 90000
	wp.Completed = true
	wp.PlayCount = 2
	require.NoError(t, wpRepo.Upsert(ctx, wp))

	got, err := wpRepo.GetByProfileAndMedia(ctx, string(p.ID), string(item.ID))
	require.NoError(t, err)
	assert.Equal(t, int64(90000), got.PositionMs)
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
	p := newTestProfile("wpp3", string(u.ID))
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("wplib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	item1 := newTestMediaItem("wpmi3a", lib.ID)
	item2 := newTestMediaItem("wpmi3b", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item1))
	require.NoError(t, itemRepo.Create(ctx, item2))

	require.NoError(t, wpRepo.Upsert(ctx, &playback.WatchProgress{
		ID: "wp3a", ProfileID: string(p.ID), MediaItemID: string(item1.ID),
		PositionMs: 1000, PlayCount: 1, LastPlayedAt: time.Now().UTC().Truncate(time.Millisecond),
	}))
	require.NoError(t, wpRepo.Upsert(ctx, &playback.WatchProgress{
		ID: "wp3b", ProfileID: string(p.ID), MediaItemID: string(item2.ID),
		PositionMs: 2000, PlayCount: 1, LastPlayedAt: time.Now().UTC().Truncate(time.Millisecond),
	}))

	list, err := wpRepo.GetByProfile(ctx, string(p.ID), 10, 0)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}
