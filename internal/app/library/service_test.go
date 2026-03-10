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

func newLibrarySvcWithSubScanner() (*applib.Service, *libfakes.FakeLibraryRepository, *libfakes.FakeMediaItemRepository, *libfakes.FakeMediaStreamRepository, *libfakes.FakeFileScanner, *libfakes.FakeStreamProber, *libfakes.FakeSubtitleScanner, *sharedfakes.FakeEventBus) {
	libRepo := libfakes.NewFakeLibraryRepository()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	personRepo := libfakes.NewFakePersonRepository()
	mediaPersonRepo := libfakes.NewFakeMediaPersonRepository()
	scanner := libfakes.NewFakeFileScanner()
	prober := libfakes.NewFakeStreamProber()
	subScanner := libfakes.NewFakeSubtitleScanner()
	search := libfakes.NewFakeSearchIndex()
	bus := sharedfakes.NewFakeEventBus()
	svc := applib.NewService(libRepo, mediaRepo, streamRepo, personRepo, mediaPersonRepo, scanner, prober, search, bus)
	svc.WithSubtitleScanner(subScanner)
	return svc, libRepo, mediaRepo, streamRepo, scanner, prober, subScanner, bus
}

func TestScanLibrary_ExternalSubtitles(t *testing.T) {
	svc, libRepo, _, streamRepo, scanner, prober, subScanner, _ := newLibrarySvcWithSubScanner()

	libRepo.Create(context.Background(), &library.Library{
		ID:    "lib-1",
		Name:  "Movies",
		Type:  "movie",
		Paths: []string{"/movies"},
	})

	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 1000, Fingerprint: "1000-123"},
	}

	prober.Results["/movies/film.mkv"] = &library.ProbeResult{
		DurationMs: 120000,
		Streams: []*library.MediaStream{
			{ID: "v1", StreamIndex: 0, StreamType: "video", Codec: "h264"},
			{ID: "a1", StreamIndex: 1, StreamType: "audio", Codec: "aac"},
		},
	}

	subScanner.Results["/movies/film.mkv"] = []library.ExternalSubtitle{
		{Path: "/movies/film.en.srt", Language: "en", Format: "srt"},
		{Path: "/movies/film.fr.forced.ass", Language: "fr", IsForced: true, Format: "ass"},
	}

	added, _, err := svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 1, added)

	items, _ := svc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{})
	require.Len(t, items, 1)

	streams, err := streamRepo.GetByMediaItemID(context.Background(), items[0].ID)
	require.NoError(t, err)
	assert.Len(t, streams, 4) // 2 internal + 2 external

	var extSubs []*library.MediaStream
	for _, s := range streams {
		if s.IsExternal {
			extSubs = append(extSubs, s)
		}
	}
	assert.Len(t, extSubs, 2)
	assert.Equal(t, "en", extSubs[0].Language)
	assert.Equal(t, "srt", extSubs[0].Codec)
	assert.False(t, extSubs[0].IsForced)
	assert.Equal(t, "/movies/film.en.srt", *extSubs[0].ExternalPath)

	assert.Equal(t, "fr", extSubs[1].Language)
	assert.Equal(t, "ass", extSubs[1].Codec)
	assert.True(t, extSubs[1].IsForced)
}

func TestScanLibrary_NoSubtitleScanner(t *testing.T) {
	svc, libRepo, _, scanner, _ := newLibrarySvc()

	libRepo.Create(context.Background(), &library.Library{
		ID:    "lib-1",
		Name:  "Movies",
		Type:  "movie",
		Paths: []string{"/movies"},
	})

	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 1000, Fingerprint: "1000-456"},
	}

	added, _, err := svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 1, added)
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
