package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/playback"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// media_watched is the one rule behind every "already seen": the home rails,
// the library lists (UnwatchedBy) and the mark each answer carries (UserData).
// Each case here is a way the copies of it once disagreed.
func TestMediaWatched_OneRuleForListsAndMarks(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	u := newTestUser("mw-u")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	p := newTestProfile("mw-p", u.ID)
	require.NoError(t, postgres.NewProfileRepository(db).Create(ctx, p))
	other := newTestProfile("mw-other", u.ID)
	require.NoError(t, postgres.NewProfileRepository(db).Create(ctx, other))
	lib := newTestLibrary("mwlib")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	itemRepo := postgres.NewMediaItemRepository(db)
	wpRepo := postgres.NewWatchProgressRepository(db)

	mk := func(id, parent string, typ library.MediaItemType) {
		it := newTestMediaItem(id, lib.ID)
		it.Type = typ
		it.ParentID = parent
		require.NoError(t, itemRepo.Create(ctx, it))
	}
	mk("mw-marked", "", library.MovieItem)  // marked by hand: completed, no position
	mk("mw-started", "", library.MovieItem) // half seen
	mk("mw-fresh", "", library.MovieItem)   // never touched
	mk("mw-done", "", library.Series)       // every episode completed, no container row
	mk("mw-done-s1", "mw-done", library.Season)
	mk("mw-done-e1", "mw-done-s1", library.Episode)
	mk("mw-done-e2", "mw-done-s1", library.Episode)
	mk("mw-part", "", library.Series) // one episode left
	mk("mw-part-s1", "mw-part", library.Season)
	mk("mw-part-s2", "mw-part", library.Season)
	mk("mw-part-e1", "mw-part-s1", library.Episode)
	mk("mw-part-e2", "mw-part-s2", library.Episode)
	mk("mw-empty", "", library.Series) // no episode yet: nothing was watched

	now := time.Now().UTC().Truncate(time.Millisecond)
	put := func(profile shared.ProfileID, item string, posMs int64, completed bool) {
		wp := &playback.WatchProgress{
			ID: "wp-" + string(profile) + "-" + item, ProfileID: profile, MediaItemID: shared.MediaItemID(item),
			Completed: completed, PlayCount: 1, LastPlayedAt: now,
		}
		if posMs > 0 {
			wp.Position = mustWPDuration(t, posMs)
		}
		require.NoError(t, wpRepo.Upsert(ctx, wp))
	}
	put(p.ID, "mw-marked", 0, true)
	put(p.ID, "mw-started", 600000, false)
	put(p.ID, "mw-done-e1", 0, true)
	put(p.ID, "mw-done-e2", 1200000, true)
	put(p.ID, "mw-part-e1", 0, true)
	put(other.ID, "mw-fresh", 0, true) // another profile's viewing is not this one's

	ids := []string{"mw-marked", "mw-started", "mw-fresh", "mw-done", "mw-done-s1", "mw-part", "mw-part-s1", "mw-part-s2", "mw-empty", "mw-unknown"}
	ud, err := wpRepo.UserData(ctx, string(p.ID), ids)
	require.NoError(t, err)
	played := map[string]bool{}
	for _, id := range ids {
		require.Contains(t, ud, id, "every asked id answers, an unknown one too")
		played[id] = ud[id].Played
	}
	assert.Equal(t, map[string]bool{
		"mw-marked": true, "mw-started": false, "mw-fresh": false,
		"mw-done": true, "mw-done-s1": true,
		"mw-part": false, "mw-part-s1": true, "mw-part-s2": false,
		"mw-empty": false, "mw-unknown": false,
	}, played)
	assert.Equal(t, int64(600000), ud["mw-started"].Position.Milliseconds(), "the progress row rides along")

	libID := lib.ID
	movies, series := string(library.MovieItem), string(library.Series)
	unwatched := func(typ *string) []string {
		f := library.ListMediaItemsFilter{LibraryID: &libID, Type: typ, UnwatchedBy: string(p.ID), SortField: "title"}
		items, err := itemRepo.List(ctx, f)
		require.NoError(t, err)
		n, err := itemRepo.Count(ctx, f)
		require.NoError(t, err)
		require.Equal(t, len(items), n, "the count reads the same filter")
		out := make([]string, 0, len(items))
		for _, it := range items {
			out = append(out, string(it.ID))
		}
		return out
	}
	assert.ElementsMatch(t, []string{"mw-started", "mw-fresh"}, unwatched(&movies), "a film marked by hand leaves the rail")
	assert.ElementsMatch(t, []string{"mw-part", "mw-empty"}, unwatched(&series), "a series leaves once every episode is done")
}

// The watched overlay the clients filtered by was a 500-row page of the watch
// history, newest first. Marking a long series writes one row per episode, so
// an older film marked watched fell off that page and came back on the home.
// The rule above has no window.
func TestMediaWatched_NoWindowAfterLongSeriesMark(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	u := newTestUser("mwl-u")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	p := newTestProfile("mwl-p", u.ID)
	require.NoError(t, postgres.NewProfileRepository(db).Create(ctx, p))
	lib := newTestLibrary("mwllib")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	itemRepo := postgres.NewMediaItemRepository(db)
	wpRepo := postgres.NewWatchProgressRepository(db)

	film := newTestMediaItem("mwl-film", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, film))
	show := newTestMediaItem("mwl-show", lib.ID)
	show.Type = library.Series
	require.NoError(t, itemRepo.Create(ctx, show))
	season := newTestMediaItem("mwl-s1", lib.ID)
	season.Type, season.ParentID = library.Season, "mwl-show"
	require.NoError(t, itemRepo.Create(ctx, season))

	old := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Millisecond)
	require.NoError(t, wpRepo.Upsert(ctx, &playback.WatchProgress{
		ID: "wpl-film", ProfileID: p.ID, MediaItemID: "mwl-film", Completed: true, PlayCount: 1, LastPlayedAt: old,
	}))
	const episodes = 600
	now := time.Now().UTC().Truncate(time.Millisecond)
	entries := make([]*playback.WatchProgress, 0, episodes)
	for i := range episodes {
		id := fmt.Sprintf("mwl-e%03d", i)
		ep := newTestMediaItem(id, lib.ID)
		ep.Type, ep.ParentID = library.Episode, "mwl-s1"
		require.NoError(t, itemRepo.Create(ctx, ep))
		entries = append(entries, &playback.WatchProgress{
			ID: "wpl-" + id, ProfileID: p.ID, MediaItemID: shared.MediaItemID(id), Completed: true, PlayCount: 1, LastPlayedAt: now,
		})
	}
	require.NoError(t, wpRepo.UpsertMany(ctx, entries))

	window, err := wpRepo.GetWatchStates(ctx, string(p.ID), 500)
	require.NoError(t, err)
	inWindow := false
	for _, wp := range window {
		inWindow = inWindow || wp.MediaItemID == "mwl-film"
	}
	assert.False(t, inWindow, "the old overlay's window lost the film: what this rule replaces")

	ud, err := wpRepo.UserData(ctx, string(p.ID), []string{"mwl-film", "mwl-show"})
	require.NoError(t, err)
	assert.True(t, ud["mwl-film"].Played, "the film is still watched")
	assert.True(t, ud["mwl-show"].Played, "and so is the series")
}
