package library_test

import (
	"context"
	"testing"
	"time"

	applib "github.com/gravity-zero/evey/internal/app/library"
	"github.com/gravity-zero/evey/internal/domain/library"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCmdSvcWithSearch() (*applib.CommandService, *libfakes.FakeLibraryCommandRepo, *libfakes.FakeLibraryQueryRepo, *libfakes.FakeMediaItemCommandRepo, *libfakes.FakeFileScanner, *sharedfakes.FakeEventBus, *libfakes.FakeSearchIndex, *libfakes.FakeMediaStreamRepository) {
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
	return svc, libCmdRepo, libQueryRepo, mediaRepo, scanner, bus, search, streamRepo
}

func TestDeleteMediaItem_RemovesItem(t *testing.T) {
	t.Parallel()
	svc, _, _, mediaRepo, _, _ := newCmdSvc()
	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "item-to-delete", LibraryID: "lib-1", FilePath: "/movies/film.mkv",
	})

	err := svc.DeleteMediaItem(context.Background(), "item-to-delete")
	require.NoError(t, err)

	_, err = mediaRepo.GetByID(context.Background(), "item-to-delete")
	require.ErrorIs(t, err, library.ErrNotFound)
}

func TestDeleteMediaItem_NotFound(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _, _ := newCmdSvc()
	err := svc.DeleteMediaItem(context.Background(), "nonexistent")
	require.Error(t, err)
}

func TestCommandService_GetMediaStreams_ReturnsStreams(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _, _, _, streamRepo := newCmdSvcWithSearch()
	streamRepo.CreateBatch(context.Background(), []*library.MediaStream{
		{ID: "s1", MediaItemID: "item-1", StreamType: "video", Codec: "h264"},
		{ID: "s2", MediaItemID: "item-1", StreamType: "audio", Codec: "aac"},
	})

	streams, err := svc.GetMediaStreams(context.Background(), "item-1")
	require.NoError(t, err)
	assert.Len(t, streams, 2)
}

func TestCommandService_GetMediaStreams_Empty(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _, _ := newCmdSvc()
	streams, err := svc.GetMediaStreams(context.Background(), "no-item")
	require.NoError(t, err)
	assert.Empty(t, streams)
}

func TestScanLibrary_LibraryNotFound(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _, _ := newCmdSvc()
	_, _, err := svc.ScanLibrary(context.Background(), "nonexistent-lib")
	require.Error(t, err)
}

func TestProbeAndStoreStreams_StoresStreams(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _, _, _, streamRepo := newCmdSvcWithSearch()

	// The default prober in newCmdSvcWithSearch has empty results, get it with data
	libCmdRepo := libfakes.NewFakeLibraryCommandRepo()
	libQueryRepo := libfakes.NewFakeLibraryQueryRepo()
	mediaRepo := libfakes.NewFakeMediaItemCommandRepo()
	streamRepo2 := libfakes.NewFakeMediaStreamRepository()
	personRepo := libfakes.NewFakePersonRepository()
	mediaPersonRepo := libfakes.NewFakeMediaPersonRepository()
	scanner := libfakes.NewFakeFileScanner()
	prober := libfakes.NewFakeStreamProber()
	search := libfakes.NewFakeSearchIndex()
	bus := sharedfakes.NewFakeEventBus()
	_ = svc
	_ = streamRepo

	prober.Results["/movies/film.mkv"] = &library.ProbeResult{
		DurationMs: 7500,
		Streams: []*library.MediaStream{
			{ID: "ps1", StreamType: "video", Codec: "h264"},
			{ID: "ps2", StreamType: "audio", Codec: "aac"},
		},
	}

	svc2 := applib.NewCommandService(libCmdRepo, libQueryRepo, mediaRepo, streamRepo2, personRepo, mediaPersonRepo, scanner, prober, search, bus)
	dur := svc2.ProbeAndStoreStreams(context.Background(), "item-1", "/movies/film.mkv")
	assert.Equal(t, int64(7500), dur)

	streams, err := streamRepo2.GetByMediaItemID(context.Background(), "item-1")
	require.NoError(t, err)
	assert.Len(t, streams, 2)
}

func TestProbeAndStoreStreams_NilProber(t *testing.T) {
	t.Parallel()
	libCmdRepo := libfakes.NewFakeLibraryCommandRepo()
	libQueryRepo := libfakes.NewFakeLibraryQueryRepo()
	mediaRepo := libfakes.NewFakeMediaItemCommandRepo()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	personRepo := libfakes.NewFakePersonRepository()
	mediaPersonRepo := libfakes.NewFakeMediaPersonRepository()
	scanner := libfakes.NewFakeFileScanner()
	search := libfakes.NewFakeSearchIndex()
	bus := sharedfakes.NewFakeEventBus()

	svc := applib.NewCommandService(libCmdRepo, libQueryRepo, mediaRepo, streamRepo, personRepo, mediaPersonRepo, scanner, nil, search, bus)
	dur := svc.ProbeAndStoreStreams(context.Background(), "item-1", "/movies/film.mkv")
	assert.Equal(t, int64(0), dur)
}

func TestCreateLibrary_SetsTimestamps(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _, _ := newCmdSvc()
	before := time.Now().Add(-time.Second)

	lib := &library.Library{ID: "lib-ts", Name: "Timestamped", Type: "movie"}
	err := svc.CreateLibrary(context.Background(), lib)
	require.NoError(t, err)

	assert.True(t, lib.CreatedAt.After(before), "CreatedAt should be set")
	assert.True(t, lib.UpdatedAt.After(before), "UpdatedAt should be set")
	assert.False(t, lib.CreatedAt.IsZero(), "CreatedAt must not be zero")
}

func TestScanLibrary_UpdatesLastScannedAt(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, _, scanner, _ := newCmdSvc()

	lib := &library.Library{
		ID:    "lib-ts",
		Name:  "Movies",
		Type:  "movie",
		Paths: []string{"/movies"},
	}
	svc.CreateLibrary(context.Background(), lib)
	libQueryRepo.Seed(lib)
	scanner.Results["/movies"] = []library.ScannedFile{}

	before := time.Now().Add(-time.Second)
	_, _, err := svc.ScanLibrary(context.Background(), "lib-ts")
	require.NoError(t, err)

	got, err := libQueryRepo.GetByID(context.Background(), "lib-ts")
	require.NoError(t, err)
	require.NotNil(t, got.LastScannedAt)
	assert.True(t, got.LastScannedAt.After(before))
}

func TestScanLibrary_PublishesScanStartedAndCompleted(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, _, scanner, bus := newCmdSvc()

	lib := &library.Library{
		ID:    "lib-events",
		Name:  "Movies",
		Type:  "movie",
		Paths: []string{"/movies"},
	}
	svc.CreateLibrary(context.Background(), lib)
	libQueryRepo.Seed(lib)
	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 1000},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-events")
	require.NoError(t, err)

	var startedCount, completedCount int
	for _, e := range bus.PublishedEvents() {
		switch e.EventName() {
		case "library.scan.started":
			startedCount++
		case "library.scan.completed":
			completedCount++
		}
	}
	assert.Equal(t, 1, startedCount, "exactly one scan.started event expected")
	assert.Equal(t, 1, completedCount, "exactly one scan.completed event expected")
}

func TestScanLibrary_NewFile_IndexesInSearch(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, _, scanner, _, search, _ := newCmdSvcWithSearch()

	lib := &library.Library{
		ID:    "lib-idx",
		Name:  "Movies",
		Type:  "movie",
		Paths: []string{"/movies"},
	}
	svc.CreateLibrary(context.Background(), lib)
	libQueryRepo.Seed(lib)
	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/batman.mkv", Size: 1000, Title: "Batman"},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-idx")
	require.NoError(t, err)

	hits, total, err := search.Search(context.Background(), "batman", nil, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "Batman", hits[0].Title)
}

func TestScanLibrary_OrphanRemovedFromSearchIndex(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _, search, _ := newCmdSvcWithSearch()

	lib := &library.Library{
		ID:    "lib-orphan",
		Name:  "Movies",
		Type:  "movie",
		Paths: []string{"/movies"},
	}
	svc.CreateLibrary(context.Background(), lib)
	libQueryRepo.Seed(lib)

	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID:        "orphan-1",
		LibraryID: "lib-orphan",
		FilePath:  "/movies/deleted.mkv",
		Title:     "Deleted Film",
	})
	search.IndexMediaItem(context.Background(), &library.MediaItem{ID: "orphan-1", Title: "Deleted Film"}, nil)

	// Scanner returns nothing — file is gone
	scanner.Results["/movies"] = []library.ScannedFile{}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-orphan")
	require.NoError(t, err)

	hits, _, err := search.Search(context.Background(), "deleted", nil, 10)
	require.NoError(t, err)
	assert.Empty(t, hits, "orphaned item should be removed from search index")
}

func TestScanLibrary_TvShow_CreatesSeriesAndSeason(t *testing.T) {
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

	s1, e1 := 1, 1
	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/myshow/s01e01.mkv", Size: 500, Title: "My Show", Season: &s1, Episode: &e1, Type: library.Episode},
	}

	added, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)
	// series + season + episode = 3 items created
	assert.GreaterOrEqual(t, added, 3)

	items := mediaRepo.All()
	types := make(map[string]bool)
	for _, item := range items {
		types[item.Type] = true
	}
	assert.True(t, types["series"], "series item should be created")
	assert.True(t, types["season"], "season item should be created")

	var seriesAdded int
	for _, e := range bus.PublishedEvents() {
		if e.EventName() == "media.item.added" {
			seriesAdded++
		}
	}
	assert.GreaterOrEqual(t, seriesAdded, 1)
}
