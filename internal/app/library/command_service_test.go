package library_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	applib "github.com/eveyproject/evey/internal/app/library"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/eveyproject/evey/internal/domain/mkv"
	mkvfakes "github.com/eveyproject/evey/internal/domain/mkv/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mustProbeDuration is a tiny helper that builds a shared.Duration for test
// fixtures. Inline NewDurationMs+nil-check would triple the line count of
// every prober.Results entry without adding signal.
func mustProbeDuration(ms int64) shared.Duration {
	d, err := shared.NewDurationMs(ms)
	if err != nil {
		panic(err)
	}
	return d
}

type cmdTestHarness struct {
	svc        *applib.CommandService
	libCmd     *libfakes.FakeLibraryCommandRepo
	libQuery   *libfakes.FakeLibraryQueryRepo
	mediaItems *libfakes.FakeMediaItemCommandRepo
	streams    *libfakes.FakeMediaStreamRepository
	scanner    *libfakes.FakeFileScanner
	prober     *libfakes.FakeStreamProber
	search     *libfakes.FakeSearchIndex
	bus        *sharedfakes.FakeEventBus
}

type harnessOpts struct {
	chapterRepo library.ChapterRepository
	ebmlReader  mkv.ContainerReader
	configRepo  shared.ServerConfigRepository
}

func newHarnessWithOpts(opts *harnessOpts) *cmdTestHarness {
	if opts == nil {
		opts = &harnessOpts{}
	}
	if opts.chapterRepo == nil {
		opts.chapterRepo = libfakes.NewFakeChapterRepository()
	}
	if opts.ebmlReader == nil {
		opts.ebmlReader = mkvfakes.NewFakeContainerReader()
	}
	h := &cmdTestHarness{
		libCmd:     libfakes.NewFakeLibraryCommandRepo(),
		libQuery:   libfakes.NewFakeLibraryQueryRepo(),
		mediaItems: libfakes.NewFakeMediaItemCommandRepo(),
		streams:    libfakes.NewFakeMediaStreamRepository(),
		scanner:    libfakes.NewFakeFileScanner(),
		prober:     libfakes.NewFakeStreamProber(),
		search:     libfakes.NewFakeSearchIndex(),
		bus:        sharedfakes.NewFakeEventBus(),
	}
	h.svc = applib.NewCommandService(
		h.libCmd, h.libQuery, h.mediaItems, h.streams,
		libfakes.NewFakePersonRepository(), libfakes.NewFakeMediaPersonRepository(),
		h.scanner, h.prober, h.search, h.bus,
		opts.chapterRepo, opts.ebmlReader,
		libfakes.NewFakeTrickplayRepository(), libfakes.NewFakeTrickplayGenerator(), "/cache", opts.configRepo,
	)
	return h
}

func newHarness() *cmdTestHarness {
	return newHarnessWithOpts(nil)
}

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
	svc := applib.NewCommandService(libCmdRepo, libQueryRepo, mediaRepo, streamRepo, personRepo, mediaPersonRepo, scanner, prober, search, bus,
		libfakes.NewFakeChapterRepository(), mkvfakes.NewFakeContainerReader(),
		libfakes.NewFakeTrickplayRepository(), libfakes.NewFakeTrickplayGenerator(), "/cache", nil,
	)
	return svc, libCmdRepo, libQueryRepo, mediaRepo, scanner, bus, search, streamRepo
}

// --- CreateLibrary ---

func TestCreateLibrary_RepoError_Propagates(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.libCmd.Err = errors.New("db down")

	err := h.svc.CreateLibrary(context.Background(), &library.Library{ID: "lib-1", Name: "Movies", Type: "movie"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create library")
	assert.Contains(t, err.Error(), "db down")
	assert.Empty(t, h.bus.PublishedEvents(), "no event on error")
}

func TestCreateLibrary_PublishError_NonFatal(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.bus.PublishErr = errors.New("bus broken")

	err := h.svc.CreateLibrary(context.Background(), &library.Library{ID: "lib-1", Name: "Movies", Type: "movie"})
	require.NoError(t, err, "publish error must not fail create")
}

func TestCreateLibrary_EventFields(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "My Movies", Type: "movie"}
	err := h.svc.CreateLibrary(context.Background(), lib)
	require.NoError(t, err)

	events := h.bus.PublishedEvents()
	require.Len(t, events, 1)
	evt := events[0].(library.LibraryCreated)
	assert.Equal(t, shared.LibraryID("lib-1"), evt.LibraryID)
	assert.Equal(t, "My Movies", evt.Name)
	assert.Equal(t, "movie", evt.Type)
	assert.False(t, evt.OccurredAt.IsZero())
}

// --- DeleteLibrary ---

func TestDeleteLibrary_RepoError_Propagates(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.libCmd.Err = errors.New("db down")

	err := h.svc.DeleteLibrary(context.Background(), "lib-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete library")
	assert.Empty(t, h.bus.PublishedEvents(), "no event on error")
}

func TestDeleteLibrary_PublishError_NonFatal(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.libCmd.Create(context.Background(), &library.Library{ID: "lib-1"})
	h.bus.PublishErr = errors.New("bus broken")

	err := h.svc.DeleteLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
}

func TestDeleteLibrary_EventFields(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.libCmd.Create(context.Background(), &library.Library{ID: "lib-del"})

	before := time.Now().Add(-time.Second)
	err := h.svc.DeleteLibrary(context.Background(), "lib-del")
	require.NoError(t, err)

	events := h.bus.PublishedEvents()
	require.Len(t, events, 1)
	evt := events[0].(library.LibraryDeleted)
	assert.Equal(t, shared.LibraryID("lib-del"), evt.LibraryID)
	assert.True(t, evt.OccurredAt.After(before))
}

// --- DeleteMediaItem ---

func TestDeleteMediaItem_RemovesItem(t *testing.T) {
	t.Parallel()
	svc, _, _, mediaRepo, _, _ := newCmdSvc()
	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "item-to-delete", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/film.mkv"},
	})

	err := svc.DeleteMediaItem(context.Background(), "item-to-delete")
	require.NoError(t, err)

	_, err = mediaRepo.GetByID(context.Background(), "item-to-delete")
	require.ErrorIs(t, err, shared.ErrNotFound)
}

func TestDeleteMediaItem_NotFound(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _, _ := newCmdSvc()
	err := svc.DeleteMediaItem(context.Background(), "nonexistent")
	require.Error(t, err)
}

// --- ScanLibrary ---

func TestScanLibrary_LibraryNotFound(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _, _ := newCmdSvc()
	_, _, err := svc.ScanLibrary(context.Background(), "nonexistent-lib")
	require.Error(t, err)
}

func TestScanLibrary_ConcurrentScanRejected(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	started := make(chan struct{})
	blocked := make(chan struct{})
	h.scanner.Results["/movies"] = []library.ScannedFile{}

	// Override scanner to block
	blockingScanner := &blockingFileScanner{ready: started, block: blocked}
	svc := applib.NewCommandService(
		h.libCmd, h.libQuery, h.mediaItems, h.streams,
		libfakes.NewFakePersonRepository(), libfakes.NewFakeMediaPersonRepository(),
		blockingScanner, h.prober, h.search, h.bus,
		libfakes.NewFakeChapterRepository(), mkvfakes.NewFakeContainerReader(),
		libfakes.NewFakeTrickplayRepository(), libfakes.NewFakeTrickplayGenerator(), "/cache", nil,
	)

	go func() {
		svc.ScanLibrary(context.Background(), "lib-1")
	}()

	<-started
	_, _, err := svc.ScanLibrary(context.Background(), "lib-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scan already in progress")
	close(blocked)
}

type blockingFileScanner struct {
	ready chan struct{}
	block chan struct{}
}

func (s *blockingFileScanner) Scan(_ context.Context, _ string) ([]library.ScannedFile, error) {
	s.ready <- struct{}{}
	<-s.block
	return nil, nil
}
func (s *blockingFileScanner) ParseTitle(_ string) string { return "" }
func (s *blockingFileScanner) FileExists(_ string) bool   { return false }

func TestScanLibrary_UnreadableRoot_DegradesToFailedPath(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)
	h.scanner.Err = errors.New("permission denied")

	// An unreadable root is reported, not turned into an opaque internal error:
	// the scan succeeds and names the root it could not read (like music/photo).
	report, err := h.svc.ScanLibraryReport(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"/movies"}, report.FailedPaths)
	assert.Equal(t, 0, report.Added)
}

func TestScanLibrary_ReturnsExactAddedUpdatedCounts(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.mediaItems.Create(context.Background(), &library.MediaItem{
		ID: "existing-1", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/old.mkv", Size: 100, StreamProbeVersion: library.StreamProbeVersion},
	})

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/new.mkv", Size: 500, Title: "New Film", Type: library.MovieItem},
		{Path: "/movies/old.mkv", Size: 100},
	}

	added, updated, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 1, added, "exactly 1 new file")
	assert.Equal(t, 0, updated, "unchanged file skipped by smart scan")
}

func TestScanLibrary_MultiplePaths(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies", "/extra"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/a.mkv", Size: 100, Title: "A", Type: library.MovieItem},
	}
	h.scanner.Results["/extra"] = []library.ScannedFile{
		{Path: "/extra/b.mkv", Size: 200, Title: "B", Type: library.MovieItem},
	}

	added, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 2, added)
}

func TestScanLibrary_ScanCompletedEvent_HasCorrectCounts(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.mediaItems.Create(context.Background(), &library.MediaItem{
		ID: "orphan-1", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/gone.mkv"},
	})

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/new.mkv", Size: 100, Title: "New", Type: library.MovieItem},
	}

	added, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 1, added)

	var completed *library.LibraryScanCompleted
	for _, e := range h.bus.PublishedEvents() {
		if sc, ok := e.(library.LibraryScanCompleted); ok {
			completed = &sc
		}
	}
	require.NotNil(t, completed)
	assert.Equal(t, 1, completed.ItemsAdded)
	assert.Equal(t, 1, completed.ItemsRemoved)
	assert.Equal(t, shared.LibraryID("lib-1"), completed.LibraryID)
}

// --- upsertMediaFile branches ---

func TestUpsertMediaFile_NewItem_ByFingerprint_FallbackToPath(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 1000, Fingerprint: "fp-123", Title: "Film", Type: library.MovieItem},
	}

	added, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 1, added)

	items := h.mediaItems.All()
	require.Len(t, items, 1)
	assert.Equal(t, "Film", items[0].Title)
	require.NotNil(t, items[0].File)
	assert.Equal(t, "fp-123", items[0].File.Fingerprint)
	assert.Equal(t, int64(1000), items[0].File.Size)
	assert.Equal(t, "/movies/film.mkv", items[0].File.Path)
	assert.Equal(t, shared.LibraryID("lib-1"), items[0].LibraryID)
}

func TestUpsertMediaFile_ExistingByFingerprint_Unchanged_NoPathChange(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.mediaItems.Create(context.Background(), &library.MediaItem{
		ID: "item-1", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/film.mkv", Size: 1000, Fingerprint: "fp-123", StreamProbeVersion: library.StreamProbeVersion},
	})

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 1000, Fingerprint: "fp-123"},
	}

	added, updated, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 0, added)
	assert.Equal(t, 0, updated, "unchanged file skipped by smart scan")
}

func TestUpsertMediaFile_ExistingByFingerprint_Unchanged_PathMoved(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.mediaItems.Create(context.Background(), &library.MediaItem{
		ID: "item-1", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/old-name.mkv", Size: 1000, Fingerprint: "fp-123"},
	})

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/new-name.mkv", Size: 1000, Fingerprint: "fp-123"},
	}

	added, updated, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 0, added)
	assert.Equal(t, 1, updated)

	item, _ := h.mediaItems.GetByID(context.Background(), "item-1")
	require.NotNil(t, item.File)
	assert.Equal(t, "/movies/new-name.mkv", item.File.Path, "path should be updated")
}

func TestUpsertMediaFile_ExistingByPath_NoFingerprint(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.mediaItems.Create(context.Background(), &library.MediaItem{
		ID: "item-1", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/film.mkv", Size: 1000, StreamProbeVersion: library.StreamProbeVersion},
	})

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 1000},
	}

	added, updated, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 0, added)
	assert.Equal(t, 0, updated, "unchanged file skipped by smart scan")
}

func TestUpsertMediaFile_ExistingChanged_UpdatesFields(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.mediaItems.Create(context.Background(), &library.MediaItem{
		ID: "item-1", LibraryID: "lib-1",
		File:     &library.MediaFile{Path: "/movies/film.mkv", Size: 500, Fingerprint: "old-fp"},
		Overview: "Has overview",
	})

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 1500, Fingerprint: "new-fp"},
	}

	added, updated, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 0, added)
	assert.Equal(t, 1, updated)

	item, _ := h.mediaItems.GetByID(context.Background(), "item-1")
	require.NotNil(t, item.File)
	assert.Equal(t, int64(1500), item.File.Size)
	assert.Equal(t, "new-fp", item.File.Fingerprint)
}

func TestUpsertMediaFile_Changed_NoOverview_ReEmitsEvent(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.mediaItems.Create(context.Background(), &library.MediaItem{
		ID: "item-1", LibraryID: "lib-1",
		File:     &library.MediaFile{Path: "/movies/film.mkv", Size: 500},
		Overview: "",
	})

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 1500},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	var addedEvents int
	for _, e := range h.bus.PublishedEvents() {
		if e.EventName() == "media.item.added" {
			addedEvents++
		}
	}
	assert.Equal(t, 1, addedEvents, "should re-emit media.item.added for item without overview")
}

func TestUpsertMediaFile_Changed_WithOverview_NoReEmit(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.mediaItems.Create(context.Background(), &library.MediaItem{
		ID: "item-1", LibraryID: "lib-1",
		File:     &library.MediaFile{Path: "/movies/film.mkv", Size: 500},
		Overview: "Already enriched",
	})

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 1500},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	for _, e := range h.bus.PublishedEvents() {
		if e.EventName() == "media.item.added" {
			t.Fatal("should NOT re-emit media.item.added when overview present")
		}
	}
}

// A file replaced on disk is a change a viewer sees (media.item.updated,
// reason file_replaced); a mere move of the same content is scan bookkeeping
// and must not fire it, or a rescan would flood the webhooks.
func TestUpsertMediaFile_ReplacedFile_AnnouncesUpdateButAMoveDoesNot(t *testing.T) {
	t.Parallel()
	updates := func(h *cmdTestHarness) []library.MediaItemUpdated {
		var out []library.MediaItemUpdated
		for _, e := range h.bus.PublishedEvents() {
			if u, ok := e.(library.MediaItemUpdated); ok {
				out = append(out, u)
			}
		}
		return out
	}
	seed := func(h *cmdTestHarness) {
		lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
		h.libQuery.Seed(lib)
		h.libCmd.Seed(lib)
		h.mediaItems.Create(context.Background(), &library.MediaItem{
			ID: "item-1", LibraryID: "lib-1", Overview: "Already enriched",
			File: &library.MediaFile{Path: "/movies/film.mkv", Size: 500, Fingerprint: "fp-old"},
		})
	}

	replaced := newHarness()
	seed(replaced)
	replaced.scanner.Results["/movies"] = []library.ScannedFile{{Path: "/movies/film.mkv", Size: 1500, Fingerprint: "fp-new"}}
	_, _, err := replaced.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	got := updates(replaced)
	require.Len(t, got, 1)
	assert.Equal(t, shared.MediaItemID("item-1"), got[0].MediaItemID)
	assert.Equal(t, shared.LibraryID("lib-1"), got[0].LibraryID)
	assert.Equal(t, library.UpdateReasonFileReplaced, got[0].Reason)

	moved := newHarness()
	seed(moved)
	moved.scanner.Results["/movies"] = []library.ScannedFile{{Path: "/movies/renamed.mkv", Size: 500, Fingerprint: "fp-old"}}
	_, _, err = moved.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Empty(t, updates(moved), "a moved file with the same content is not an update")
}

func TestUpsertMediaFile_NewItem_PublishesMediaItemAdded(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/some-movie.mkv", Size: 100, Title: "Some Movie", Type: library.MovieItem},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	var evt *library.MediaItemAdded
	for _, e := range h.bus.PublishedEvents() {
		if a, ok := e.(library.MediaItemAdded); ok {
			evt = &a
		}
	}
	require.NotNil(t, evt)
	assert.Equal(t, "Some Movie", evt.Title)
	assert.Equal(t, shared.LibraryID("lib-1"), evt.LibraryID)
	assert.Equal(t, library.MovieItem, evt.Type)
	assert.Empty(t, evt.SeriesTitle, "non-tvshow should have empty SeriesTitle")
}

func TestScanLibrary_SkipsItemFailingDomainInvariants(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)
	// A degenerate file whose parser yielded no title fails NewMediaItem's
	// non-empty-title invariant and must be skipped, not persisted as a broken
	// row - while the valid sibling is still created.
	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/good.mkv", Size: 100, Title: "Good", Type: library.MovieItem},
		{Path: "/movies/blank.mkv", Size: 100, Title: "", Type: library.MovieItem},
	}
	added, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 1, added, "the empty-title file must be skipped, the valid one kept")
}

func TestUpsertMediaFile_CreateError_LoggedNotReturned(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)
	h.mediaItems.ErrCreate = errors.New("disk full")

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 100, Title: "Film", Type: library.MovieItem},
	}

	added, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err, "worker pool logs upsert errors")
	assert.Equal(t, 0, added)
}

func TestUpsertMediaFile_LookupError_LoggedNotReturned(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)
	h.mediaItems.GetByFileFingerprintErr = errors.New("db timeout")

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 100, Fingerprint: "fp-1"},
	}

	added, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err, "worker pool logs upsert errors")
	assert.Equal(t, 0, added)
}

func TestUpsertMediaFile_FingerprintNotFound_FallsBackToPath(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.mediaItems.Create(context.Background(), &library.MediaItem{
		ID: "item-1", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/film.mkv", Size: 1000, Fingerprint: "old-fp"},
	})

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 1000, Fingerprint: "new-fp"},
	}

	added, updated, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 0, added)
	assert.Equal(t, 1, updated)
}

func TestUpsertMediaFile_NewItem_WithDuration(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.prober.Results["/movies/film.mkv"] = &library.ProbeResult{
		Duration: mustProbeDuration(120000),
		Streams:  []*library.MediaStream{{ID: "s1", StreamType: "video"}},
	}

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 100, Title: "Film", Type: library.MovieItem},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	items := h.mediaItems.All()
	require.Len(t, items, 1)
	require.NotNil(t, items[0].File)
	assert.Equal(t, int64(120000), items[0].File.Duration.Milliseconds())
}

func TestUpsertMediaFile_NewItem_WithYear(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	yr := 2024
	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 100, Title: "Film", Year: &yr, Type: library.MovieItem},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	items := h.mediaItems.All()
	require.Len(t, items, 1)
	require.NotNil(t, items[0].Catalog)
	require.NotNil(t, items[0].Catalog.Year)
	assert.Equal(t, 2024, *items[0].Catalog.Year)
}

func TestUpsertMediaFile_UnchangedSizeEmptyFingerprint_CountsAsUnchanged(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.mediaItems.Create(context.Background(), &library.MediaItem{
		ID: "item-1", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/film.mkv", Size: 1000, Fingerprint: "existing-fp", StreamProbeVersion: library.StreamProbeVersion},
	})

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 1000, Fingerprint: ""},
	}

	// Size+mtime match + new fingerprint empty (transient read failure) =
	// fully skipped by detectChanges - no reprocess, counter stays 0.
	added, updated, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 0, added)
	assert.Equal(t, 0, updated)
}

func TestUpsertMediaFile_Changed_DifferentFingerprint(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.mediaItems.Create(context.Background(), &library.MediaItem{
		ID: "item-1", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/film.mkv", Size: 1000, Fingerprint: "fp-old"},
	})

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 1000, Fingerprint: "fp-new"},
	}

	added, updated, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 0, added)
	assert.Equal(t, 1, updated)

	item, _ := h.mediaItems.GetByID(context.Background(), "item-1")
	require.NotNil(t, item.File)
	assert.Equal(t, "fp-new", item.File.Fingerprint)
}

// --- extractAndStoreChapters ---

func TestExtractChapters_NonMKV_Skipped(t *testing.T) {
	t.Parallel()
	chapterRepo := libfakes.NewFakeChapterRepository()
	reader := mkvfakes.NewFakeContainerReader()
	h := newHarnessWithOpts(&harnessOpts{chapterRepo: chapterRepo, ebmlReader: reader})

	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)
	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mp4", Size: 100},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Empty(t, chapterRepo.All(), "no chapters for non-mkv")
}

func TestExtractChapters_MKV_NoChaptersInFile(t *testing.T) {
	t.Parallel()
	chapterRepo := libfakes.NewFakeChapterRepository()
	reader := mkvfakes.NewFakeContainerReader()
	reader.Containers["/movies/film.mkv"] = &mkv.Container{
		Path:     "/movies/film.mkv",
		Chapters: []mkv.Chapter{},
	}
	h := newHarnessWithOpts(&harnessOpts{chapterRepo: chapterRepo, ebmlReader: reader})

	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)
	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 100, Title: "Film", Type: library.MovieItem},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Empty(t, chapterRepo.All())
}

func TestExtractChapters_MKV_ChaptersStored(t *testing.T) {
	t.Parallel()
	chapterRepo := libfakes.NewFakeChapterRepository()
	reader := mkvfakes.NewFakeContainerReader()
	reader.Containers["/movies/film.mkv"] = &mkv.Container{
		Path: "/movies/film.mkv",
		Chapters: []mkv.Chapter{
			{ID: 1, Title: "Intro", StartMs: 0, EndMs: 5000},
			{ID: 2, Title: "Main", StartMs: 5000, EndMs: 100000},
		},
	}
	h := newHarnessWithOpts(&harnessOpts{chapterRepo: chapterRepo, ebmlReader: reader})

	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)
	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 100, Title: "Film", Type: library.MovieItem},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	chs := chapterRepo.All()
	require.Len(t, chs, 2)
	assert.Equal(t, "Intro", chs[0].Title)
	assert.Equal(t, 0, chs[0].SortOrder)
	assert.Equal(t, "Main", chs[1].Title)
	assert.Equal(t, 1, chs[1].SortOrder)
}

func TestExtractChapters_ReadError_NonFatal(t *testing.T) {
	t.Parallel()
	chapterRepo := libfakes.NewFakeChapterRepository()
	reader := mkvfakes.NewFakeContainerReader()
	reader.Err = errors.New("corrupt mkv")
	h := newHarnessWithOpts(&harnessOpts{chapterRepo: chapterRepo, ebmlReader: reader})

	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)
	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 100, Title: "Film", Type: library.MovieItem},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err, "chapter read error should not fail scan")
}

// --- discoverExternalSubtitles ---

func TestDiscoverExternalSubtitles_VttCodecRenamed(t *testing.T) {
	t.Parallel()
	h := newHarness()
	subScanner := libfakes.NewFakeSubtitleScanner()
	h.svc.WithSubtitleScanner(subScanner)

	subScanner.Results["/movies/film.mkv"] = []library.ExternalSubtitle{
		{Path: "/movies/film.en.vtt", Language: "en", Format: "vtt"},
	}

	h.prober.Results["/movies/film.mkv"] = &library.ProbeResult{
		Duration: mustProbeDuration(5000),
		Streams:  []*library.MediaStream{{ID: "v1", StreamType: "video", StreamIndex: 0}},
	}

	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)
	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 100, Title: "Film", Type: library.MovieItem},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	streams, _ := h.streams.GetByMediaItemID(context.Background(), shared.MediaItemID(h.mediaItems.All()[0].ID))
	var subStream *library.MediaStream
	for _, s := range streams {
		if s.StreamType == "subtitle" {
			subStream = s
		}
	}
	require.NotNil(t, subStream)
	assert.Equal(t, "webvtt", subStream.Codec, "vtt should be renamed to webvtt")
	assert.True(t, subStream.IsExternal)
	assert.Equal(t, "en", subStream.Language)
	assert.NotNil(t, subStream.ExternalPath)
	assert.Equal(t, "/movies/film.en.vtt", *subStream.ExternalPath)
}

func TestDiscoverExternalSubtitles_NonVttCodecKept(t *testing.T) {
	t.Parallel()
	h := newHarness()
	subScanner := libfakes.NewFakeSubtitleScanner()
	h.svc.WithSubtitleScanner(subScanner)

	subScanner.Results["/movies/film.mkv"] = []library.ExternalSubtitle{
		{Path: "/movies/film.en.srt", Language: "en", Format: "srt"},
	}

	h.prober.Results["/movies/film.mkv"] = &library.ProbeResult{
		Duration: mustProbeDuration(5000),
		Streams:  []*library.MediaStream{},
	}

	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)
	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 100, Title: "Film", Type: library.MovieItem},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	streams, _ := h.streams.GetByMediaItemID(context.Background(), shared.MediaItemID(h.mediaItems.All()[0].ID))
	var subStream *library.MediaStream
	for _, s := range streams {
		if s.StreamType == "subtitle" {
			subStream = s
		}
	}
	require.NotNil(t, subStream)
	assert.Equal(t, "srt", subStream.Codec)
}

func TestDiscoverExternalSubtitles_MaxIdxContinuesFromExisting(t *testing.T) {
	t.Parallel()
	h := newHarness()
	subScanner := libfakes.NewFakeSubtitleScanner()
	h.svc.WithSubtitleScanner(subScanner)

	subScanner.Results["/movies/film.mkv"] = []library.ExternalSubtitle{
		{Path: "/movies/film.en.srt", Language: "en", Format: "srt"},
	}

	h.prober.Results["/movies/film.mkv"] = &library.ProbeResult{
		Duration: mustProbeDuration(5000),
		Streams: []*library.MediaStream{
			{ID: "v1", StreamType: "video", StreamIndex: 0},
			{ID: "a1", StreamType: "audio", StreamIndex: 1},
			{ID: "s1", StreamType: "subtitle", StreamIndex: 2},
		},
	}

	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)
	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 100, Title: "Film", Type: library.MovieItem},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	streams, _ := h.streams.GetByMediaItemID(context.Background(), shared.MediaItemID(h.mediaItems.All()[0].ID))
	var extSub *library.MediaStream
	for _, s := range streams {
		if s.IsExternal {
			extSub = s
		}
	}
	require.NotNil(t, extSub)
	assert.Equal(t, 3, extSub.StreamIndex, "external sub should continue after existing stream indexes")
}

func TestDiscoverExternalSubtitles_ForcedFlag(t *testing.T) {
	t.Parallel()
	h := newHarness()
	subScanner := libfakes.NewFakeSubtitleScanner()
	h.svc.WithSubtitleScanner(subScanner)

	subScanner.Results["/movies/film.mkv"] = []library.ExternalSubtitle{
		{Path: "/movies/film.en.forced.srt", Language: "en", Format: "srt", IsForced: true},
	}

	h.prober.Results["/movies/film.mkv"] = &library.ProbeResult{
		Duration: mustProbeDuration(5000),
		Streams:  []*library.MediaStream{},
	}

	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)
	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 100, Title: "Film", Type: library.MovieItem},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	streams, _ := h.streams.GetByMediaItemID(context.Background(), shared.MediaItemID(h.mediaItems.All()[0].ID))
	require.Len(t, streams, 1)
	assert.True(t, streams[0].IsForced)
}

func TestDiscoverExternalSubtitles_NilScanner_NoSubs(t *testing.T) {
	t.Parallel()
	h := newHarness()

	h.prober.Results["/movies/film.mkv"] = &library.ProbeResult{
		Duration: mustProbeDuration(5000),
		Streams:  []*library.MediaStream{{ID: "v1", StreamType: "video"}},
	}

	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)
	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 100, Title: "Film", Type: library.MovieItem},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	streams, _ := h.streams.GetByMediaItemID(context.Background(), shared.MediaItemID(h.mediaItems.All()[0].ID))
	for _, s := range streams {
		assert.False(t, s.IsExternal)
	}
}

// --- ProbeAndStoreStreams ---

func TestProbeAndStoreStreams_StoresStreams(t *testing.T) {
	t.Parallel()
	h := newHarness()

	h.prober.Results["/movies/film.mkv"] = &library.ProbeResult{
		Duration: mustProbeDuration(7500),
		Streams: []*library.MediaStream{
			{ID: "ps1", StreamType: "video", Codec: "h264"},
			{ID: "ps2", StreamType: "audio", Codec: "aac"},
		},
	}

	dur := h.svc.ProbeAndStoreStreams(context.Background(), "item-1", "/movies/film.mkv")
	assert.Equal(t, int64(7500), dur.Milliseconds())

	streams, err := h.streams.GetByMediaItemID(context.Background(), "item-1")
	require.NoError(t, err)
	assert.Len(t, streams, 2)
	for _, s := range streams {
		assert.Equal(t, shared.MediaItemID("item-1"), s.MediaItemID)
	}
}

func TestProbeAndStoreStreams_NilProber(t *testing.T) {
	t.Parallel()
	svc := applib.NewCommandService(
		libfakes.NewFakeLibraryCommandRepo(), libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemCommandRepo(), libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakePersonRepository(), libfakes.NewFakeMediaPersonRepository(),
		libfakes.NewFakeFileScanner(), nil, libfakes.NewFakeSearchIndex(),
		sharedfakes.NewFakeEventBus(),
		libfakes.NewFakeChapterRepository(), mkvfakes.NewFakeContainerReader(),
		libfakes.NewFakeTrickplayRepository(), libfakes.NewFakeTrickplayGenerator(), "/cache", nil,
	)
	dur := svc.ProbeAndStoreStreams(context.Background(), "item-1", "/movies/film.mkv")
	assert.True(t, dur.IsZero())
}

func TestProbeAndStoreStreams_ProbeError_Returns0(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.prober.Err = errors.New("ffprobe missing")

	dur := h.svc.ProbeAndStoreStreams(context.Background(), "item-1", "/movies/film.mkv")
	assert.True(t, dur.IsZero())
}

func TestProbeAndStoreStreams_DeletesOldStreams(t *testing.T) {
	t.Parallel()
	h := newHarness()

	h.streams.CreateBatch(context.Background(), []*library.MediaStream{
		{ID: "old-1", MediaItemID: "item-1", StreamType: "video"},
	})

	h.prober.Results["/movies/film.mkv"] = &library.ProbeResult{
		Duration: mustProbeDuration(1000),
		Streams:  []*library.MediaStream{{ID: "new-1", StreamType: "audio"}},
	}

	h.svc.ProbeAndStoreStreams(context.Background(), "item-1", "/movies/film.mkv")

	streams, _ := h.streams.GetByMediaItemID(context.Background(), "item-1")
	require.Len(t, streams, 1)
	assert.Equal(t, "new-1", streams[0].ID)
}

// --- scanTVShowFiles ---

func TestScanTVShow_CreatesSeriesSeasonEpisode(t *testing.T) {
	t.Parallel()
	h := newHarness()
	s1, e1 := 1, 1
	lib := &library.Library{ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/show/s01e01.mkv", Size: 500, Title: "My Show", Season: &s1, Episode: &e1, Type: library.Episode},
	}

	added, _, err := h.svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)
	assert.Equal(t, 3, added, "series + season + episode = 3")

	items := h.mediaItems.All()
	typeMap := map[string]int{}
	for _, item := range items {
		typeMap[string(item.Type)]++
	}
	assert.Equal(t, 1, typeMap["series"])
	assert.Equal(t, 1, typeMap["season"])
}

func TestScanTVShow_EpisodeTitleFormat(t *testing.T) {
	t.Parallel()
	h := newHarness()
	s2, e3 := 2, 3
	lib := &library.Library{ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/show/s02e03.mkv", Size: 500, Title: "My Show", Season: &s2, Episode: &e3, Type: library.Episode},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	items := h.mediaItems.All()
	var episodeTitle string
	for _, item := range items {
		if item.File != nil && item.File.Path == "/tv/show/s02e03.mkv" {
			episodeTitle = item.Title
		}
	}
	assert.Equal(t, "S02E03", episodeTitle)
}

func TestScanTVShow_NilSeason_DefaultsTo0(t *testing.T) {
	t.Parallel()
	h := newHarness()
	e1 := 1
	lib := &library.Library{ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/show/special.mkv", Size: 500, Title: "My Show", Season: nil, Episode: &e1, Type: library.Episode},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	items := h.mediaItems.All()
	var seasonTitle string
	for _, item := range items {
		if item.Type == "season" {
			seasonTitle = item.Title
		}
	}
	assert.Equal(t, "Season 0", seasonTitle)
}

func TestScanTVShow_NoEpisodeNumber_KeepsOriginalTitle(t *testing.T) {
	t.Parallel()
	h := newHarness()
	s1 := 1
	lib := &library.Library{ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/show/bonus.mkv", Size: 500, Title: "My Show", Season: &s1, Episode: nil, Type: library.Episode},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	items := h.mediaItems.All()
	var episodeTitle string
	for _, item := range items {
		if item.File != nil && item.File.Path == "/tv/show/bonus.mkv" {
			episodeTitle = item.Title
		}
	}
	assert.Equal(t, "My Show", episodeTitle, "no episode num = keep original title")
}

func TestScanTVShow_SameShowTwoEpisodes_OneSeriesOneSeason(t *testing.T) {
	t.Parallel()
	h := newHarness()
	s1, e1, e2 := 1, 1, 2
	lib := &library.Library{ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/show/s01e01.mkv", Size: 500, Title: "My Show", Season: &s1, Episode: &e1, Type: library.Episode},
		{Path: "/tv/show/s01e02.mkv", Size: 600, Title: "My Show", Season: &s1, Episode: &e2, Type: library.Episode},
	}

	added, _, err := h.svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)
	assert.Equal(t, 4, added, "1 series + 1 season + 2 episodes = 4")
}

func TestScanTVShow_TwoSeasons(t *testing.T) {
	t.Parallel()
	h := newHarness()
	s1, s2, e1 := 1, 2, 1
	lib := &library.Library{ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/show/s01e01.mkv", Size: 500, Title: "My Show", Season: &s1, Episode: &e1, Type: library.Episode},
		{Path: "/tv/show/s02e01.mkv", Size: 600, Title: "My Show", Season: &s2, Episode: &e1, Type: library.Episode},
	}

	added, _, err := h.svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)
	assert.Equal(t, 5, added, "1 series + 2 seasons + 2 episodes = 5")
}

func TestScanTVShow_SeriesEventHasSeriesTitle(t *testing.T) {
	t.Parallel()
	h := newHarness()
	s1, e1 := 1, 1
	lib := &library.Library{ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/show/s01e01.mkv", Size: 500, Title: "Some Series", Season: &s1, Episode: &e1, Type: library.Episode},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	var episodeEvt *library.MediaItemAdded
	for _, e := range h.bus.PublishedEvents() {
		if a, ok := e.(library.MediaItemAdded); ok && a.Type != "series" {
			if a.SeriesTitle != "" {
				episodeEvt = &a
			}
		}
	}
	require.NotNil(t, episodeEvt)
	assert.Equal(t, "Some Series", episodeEvt.SeriesTitle)
}

func TestScanTVShow_GetOrCreateItem_FindsByFingerprint(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.mediaItems.Create(context.Background(), &library.MediaItem{
		ID: "existing-series", LibraryID: "lib-tv", Type: "series",
		Title: "My Show",
		File:  &library.MediaFile{Fingerprint: "series:lib-tv:My Show"},
	})

	s1, e1 := 1, 1
	h.scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/show/s01e01.mkv", Size: 500, Title: "My Show", Season: &s1, Episode: &e1, Type: library.Episode},
	}

	added, _, err := h.svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)
	// series already exists so only season + episode added
	assert.Equal(t, 2, added)
}

// --- Trickplay ---

func TestGenerateTrickplay_GenError_Propagates(t *testing.T) {
	t.Parallel()
	trickplayGen := libfakes.NewFakeTrickplayGenerator()
	trickplayGen.Err = errors.New("ffmpeg crash")
	h := newHarnessWithOpts(&harnessOpts{})
	// override trickplay via fresh constructor
	h.svc = applib.NewCommandService(
		h.libCmd, h.libQuery, h.mediaItems, h.streams,
		libfakes.NewFakePersonRepository(), libfakes.NewFakeMediaPersonRepository(),
		h.scanner, h.prober, h.search, h.bus,
		libfakes.NewFakeChapterRepository(), mkvfakes.NewFakeContainerReader(),
		libfakes.NewFakeTrickplayRepository(), trickplayGen, "/cache", nil,
	)

	h.mediaItems.Create(context.Background(), &library.MediaItem{
		ID: "m1", File: &library.MediaFile{Path: "/movies/film.mkv"},
	})

	err := h.svc.GenerateTrickplay(context.Background(), "m1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "generate trickplay")
	assert.Contains(t, err.Error(), "ffmpeg crash")
}

func TestGenerateTrickplay_StoreError_Propagates(t *testing.T) {
	t.Parallel()
	trickplayRepo := libfakes.NewFakeTrickplayRepository()
	trickplayRepo.Err = errors.New("db write failed")
	h := newHarnessWithOpts(&harnessOpts{})
	h.svc = applib.NewCommandService(
		h.libCmd, h.libQuery, h.mediaItems, h.streams,
		libfakes.NewFakePersonRepository(), libfakes.NewFakeMediaPersonRepository(),
		h.scanner, h.prober, h.search, h.bus,
		libfakes.NewFakeChapterRepository(), mkvfakes.NewFakeContainerReader(),
		trickplayRepo, libfakes.NewFakeTrickplayGenerator(), t.TempDir(), nil,
	)

	h.mediaItems.Create(context.Background(), &library.MediaItem{
		ID: "m1", File: &library.MediaFile{Path: "/movies/film.mkv"},
	})

	err := h.svc.GenerateTrickplay(context.Background(), "m1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store trickplay manifest")
}

func TestGenerateAllMissingTrickplays_ListError(t *testing.T) {
	t.Parallel()
	trickplayRepo := libfakes.NewFakeTrickplayRepository()
	trickplayRepo.Err = errors.New("db down")
	h := newHarnessWithOpts(&harnessOpts{})
	h.svc = applib.NewCommandService(
		h.libCmd, h.libQuery, h.mediaItems, h.streams,
		libfakes.NewFakePersonRepository(), libfakes.NewFakeMediaPersonRepository(),
		h.scanner, h.prober, h.search, h.bus,
		libfakes.NewFakeChapterRepository(), mkvfakes.NewFakeContainerReader(),
		trickplayRepo, libfakes.NewFakeTrickplayGenerator(), "/cache", nil,
	)

	count, err := h.svc.GenerateAllMissingTrickplays(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list missing trickplays")
	assert.Equal(t, 0, count)
}

// --- removeOrphanedItems ---

func TestRemoveOrphans_PublishesMediaItemRemoved(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.mediaItems.Create(context.Background(), &library.MediaItem{
		ID: "orphan-1", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/gone.mkv"},
	})
	// Kept item keeps the scan non-empty (else the degraded-mount guard refuses
	// to prune); only orphan-1 is the actual orphan.
	h.mediaItems.Create(context.Background(), &library.MediaItem{
		ID: "keep-1", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/keep.mkv", Size: 100, Fingerprint: "fp-keep"},
	})

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/keep.mkv", Size: 100, Fingerprint: "fp-keep", Type: library.MovieItem},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	var removedEvt *library.MediaItemRemoved
	for _, e := range h.bus.PublishedEvents() {
		if r, ok := e.(library.MediaItemRemoved); ok {
			removedEvt = &r
		}
	}
	require.NotNil(t, removedEvt)
	assert.Equal(t, shared.MediaItemID("orphan-1"), removedEvt.MediaItemID)
	assert.Equal(t, shared.LibraryID("lib-1"), removedEvt.LibraryID)
}

func TestRemoveOrphans_AlsoRemovesFromSearchIndex(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.mediaItems.Create(context.Background(), &library.MediaItem{
		ID: "orphan-1", LibraryID: "lib-1",
		File:  &library.MediaFile{Path: "/movies/gone.mkv"},
		Title: "Gone Film",
	})
	h.mediaItems.Create(context.Background(), &library.MediaItem{
		ID: "keep-1", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/keep.mkv", Size: 100, Fingerprint: "fp-keep"},
	})
	h.search.IndexMediaItem(context.Background(), &library.MediaItem{ID: "orphan-1", Title: "Gone Film"})

	// Kept file keeps the scan non-empty (avoids the degraded-mount guard).
	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/keep.mkv", Size: 100, Fingerprint: "fp-keep", Type: library.MovieItem},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	hits, _, _ := h.search.Search(context.Background(), "gone", nil, nil, 10)
	assert.Empty(t, hits)
}

// --- WithShutdownCtx ---

func TestWithShutdownCtx_SetsContext(t *testing.T) {
	t.Parallel()
	h := newHarness()
	applib.WithShutdownCtx(t.Context())(h.svc)
	// No panic = success; the shutdownCtx is used internally by GenerateAllMissingTrickplays
}

// --- WithSubtitleScanner ---

func TestWithSubtitleScanner_SetsScanner(t *testing.T) {
	t.Parallel()
	h := newHarness()
	subScanner := libfakes.NewFakeSubtitleScanner()
	h.svc.WithSubtitleScanner(subScanner)
	// No panic = success
}

// --- ScanLibrary edge: tvshow upsertMediaFile error ---

func TestScanTVShow_UpsertError_Propagates(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	s1, e1 := 1, 1
	h.scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/show/s01e01.mkv", Size: 500, Title: "My Show", Season: &s1, Episode: &e1, Type: library.Episode},
	}
	h.mediaItems.ErrCreate = errors.New("disk full")

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-tv")
	require.Error(t, err)
}

// --- ScanLibrary: movie upsertMediaFile error ---

func TestScanLibrary_Movie_UpsertError_LoggedNotReturned(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 100, Title: "Film", Type: library.MovieItem},
	}
	h.mediaItems.ErrCreate = errors.New("disk full")

	added, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err, "worker pool logs upsert errors")
	assert.Equal(t, 0, added)
}

func TestScanLibrary_NewFile_IndexedInSearch(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/some-movie.mkv", Size: 1000, Title: "Some Movie", Type: library.MovieItem},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	hits, total, err := h.search.Search(context.Background(), "movie", nil, nil, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "Some Movie", hits[0].Title)
}

func TestScanLibrary_UpdatesLastScannedAt(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)
	h.scanner.Results["/movies"] = []library.ScannedFile{}

	before := time.Now().Add(-time.Second)
	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	got, _ := h.libQuery.GetByID(context.Background(), "lib-1")
	require.NotNil(t, got.LastScannedAt)
	assert.True(t, got.LastScannedAt.After(before))
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
		{Path: "/movies/some-movie.mkv", Size: 1000, Title: "Some Movie", Type: library.MovieItem},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-idx")
	require.NoError(t, err)

	hits, total, err := search.Search(context.Background(), "movie", nil, nil, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "Some Movie", hits[0].Title)
}

func TestScanLibrary_OrphanEpisodeNotPersisted(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, _, scanner, _, search, _ := newCmdSvcWithSearch()

	lib := &library.Library{
		ID:    "lib-orph",
		Name:  "Movies",
		Type:  "movie",
		Paths: []string{"/movies"},
	}
	svc.CreateLibrary(context.Background(), lib)
	libQueryRepo.Seed(lib)
	// An episode-typed file scanned flat (no resolvable parent) must NOT be
	// persisted as an orphan row - the hierarchy invariant rejects it.
	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/loose.mkv", Size: 1000, Title: "Loose", Type: library.Episode},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-orph")
	require.NoError(t, err)

	_, total, err := search.Search(context.Background(), "loose", nil, nil, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(0), total, "orphan episode must not be created or indexed")
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
		File:      &library.MediaFile{Path: "/movies/deleted.mkv"},
		Title:     "Deleted Film",
	})
	mediaRepo.Create(context.Background(), &library.MediaItem{
		ID:        "keep-1",
		LibraryID: "lib-orphan",
		File:      &library.MediaFile{Path: "/movies/keep.mkv", Size: 100, Fingerprint: "fp-keep"},
	})
	search.IndexMediaItem(context.Background(), &library.MediaItem{ID: "orphan-1", Title: "Deleted Film"})

	// Kept file keeps the scan non-empty (avoids the degraded-mount guard).
	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/keep.mkv", Size: 100, Fingerprint: "fp-keep", Type: library.MovieItem},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-orphan")
	require.NoError(t, err)

	hits, _, err := search.Search(context.Background(), "deleted", nil, nil, 10)
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
	assert.GreaterOrEqual(t, added, 3)

	items := mediaRepo.All()
	types := make(map[string]bool)
	for _, item := range items {
		types[string(item.Type)] = true
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

func TestUpsertMediaFile_Changed_WithDuration_UpdatesDuration(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	d1s, _ := shared.NewDurationMs(1000)
	h.mediaItems.Create(context.Background(), &library.MediaItem{
		ID: "item-1", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/film.mkv", Size: 500, Duration: d1s},
	})

	h.prober.Results["/movies/film.mkv"] = &library.ProbeResult{
		Duration: mustProbeDuration(9999),
		Streams:  []*library.MediaStream{{ID: "v1", StreamType: "video"}},
	}

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 2000},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	item, _ := h.mediaItems.GetByID(context.Background(), "item-1")
	require.NotNil(t, item.File)
	assert.Equal(t, int64(9999), item.File.Duration.Milliseconds())
}

func TestUpsertMediaFile_Changed_ZeroDuration_KeepsOldDuration(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	d5s, _ := shared.NewDurationMs(5000)
	h.mediaItems.Create(context.Background(), &library.MediaItem{
		ID: "item-1", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/film.mkv", Size: 500, Duration: d5s},
	})

	// Prober returns 0 duration
	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 2000},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	item, _ := h.mediaItems.GetByID(context.Background(), "item-1")
	require.NotNil(t, item.File)
	assert.Equal(t, int64(5000), item.File.Duration.Milliseconds(), "should keep old duration when prober returns 0")
}

func TestUpsertMediaFile_NewItem_ParentID_Propagated(t *testing.T) {
	t.Parallel()
	h := newHarness()
	s1, e1 := 1, 1
	lib := &library.Library{ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/show/s01e01.mkv", Size: 500, Title: "My Show", Season: &s1, Episode: &e1, Type: library.Episode},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	items := h.mediaItems.All()
	var episode *library.MediaItem
	var seasonID string
	for _, item := range items {
		if item.Type == "season" {
			seasonID = string(item.ID)
		}
		if item.File != nil && item.File.Path == "/tv/show/s01e01.mkv" {
			episode = item
		}
	}
	require.NotNil(t, episode)
	assert.Equal(t, seasonID, episode.ParentID, "episode ParentID should be the season ID")
}

func TestUpsertMediaFile_ExistingChanged_UpdatesParentID(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	h.mediaItems.Create(context.Background(), &library.MediaItem{
		ID: "item-1", LibraryID: "lib-1",
		File:     &library.MediaFile{Path: "/movies/film.mkv", Size: 500},
		ParentID: "old-parent",
	})

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/film.mkv", Size: 1500},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)

	item, _ := h.mediaItems.GetByID(context.Background(), "item-1")
	assert.Empty(t, item.ParentID, "movie scan passes empty parentID")
}

// Deleting a media item left its document in the search index: the row vanished
// from the DB but the title stayed findable until ReconcileSearchIndex noticed
// the count mismatch (15 min ticker) and rebuilt. The music path
// (music/command_service.go DeleteTrack) and the orphan pruner both remove from
// the index straight away - only the admin delete didn't.
func TestDeleteMediaItem_RemovesFromSearchIndex(t *testing.T) {
	t.Parallel()
	svc, _, _, mediaRepo, _, _, search, _ := newCmdSvcWithSearch()
	ctx := context.Background()

	item := &library.MediaItem{
		ID: "item-to-delete", LibraryID: "lib-1", Title: "Findable",
		File: &library.MediaFile{Path: "/movies/film.mkv"},
	}
	require.NoError(t, mediaRepo.Create(ctx, item))
	require.NoError(t, search.IndexMediaItem(ctx, item))
	if _, ok := search.Get("item-to-delete"); !ok {
		t.Fatal("precondition: the item should be indexed before deletion")
	}

	require.NoError(t, svc.DeleteMediaItem(ctx, "item-to-delete"))

	if _, ok := search.Get("item-to-delete"); ok {
		t.Error("the deleted item is still searchable")
	}
}

// A search backend outage must not fail the delete: the DB row is already gone
// and the resilient adapter queues the removal for replay.
func TestDeleteMediaItem_SearchFailureDoesNotFailTheDelete(t *testing.T) {
	t.Parallel()
	svc, _, _, mediaRepo, _, _, search, _ := newCmdSvcWithSearch()
	ctx := context.Background()

	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: "item-2", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/f.mkv"},
	}))
	search.Err = errors.New("meili down")

	require.NoError(t, svc.DeleteMediaItem(ctx, "item-2"))
	_, err := mediaRepo.GetByID(ctx, "item-2")
	require.ErrorIs(t, err, shared.ErrNotFound)
}

// A rescan where nothing changed must not re-query the DB once per episode.
// The TV path preloads the whole library to resolve show/season containers, and
// then used to look each episode up again by path - one SELECT per episode, per
// pass, every couple of minutes. On a large series library that is thousands of
// pointless queries against a Postgres colocated on the same weak NAS.
func TestScanTVShow_RescanResolvesFromPreloadedIndex(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	const episodes = 12
	files := make([]library.ScannedFile, 0, episodes)
	for i := 1; i <= episodes; i++ {
		s1, ep := 1, i
		files = append(files, library.ScannedFile{
			Path: fmt.Sprintf("/tv/show/s01e%02d.mkv", i), Size: int64(100 + i),
			Title: "My Show", Season: &s1, Episode: &ep, Type: library.Episode,
		})
	}
	h.scanner.Results["/tv"] = files

	// First pass creates everything - lookups here are expected.
	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	// Second pass: nothing changed on disk. Every episode is already in the
	// preloaded snapshot, so no per-file lookup should be needed.
	h.mediaItems.GetByFilePathCalls.Store(0)
	_, _, err = h.svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	assert.Zero(t, h.mediaItems.GetByFilePathCalls.Load(),
		"an unchanged rescan must resolve every episode from the preloaded index, not from the DB")
}
