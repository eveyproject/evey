package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/feed"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/playback"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Recommended must return only items sharing at least one of the profile's
// genres (the GIN ?| pre-filter must be behaviour-equivalent to the old
// per-row score>=1 filter).
func TestFeedQuerier_Recommended_GenrePrefilter(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	fq := postgres.NewFeedQuerier(db)
	ctx := context.Background()

	lib := newTestLibrary("recolib")
	require.NoError(t, libRepo.Create(ctx, lib))
	mk := func(id string, genres []string) {
		it := newTestMediaItem(id, lib.ID)
		it.Catalog.Genres = genres
		require.NoError(t, itemRepo.Create(ctx, it))
	}
	mk("rec-a", []string{"Action", "Drama"})
	mk("rec-b", []string{"Comedy"})
	mk("rec-c", []string{"Action"})

	items, err := fq.Recommended(ctx, "prf-x", []string{"Action"}, 10)
	require.NoError(t, err)
	got := map[string]bool{}
	for _, it := range items {
		got[string(it.MediaItemID)] = true
	}
	assert.True(t, got["rec-a"], "rec-a shares Action")
	assert.True(t, got["rec-c"], "rec-c shares Action")
	assert.False(t, got["rec-b"], "rec-b shares no genre and must be excluded")
}

// SimilarItems must return only items sharing a genre with the target, ranked
// by the number of shared genres, excluding the target itself.
func TestFeedQuerier_SimilarItems_GenrePrefilter(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	fq := postgres.NewFeedQuerier(db)
	ctx := context.Background()

	lib := newTestLibrary("simlib")
	require.NoError(t, libRepo.Create(ctx, lib))
	mk := func(id string, genres []string) {
		it := newTestMediaItem(id, lib.ID)
		it.Catalog.Genres = genres
		require.NoError(t, itemRepo.Create(ctx, it))
	}
	mk("sim-t", []string{"Action", "Drama"}) // target
	mk("sim-a", []string{"Action"})          // shares 1
	mk("sim-b", []string{"Comedy"})          // shares 0
	mk("sim-c", []string{"Action", "Drama"}) // shares 2

	items, err := fq.SimilarItems(ctx, "sim-t", "prf-x", 10)
	require.NoError(t, err)

	scores := map[string]int{}
	for _, it := range items {
		scores[string(it.MediaItemID)] = it.Score
	}
	_, hasTarget := scores["sim-t"]
	assert.False(t, hasTarget, "the target itself must be excluded")
	_, hasB := scores["sim-b"]
	assert.False(t, hasB, "an item sharing no genre must be excluded")
	assert.Equal(t, 1, scores["sim-a"], "sim-a shares 1 genre")
	assert.Equal(t, 2, scores["sim-c"], "sim-c shares 2 genres")
	// Results must be ordered by shared-genre score descending.
	require.GreaterOrEqual(t, len(items), 2)
	for i := 1; i < len(items); i++ {
		assert.GreaterOrEqual(t, items[i-1].Score, items[i].Score, "items must be score-ordered")
	}
}

// seedSeries builds series -> season -> episodes with index numbers, each
// episode with a file of the given duration, and returns the episode ids.
func seedSeries(t *testing.T, itemRepo *postgres.MediaItemRepository, libID shared.LibraryID, prefix string, seasonNo int, episodes int, dur time.Duration) (seriesID string, episodeIDs []string) {
	t.Helper()
	ctx := context.Background()
	series := newTestMediaItem(prefix+"-series", libID)
	series.Type = library.Series
	require.NoError(t, itemRepo.Create(ctx, series))
	season := newTestMediaItem(prefix+"-s"+string(rune('0'+seasonNo)), libID)
	season.Type = library.Season
	season.ParentID = string(series.ID)
	season.IndexNumber = new(seasonNo)
	require.NoError(t, itemRepo.Create(ctx, season))
	d, err := shared.NewDurationFromTime(dur)
	require.NoError(t, err)
	for i := 1; i <= episodes; i++ {
		ep := newTestMediaItem(prefix+"-e"+string(rune('0'+i)), libID)
		ep.Type = library.Episode
		ep.ParentID = string(season.ID)
		ep.IndexNumber = new(i)
		ep.File = &library.MediaFile{Path: "/media/" + string(ep.ID) + ".mkv", Duration: d}
		require.NoError(t, itemRepo.Create(ctx, ep))
		episodeIDs = append(episodeIDs, string(ep.ID))
	}
	return string(series.ID), episodeIDs
}

func seedProgress(t *testing.T, db *sql.DB, profileID, mediaItemID string, position time.Duration, completed bool, at time.Time) {
	t.Helper()
	pos, err := shared.NewDurationFromTime(position)
	require.NoError(t, err)
	require.NoError(t, postgres.NewWatchProgressRepository(db).Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-" + profileID + "-" + mediaItemID, ProfileID: shared.ProfileID(profileID), MediaItemID: shared.MediaItemID(mediaItemID),
		Position: pos, Completed: completed, PlayCount: 1, LastPlayedAt: at,
	}))
}

// The home "continue watching" row: a film resumes only past the 5% floor,
// a completed film is gone, an in-progress episode resumes, and a series the
// profile has touched offers its first unwatched episode - once per series.
func TestFeedQuerier_ContinueWatching_AndNextEpisode(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	u := newTestUser("feed-u")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	p := newTestProfile("feed-p", u.ID)
	require.NoError(t, postgres.NewProfileRepository(db).Create(ctx, p))
	lib := newTestLibrary("feedlib")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	itemRepo := postgres.NewMediaItemRepository(db)
	fq := postgres.NewFeedQuerier(db)

	twoHours, _ := shared.NewDurationFromTime(2 * time.Hour)
	mk := func(id string) {
		it := newTestMediaItem(id, lib.ID)
		it.File = &library.MediaFile{Path: "/media/" + id + ".mkv", Duration: twoHours}
		require.NoError(t, itemRepo.Create(ctx, it))
	}
	mk("feed-resume")
	mk("feed-sampled")
	mk("feed-done")
	now := time.Now().UTC().Truncate(time.Millisecond)
	seedProgress(t, db, "feed-p", "feed-resume", 30*time.Minute, false, now.Add(-time.Hour))
	seedProgress(t, db, "feed-p", "feed-sampled", 2*time.Minute, false, now)
	seedProgress(t, db, "feed-p", "feed-done", 2*time.Hour, true, now)

	_, eps := seedSeries(t, itemRepo, lib.ID, "feedA", 1, 3, 40*time.Minute)
	seedProgress(t, db, "feed-p", eps[0], 40*time.Minute, true, now.Add(-2*time.Hour))
	seedProgress(t, db, "feed-p", eps[1], 10*time.Minute, false, now.Add(-30*time.Minute))
	_, epsB := seedSeries(t, itemRepo, lib.ID, "feedB", 1, 2, 40*time.Minute)
	seedProgress(t, db, "feed-p", epsB[0], 40*time.Minute, true, now.Add(-3*time.Hour))

	items, err := fq.ContinueWatching(ctx, "feed-p", 20)
	require.NoError(t, err)
	got := map[string]*feed.FeedItem{}
	for _, it := range items {
		got[string(it.MediaItemID)] = it
	}
	require.Contains(t, got, "feed-resume", "a film past the resume floor")
	assert.Equal(t, (30 * time.Minute).Milliseconds(), got["feed-resume"].Position.Milliseconds())
	assert.NotContains(t, got, "feed-sampled", "2 minutes of a 2-hour film is below the 5%% floor")
	assert.NotContains(t, got, "feed-done", "a completed film is not resumable")
	require.Contains(t, got, eps[1], "the in-progress episode resumes")
	assert.Equal(t, 1, got[eps[1]].SeasonNumber)
	assert.Equal(t, 2, got[eps[1]].EpisodeNumber)
	// Observed 2026-09-16: a series with an in-progress episode ALSO gets its
	// next unwatched episode as a next-up card (the exclusion is per episode,
	// not per series). Pinned as-is; whether the row should show one card per
	// series is a product call, not this test's.
	require.Contains(t, got, eps[2])
	assert.Zero(t, got[eps[2]].Position.Milliseconds())
	require.Contains(t, got, epsB[1], "series B: the first unwatched episode is offered")
	assert.NotContains(t, got, epsB[0])
	assert.Zero(t, got[epsB[1]].Position.Milliseconds())

	next, err := fq.NextEpisode(ctx, "feed-p", 20)
	require.NoError(t, err)
	nextIDs := make([]string, 0, len(next))
	for _, it := range next {
		nextIDs = append(nextIDs, string(it.MediaItemID))
	}
	assert.ElementsMatch(t, []string{eps[1], epsB[1]}, nextIDs, "one next episode per series with a completed episode: the first not completed")

	// "Resume playback" is the same set ordered by the last interaction
	// (a next-up card dated by the series' last completed episode), with the
	// limit taking the most recent first.
	resume, err := fq.ResumePlayback(ctx, "feed-p", 20)
	require.NoError(t, err)
	resumeIDs := make([]string, 0, len(resume))
	for _, it := range resume {
		resumeIDs = append(resumeIDs, string(it.MediaItemID))
	}
	assert.Equal(t, []string{eps[1], "feed-resume", eps[2], epsB[1]}, resumeIDs)
	top, err := fq.ResumePlayback(ctx, "feed-p", 2)
	require.NoError(t, err)
	require.Len(t, top, 2)
	assert.Equal(t, eps[1], string(top[0].MediaItemID))
	assert.Equal(t, "feed-resume", string(top[1].MediaItemID))

	// Another profile sees none of this.
	other, err := fq.ContinueWatching(ctx, "feed-nobody", 20)
	require.NoError(t, err)
	assert.Empty(t, other)
	other, err = fq.ResumePlayback(ctx, "feed-nobody", 20)
	require.NoError(t, err)
	assert.Empty(t, other)
}

// "Recently added": a film by its own added_at, a series by its NEWEST
// episode (an old show with a fresh episode re-surfaces), a fully watched
// film leaves the row, and a series leaves only once EVERY episode is
// completed - a new episode brings it back.
func TestFeedQuerier_RecentlyAdded_WindowAndWatchedFilter(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	u := newTestUser("ra-u")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	p := newTestProfile("ra-p", u.ID)
	require.NoError(t, postgres.NewProfileRepository(db).Create(ctx, p))
	lib := newTestLibrary("ralib")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	itemRepo := postgres.NewMediaItemRepository(db)
	fq := postgres.NewFeedQuerier(db)
	now := time.Now().UTC().Truncate(time.Millisecond)

	mkMovie := func(id string, added time.Time) {
		it := newTestMediaItem(id, lib.ID)
		it.AddedAt = added
		require.NoError(t, itemRepo.Create(ctx, it))
	}
	mkMovie("ra-fresh", now.Add(-24*time.Hour))
	mkMovie("ra-old", now.Add(-90*24*time.Hour))
	mkMovie("ra-watched", now.Add(-2*time.Hour))
	seedProgress(t, db, "ra-p", "ra-watched", 2*time.Hour, true, now)

	// An old series whose latest episode landed yesterday.
	seriesID, eps := seedSeries(t, itemRepo, lib.ID, "raS", 1, 2, 40*time.Minute)
	_, err := db.ExecContext(ctx, `UPDATE media_items SET added_at = $1 WHERE id = $2`, now.Add(-200*24*time.Hour), seriesID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE media_items SET added_at = $1 WHERE id = $2`, now.Add(-200*24*time.Hour), eps[0])
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE media_items SET added_at = $1 WHERE id = $2`, now.Add(-12*time.Hour), eps[1])
	require.NoError(t, err)

	ids := func(items []*feed.FeedItem) []string {
		out := make([]string, 0, len(items))
		for _, it := range items {
			out = append(out, string(it.MediaItemID))
		}
		return out
	}
	items, err := fq.RecentlyAdded(ctx, "ra-p", 30, 20)
	require.NoError(t, err)
	assert.Equal(t, []string{seriesID, "ra-fresh"}, ids(items), "newest first; the old film is outside the window, the watched film is hidden, the series rides its newest episode")

	// Without a profile nothing is filtered on watch state.
	items, err = fq.RecentlyAdded(ctx, "", 30, 20)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{seriesID, "ra-fresh", "ra-watched"}, ids(items))

	// Completing every episode hides the series; a new episode brings it back.
	seedProgress(t, db, "ra-p", eps[0], 40*time.Minute, true, now)
	seedProgress(t, db, "ra-p", eps[1], 40*time.Minute, true, now)
	items, err = fq.RecentlyAdded(ctx, "ra-p", 30, 20)
	require.NoError(t, err)
	assert.Equal(t, []string{"ra-fresh"}, ids(items))
	e3 := newTestMediaItem("raS-e3", lib.ID)
	e3.Type = library.Episode
	e3.ParentID = "raS-s1"
	e3.IndexNumber = new(3)
	e3.AddedAt = now
	require.NoError(t, itemRepo.Create(ctx, e3))
	items, err = fq.RecentlyAdded(ctx, "ra-p", 30, 20)
	require.NoError(t, err)
	assert.Equal(t, []string{seriesID, "ra-fresh"}, ids(items), "an unwatched new episode re-surfaces the series, first")

	// The window is honoured: a 1-day window drops yesterday's film.
	items, err = fq.RecentlyAdded(ctx, "", 1, 20)
	require.NoError(t, err)
	assert.Equal(t, []string{seriesID, "ra-watched"}, ids(items))
}
