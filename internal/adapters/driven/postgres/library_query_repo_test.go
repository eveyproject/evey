package postgres_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMediaItemQueryRepoAdapter_GetCastByMediaItem(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	personRepo := postgres.NewPersonRepository(db)
	mpRepo := postgres.NewMediaPersonRepository(db)
	queryRepo := postgres.NewMediaItemQueryRepoAdapter(db)

	lib := newTestLibrary("cq-lib1")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("cq-mi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	thumb := "/thumbs/actor1.jpg"
	p1 := &library.Person{ID: "cq-p1", Name: "Alice Doe", ProviderIDs: map[string]string{}, ThumbPath: &thumb}
	p2 := &library.Person{ID: "cq-p2", Name: "Bob Smith", ProviderIDs: map[string]string{}, ThumbPath: nil}
	require.NoError(t, personRepo.Create(ctx, p1))
	require.NoError(t, personRepo.Create(ctx, p2))

	char := "Hero"
	require.NoError(t, mpRepo.Create(ctx, &library.MediaPerson{
		MediaItemID:   item.ID,
		PersonID:      p1.ID,
		RoleType:      "actor",
		CharacterName: &char,
		SortOrder:     0,
	}))
	require.NoError(t, mpRepo.Create(ctx, &library.MediaPerson{
		MediaItemID:   item.ID,
		PersonID:      p2.ID,
		RoleType:      "director",
		CharacterName: nil,
		SortOrder:     1,
	}))

	cast, err := queryRepo.GetCastByMediaItem(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, cast, 2)

	assert.Equal(t, p1.ID, cast[0].PersonID)
	assert.Equal(t, "Alice Doe", cast[0].PersonName)
	assert.Equal(t, "actor", cast[0].RoleType)
	require.NotNil(t, cast[0].CharacterName)
	assert.Equal(t, "Hero", *cast[0].CharacterName)
	assert.Equal(t, 0, cast[0].SortOrder)
	require.NotNil(t, cast[0].ThumbPath)
	assert.Equal(t, thumb, *cast[0].ThumbPath)

	assert.Equal(t, p2.ID, cast[1].PersonID)
	assert.Equal(t, "Bob Smith", cast[1].PersonName)
	assert.Equal(t, "director", cast[1].RoleType)
	assert.Nil(t, cast[1].CharacterName)
	assert.Equal(t, 1, cast[1].SortOrder)
	assert.Nil(t, cast[1].ThumbPath)
}

func TestMediaItemQueryRepoAdapter_GetCastByMediaItem_Empty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	queryRepo := postgres.NewMediaItemQueryRepoAdapter(db)

	lib := newTestLibrary("cq-lib2")
	require.NoError(t, libRepo.Create(ctx, lib))

	item := newTestMediaItem("cq-mi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	cast, err := queryRepo.GetCastByMediaItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Empty(t, cast)
}

func TestMediaItemQueryRepoAdapter_GetCastForItems(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	personRepo := postgres.NewPersonRepository(db)
	mpRepo := postgres.NewMediaPersonRepository(db)
	queryRepo := postgres.NewMediaItemQueryRepoAdapter(db)

	lib := newTestLibrary("cq-blib1")
	require.NoError(t, libRepo.Create(ctx, lib))

	item1 := newTestMediaItem("cq-bmi1", lib.ID)
	item2 := newTestMediaItem("cq-bmi2", lib.ID)
	item3 := newTestMediaItem("cq-bmi3", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item1))
	require.NoError(t, itemRepo.Create(ctx, item2))
	require.NoError(t, itemRepo.Create(ctx, item3))

	thumb := "/thumbs/p.jpg"
	p1 := &library.Person{ID: "cq-bp1", Name: "Actor One", ProviderIDs: map[string]string{}, ThumbPath: &thumb}
	p2 := &library.Person{ID: "cq-bp2", Name: "Actor Two", ProviderIDs: map[string]string{}, ThumbPath: nil}
	require.NoError(t, personRepo.Create(ctx, p1))
	require.NoError(t, personRepo.Create(ctx, p2))

	char := "Hero"
	require.NoError(t, mpRepo.Create(ctx, &library.MediaPerson{MediaItemID: item1.ID, PersonID: p1.ID, RoleType: "actor", CharacterName: &char, SortOrder: 0}))
	require.NoError(t, mpRepo.Create(ctx, &library.MediaPerson{MediaItemID: item2.ID, PersonID: p2.ID, RoleType: "director", CharacterName: nil, SortOrder: 0}))
	// item3 has no cast

	result, err := queryRepo.GetCastForItems(ctx, []shared.MediaItemID{item1.ID, item2.ID, item3.ID})
	require.NoError(t, err)

	require.Len(t, result[item1.ID], 1)
	assert.Equal(t, p1.ID, result[item1.ID][0].PersonID)
	assert.Equal(t, "actor", result[item1.ID][0].RoleType)

	require.Len(t, result[item2.ID], 1)
	assert.Equal(t, p2.ID, result[item2.ID][0].PersonID)
	assert.Equal(t, "director", result[item2.ID][0].RoleType)

	assert.Empty(t, result[item3.ID])
}

func TestMediaItemQueryRepoAdapter_GetCastForItems_Empty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	queryRepo := postgres.NewMediaItemQueryRepoAdapter(db)
	result, err := queryRepo.GetCastForItems(context.Background(), []shared.MediaItemID{})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestMediaItemQueryRepoAdapter_List_Count(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	queryRepo := postgres.NewMediaItemQueryRepoAdapter(db)

	lib := newTestLibrary("cq-lib3")
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("cq-mi3a", lib.ID)))
	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("cq-mi3b", lib.ID)))
	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("cq-mi3c", lib.ID)))

	libID := lib.ID
	filter := library.ListMediaItemsFilter{LibraryID: &libID, Limit: 10}

	list, err := queryRepo.List(ctx, filter)
	require.NoError(t, err)
	assert.Len(t, list, 3)

	count, err := queryRepo.Count(ctx, filter)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestMediaItemQueryRepoAdapter_ListGenres(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	queryRepo := postgres.NewMediaItemQueryRepoAdapter(db)

	lib := newTestLibrary("cq-genres-lib")
	require.NoError(t, libRepo.Create(ctx, lib))
	other := newTestLibrary("cq-genres-other")
	require.NoError(t, libRepo.Create(ctx, other))

	a := newTestMediaItem("cq-g-a", lib.ID)
	a.Catalog.Genres = []string{"Drama", "Comedy"}
	require.NoError(t, itemRepo.Create(ctx, a))

	b := newTestMediaItem("cq-g-b", lib.ID)
	b.Catalog.Genres = []string{"Comedy", "Action"} // Comedy duplicated across items
	require.NoError(t, itemRepo.Create(ctx, b))

	empty := newTestMediaItem("cq-g-empty", lib.ID)
	empty.Catalog.Genres = []string{}
	require.NoError(t, itemRepo.Create(ctx, empty))

	// Item in a different library must not leak into this library's genres.
	leak := newTestMediaItem("cq-g-leak", other.ID)
	leak.Catalog.Genres = []string{"Horror"}
	require.NoError(t, itemRepo.Create(ctx, leak))

	genres, err := queryRepo.ListGenres(ctx, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"Action", "Comedy", "Drama"}, genres,
		"distinct + sorted, scoped to the library, Horror excluded")

	// Library with no items at all returns no genres (not an error).
	none, err := queryRepo.ListGenres(ctx, shared.LibraryID("cq-genres-nonexistent"))
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestMediaItemRepo_List_GenresOR(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)

	lib := newTestLibrary("cq-genor-lib")
	require.NoError(t, libRepo.Create(ctx, lib))

	action := newTestMediaItem("cq-go-action", lib.ID)
	action.Catalog.Genres = []string{"Action"}
	require.NoError(t, itemRepo.Create(ctx, action))

	drama := newTestMediaItem("cq-go-drama", lib.ID)
	drama.Catalog.Genres = []string{"Drama", "Romance"}
	require.NoError(t, itemRepo.Create(ctx, drama))

	horror := newTestMediaItem("cq-go-horror", lib.ID)
	horror.Catalog.Genres = []string{"Horror"}
	require.NoError(t, itemRepo.Create(ctx, horror))

	libID := lib.ID
	list, err := itemRepo.List(ctx, library.ListMediaItemsFilter{
		LibraryID: &libID,
		Genres:    []string{"Action", "Drama"},
		Limit:     10,
	})
	require.NoError(t, err)
	ids := make(map[string]bool)
	for _, it := range list {
		ids[string(it.ID)] = true
	}
	assert.Len(t, list, 2, "OR match: an item with Action OR Drama is kept")
	assert.True(t, ids["cq-go-action"])
	assert.True(t, ids["cq-go-drama"])
	assert.False(t, ids["cq-go-horror"], "Horror-only item must be excluded")
}

func TestMediaItemRepo_List_AllowedRatings(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)

	lib := newTestLibrary("cq-lib4")
	require.NoError(t, libRepo.Create(ctx, lib))

	pg := newTestMediaItem("cq-mi-pg", lib.ID)
	pg.Classification = &library.MediaClassification{OfficialRating: "PG"}
	require.NoError(t, itemRepo.Create(ctx, pg))

	r := newTestMediaItem("cq-mi-r", lib.ID)
	r.Classification = &library.MediaClassification{OfficialRating: "R"}
	require.NoError(t, itemRepo.Create(ctx, r))

	noRating := newTestMediaItem("cq-mi-norating", lib.ID)
	noRating.Classification = nil
	require.NoError(t, itemRepo.Create(ctx, noRating))

	libID := lib.ID
	filter := library.ListMediaItemsFilter{
		LibraryID:      &libID,
		AllowedRatings: []string{"G", "TV-Y", "TV-Y7", "TV-G", "PG", "TV-PG", "PG-13", "TV-14"},
		Limit:          10,
	}

	list, err := itemRepo.List(ctx, filter)
	require.NoError(t, err)
	assert.Len(t, list, 2, "PG and empty-rating items should be returned; R should be excluded")

	ids := make(map[string]bool)
	for _, item := range list {
		ids[string(item.ID)] = true
	}
	assert.True(t, ids["cq-mi-pg"])
	assert.True(t, ids["cq-mi-norating"])
	assert.False(t, ids["cq-mi-r"])
}

// The autoplay/warm-up handoff: the next episode is the next index in the
// same season, else the first episode of the next season, and nil (no
// error) after the finale. Season and episode aggregates count episodes
// through the whole tree.
func TestMediaItemQueryRepoAdapter_NextEpisodeOf_AndSeriesAggregate(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	lib := newTestLibrary("nextlib")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	itemRepo := postgres.NewMediaItemRepository(db)
	queryRepo := postgres.NewMediaItemQueryRepoAdapter(db)

	seriesID, s1 := seedSeries(t, itemRepo, lib.ID, "nx", 1, 2, 40*time.Minute)
	// Season 2 hangs off the same series: two more episodes.
	s2 := newTestMediaItem("nx-s2", lib.ID)
	s2.Type = library.Season
	s2.ParentID = seriesID
	s2.IndexNumber = new(2)
	require.NoError(t, itemRepo.Create(ctx, s2))
	d, _ := shared.NewDurationFromTime(50 * time.Minute)
	for i := 1; i <= 2; i++ {
		ep := newTestMediaItem("nx-s2e"+string(rune('0'+i)), lib.ID)
		ep.Type = library.Episode
		ep.ParentID = string(s2.ID)
		ep.IndexNumber = new(i)
		ep.File = &library.MediaFile{Path: "/media/" + string(ep.ID) + ".mkv", Duration: d}
		require.NoError(t, itemRepo.Create(ctx, ep))
	}
	// A special without an index number is never "next" and never breaks the walk.
	sp := newTestMediaItem("nx-special", lib.ID)
	sp.Type = library.Episode
	sp.ParentID = string(s2.ID)
	require.NoError(t, itemRepo.Create(ctx, sp))

	next, err := queryRepo.NextEpisodeOf(ctx, shared.MediaItemID(s1[0]))
	require.NoError(t, err)
	require.NotNil(t, next)
	assert.Equal(t, s1[1], string(next.ID), "same season, next index")

	next, err = queryRepo.NextEpisodeOf(ctx, shared.MediaItemID(s1[1]))
	require.NoError(t, err)
	require.NotNil(t, next)
	assert.Equal(t, "nx-s2e1", string(next.ID), "end of season 1: first episode of season 2")

	next, err = queryRepo.NextEpisodeOf(ctx, "nx-s2e2")
	require.NoError(t, err)
	assert.Nil(t, next, "the finale has no next episode, and that is not an error")

	next, err = queryRepo.NextEpisodeOf(ctx, "nx-special")
	require.NoError(t, err)
	assert.Nil(t, next, "an unnumbered episode cannot be walked from")

	agg, err := queryRepo.SeriesAggregate(ctx, shared.MediaItemID(seriesID))
	require.NoError(t, err)
	assert.Equal(t, 5, agg.EpisodeCount, "every episode in the tree, the special included")
	assert.Equal(t, 2, agg.SeasonCount)
	assert.Equal(t, (2*40*time.Minute + 2*50*time.Minute).Milliseconds(), agg.TotalDuration.Milliseconds(), "only episodes with a file count toward the runtime")

	empty, err := queryRepo.SeriesAggregate(ctx, "ghost")
	require.NoError(t, err)
	assert.Zero(t, empty.EpisodeCount)
}

// RefreshFileState is the "file unchanged" fast path of a repair: it stamps
// the new size/mtime/fingerprint on the file row without touching streams,
// keyed by path, and is a no-op for an unknown path. BackfillEmptySortTitles
// repairs pre-fix rows once and only once.
func TestMediaItemRepo_RefreshFileState_AndBackfillSortTitles(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	lib := newTestLibrary("rfslib")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	itemRepo := postgres.NewMediaItemRepository(db)
	queryRepo := postgres.NewMediaItemQueryRepoAdapter(db)

	item := newTestMediaItem("rfs-1", lib.ID)
	item.File = &library.MediaFile{Path: "/media/rfs/one.mkv", Size: 100, Fingerprint: "100-1"}
	require.NoError(t, itemRepo.Create(ctx, item))

	mod := time.Date(2026, 9, 16, 3, 0, 0, 0, time.UTC)
	require.NoError(t, itemRepo.RefreshFileState(ctx, "/media/rfs/one.mkv", 250, mod))
	got, err := queryRepo.GetByFilePath(ctx, "/media/rfs/one.mkv")
	require.NoError(t, err)
	require.NotNil(t, got.File)
	assert.EqualValues(t, 250, got.File.Size)
	assert.Equal(t, "250-"+strconv.FormatInt(mod.UnixNano(), 10), got.File.Fingerprint, "the scanner's size-mtime format")
	require.NoError(t, itemRepo.RefreshFileState(ctx, "/media/rfs/missing.mkv", 1, mod), "an unknown path is a no-op")
	_, err = queryRepo.GetByFilePath(ctx, "/media/rfs/missing.mkv")
	require.Error(t, err)

	// Rows left with an empty sort_title by a pre-fix scan.
	for _, id := range []string{"rfs-bf1", "rfs-bf2"} {
		it := newTestMediaItem(id, lib.ID)
		it.Title = "The " + id
		it.SortTitle = "x"
		require.NoError(t, itemRepo.Create(ctx, it))
	}
	_, err = db.ExecContext(ctx, `UPDATE media_items SET sort_title = '' WHERE id IN ('rfs-bf1', 'rfs-bf2')`)
	require.NoError(t, err)
	n, err := itemRepo.BackfillEmptySortTitles(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	fixed, err := queryRepo.GetByID(ctx, "rfs-bf1")
	require.NoError(t, err)
	assert.Equal(t, library.NormalizeSortTitle("The rfs-bf1"), fixed.SortTitle)
	n, err = itemRepo.BackfillEmptySortTitles(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "idempotent")
}
