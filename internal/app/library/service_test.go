package library_test

import (
	"context"
	"errors"
	"testing"

	applib "github.com/gravity-zero/evey/internal/app/library"
	"github.com/gravity-zero/evey/internal/domain/library"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/mkv"
	mkvfakes "github.com/gravity-zero/evey/internal/domain/mkv/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/gravity-zero/evey/internal/pkg/ratings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCmdSvc() (*applib.CommandService, *libfakes.FakeLibraryCommandRepo, *libfakes.FakeLibraryQueryRepo, *libfakes.FakeMediaItemCommandRepo, *libfakes.FakeFileScanner, *sharedfakes.FakeEventBus) {
	libCmdRepo := libfakes.NewFakeLibraryCommandRepo()
	libQueryRepo := libfakes.NewFakeLibraryQueryRepo()
	mediaRepo := libfakes.NewFakeMediaItemCommandRepo()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	personRepo := libfakes.NewFakePersonRepository()
	mediaPersonRepo := libfakes.NewFakeMediaPersonRepository()
	scanner := libfakes.NewFakeFileScanner()
	prober := libfakes.NewFakeStreamProber()
	search := libfakes.NewFakeSearchIndex()
	bus := sharedfakes.NewFakeEventBus()
	svc := applib.NewCommandService(libCmdRepo, libQueryRepo, mediaRepo, streamRepo, personRepo, mediaPersonRepo, scanner, prober, search, bus)
	return svc, libCmdRepo, libQueryRepo, mediaRepo, scanner, bus
}

func newQuerySvc() (*applib.QueryService, *libfakes.FakeMediaItemQueryRepo, *libfakes.FakeLibraryQueryRepo) {
	libQueryRepo := libfakes.NewFakeLibraryQueryRepo()
	mediaQueryRepo := libfakes.NewFakeMediaItemQueryRepo()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	search := libfakes.NewFakeSearchIndex()
	svc := applib.NewQueryService(libQueryRepo, mediaQueryRepo, streamRepo, search)
	return svc, mediaQueryRepo, libQueryRepo
}

func TestCreateLibrary(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, _, _, bus := newCmdSvc()

	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie"}
	err := svc.CreateLibrary(context.Background(), lib)
	require.NoError(t, err)

	// Verify via query repo (shared backing store: seed it for queries)
	libQueryRepo.Seed(lib)
	querySvc, _, queryLibRepo := newQuerySvc()
	queryLibRepo.Seed(lib)
	got, err := querySvc.GetLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, "Movies", got.Name)

	events := bus.PublishedEvents()
	require.Len(t, events, 1)
	assert.Equal(t, "library.created", events[0].EventName())
}

func TestScanLibrary_NewFiles(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, bus := newCmdSvc()

	lib := &library.Library{
		ID:    "lib-1",
		Name:  "Movies",
		Type:  "movie",
		Paths: []string{"/movies"},
	}
	svc.CreateLibrary(context.Background(), lib)
	libQueryRepo.Seed(lib)

	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film1.mkv", Size: 1000},
		{Path: "/movies/film2.mkv", Size: 2000},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	items := mediaRepo.All()
	assert.Len(t, items, 2)

	for _, item := range items {
		assert.NotEmpty(t, item.ID)
		assert.NotContains(t, item.ID, "wp-")
	}

	var addedEvents []string
	for _, e := range bus.PublishedEvents() {
		if e.EventName() == "media.item.added" {
			addedEvents = append(addedEvents, e.EventName())
		}
	}
	assert.Len(t, addedEvents, 2)
}

func TestScanLibrary_ExistingFile(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, bus := newCmdSvc()

	lib := &library.Library{
		ID:    "lib-1",
		Name:  "Movies",
		Type:  "movie",
		Paths: []string{"/movies"},
	}
	svc.CreateLibrary(context.Background(), lib)
	libQueryRepo.Seed(lib)

	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID:        "item-existing",
		LibraryID: "lib-1",
		FilePath:  "/movies/film1.mkv",
		FileSize:  500,
		Overview:  "Already enriched",
	})

	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film1.mkv", Size: 1500},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	item, err := mediaRepo.GetByID(context.Background(), "item-existing")
	require.NoError(t, err)
	assert.Equal(t, int64(1500), item.FileSize)

	for _, e := range bus.PublishedEvents() {
		if e.EventName() == "media.item.added" {
			t.Fatal("unexpected media.item.added event for existing item with overview")
		}
	}
}

func TestGetMediaItem_NotFound(t *testing.T) {
	t.Parallel()
	querySvc, _, _ := newQuerySvc()
	_, err := querySvc.GetMediaItem(context.Background(), "nonexistent")
	require.Error(t, err)
}

func TestScanTVShow_TransientDBError_NoDuplicate(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID:    "lib-1",
		Name:  "TV Shows",
		Type:  "tvshow",
		Paths: []string{"/tv"},
	})

	s1 := 1
	e1 := 1
	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/show/s01e01.mkv", Size: 100, Title: "MyShow", Season: &s1, Episode: &e1, Type: library.Episode},
	}

	dbErr := errors.New("connection reset by peer")
	mediaRepo.GetByFileFingerprintErr = dbErr

	_, _, err := svc.ScanLibrary(context.Background(), "lib-1")
	require.Error(t, err)
	require.ErrorContains(t, err, "connection reset by peer")

	assert.Empty(t, mediaRepo.All(), "transient DB error must not create items")
}

func TestScanTVShow_TitleLookupTransientError_NoDuplicate(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID:    "lib-1",
		Name:  "TV Shows",
		Type:  "tvshow",
		Paths: []string{"/tv"},
	})

	s1 := 1
	e1 := 1
	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/show/s01e01.mkv", Size: 100, Title: "MyShow", Season: &s1, Episode: &e1, Type: library.Episode},
	}

	dbErr := errors.New("connection reset by peer")
	mediaRepo.GetByLibraryTypeTitleErr = dbErr

	_, _, err := svc.ScanLibrary(context.Background(), "lib-1")
	require.Error(t, err)
	require.ErrorContains(t, err, "connection reset by peer")

	assert.Empty(t, mediaRepo.All(), "transient DB error on title lookup must not create items")
}

func TestDeleteLibrary(t *testing.T) {
	t.Parallel()
	svc, libCmdRepo, libQueryRepo, _, _, bus := newCmdSvc()

	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie"}
	libCmdRepo.Create(context.Background(), lib)
	libQueryRepo.Seed(lib)

	err := svc.DeleteLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	var deletedEvent bool
	for _, e := range bus.PublishedEvents() {
		if e.EventName() == "library.deleted" {
			deletedEvent = true
		}
	}
	assert.True(t, deletedEvent)
}

func TestQueryService_ListLibraries(t *testing.T) {
	t.Parallel()
	querySvc, _, libRepo := newQuerySvc()
	libRepo.Seed(&library.Library{ID: "lib-1", Name: "Movies", Type: "movie"})
	libRepo.Seed(&library.Library{ID: "lib-2", Name: "Shows", Type: "tvshow"})

	libs, err := querySvc.ListLibraries(context.Background())
	require.NoError(t, err)
	assert.Len(t, libs, 2)
}

func TestQueryService_GetCast(t *testing.T) {
	t.Parallel()
	querySvc, mediaQueryRepo, _ := newQuerySvc()
	mediaQueryRepo.SeedCast("item-1", []library.CastMember{
		{PersonID: "p1", PersonName: "Actor One", RoleType: "actor", SortOrder: 0},
		{PersonID: "p2", PersonName: "Actor Two", RoleType: "actor", SortOrder: 1},
	})

	cast, err := querySvc.GetCast(context.Background(), "item-1")
	require.NoError(t, err)
	assert.Len(t, cast, 2)
	assert.Equal(t, "Actor One", cast[0].PersonName)
}

func TestScanLibrary_ExtractsChapters(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, _, scanner, _ := newCmdSvc()

	chapterRepo := libfakes.NewFakeChapterRepository()
	reader := mkvfakes.NewFakeContainerReader()
	reader.Containers["/movies/film.mkv"] = &mkv.Container{
		Path: "/movies/film.mkv",
		Chapters: []mkv.Chapter{
			{ID: 1, Title: "Opening", StartMs: 0, EndMs: 90000},
			{ID: 2, Title: "Act 1", StartMs: 90000, EndMs: 3600000},
		},
	}
	svc.WithChapters(chapterRepo, reader)

	libQueryRepo.Seed(&library.Library{
		ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"},
	})
	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 5000},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	allChapters := chapterRepo.All()
	require.Len(t, allChapters, 2)
	assert.Equal(t, "Opening", allChapters[0].Title)
	assert.Equal(t, int64(0), allChapters[0].StartTimeMs)
	assert.Equal(t, int64(90000), allChapters[0].EndTimeMs)
	assert.Equal(t, "Act 1", allChapters[1].Title)
}

func TestGetChapters_NilRepo(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _, _ := newCmdSvc()
	chapters, err := svc.GetChapters(context.Background(), "any-id")
	require.NoError(t, err)
	assert.Nil(t, chapters)
}

func TestGenerateTrickplay_Success(t *testing.T) {
	t.Parallel()
	svc, _, _, mediaRepo, _, _ := newCmdSvc()
	trickplayRepo := libfakes.NewFakeTrickplayRepository()
	trickplayGen := libfakes.NewFakeTrickplayGenerator()
	svc.WithTrickplay(trickplayRepo, trickplayGen, "/cache/trickplay")

	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID:       "media-1",
		FilePath: "/movies/test.mkv",
		Title:    "Test Movie",
	})

	trickplayGen.Results["/movies/test.mkv"] = &library.TrickplayResult{
		ImageDir:   "/cache/trickplay/media-1",
		ImageCount: 3,
	}

	err := svc.GenerateTrickplay(context.Background(), "media-1")
	require.NoError(t, err)

	manifest, err := svc.GetTrickplayManifest(context.Background(), "media-1")
	require.NoError(t, err)
	assert.Equal(t, "media-1", manifest.MediaItemID)
	assert.Equal(t, 10, manifest.Interval)
	assert.Equal(t, 160, manifest.Width)
	assert.Equal(t, 10, manifest.TileWidth)
	assert.Equal(t, 10, manifest.TileHeight)
	assert.Equal(t, 3, manifest.ImageCount)
}

func TestGenerateTrickplay_UsesCacheDir(t *testing.T) {
	t.Parallel()
	svc, _, _, mediaRepo, _, _ := newCmdSvc()
	trickplayRepo := libfakes.NewFakeTrickplayRepository()
	trickplayGen := libfakes.NewFakeTrickplayGenerator()
	svc.WithTrickplay(trickplayRepo, trickplayGen, "/my/custom/cache")

	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID:       "media-2",
		FilePath: "/movies/film.mkv",
		Title:    "Film",
	})

	err := svc.GenerateTrickplay(context.Background(), "media-2")
	require.NoError(t, err)
	assert.Equal(t, "/my/custom/cache/media-2", trickplayGen.LastOutputDir)
}

func TestGenerateTrickplay_MediaNotFound(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _, _ := newCmdSvc()
	trickplayRepo := libfakes.NewFakeTrickplayRepository()
	trickplayGen := libfakes.NewFakeTrickplayGenerator()
	svc.WithTrickplay(trickplayRepo, trickplayGen, "/cache/trickplay")

	err := svc.GenerateTrickplay(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get media item")
}

func TestGenerateTrickplay_NoFilePath(t *testing.T) {
	t.Parallel()
	svc, _, _, mediaRepo, _, _ := newCmdSvc()
	trickplayRepo := libfakes.NewFakeTrickplayRepository()
	trickplayGen := libfakes.NewFakeTrickplayGenerator()
	svc.WithTrickplay(trickplayRepo, trickplayGen, "/cache/trickplay")

	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID:       "media-1",
		FilePath: "",
		Title:    "No File",
	})

	err := svc.GenerateTrickplay(context.Background(), "media-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no file path")
}

func TestGenerateTrickplay_NotConfigured(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _, _ := newCmdSvc()

	err := svc.GenerateTrickplay(context.Background(), "media-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestGetTrickplayManifest_NotFound(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _, _ := newCmdSvc()
	trickplayRepo := libfakes.NewFakeTrickplayRepository()
	trickplayGen := libfakes.NewFakeTrickplayGenerator()
	svc.WithTrickplay(trickplayRepo, trickplayGen, "/cache/trickplay")

	_, err := svc.GetTrickplayManifest(context.Background(), "nonexistent")
	require.Error(t, err)
}

func TestListMediaItems_ParentalFilter_ExcludesRatedR(t *testing.T) {
	t.Parallel()
	querySvc, mediaQueryRepo, _ := newQuerySvc()

	mediaQueryRepo.Seed(&library.MediaItem{ID: "g-movie", Title: "G Movie", OfficialRating: "G"})
	mediaQueryRepo.Seed(&library.MediaItem{ID: "pg-movie", Title: "PG Movie", OfficialRating: "PG"})
	mediaQueryRepo.Seed(&library.MediaItem{ID: "r-movie", Title: "R Movie", OfficialRating: "R"})
	mediaQueryRepo.Seed(&library.MediaItem{ID: "nc17-movie", Title: "NC-17 Movie", OfficialRating: "NC-17"})

	items, err := querySvc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{
		AllowedRatings: ratings.AllowedRatingsFor("PG"),
	})
	require.NoError(t, err)

	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	assert.Contains(t, ids, "g-movie")
	assert.Contains(t, ids, "pg-movie")
	assert.NotContains(t, ids, "r-movie")
	assert.NotContains(t, ids, "nc17-movie")
}

func TestListMediaItems_ParentalFilter_NoRating(t *testing.T) {
	t.Parallel()
	querySvc, mediaQueryRepo, _ := newQuerySvc()

	mediaQueryRepo.Seed(&library.MediaItem{ID: "g-movie", Title: "G Movie", OfficialRating: "G"})
	mediaQueryRepo.Seed(&library.MediaItem{ID: "r-movie", Title: "R Movie", OfficialRating: "R"})
	mediaQueryRepo.Seed(&library.MediaItem{ID: "unrated", Title: "Unrated Movie", OfficialRating: ""})

	// no AllowedRatings = no parental filter
	items, err := querySvc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{})
	require.NoError(t, err)

	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	assert.Contains(t, ids, "g-movie")
	assert.Contains(t, ids, "r-movie")
	assert.Contains(t, ids, "unrated")
}

func TestListMediaItems_ParentalFilter_UnratedItemVisible(t *testing.T) {
	t.Parallel()
	querySvc, mediaQueryRepo, _ := newQuerySvc()

	mediaQueryRepo.Seed(&library.MediaItem{ID: "unrated", Title: "Unrated Movie", OfficialRating: ""})
	mediaQueryRepo.Seed(&library.MediaItem{ID: "r-movie", Title: "R Movie", OfficialRating: "R"})

	items, err := querySvc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{
		AllowedRatings: ratings.AllowedRatingsFor("PG"),
	})
	require.NoError(t, err)

	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	assert.Contains(t, ids, "unrated", "unrated items should pass through parental filter")
	assert.NotContains(t, ids, "r-movie")
}

func TestScanLibrary_RemovesOrphanedItems(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, bus := newCmdSvc()

	lib := &library.Library{
		ID:    "lib-1",
		Name:  "Movies",
		Type:  "movie",
		Paths: []string{"/movies"},
	}
	svc.CreateLibrary(context.Background(), lib)
	libQueryRepo.Seed(lib)

	// Pre-existing item whose file no longer exists on disk
	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID:        "item-deleted",
		LibraryID: "lib-1",
		FilePath:  "/movies/deleted.mkv",
		FileSize:  1000,
	})
	// Pre-existing item whose file still exists
	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID:        "item-keep",
		LibraryID: "lib-1",
		FilePath:  "/movies/keep.mkv",
		FileSize:  2000,
	})

	// Scanner only returns the surviving file
	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/keep.mkv", Size: 2000},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	items := mediaRepo.All()
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	assert.Contains(t, ids, "item-keep", "surviving file must remain in DB")
	assert.NotContains(t, ids, "item-deleted", "deleted file must be removed from DB")

	var removedEvents []string
	for _, e := range bus.PublishedEvents() {
		if e.EventName() == "media.item.removed" {
			removedEvents = append(removedEvents, e.EventName())
		}
	}
	assert.Len(t, removedEvents, 1, "one media.item.removed event expected")
}

func TestScanLibrary_OrphanCleanup_TVShow_RemovesDeletedEpisode(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, bus := newCmdSvc()

	lib := &library.Library{
		ID:    "lib-tv",
		Name:  "TV",
		Type:  "tvshow",
		Paths: []string{"/tv"},
	}
	svc.CreateLibrary(context.Background(), lib)
	libQueryRepo.Seed(lib)

	// Pre-seed: series, season, and two episodes (one will be "deleted")
	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID:        "ep-keep",
		LibraryID: "lib-tv",
		FilePath:  "/tv/show/s01e01.mkv",
		FileSize:  100,
	})
	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID:        "ep-gone",
		LibraryID: "lib-tv",
		FilePath:  "/tv/show/s01e02.mkv",
		FileSize:  100,
	})

	s1, e1 := 1, 1
	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/show/s01e01.mkv", Size: 100, Title: "MyShow", Season: &s1, Episode: &e1, Type: library.Episode},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	_, err = mediaRepo.GetByID(context.Background(), "ep-gone")
	require.ErrorIs(t, err, shared.ErrNotFound, "deleted episode must be removed from DB")

	_, err = mediaRepo.GetByID(context.Background(), "ep-keep")
	require.NoError(t, err, "surviving episode must remain in DB")

	var removedEvents int
	for _, e := range bus.PublishedEvents() {
		if e.EventName() == "media.item.removed" {
			removedEvents++
		}
	}
	assert.Equal(t, 1, removedEvents)
}

func TestScanLibrary_ItemsRemovedCountInEvent(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, bus := newCmdSvc()

	lib := &library.Library{
		ID:    "lib-1",
		Name:  "Movies",
		Type:  "movie",
		Paths: []string{"/movies"},
	}
	svc.CreateLibrary(context.Background(), lib)
	libQueryRepo.Seed(lib)

	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "gone-1", LibraryID: "lib-1", FilePath: "/movies/gone1.mkv",
	})
	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "gone-2", LibraryID: "lib-1", FilePath: "/movies/gone2.mkv",
	})

	scanner.Results["/movies"] = []library.ScannedFile{}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	var scanCompleted *library.LibraryScanCompleted
	for _, e := range bus.PublishedEvents() {
		if sc, ok := e.(library.LibraryScanCompleted); ok {
			sc := sc
			scanCompleted = &sc
		}
	}
	require.NotNil(t, scanCompleted)
	assert.Equal(t, 2, scanCompleted.ItemsRemoved)
}
