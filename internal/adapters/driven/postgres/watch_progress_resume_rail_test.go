package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/playback"
)

// The home's single "continue watching" rail, as GetResumeItems serves it to
// every client: a title counts only past MinResumeRailPosition (a play skipped
// after a few seconds or minutes - a shuffle draw, a zap - never does), one
// card per series (the episode left unfinished, else the next one in viewing
// order, across seasons), most recent activity first.
func TestWatchProgressRepo_GetResumeItems_Rail(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	u := newTestUser("rail-u")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	p := newTestProfile("rail-p", u.ID)
	require.NoError(t, postgres.NewProfileRepository(db).Create(ctx, p))
	lib := newTestLibrary("raillib")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	itemRepo := postgres.NewMediaItemRepository(db)
	wpRepo := postgres.NewWatchProgressRepository(db)

	twoHours := 2 * time.Hour
	for _, id := range []string{"rail-film-long", "rail-film-floor", "rail-film-zap", "rail-film-done"} {
		it := newTestMediaItem(id, lib.ID)
		d := mustWPDuration(t, twoHours.Milliseconds())
		it.File = &library.MediaFile{Path: "/media/" + id + ".mkv", Duration: d}
		require.NoError(t, itemRepo.Create(ctx, it))
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	// Over an hour of a film: on the rail.
	seedProgress(t, db, "rail-p", "rail-film-long", 75*time.Minute, false, now.Add(-10*time.Minute))
	// Exactly the floor counts.
	seedProgress(t, db, "rail-p", "rail-film-floor", playback.MinResumeRailPosition, false, now.Add(-7*time.Hour))
	// A draw skipped after two minutes, the most recent thing played: not on it.
	seedProgress(t, db, "rail-p", "rail-film-zap", 2*time.Minute, false, now)
	seedProgress(t, db, "rail-p", "rail-film-done", 2*time.Hour, true, now.Add(-time.Minute))

	// Series A: E1 finished, E2 left at ten minutes - one card, E2 resuming.
	_, epsA := seedSeries(t, itemRepo, lib.ID, "railA", 1, 3, 40*time.Minute)
	seedProgress(t, db, "rail-p", epsA[0], 40*time.Minute, true, now.Add(-3*time.Hour))
	seedProgress(t, db, "rail-p", epsA[1], 10*time.Minute, false, now.Add(-time.Hour))

	// Series B: the last episode of season 1 finished - its next card is the
	// first episode of season 2, dated by that finish.
	seriesB, epsB := seedSeries(t, itemRepo, lib.ID, "railB", 1, 2, 40*time.Minute)
	s2 := newTestMediaItem("railB-s2", lib.ID)
	s2.Type = library.Season
	s2.ParentID = seriesB
	s2.IndexNumber = new(2)
	require.NoError(t, itemRepo.Create(ctx, s2))
	b2e1 := newTestMediaItem("railB-s2e1", lib.ID)
	b2e1.Type = library.Episode
	b2e1.ParentID = string(s2.ID)
	b2e1.IndexNumber = new(1)
	require.NoError(t, itemRepo.Create(ctx, b2e1))
	seedProgress(t, db, "rail-p", epsB[0], 40*time.Minute, true, now.Add(-5*time.Hour))
	seedProgress(t, db, "rail-p", epsB[1], 40*time.Minute, true, now.Add(-2*time.Hour))

	// Series C: E1 finished, E2 opened for thirty seconds since. The zap is no
	// activity: the card is E2 as the next episode, at no position, dated by E1.
	_, epsC := seedSeries(t, itemRepo, lib.ID, "railC", 1, 3, 40*time.Minute)
	seedProgress(t, db, "rail-p", epsC[0], 40*time.Minute, true, now.Add(-4*time.Hour))
	seedProgress(t, db, "rail-p", epsC[1], 30*time.Second, false, now.Add(-time.Second))

	// Series D: finished to its last episode - nothing to offer.
	_, epsD := seedSeries(t, itemRepo, lib.ID, "railD", 1, 2, 40*time.Minute)
	seedProgress(t, db, "rail-p", epsD[0], 40*time.Minute, true, now.Add(-6*time.Hour))
	seedProgress(t, db, "rail-p", epsD[1], 40*time.Minute, true, now.Add(-6*time.Hour+time.Minute))

	// Series E: only zapped - never on the rail.
	_, epsE := seedSeries(t, itemRepo, lib.ID, "railE", 1, 2, 40*time.Minute)
	seedProgress(t, db, "rail-p", epsE[0], time.Minute, false, now.Add(-2*time.Second))

	got, err := wpRepo.GetResumeItems(ctx, string(p.ID), 20)
	require.NoError(t, err)
	ids := make([]string, 0, len(got))
	byID := map[string]*playback.WatchProgress{}
	for _, wp := range got {
		ids = append(ids, string(wp.MediaItemID))
		byID[string(wp.MediaItemID)] = wp
	}
	assert.Equal(t, []string{
		"rail-film-long",  // -10m
		epsA[1],           // -1h
		"railB-s2e1",      // -2h, the finish of S1E2
		epsC[1],           // -4h, the finish of E1
		"rail-film-floor", // -7h
	}, ids)
	for _, id := range []string{"rail-film-long", epsA[1], "railB-s2e1", epsC[1]} {
		require.Contains(t, byID, id)
	}
	assert.Equal(t, (75 * time.Minute).Milliseconds(), byID["rail-film-long"].Position.Milliseconds())
	assert.Equal(t, (10 * time.Minute).Milliseconds(), byID[epsA[1]].Position.Milliseconds())
	assert.Zero(t, byID["railB-s2e1"].Position.Milliseconds(), "a next episode carries no position")
	assert.Zero(t, byID[epsC[1]].Position.Milliseconds(), "a zapped next episode is offered from its start")
	assert.False(t, byID[epsC[1]].Completed)

	top, err := wpRepo.GetResumeItems(ctx, string(p.ID), 2)
	require.NoError(t, err)
	require.Len(t, top, 2)
	assert.Equal(t, "rail-film-long", string(top[0].MediaItemID))
	assert.Equal(t, epsA[1], string(top[1].MediaItemID))

	other, err := wpRepo.GetResumeItems(ctx, "rail-nobody", 20)
	require.NoError(t, err)
	assert.Empty(t, other)
}

// A finished title played again: the completed flag is sticky, but a second
// viewing left short of the completion point is something to resume, on the
// rail as on the title page. Past that point, below the rail floor, or with no
// known length, the title stays finished.
func TestWatchProgressRepo_GetResumeItems_Rewatch(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	u := newTestUser("rw-u")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	p := newTestProfile("rw-p", u.ID)
	require.NoError(t, postgres.NewProfileRepository(db).Create(ctx, p))
	lib := newTestLibrary("rwlib")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	itemRepo := postgres.NewMediaItemRepository(db)
	wpRepo := postgres.NewWatchProgressRepository(db)

	twoHours := 2 * time.Hour
	for _, id := range []string{"rw-film-mid", "rw-film-late", "rw-film-zap"} {
		it := newTestMediaItem(id, lib.ID)
		it.File = &library.MediaFile{Path: "/media/" + id + ".mkv", Duration: mustWPDuration(t, twoHours.Milliseconds())}
		require.NoError(t, itemRepo.Create(ctx, it))
	}
	// No file, so no known length.
	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("rw-film-nolen", lib.ID)))

	now := time.Now().UTC().Truncate(time.Millisecond)
	finishedAt := now.Add(-48 * time.Hour)
	rewatch := func(id string, pos time.Duration, at time.Time) {
		seedProgress(t, db, "rw-p", id, twoHours, true, finishedAt)
		seedProgress(t, db, "rw-p", id, pos, false, at)
	}
	// Seen, then played again to 40 minutes: back on the rail.
	rewatch("rw-film-mid", 40*time.Minute, now.Add(-time.Hour))
	// Played again past the completion point: finished again.
	rewatch("rw-film-late", 110*time.Minute, now.Add(-2*time.Minute))
	// Played again for a minute: a zap, not a second viewing.
	rewatch("rw-film-zap", time.Minute, now.Add(-3*time.Minute))
	// No length to measure the position against: stays finished.
	rewatch("rw-film-nolen", 40*time.Minute, now.Add(-4*time.Minute))

	// Series: E1 seen, then E1 played again to 15 minutes. The card resumes
	// E1, not the next episode.
	_, eps := seedSeries(t, itemRepo, lib.ID, "rwS", 1, 3, 40*time.Minute)
	seedProgress(t, db, "rw-p", eps[0], 40*time.Minute, true, finishedAt)
	seedProgress(t, db, "rw-p", eps[0], 15*time.Minute, false, now.Add(-30*time.Minute))

	got, err := wpRepo.GetResumeItems(ctx, string(p.ID), 20)
	require.NoError(t, err)
	ids := make([]string, 0, len(got))
	byID := map[string]*playback.WatchProgress{}
	for _, wp := range got {
		ids = append(ids, string(wp.MediaItemID))
		byID[string(wp.MediaItemID)] = wp
	}
	assert.Equal(t, []string{eps[0], "rw-film-mid"}, ids)
	require.Contains(t, byID, "rw-film-mid")
	require.Contains(t, byID, eps[0])
	assert.Equal(t, (40 * time.Minute).Milliseconds(), byID["rw-film-mid"].Position.Milliseconds())
	assert.True(t, byID["rw-film-mid"].Completed, "the flag stays what it is: the title was seen")
	assert.Equal(t, (15 * time.Minute).Milliseconds(), byID[eps[0]].Position.Milliseconds())
}
