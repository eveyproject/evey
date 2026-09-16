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
// series the profile watched most recently first - and, under a LIMIT, keeps the
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

// TestWatchProgressRepo_Upsert_StaleWriteDoesNotRegressPosition proves a tick
// that arrives late (e.g. a retried request, or a second device whose tick
// was in flight before a fresher one landed) cannot roll the stored position
// backwards. The guard is on last_played_at freshness, not on position value,
// so it must NOT block a legitimate rewind (see the next test).
func TestWatchProgressRepo_Upsert_StaleWriteDoesNotRegressPosition(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	wpRepo := postgres.NewWatchProgressRepository(db)
	ctx := context.Background()

	u := newTestUser("wpu4")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("wpp4", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("wplib4")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("wpmi4", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	now := time.Now().UTC().Truncate(time.Millisecond)
	older := now.Add(-10 * time.Second)

	// Fresher tick lands first: position 90s.
	require.NoError(t, wpRepo.Upsert(ctx, &playback.WatchProgress{
		ID: "wp4", ProfileID: p.ID, MediaItemID: item.ID,
		Position: mustWPDuration(t, 90000), LastPlayedAt: now,
	}))

	// A stale tick (older last_played_at) arrives afterwards - e.g. a second
	// device's tick that was in flight, or a client retry of a request the
	// server already applied. It must NOT drag the position back to 20s.
	require.NoError(t, wpRepo.Upsert(ctx, &playback.WatchProgress{
		ID: "wp4", ProfileID: p.ID, MediaItemID: item.ID,
		Position: mustWPDuration(t, 20000), LastPlayedAt: older,
	}))

	got, err := wpRepo.GetByProfileAndMedia(ctx, string(p.ID), string(item.ID))
	require.NoError(t, err)
	assert.Equal(t, int64(90000), got.Position.Milliseconds(), "stale write must not regress position")
	assert.True(t, got.LastPlayedAt.Equal(now), "last_played_at must not move backwards either")
}

// TestWatchProgressRepo_Upsert_VoluntaryRewindIsApplied proves the guard is
// about freshness, not position magnitude: a user who rewinds is followed by
// a NEWER tick reporting a LOWER position, and that must be applied.
func TestWatchProgressRepo_Upsert_VoluntaryRewindIsApplied(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	wpRepo := postgres.NewWatchProgressRepository(db)
	ctx := context.Background()

	u := newTestUser("wpu5")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("wpp5", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("wplib5")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("wpmi5", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	now := time.Now().UTC().Truncate(time.Millisecond)
	later := now.Add(10 * time.Second)

	require.NoError(t, wpRepo.Upsert(ctx, &playback.WatchProgress{
		ID: "wp5", ProfileID: p.ID, MediaItemID: item.ID,
		Position: mustWPDuration(t, 90000), LastPlayedAt: now,
	}))

	// User rewinds to 20s; the next tick reporting that is NEWER, so it must win.
	require.NoError(t, wpRepo.Upsert(ctx, &playback.WatchProgress{
		ID: "wp5", ProfileID: p.ID, MediaItemID: item.ID,
		Position: mustWPDuration(t, 20000), LastPlayedAt: later,
	}))

	got, err := wpRepo.GetByProfileAndMedia(ctx, string(p.ID), string(item.ID))
	require.NoError(t, err)
	assert.Equal(t, int64(20000), got.Position.Milliseconds(), "a fresher tick reporting a rewind must be applied")
	assert.True(t, got.LastPlayedAt.Equal(later))
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

// UpsertMany must carry Upsert's EXACT conflict semantics (sticky completed,
// freshness-guarded position, monotonic last_played_at) in one statement -
// the flusher used to issue one round trip per viewer per tick.
func TestWatchProgressRepo_UpsertMany_BatchSemantics(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewWatchProgressRepository(db)
	ctx := context.Background()

	u := newTestUser("um-u")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("um-p", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("um-lib")
	require.NoError(t, libRepo.Create(ctx, lib))
	for _, id := range []string{"um-m1", "um-m2"} {
		require.NoError(t, itemRepo.Create(ctx, newTestMediaItem(id, lib.ID)))
	}

	base := time.Now().UTC().Truncate(time.Millisecond)
	mk := func(id, media string, posMs int64, completed bool, at time.Time) *playback.WatchProgress {
		pos, err := shared.NewDurationMs(posMs)
		require.NoError(t, err)
		return &playback.WatchProgress{
			ID: id, ProfileID: p.ID, MediaItemID: shared.MediaItemID(media),
			Position: pos, Completed: completed, LastPlayedAt: at,
		}
	}

	// Fresh batch insert of two entries in ONE call.
	require.NoError(t, repo.UpsertMany(ctx, []*playback.WatchProgress{
		mk("um-w1", "um-m1", 10_000, false, base),
		mk("um-w2", "um-m2", 20_000, true, base),
	}))
	got, err := repo.GetByProfileAndMedia(ctx, string(p.ID), "um-m1")
	require.NoError(t, err)
	assert.Equal(t, int64(10_000), got.Position.Milliseconds())

	// Conflict batch: m1 fresher tick advances; m2 STALE tick must not drag
	// the position back, and completed stays sticky.
	require.NoError(t, repo.UpsertMany(ctx, []*playback.WatchProgress{
		mk("um-w3", "um-m1", 30_000, false, base.Add(time.Minute)),
		mk("um-w4", "um-m2", 5_000, false, base.Add(-time.Minute)),
	}))
	got, err = repo.GetByProfileAndMedia(ctx, string(p.ID), "um-m1")
	require.NoError(t, err)
	assert.Equal(t, int64(30_000), got.Position.Milliseconds(), "fresher tick advances")
	got, err = repo.GetByProfileAndMedia(ctx, string(p.ID), "um-m2")
	require.NoError(t, err)
	assert.Equal(t, int64(20_000), got.Position.Milliseconds(), "stale tick rejected")
	assert.True(t, got.Completed, "completed is sticky")

	// Empty batch is a no-op.
	require.NoError(t, repo.UpsertMany(ctx, nil))
}

// The watch-history keyset walk (newest first, tied timestamps broken by
// media id), the resume shortlist (in-progress rows only), and a targeted
// delete that leaves the profile's other rows alone.
func TestWatchProgressRepo_Cursor_Resume_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	u := newTestUser("wpc-u")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	p := newTestProfile("wpc-p", u.ID)
	require.NoError(t, postgres.NewProfileRepository(db).Create(ctx, p))
	lib := newTestLibrary("wpclib")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	itemRepo := postgres.NewMediaItemRepository(db)
	wpRepo := postgres.NewWatchProgressRepository(db)
	now := time.Now().UTC().Truncate(time.Millisecond)
	rows := []struct {
		id        string
		pos       int64
		completed bool
		at        time.Time
	}{
		{"wpc-a", 1000, false, now.Add(-3 * time.Hour)},
		{"wpc-b", 0, false, now.Add(-time.Hour)}, // never really started
		{"wpc-c", 5000, true, now.Add(-time.Hour)},
		{"wpc-d", 9000, false, now},
	}
	for _, r := range rows {
		require.NoError(t, itemRepo.Create(ctx, newTestMediaItem(r.id, lib.ID)))
		require.NoError(t, wpRepo.Upsert(ctx, &playback.WatchProgress{
			ID: "wp-" + r.id, ProfileID: p.ID, MediaItemID: shared.MediaItemID(r.id),
			Position: mustWPDuration(t, r.pos), Completed: r.completed, PlayCount: 1, LastPlayedAt: r.at,
		}))
	}
	ids := func(in []*playback.WatchProgress) []string {
		out := make([]string, 0, len(in))
		for _, w := range in {
			out = append(out, string(w.MediaItemID))
		}
		return out
	}

	page, err := wpRepo.GetByProfileCursor(ctx, string(p.ID), nil, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"wpc-d", "wpc-c"}, ids(page), "newest first, the tie broken by media id descending")
	last := page[len(page)-1]
	page, err = wpRepo.GetByProfileCursor(ctx, string(p.ID), &playback.WatchHistoryCursor{LastPlayedAt: last.LastPlayedAt, MediaItemID: string(last.MediaItemID)}, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"wpc-b", "wpc-a"}, ids(page), "the tied sibling is neither repeated nor lost")
	last = page[len(page)-1]
	page, err = wpRepo.GetByProfileCursor(ctx, string(p.ID), &playback.WatchHistoryCursor{LastPlayedAt: last.LastPlayedAt, MediaItemID: string(last.MediaItemID)}, 2)
	require.NoError(t, err)
	assert.Empty(t, page)

	resume, err := wpRepo.GetResumeItems(ctx, string(p.ID), 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"wpc-d", "wpc-a"}, ids(resume), "in progress only: no completed row, no zero position")

	require.NoError(t, wpRepo.Delete(ctx, string(p.ID), "wpc-d"))
	resume, err = wpRepo.GetResumeItems(ctx, string(p.ID), 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"wpc-a"}, ids(resume))
	require.NoError(t, wpRepo.Delete(ctx, string(p.ID), "wpc-d"), "deleting twice is a no-op")
	all, err := wpRepo.GetByProfileCursor(ctx, string(p.ID), nil, 10)
	require.NoError(t, err)
	assert.Len(t, all, 3, "the other rows stay")
}
