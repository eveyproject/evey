package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/library"
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

// TestWatchProgressRepo_GetNextUp_OrdersByRecency proves Next Up surfaces the
// series the profile watched most recently first — and, under a LIMIT, keeps the
// most-recent series rather than the alphabetically-first ones. The old query
// ordered the DISTINCT ON result by parent_id, so it returned series in id order
// and a LIMIT could drop the series you just watched.
func TestWatchProgressRepo_GetNextUp_OrdersByRecency(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	wpRepo := postgres.NewWatchProgressRepository(db)
	ctx := context.Background()

	u := newTestUser("nuu1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("nup1", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("nulib1")
	require.NoError(t, libRepo.Create(ctx, lib))

	mkSeries := func(id string) *library.MediaItem {
		it := newTestMediaItem(id, lib.ID)
		it.Type = library.Series
		return it
	}
	mkEpisode := func(id, parent string, idx int) *library.MediaItem {
		it := newTestMediaItem(id, lib.ID)
		it.Type = library.Episode
		it.ParentID = parent
		n := idx
		it.IndexNumber = &n
		return it
	}
	// Series "nu-aaa" sorts before "nu-zzz" by id but is watched LESS recently.
	for _, it := range []*library.MediaItem{
		mkSeries("nu-aaa"), mkSeries("nu-zzz"),
		mkEpisode("nu-aaa-e1", "nu-aaa", 1), mkEpisode("nu-aaa-e2", "nu-aaa", 2),
		mkEpisode("nu-zzz-e1", "nu-zzz", 1), mkEpisode("nu-zzz-e2", "nu-zzz", 2),
	} {
		require.NoError(t, itemRepo.Create(ctx, it))
	}

	older := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Millisecond)
	newer := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Millisecond)
	require.NoError(t, wpRepo.Upsert(ctx, &playback.WatchProgress{
		ID: "nu-wp-a1", ProfileID: p.ID, MediaItemID: "nu-aaa-e1",
		Completed: true, LastPlayedAt: older, Position: mustWPDuration(t, 1000),
	}))
	require.NoError(t, wpRepo.Upsert(ctx, &playback.WatchProgress{
		ID: "nu-wp-z1", ProfileID: p.ID, MediaItemID: "nu-zzz-e1",
		Completed: true, LastPlayedAt: newer, Position: mustWPDuration(t, 1000),
	}))

	// LIMIT 1 must return the next episode of the most-recently-watched series.
	got, err := wpRepo.GetNextUp(ctx, string(p.ID), 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, shared.MediaItemID("nu-zzz-e2"), got[0].MediaItemID)

	// Full list is ordered by recency: most-recent series first.
	all, err := wpRepo.GetNextUp(ctx, string(p.ID), 10)
	require.NoError(t, err)
	require.Len(t, all, 2)
	assert.Equal(t, shared.MediaItemID("nu-zzz-e2"), all[0].MediaItemID)
	assert.Equal(t, shared.MediaItemID("nu-aaa-e2"), all[1].MediaItemID)
}

func TestWatchProgressRepo_DeleteByProfile(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	wpRepo := postgres.NewWatchProgressRepository(db)
	ctx := context.Background()

	u := newTestUser("wpdelu")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("wpdelp", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("wpdellib")
	require.NoError(t, libRepo.Create(ctx, lib))

	for _, id := range []string{"wpd1", "wpd2"} {
		item := newTestMediaItem(id, lib.ID)
		require.NoError(t, itemRepo.Create(ctx, item))
		require.NoError(t, wpRepo.Upsert(ctx, &playback.WatchProgress{
			ID: "wp-" + id, ProfileID: p.ID, MediaItemID: item.ID,
			Position: mustWPDuration(t, 1000), LastPlayedAt: time.Now().UTC().Truncate(time.Millisecond),
		}))
	}

	deleted, err := wpRepo.DeleteByProfile(ctx, string(p.ID))
	require.NoError(t, err)
	assert.Equal(t, 2, deleted)

	_, err = wpRepo.GetByProfileAndMedia(ctx, string(p.ID), "wpd1")
	require.Error(t, err)

	// Idempotent: a second clear removes nothing.
	deleted, err = wpRepo.DeleteByProfile(ctx, string(p.ID))
	require.NoError(t, err)
	assert.Equal(t, 0, deleted)
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
