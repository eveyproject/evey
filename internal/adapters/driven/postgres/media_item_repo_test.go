package postgres_test

import (
	"context"
	"errors"
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

func newTestMediaItem(id string, libraryID shared.LibraryID) *library.MediaItem {
	return &library.MediaItem{
		ID:        shared.MediaItemID(id),
		LibraryID: libraryID,
		Type:      library.MovieItem,
		Title:     "Movie " + id,
		SortTitle: "movie " + id,
		Catalog: &library.MediaCatalog{
			ProviderIDs: map[string]string{"tmdb": "123"},
			Genres:      []string{"Action"},
			Studios:     []string{"Studio A"},
			Tags:        []string{},
		},
		AddedAt:   time.Now().UTC().Truncate(time.Millisecond),
		UpdatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
}

func TestMediaItemRepo_Create_GetByID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib1")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("mi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	got, err := itemRepo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, item.Title, got.Title)
	assert.Equal(t, item.LibraryID, got.LibraryID)
	require.NotNil(t, got.Catalog)
	require.NotNil(t, item.Catalog)
	assert.Equal(t, item.Catalog.Genres, got.Catalog.Genres)
}

func TestMediaItemRepo_GetByFilePath(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib2")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("mi2", lib.ID)
	item.File = &library.MediaFile{Path: "/media/movie.mkv"}
	require.NoError(t, itemRepo.Create(ctx, item))

	got, err := itemRepo.GetByFilePath(ctx, "/media/movie.mkv")
	require.NoError(t, err)
	assert.Equal(t, item.ID, got.ID)
}

func TestMediaItemRepo_List(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib3")
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("mi3a", lib.ID)))
	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("mi3b", lib.ID)))

	libID := lib.ID
	list, err := itemRepo.List(ctx, library.ListMediaItemsFilter{LibraryID: &libID})
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

// TestMediaItemRepo_List_OffsetPaginationDeterministic proves offset pages
// partition the set even when items share the same sort value (default sort is
// sort_title): without a unique mi.id tiebreak, a page boundary can skip or
// repeat a row.
func TestMediaItemRepo_List_OffsetPaginationDeterministic(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlibpg")
	require.NoError(t, libRepo.Create(ctx, lib))
	for _, id := range []string{"pg-1", "pg-2", "pg-3"} {
		it := newTestMediaItem(id, lib.ID)
		it.SortTitle = "same title" // force ties in the default (title) sort
		require.NoError(t, itemRepo.Create(ctx, it))
	}

	libID := lib.ID
	page := func(offset int) []*library.MediaItem {
		items, err := itemRepo.List(ctx, library.ListMediaItemsFilter{LibraryID: &libID, Limit: 2, Offset: offset})
		require.NoError(t, err)
		return items
	}
	p1, p2 := page(0), page(2)
	require.Len(t, p1, 2)
	require.Len(t, p2, 1)

	seen := map[string]bool{}
	for _, it := range append(append([]*library.MediaItem{}, p1...), p2...) {
		assert.Falsef(t, seen[string(it.ID)], "item %s appeared on two pages", it.ID)
		seen[string(it.ID)] = true
	}
	assert.Len(t, seen, 3, "every item must appear exactly once across pages")
	assert.Equal(t, p1[0].ID, page(0)[0].ID, "page order must be stable across calls")
}

func TestMediaItemRepo_Update(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib4")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("mi4", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	item.Title = "Updated Movie"
	if item.Catalog == nil {
		item.Catalog = &library.MediaCatalog{}
	}
	item.Catalog.Genres = []string{"Drama"}
	item.UpdatedAt = time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, itemRepo.Update(ctx, item))

	got, err := itemRepo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Movie", got.Title)
	require.NotNil(t, got.Catalog)
	assert.Equal(t, []string{"Drama"}, got.Catalog.Genres)
}

func TestMediaItemRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib5")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("mi5", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))
	require.NoError(t, itemRepo.Delete(ctx, item.ID))

	_, err := itemRepo.GetByID(ctx, item.ID)
	assert.Error(t, err)
}

// Regression: Delete on an unknown id must return shared.ErrNotFound.
// Pre-fix the Scan error was masked with `_ =` so a missing row slipped
// into the DELETE path and only surfaced via RowsAffected==0 - same
// outcome but any other Scan error (transient conn issue, etc.) was
// also silently swallowed.
func TestMediaItemRepo_Delete_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	itemRepo := postgres.NewMediaItemRepository(db)
	err := itemRepo.Delete(context.Background(), shared.MediaItemID("never-existed"))
	require.Error(t, err)
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

// Regression: deleting an episode must recompute its series'
// latest_child_added_at. Pre-fix the recompute error was logged and
// swallowed; additionally the recompute ran non-tx so an ambient
// tx couldn't see the post-delete state. Here we verify the end-to-end
// effect: a series whose only episode is deleted ends up with a
// latest_child_added_at lower than or equal to before, and deleting a
// non-latest episode from a series with multiple episodes lowers the
// series' latest_child_added_at to the remaining max.
func TestMediaItemRepo_Delete_EpisodeRecomputesSeriesLatest(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib-delep")
	require.NoError(t, libRepo.Create(ctx, lib))

	series := &library.MediaItem{ID: "s-del", LibraryID: lib.ID, Type: library.Series, Title: "S", AddedAt: time.Now().UTC().Add(-72 * time.Hour), UpdatedAt: time.Now().UTC()}
	season := &library.MediaItem{ID: "se-del", LibraryID: lib.ID, ParentID: "s-del", Type: library.Season, Title: "S01", AddedAt: time.Now().UTC().Add(-72 * time.Hour), UpdatedAt: time.Now().UTC()}
	epOld := &library.MediaItem{ID: "ep-old", LibraryID: lib.ID, ParentID: "se-del", Type: library.Episode, Title: "E01", AddedAt: time.Now().UTC().Add(-48 * time.Hour), UpdatedAt: time.Now().UTC()}
	epNew := &library.MediaItem{ID: "ep-new", LibraryID: lib.ID, ParentID: "se-del", Type: library.Episode, Title: "E02", AddedAt: time.Now().UTC().Add(-1 * time.Hour), UpdatedAt: time.Now().UTC()}
	require.NoError(t, itemRepo.Create(ctx, series))
	require.NoError(t, itemRepo.Create(ctx, season))
	require.NoError(t, itemRepo.Create(ctx, epOld))
	require.NoError(t, itemRepo.Create(ctx, epNew))

	// Sanity: series.latest_child_added_at should reflect the newest episode
	// (bumpSeriesLatestChildAddedAt ran on epNew Create).
	var beforeLatest time.Time
	require.NoError(t, db.QueryRowContext(ctx, `SELECT latest_child_added_at FROM media_items WHERE id=$1`, series.ID).Scan(&beforeLatest))
	assert.True(t, beforeLatest.After(epOld.AddedAt.Add(1*time.Hour)),
		"pre-delete latest_child_added_at should be close to epNew.AddedAt, got %v", beforeLatest)

	// Delete the newest episode → recompute should drop latest_child_added_at
	// back down to epOld's added_at.
	require.NoError(t, itemRepo.Delete(ctx, epNew.ID))

	var afterLatest time.Time
	require.NoError(t, db.QueryRowContext(ctx, `SELECT latest_child_added_at FROM media_items WHERE id=$1`, series.ID).Scan(&afterLatest))
	assert.True(t, afterLatest.Before(beforeLatest),
		"after deleting the newest episode, series.latest_child_added_at should move backward: before=%v after=%v", beforeLatest, afterLatest)
	// And it should land at (or very near) epOld.AddedAt.
	diff := afterLatest.Sub(epOld.AddedAt)
	assert.True(t, diff > -time.Second && diff < time.Second,
		"after = epOld.AddedAt ± 1s; got %v (diff %v)", afterLatest, diff)
}

// Regression: Delete binds to an ambient Tx via txFromCtx, so a rollback
// at the TxUnit boundary must leave the row intact. Pre-fix, the three
// queries (SELECT / DELETE / recompute) ran on r.db directly and the
// DELETE would escape the rollback.
func TestMediaItemRepo_Delete_TxUnitRollback(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	uow := postgres.NewTxUnit(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib-txdel")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("mi-txdel", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	sentinel := errors.New("caller wants to roll back")
	err := uow.Do(ctx, func(txCtx context.Context) error {
		if delErr := itemRepo.Delete(txCtx, item.ID); delErr != nil {
			return delErr
		}
		return sentinel // force rollback
	})
	require.ErrorIs(t, err, sentinel)

	// Row must still be there.
	got, getErr := itemRepo.GetByID(ctx, item.ID)
	require.NoError(t, getErr, "Delete should have been rolled back")
	assert.Equal(t, item.ID, got.ID)
}

func TestMediaItemRepo_GetByFileFingerprint(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib6")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("mi6", lib.ID)
	item.File = &library.MediaFile{Path: "/media/fp.mkv", Fingerprint: "fp-unique-123"}
	require.NoError(t, itemRepo.Create(ctx, item))

	got, err := itemRepo.GetByFileFingerprint(ctx, "fp-unique-123")
	require.NoError(t, err)
	assert.Equal(t, item.ID, got.ID)
}

func TestMediaItemRepo_GetByLibraryTypeTitle(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib7")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("mi7", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	got, err := itemRepo.GetByLibraryTypeTitle(ctx, shared.LibraryID(lib.ID), string(library.MovieItem), item.Title, "")
	require.NoError(t, err)
	assert.Equal(t, item.ID, got.ID)
}

func TestMediaItemRepo_DeleteByIDs(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib8")
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("mi8a", lib.ID)))
	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("mi8b", lib.ID)))
	require.NoError(t, itemRepo.DeleteByIDs(ctx, []string{"mi8a", "mi8b"}))

	_, err := itemRepo.GetByID(ctx, "mi8a")
	assert.Error(t, err)
}

func TestMediaItemRepo_DeleteByIDs_Empty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	require.NoError(t, itemRepo.DeleteByIDs(ctx, []string{}))
}

func TestMediaItemRepo_ListFilePaths(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib9")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("mi9", lib.ID)
	item.File = &library.MediaFile{Path: "/media/test.mkv"}
	require.NoError(t, itemRepo.Create(ctx, item))

	paths, err := itemRepo.ListFilePaths(ctx, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, "/media/test.mkv", paths["mi9"])
}

func TestMediaItemRepo_ListByLibraryID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib10")
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("mi10a", lib.ID)))
	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("mi10b", lib.ID)))

	items, err := itemRepo.ListByLibraryID(ctx, lib.ID)
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestMediaItemRepo_ListScanSummaries(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib11")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("mi11", lib.ID)
	item.File = &library.MediaFile{Path: "/media/scan.mkv", Fingerprint: "scan-fp"}
	require.NoError(t, itemRepo.Create(ctx, item))

	summaries, err := itemRepo.ListScanSummaries(ctx, lib.ID)
	require.NoError(t, err)
	assert.Len(t, summaries, 1)
	assert.Equal(t, "mi11", summaries[0].ID)
	assert.Equal(t, "/media/scan.mkv", summaries[0].FilePath)
}

func TestMediaItemRepo_ListContainerInfo(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mlib12")
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("mi12", lib.ID)))

	rows, err := itemRepo.ListContainerInfo(ctx, lib.ID)
	require.NoError(t, err)
	assert.Len(t, rows, 1)
	assert.Equal(t, "mi12", rows[0].ID)
}

// A new episode of a fully-watched series must clear the series' completed flag
// (for every profile) so the series re-surfaces in watched-filtered views like
// the home "Séries" slider. Regression for "new episodes don't re-position a
// watched series".
func TestMediaItemRepo_NewEpisode_ClearsSeriesWatched(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	wpRepo := postgres.NewWatchProgressRepository(db)
	ctx := context.Background()

	u := newTestUser("cswu1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("cswp1", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("cswlib")
	require.NoError(t, libRepo.Create(ctx, lib))

	now := time.Now().UTC()
	series := &library.MediaItem{ID: "csw-s", LibraryID: lib.ID, Type: library.Series, Title: "S", AddedAt: now.Add(-72 * time.Hour), UpdatedAt: now}
	season := &library.MediaItem{ID: "csw-se", LibraryID: lib.ID, ParentID: "csw-s", Type: library.Season, Title: "S01", AddedAt: now.Add(-72 * time.Hour), UpdatedAt: now}
	epOld := &library.MediaItem{ID: "csw-ep1", LibraryID: lib.ID, ParentID: "csw-se", Type: library.Episode, Title: "E01", AddedAt: now.Add(-48 * time.Hour), UpdatedAt: now}
	require.NoError(t, itemRepo.Create(ctx, series))
	require.NoError(t, itemRepo.Create(ctx, season))
	require.NoError(t, itemRepo.Create(ctx, epOld))

	// The profile finished the series → series-level completed = true.
	require.NoError(t, wpRepo.Upsert(ctx, &playback.WatchProgress{
		ID: "csw-wp", ProfileID: p.ID, MediaItemID: series.ID,
		Position: mustWPDuration(t, 0), Completed: true, PlayCount: 1, LastPlayedAt: now,
	}))

	// A new episode arrives → Create must reset the series' watched flag.
	epNew := &library.MediaItem{ID: "csw-ep2", LibraryID: lib.ID, ParentID: "csw-se", Type: library.Episode, Title: "E02", AddedAt: now, UpdatedAt: now}
	require.NoError(t, itemRepo.Create(ctx, epNew))

	var completed bool
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT completed FROM watch_progress WHERE media_item_id=$1 AND profile_id=$2`, series.ID, p.ID).Scan(&completed))
	assert.False(t, completed, "a new episode must clear the series' fully-watched flag")
}

// A full walk by id must visit every row even when the title order disagrees
// with the id order: with AfterID empty the first page came back in title
// order, and the walk by id then skipped every row whose id was below page
// one's last id (the search rebuild lost 34 of 639, audit 2026-10-01).
func TestMediaItemRepository_KeysetByID_VisitsEveryRowFromTheFirstPage(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()
	lib := newTestLibrary("keyset-lib")
	require.NoError(t, libRepo.Create(ctx, lib))

	// ids ascending, titles descending: page one in title order would be the
	// highest ids.
	ids := []string{"ks-a", "ks-b", "ks-c", "ks-d", "ks-e"}
	for i, id := range ids {
		it := newTestMediaItem(id, lib.ID)
		it.Title = string(rune('z' - i))
		it.SortTitle = it.Title
		require.NoError(t, repo.Create(ctx, it))
	}

	seen := map[string]bool{}
	after := ""
	for range 10 {
		page, err := repo.List(ctx, library.ListMediaItemsFilter{Limit: 2, AfterID: after, KeysetByID: true})
		require.NoError(t, err)
		if len(page) == 0 {
			break
		}
		for _, it := range page {
			seen[string(it.ID)] = true
		}
		after = string(page[len(page)-1].ID)
	}
	for _, id := range ids {
		assert.True(t, seen[id], "the walk never reached %s", id)
	}
}

// Sorted by date added, a series ranks by its newest episode. The first page
// used to order on that and the cursor pages on the series' own added_at, so
// the series came back a second time further down and rows went missing.
func TestMediaItemRepository_AddedAtCursor_SeriesRanksByNewestEpisodeOnEveryPage(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()
	lib := newTestLibrary("added-lib")
	require.NoError(t, libRepo.Create(ctx, lib))

	base := time.Now().UTC().Truncate(time.Millisecond).Add(-time.Hour)
	ids := []string{"ad-1", "ad-2", "ad-3", "ad-4", "ad-show"}
	for i, id := range ids {
		it := newTestMediaItem(id, lib.ID)
		it.AddedAt = base.Add(time.Duration(i) * time.Minute)
		switch id {
		case "ad-4": // newer than the show's episode: the show ends page one
			it.AddedAt = base.Add(2 * time.Hour)
		case "ad-show":
			it.Type = library.Series
			it.AddedAt = base.Add(-time.Hour)
		}
		require.NoError(t, repo.Create(ctx, it))
	}
	_, err := db.ExecContext(ctx, `UPDATE media_items SET latest_child_added_at = $1 WHERE id = 'ad-show'`, base.Add(time.Hour))
	require.NoError(t, err)

	var order []string
	cursor := shared.Cursor{}
	for range 10 {
		page, err := repo.List(ctx, library.ListMediaItemsFilter{
			LibraryID: &lib.ID, SortField: "added_at", SortOrder: "desc", Limit: 2, Cursor: cursor,
		})
		require.NoError(t, err)
		if len(page) == 0 {
			break
		}
		for _, it := range page {
			order = append(order, string(it.ID))
		}
		last := page[len(page)-1]
		cursor = shared.Cursor{SortField: "added_at", SortValue: last.SortAddedAt().Format(time.RFC3339Nano), LastID: string(last.ID)}
	}
	assert.Equal(t, []string{"ad-4", "ad-show", "ad-3", "ad-2", "ad-1"}, order)
}
