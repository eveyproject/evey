package library_test

import (
	"context"
	"testing"

	applib "github.com/gravity-zero/evey/internal/app/library"
	"github.com/gravity-zero/evey/internal/domain/library"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
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
	search := libfakes.NewFakeSearchIndex()
	bus := sharedfakes.NewFakeEventBus()
	svc := applib.NewService(libRepo, mediaRepo, streamRepo, personRepo, mediaPersonRepo, scanner, search, bus)
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
