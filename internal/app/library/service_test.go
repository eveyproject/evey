package library_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	applib "github.com/evey-project/evey/internal/app/library"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/mkv"
	mkvfakes "github.com/evey-project/evey/internal/domain/mkv/fakes"
	"github.com/evey-project/evey/internal/domain/ratings"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cmdSvcDeps struct {
	chapterRepo   library.ChapterRepository
	ebmlReader    mkv.ContainerReader
	trickplayRepo library.TrickplayRepository
	trickplayGen  library.TrickplayGenerator
	cacheDir      string
	configRepo    shared.ServerConfigRepository
	prober        *libfakes.FakeStreamProber // optional: pre-seed probe results
}

func newCmdSvcWith(d *cmdSvcDeps) (*applib.CommandService, *libfakes.FakeLibraryCommandRepo, *libfakes.FakeLibraryQueryRepo, *libfakes.FakeMediaItemCommandRepo, *libfakes.FakeFileScanner, *sharedfakes.FakeEventBus) {
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
	if d == nil {
		d = &cmdSvcDeps{}
	}
	if d.prober != nil {
		prober = d.prober
	}
	if d.chapterRepo == nil {
		d.chapterRepo = libfakes.NewFakeChapterRepository()
	}
	if d.ebmlReader == nil {
		d.ebmlReader = mkvfakes.NewFakeContainerReader()
	}
	if d.trickplayRepo == nil {
		d.trickplayRepo = libfakes.NewFakeTrickplayRepository()
	}
	if d.trickplayGen == nil {
		d.trickplayGen = libfakes.NewFakeTrickplayGenerator()
	}
	if d.cacheDir == "" {
		d.cacheDir = "/cache"
	}
	svc := applib.NewCommandService(libCmdRepo, libQueryRepo, mediaRepo, streamRepo, personRepo, mediaPersonRepo, scanner, prober, search, bus,
		d.chapterRepo, d.ebmlReader, d.trickplayRepo, d.trickplayGen, d.cacheDir, d.configRepo,
	)
	return svc, libCmdRepo, libQueryRepo, mediaRepo, scanner, bus
}

func newCmdSvc() (*applib.CommandService, *libfakes.FakeLibraryCommandRepo, *libfakes.FakeLibraryQueryRepo, *libfakes.FakeMediaItemCommandRepo, *libfakes.FakeFileScanner, *sharedfakes.FakeEventBus) {
	return newCmdSvcWith(nil)
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
		{Path: "/movies/film1.mkv", Size: 1000, Title: "Film 1", Type: library.MovieItem},
		{Path: "/movies/film2.mkv", Size: 2000, Title: "Film 2", Type: library.MovieItem},
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
		File:      &library.MediaFile{Path: "/movies/film1.mkv", Size: 500},
		Overview:  "Already enriched",
	})

	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film1.mkv", Size: 1500},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	item, err := mediaRepo.GetByID(context.Background(), "item-existing")
	require.NoError(t, err)
	require.NotNil(t, item.File)
	assert.Equal(t, int64(1500), item.File.Size)

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
	mediaRepo.ListByLibraryIDErr = dbErr

	_, _, err := svc.ScanLibrary(context.Background(), "lib-1")
	require.Error(t, err)
	require.ErrorContains(t, err, "connection reset by peer")

	assert.Empty(t, mediaRepo.All(), "transient DB error must not create items")
}

func TestScanTVShow_CreateError_NoDuplicate(t *testing.T) {
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
	mediaRepo.ErrCreate = dbErr

	_, _, err := svc.ScanLibrary(context.Background(), "lib-1")
	require.Error(t, err)
	require.ErrorContains(t, err, "connection reset by peer")

	assert.Empty(t, mediaRepo.All(), "transient DB error on create must not leave partial items")
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
	chapterRepo := libfakes.NewFakeChapterRepository()
	reader := mkvfakes.NewFakeContainerReader()
	reader.Containers["/movies/film.mkv"] = &mkv.Container{
		Path: "/movies/film.mkv",
		Chapters: []mkv.Chapter{
			{ID: 1, Title: "Opening", StartMs: 0, EndMs: 90000},
			{ID: 2, Title: "Act 1", StartMs: 90000, EndMs: 3600000},
		},
	}
	svc, _, libQueryRepo, _, scanner, _ := newCmdSvcWith(&cmdSvcDeps{chapterRepo: chapterRepo, ebmlReader: reader})

	libQueryRepo.Seed(&library.Library{
		ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"},
	})
	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 5000, Title: "Film", Type: library.MovieItem},
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

func TestGenerateTrickplay_Success(t *testing.T) {
	t.Parallel()
	trickplayRepo := libfakes.NewFakeTrickplayRepository()
	trickplayGen := libfakes.NewFakeTrickplayGenerator()
	cacheDir := t.TempDir()
	svc, _, _, mediaRepo, _, _ := newCmdSvcWith(&cmdSvcDeps{trickplayRepo: trickplayRepo, trickplayGen: trickplayGen, cacheDir: cacheDir})

	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID:    "media-1",
		File:  &library.MediaFile{Path: "/movies/test.mkv"},
		Title: "Test Movie",
	})

	trickplayGen.Results["/movies/test.mkv"] = &library.TrickplayResult{
		ImageDir:   cacheDir + "/media-1",
		ImageCount: 3,
	}

	err := svc.GenerateTrickplay(context.Background(), "media-1")
	require.NoError(t, err)

	manifest, err := trickplayRepo.GetByMediaItemID(context.Background(), "media-1")
	require.NoError(t, err)
	assert.Equal(t, shared.MediaItemID("media-1"), manifest.MediaItemID)
	assert.Equal(t, 7, manifest.Interval)
	assert.Equal(t, 320, manifest.Width)
	assert.Equal(t, 5, manifest.TileWidth)
	assert.Equal(t, 5, manifest.TileHeight)
	assert.Equal(t, 3, manifest.ImageCount)
}

func TestGenerateTrickplay_UsesCacheDir(t *testing.T) {
	t.Parallel()
	trickplayRepo := libfakes.NewFakeTrickplayRepository()
	trickplayGen := libfakes.NewFakeTrickplayGenerator()
	cacheDir := t.TempDir()
	svc, _, _, mediaRepo, _, _ := newCmdSvcWith(&cmdSvcDeps{trickplayRepo: trickplayRepo, trickplayGen: trickplayGen, cacheDir: cacheDir})

	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID:    "media-2",
		File:  &library.MediaFile{Path: "/movies/film.mkv"},
		Title: "Film",
	})

	err := svc.GenerateTrickplay(context.Background(), "media-2")
	require.NoError(t, err)
	assert.Equal(t, cacheDir+"/media-2", trickplayGen.LastOutputDir)
}

func TestGenerateTrickplay_MediaNotFound(t *testing.T) {
	t.Parallel()
	trickplayRepo := libfakes.NewFakeTrickplayRepository()
	trickplayGen := libfakes.NewFakeTrickplayGenerator()
	svc, _, _, _, _, _ := newCmdSvcWith(&cmdSvcDeps{trickplayRepo: trickplayRepo, trickplayGen: trickplayGen, cacheDir: "/cache/trickplay"})

	err := svc.GenerateTrickplay(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get media item")
}

func TestGenerateTrickplay_NoFilePath(t *testing.T) {
	t.Parallel()
	trickplayRepo := libfakes.NewFakeTrickplayRepository()
	trickplayGen := libfakes.NewFakeTrickplayGenerator()
	svc, _, _, mediaRepo, _, _ := newCmdSvcWith(&cmdSvcDeps{trickplayRepo: trickplayRepo, trickplayGen: trickplayGen, cacheDir: "/cache/trickplay"})

	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID:    "media-1",
		File:  &library.MediaFile{Path: ""},
		Title: "No File",
	})

	err := svc.GenerateTrickplay(context.Background(), "media-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no file path")
}

// --- Trickplay: waitForIdleStreams ---

type fakeStreamChecker struct {
	mu    sync.Mutex
	count int
}

func (f *fakeStreamChecker) ActivePlaybackCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.count
}

func TestGenerateTrickplay_BlocksWhileStreamsActive(t *testing.T) {
	t.Parallel()
	trickplayRepo := libfakes.NewFakeTrickplayRepository()
	trickplayGen := libfakes.NewFakeTrickplayGenerator()
	svc, _, _, mediaRepo, _, _ := newCmdSvcWith(&cmdSvcDeps{trickplayRepo: trickplayRepo, trickplayGen: trickplayGen, cacheDir: "/cache/trickplay"})

	checker := &fakeStreamChecker{count: 1}
	svc.SetTrickplayActiveStreamChecker(checker)

	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID:    "media-wait",
		File:  &library.MediaFile{Path: "/movies/test.mkv"},
		Title: "Test",
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		svc.GenerateTrickplay(ctx, "media-wait")
		close(done)
	}()

	// With active streams, GenerateTrickplay should block
	select {
	case <-done:
		t.Fatal("GenerateTrickplay should block while streams are active")
	case <-time.After(200 * time.Millisecond):
	}

	// Cancel context to unblock
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("GenerateTrickplay should unblock on context cancel")
	}
}

func TestGenerateTrickplay_ProceedsWhenNoActiveStreams(t *testing.T) {
	t.Parallel()
	trickplayRepo := libfakes.NewFakeTrickplayRepository()
	trickplayGen := libfakes.NewFakeTrickplayGenerator()
	svc, _, _, mediaRepo, _, _ := newCmdSvcWith(&cmdSvcDeps{trickplayRepo: trickplayRepo, trickplayGen: trickplayGen, cacheDir: t.TempDir()})

	checker := &fakeStreamChecker{count: 0}
	svc.SetTrickplayActiveStreamChecker(checker)

	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID:    "media-idle",
		File:  &library.MediaFile{Path: "/movies/test.mkv"},
		Title: "Test",
	})

	err := svc.GenerateTrickplay(context.Background(), "media-idle")
	require.NoError(t, err)
}

func TestGenerateTrickplay_NilCheckerProceeds(t *testing.T) {
	t.Parallel()
	trickplayRepo := libfakes.NewFakeTrickplayRepository()
	trickplayGen := libfakes.NewFakeTrickplayGenerator()
	svc, _, _, mediaRepo, _, _ := newCmdSvcWith(&cmdSvcDeps{trickplayRepo: trickplayRepo, trickplayGen: trickplayGen, cacheDir: t.TempDir()})
	// no SetActiveStreamChecker call → nil checker

	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID:    "media-nil-check",
		File:  &library.MediaFile{Path: "/movies/test.mkv"},
		Title: "Test",
	})

	err := svc.GenerateTrickplay(context.Background(), "media-nil-check")
	require.NoError(t, err)
}

func TestListMediaItems_ParentalFilter_ExcludesRatedR(t *testing.T) {
	t.Parallel()
	querySvc, mediaQueryRepo, _ := newQuerySvc()

	mediaQueryRepo.Seed(&library.MediaItem{ID: "g-movie", Title: "G Movie", Classification: &library.MediaClassification{OfficialRating: "G"}})
	mediaQueryRepo.Seed(&library.MediaItem{ID: "pg-movie", Title: "PG Movie", Classification: &library.MediaClassification{OfficialRating: "PG"}})
	mediaQueryRepo.Seed(&library.MediaItem{ID: "r-movie", Title: "R Movie", Classification: &library.MediaClassification{OfficialRating: "R"}})
	mediaQueryRepo.Seed(&library.MediaItem{ID: "nc17-movie", Title: "NC-17 Movie", Classification: &library.MediaClassification{OfficialRating: "NC-17"}})

	items, err := querySvc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{
		AllowedRatings: ratings.AllowedRatingsForAge(10),
	})
	require.NoError(t, err)

	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, string(item.ID))
	}
	assert.Contains(t, ids, "g-movie")
	assert.Contains(t, ids, "pg-movie")
	assert.NotContains(t, ids, "r-movie")
	assert.NotContains(t, ids, "nc17-movie")
}

func TestListMediaItems_ParentalFilter_NoRating(t *testing.T) {
	t.Parallel()
	querySvc, mediaQueryRepo, _ := newQuerySvc()

	mediaQueryRepo.Seed(&library.MediaItem{ID: "g-movie", Title: "G Movie", Classification: &library.MediaClassification{OfficialRating: "G"}})
	mediaQueryRepo.Seed(&library.MediaItem{ID: "r-movie", Title: "R Movie", Classification: &library.MediaClassification{OfficialRating: "R"}})
	mediaQueryRepo.Seed(&library.MediaItem{ID: "unrated", Title: "Unrated Movie"})

	// no AllowedRatings = no parental filter
	items, err := querySvc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{})
	require.NoError(t, err)

	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, string(item.ID))
	}
	assert.Contains(t, ids, "g-movie")
	assert.Contains(t, ids, "r-movie")
	assert.Contains(t, ids, "unrated")
}

func TestListMediaItems_ParentalFilter_UnratedItemVisible(t *testing.T) {
	t.Parallel()
	querySvc, mediaQueryRepo, _ := newQuerySvc()

	mediaQueryRepo.Seed(&library.MediaItem{ID: "unrated", Title: "Unrated Movie"})
	mediaQueryRepo.Seed(&library.MediaItem{ID: "r-movie", Title: "R Movie", Classification: &library.MediaClassification{OfficialRating: "R"}})

	items, err := querySvc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{
		AllowedRatings: ratings.AllowedRatingsForAge(10),
	})
	require.NoError(t, err)

	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, string(item.ID))
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
		File:      &library.MediaFile{Path: "/movies/deleted.mkv", Size: 1000},
	})
	// Pre-existing item whose file still exists
	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID:        "item-keep",
		LibraryID: "lib-1",
		File:      &library.MediaFile{Path: "/movies/keep.mkv", Size: 2000},
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
		ids = append(ids, string(item.ID))
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
		File:      &library.MediaFile{Path: "/tv/show/s01e01.mkv", Size: 100},
	})
	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID:        "ep-gone",
		LibraryID: "lib-tv",
		File:      &library.MediaFile{Path: "/tv/show/s01e02.mkv", Size: 100},
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

func TestGenerateTrickplay_DisabledByConfig(t *testing.T) {
	t.Parallel()
	trickplayRepo := libfakes.NewFakeTrickplayRepository()
	trickplayGen := libfakes.NewFakeTrickplayGenerator()
	configRepo := sharedfakes.NewFakeServerConfigRepository()
	configRepo.Set(context.Background(), "trickplay.enabled", "false")
	svc, _, _, mediaRepo, _, _ := newCmdSvcWith(&cmdSvcDeps{trickplayRepo: trickplayRepo, trickplayGen: trickplayGen, cacheDir: "/cache", configRepo: configRepo})

	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "m1", File: &library.MediaFile{Path: "/movies/test.mkv"},
	})

	err := svc.GenerateTrickplay(context.Background(), "m1")
	require.NoError(t, err)

	manifest, err := trickplayRepo.GetByMediaItemID(context.Background(), "m1")
	require.Error(t, err, "no manifest created when disabled")
	assert.Nil(t, manifest)
}

func TestGenerateAllMissingTrickplays_DisabledByConfig(t *testing.T) {
	t.Parallel()
	trickplayRepo := libfakes.NewFakeTrickplayRepository()
	trickplayGen := libfakes.NewFakeTrickplayGenerator()
	configRepo := sharedfakes.NewFakeServerConfigRepository()
	configRepo.Set(context.Background(), "trickplay.enabled", "false")
	svc, _, _, _, _, _ := newCmdSvcWith(&cmdSvcDeps{trickplayRepo: trickplayRepo, trickplayGen: trickplayGen, cacheDir: "/cache", configRepo: configRepo})

	n, err := svc.GenerateAllMissingTrickplays(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestGenerateTrickplay_AdaptiveCPUDisabled(t *testing.T) {
	t.Parallel()
	trickplayRepo := libfakes.NewFakeTrickplayRepository()
	trickplayGen := libfakes.NewFakeTrickplayGenerator()
	configRepo := sharedfakes.NewFakeServerConfigRepository()
	configRepo.Set(context.Background(), "trickplay.adaptive_cpu", "false")
	svc, _, _, mediaRepo, _, _ := newCmdSvcWith(&cmdSvcDeps{trickplayRepo: trickplayRepo, trickplayGen: trickplayGen, cacheDir: t.TempDir(), configRepo: configRepo})

	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "m1", File: &library.MediaFile{Path: "/movies/test.mkv"},
	})

	err := svc.GenerateTrickplay(context.Background(), "m1")
	require.NoError(t, err)
}

func TestQueryService_GetMediaItemsByIDs(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	f.media.Seed(&library.MediaItem{ID: "m1", Title: "A"})
	f.media.Seed(&library.MediaItem{ID: "m2", Title: "B"})
	f.media.Seed(&library.MediaItem{ID: "m3", Title: "C"})

	items, err := f.svc.GetMediaItemsByIDs(context.Background(), []string{"m1", "m3"})
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestQueryService_GetMediaItemsByIDs_Empty(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	items, err := f.svc.GetMediaItemsByIDs(context.Background(), []string{})
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestStartPeriodicScan_CancelledImmediately(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, _, _, _ := newCmdSvc()
	libQueryRepo.Seed(&library.Library{ID: "lib-1", Name: "Movies", Type: "movie"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc.StartPeriodicScan(ctx, libQueryRepo, 50*time.Millisecond, nil)
	time.Sleep(100 * time.Millisecond)
}

func TestStartPeriodicScan_WithSpecificLibraryIDs(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, _, _, _ := newCmdSvc()
	libQueryRepo.Seed(&library.Library{ID: "lib-1", Name: "Movies", Type: "movie"})

	ctx, cancel := context.WithCancel(context.Background())
	svc.StartPeriodicScan(ctx, libQueryRepo, 50*time.Millisecond, []string{"lib-1"})
	time.Sleep(100 * time.Millisecond)
	cancel()
}

func TestStartPeriodicScan_DefaultInterval(t *testing.T) {
	// Deliberately NOT t.Parallel(): swaps the package-global slog default to
	// capture output, which would race any other test logging concurrently.
	svc, _, libQueryRepo, _, _, _ := newCmdSvc()

	// The resolved interval isn't exposed by any return value or repo side
	// effect - the only externally observable trace of interval<=0 -> 2min is
	// the "interval=" field StartPeriodicScan logs on startup. Capture it
	// instead of asserting on a smoke-test side effect that can't fail.
	// Locked writer, not a bare bytes.Buffer: the scanner logs from its own
	// goroutine and other tests in this package log concurrently, so an
	// unsynchronised buffer trips the race detector under `go test -race`.
	// Stray lines from other tests are harmless here - the assertion is a
	// substring match, not an equality.
	buf := &lockedBuffer{}
	prevLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, nil)))
	defer slog.SetDefault(prevLogger)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc.StartPeriodicScan(ctx, libQueryRepo, 0, nil)
	time.Sleep(50 * time.Millisecond)

	if got := buf.String(); !strings.Contains(got, "interval=2m0s") {
		t.Errorf("expected StartPeriodicScan(interval=0) to log the 2min default, got log output: %s", got)
	}
}

func TestStartBackgroundGeneration_CancelledCtx(t *testing.T) {
	t.Parallel()
	configRepo := sharedfakes.NewFakeServerConfigRepository()
	svc, _, _, _, _, _ := newCmdSvcWith(&cmdSvcDeps{configRepo: configRepo})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc.StartBackgroundGeneration(ctx)
	time.Sleep(50 * time.Millisecond)
}

func TestScanLibrary_PrunesEmptySeasonAndSeries(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	lib := &library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	}
	svc.CreateLibrary(context.Background(), lib)
	libQueryRepo.Seed(lib)

	// Pre-seed: series-1 -> season-1 -> ep-1 (will be orphaned) AND a kept
	// series-2 -> season-2 -> ep-2 whose file the scan still sees. The kept
	// episode keeps the scan non-empty so the degraded-mount safety guard does
	// not trip (an empty scan of a non-empty library refuses to prune).
	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "series-1", LibraryID: "lib-tv", Type: "series", Title: "Show",
	})
	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "season-1", LibraryID: "lib-tv", Type: "season", ParentID: "series-1", Title: "Season 1",
	})
	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "ep-1", LibraryID: "lib-tv", Type: "episode", ParentID: "season-1",
		File: &library.MediaFile{Path: "/tv/show/s01e01.mkv", Size: 100},
	})
	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "series-2", LibraryID: "lib-tv", Type: "series", Title: "Other Show",
	})
	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "season-2", LibraryID: "lib-tv", Type: "season", ParentID: "series-2", Title: "Season 1",
	})
	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "ep-2", LibraryID: "lib-tv", Type: "episode", ParentID: "season-2",
		File: &library.MediaFile{Path: "/tv/other/s01e01.mkv", Size: 100},
	})

	// scanner sees only ep-2 -> ep-1 is orphaned -> its empty season/series pruned.
	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/other/s01e01.mkv", Size: 100},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	items := mediaRepo.All()
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, string(item.ID))
	}
	assert.NotContains(t, ids, "ep-1", "orphaned episode removed")
	assert.NotContains(t, ids, "season-1", "empty season pruned")
	assert.NotContains(t, ids, "series-1", "empty series pruned")
	assert.Contains(t, ids, "ep-2", "still-present episode kept")
}

func TestScanLibrary_PrunesEmptySeason_KeepsSeriesWithOtherChildren(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	lib := &library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	}
	svc.CreateLibrary(context.Background(), lib)
	libQueryRepo.Seed(lib)

	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "series-1", LibraryID: "lib-tv", Type: "series", Title: "Show",
	})
	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "season-1", LibraryID: "lib-tv", Type: "season", ParentID: "series-1",
	})
	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "season-2", LibraryID: "lib-tv", Type: "season", ParentID: "series-1",
	})
	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "ep-1", LibraryID: "lib-tv", Type: "episode", ParentID: "season-1",
		File: &library.MediaFile{Path: "/tv/show/s01e01.mkv", Size: 100},
	})
	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "ep-2", LibraryID: "lib-tv", Type: "episode", ParentID: "season-2",
		File: &library.MediaFile{Path: "/tv/show/s02e01.mkv", Size: 100},
	})

	s2, e1 := 2, 1
	// only season 2 ep survives
	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/show/s02e01.mkv", Size: 100, Title: "Show", Season: &s2, Episode: &e1, Type: library.Episode},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	items := mediaRepo.All()
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, string(item.ID))
	}
	assert.NotContains(t, ids, "season-1", "empty season-1 pruned")
	assert.NotContains(t, ids, "ep-1", "orphaned episode removed")
	// series-1 still has season-2, so it should remain
	// (series gets re-created by scan anyway)
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
		ID: "gone-1", LibraryID: "lib-1", File: &library.MediaFile{Path: "/movies/gone1.mkv"},
	})
	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "gone-2", LibraryID: "lib-1", File: &library.MediaFile{Path: "/movies/gone2.mkv"},
	})
	// Kept item keeps the scan non-empty (avoids the degraded-mount guard);
	// gone-1 and gone-2 are the real orphans.
	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "keep-1", LibraryID: "lib-1", File: &library.MediaFile{Path: "/movies/keep.mkv", Size: 100, Fingerprint: "fp-keep"},
	})

	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/keep.mkv", Size: 100, Fingerprint: "fp-keep", Type: library.MovieItem},
	}

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

// A probe that finds streams but reports no duration used to leave
// ContainerFormat unwritten: the persist was gated on `!dur.IsZero() ||
// !probedOK`, and a successful probe never sets StreamProbeAttempts, so
// NeedsReprobe stayed false and the unchanged-file fast path skipped the item
// forever. The field feeds direct-play eligibility, so a permanently blank
// container means needless transcoding.
func TestScan_PersistsContainerFormatWhenProbeHasNoDuration(t *testing.T) {
	t.Parallel()
	prober := libfakes.NewFakeStreamProber()
	prober.Results["/movies/film.mkv"] = &library.ProbeResult{
		// Zero duration, but real streams and a known container.
		Container: "matroska,webm",
		Streams: []*library.MediaStream{
			{ID: "ps1", StreamType: "video", Codec: "h264"},
		},
	}
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvcWith(&cmdSvcDeps{prober: prober})

	libQueryRepo.Seed(&library.Library{
		ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"},
	})
	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 100, Title: "Film", Type: library.MovieItem},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	// Assert on the Update CALL, not on the stored struct: the fake keeps the
	// caller's pointer, so the in-memory mutation would look persisted either way.
	assert.NotEmpty(t, mediaRepo.UpdatedIDs,
		"a successful probe must persist the container even when the duration is unknown")
	items := mediaRepo.All()
	require.Len(t, items, 1)
	assert.Equal(t, "matroska,webm", items[0].File.ContainerFormat)
}

// lockedBuffer is a bytes.Buffer safe for concurrent writes, for tests that
// capture the package-global slog output while goroutines are still logging.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// countingDirScanner is a TypedDirectoryScanner that only counts calls - the
// observable for cadence tests, with no filesystem involved.
type countingDirScanner struct{ calls atomic.Int64 }

func (c *countingDirScanner) ScanDirectory(_ context.Context, _ string, _ string) (int, error) {
	c.calls.Add(1)
	return 0, nil
}

// Prod 2026-08-26: every library had RealtimeMonitoring=false, so the watcher
// vouched for NOTHING - and the periodic pass still demoted every library to
// the hourly backstop, because it read absence from the health map as "watch
// alive". 365 fresh music files sat invisible for hours. The 2026-07-21
// design says the opposite: only a watch PROVEN alive may space the walks out;
// a library nobody watches keeps the operator's periodic cadence.
func TestStartPeriodicScan_MonitoringOffLibraryKeepsOperatorCadence(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, _, _, _ := newCmdSvc()
	libQueryRepo.Seed(&library.Library{
		ID: "lib-m", Name: "Musique", Type: "music", Paths: []string{"/music"},
		Enabled: true, RealtimeMonitoring: false,
	})
	counter := &countingDirScanner{}
	svc.WithMusicScanner(counter)
	// The watcher service is running but watches nothing (monitoring is off
	// everywhere) - its health snapshot vouches for no library.
	svc.WithWatchHealth(func() map[string]bool { return map[string]bool{} })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.StartPeriodicScan(ctx, libQueryRepo, 30*time.Millisecond, nil)
	time.Sleep(300 * time.Millisecond)
	cancel()

	// Startup pass + several periodic ticks. Under the demotion bug only the
	// startup pass and ONE backstop-armed tick scan (2 calls), then every tick
	// skips for an hour.
	require.GreaterOrEqual(t, counter.calls.Load(), int64(3),
		"a library nobody watches must keep the operator cadence, not be demoted to the hourly backstop")
}
