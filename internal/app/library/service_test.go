package library_test

import (
	"context"
	"testing"

	applib "github.com/gravity-zero/evey/internal/app/library"
	"github.com/gravity-zero/evey/internal/domain/library"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	"github.com/gravity-zero/evey/internal/domain/mkv"
	mkvfakes "github.com/gravity-zero/evey/internal/domain/mkv/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newLibrarySvc() (*applib.Service, *libfakes.FakeLibraryRepository, *libfakes.FakeMediaItemRepository, *libfakes.FakeFileScanner, *sharedfakes.FakeEventBus) {
	libRepo := libfakes.NewFakeLibraryRepository()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	personRepo := libfakes.NewFakePersonRepository()
	mediaPersonRepo := libfakes.NewFakeMediaPersonRepository()
	scanner := libfakes.NewFakeFileScanner()
	prober := libfakes.NewFakeStreamProber()
	search := libfakes.NewFakeSearchIndex()
	bus := sharedfakes.NewFakeEventBus()
	svc := applib.NewService(libRepo, mediaRepo, streamRepo, personRepo, mediaPersonRepo, scanner, prober, search, bus)
	return svc, libRepo, mediaRepo, scanner, bus
}

func TestCreateLibrary(t *testing.T) {
	svc, _, _, _, bus := newLibrarySvc()

	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie"}
	err := svc.CreateLibrary(context.Background(), lib)
	require.NoError(t, err)

	got, err := svc.GetLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, "Movies", got.Name)

	events := bus.PublishedEvents()
	require.Len(t, events, 1)
	assert.Equal(t, "library.created", events[0].EventName())
}

func TestScanLibrary_NewFiles(t *testing.T) {
	svc, libRepo, mediaRepo, scanner, bus := newLibrarySvc()

	libRepo.Create(context.Background(), &library.Library{
		ID:    "lib-1",
		Name:  "Movies",
		Type:  "movie",
		Paths: []string{"/movies"},
	})

	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film1.mkv", Size: 1000},
		{Path: "/movies/film2.mkv", Size: 2000},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	items, err := mediaRepo.List(context.Background(), library.ListMediaItemsFilter{})
	require.NoError(t, err)
	assert.Len(t, items, 2)

	// IDs must be UUIDs (not unixnano-style)
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
	svc, libRepo, mediaRepo, scanner, bus := newLibrarySvc()

	libRepo.Create(context.Background(), &library.Library{
		ID:    "lib-1",
		Name:  "Movies",
		Type:  "movie",
		Paths: []string{"/movies"},
	})
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

	// file size updated
	item, err := mediaRepo.GetByID(context.Background(), "item-existing")
	require.NoError(t, err)
	assert.Equal(t, int64(1500), item.FileSize)

	// no MediaItemAdded event
	for _, e := range bus.PublishedEvents() {
		assert.NotEqual(t, "media.item.added", e.EventName())
	}
}

func TestGetMediaItem_NotFound(t *testing.T) {
	svc, _, _, _, _ := newLibrarySvc()

	_, err := svc.GetMediaItem(context.Background(), "nonexistent")
	require.Error(t, err)
}

func TestDeleteLibrary(t *testing.T) {
	svc, libRepo, _, _, bus := newLibrarySvc()

	libRepo.Create(context.Background(), &library.Library{ID: "lib-1", Name: "Movies", Type: "movie"})

	err := svc.DeleteLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	_, err = svc.GetLibrary(context.Background(), "lib-1")
	require.Error(t, err)

	var deletedEvent bool
	for _, e := range bus.PublishedEvents() {
		if e.EventName() == "library.deleted" {
			deletedEvent = true
		}
	}
	assert.True(t, deletedEvent)
}

func TestScanLibrary_ExtractsChapters(t *testing.T) {
	svc, libRepo, _, scanner, _ := newLibrarySvc()

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

	libRepo.Create(context.Background(), &library.Library{
		ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"},
	})
	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 5000},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	// Find the created media item to get its ID
	items, _ := svc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{})
	require.Len(t, items, 1)

	chapters, err := svc.GetChapters(context.Background(), items[0].ID)
	require.NoError(t, err)
	require.Len(t, chapters, 2)
	assert.Equal(t, "Opening", chapters[0].Title)
	assert.Equal(t, int64(0), chapters[0].StartTimeMs)
	assert.Equal(t, int64(90000), chapters[0].EndTimeMs)
	assert.Equal(t, "Act 1", chapters[1].Title)
}

func TestGetChapters_NilRepo(t *testing.T) {
	svc, _, _, _, _ := newLibrarySvc()
	chapters, err := svc.GetChapters(context.Background(), "any-id")
	require.NoError(t, err)
	assert.Nil(t, chapters)
}
