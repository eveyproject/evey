package library_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	applib "github.com/evey-project/evey/internal/app/library"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTypedDirScanner is a minimal inline fake for TypedDirectoryScanner.
// We keep it local to the tests (scope-lock forbids touching fakes package).
type fakeTypedDirScanner struct {
	mu      sync.Mutex
	Results map[string]int
	Errs    map[string]error
	Calls   []string
}

func newFakeTypedDirScanner() *fakeTypedDirScanner {
	return &fakeTypedDirScanner{
		Results: make(map[string]int),
		Errs:    make(map[string]error),
	}
}

func (f *fakeTypedDirScanner) ScanDirectory(_ context.Context, _ string, dir string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, dir)
	if err, ok := f.Errs[dir]; ok {
		return 0, err
	}
	return f.Results[dir], nil
}

// --- GetByID error: library disappeared ---

func TestScanService_GetLibraryError_PropagatesError(t *testing.T) {
	h := newScanHarness()
	// libQuery not seeded → GetByID returns ErrNotFound.

	_, _, err := h.svc.ScanLibrary(context.Background(), "missing-lib")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get library")
}

// --- Concurrent scan guard: second call returns ErrScanInProgress ---

func TestScanService_ConcurrentScanSameLibrary_Rejected(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})

	release := make(chan struct{})
	started := make(chan struct{})
	var startedOnce sync.Once
	// Use music scanner to hold the first scan open inside ScanLibrary.
	music := newFakeTypedDirScanner()
	h.libQuery.Seed(&library.Library{ID: "lib-m", Type: "music", Paths: []string{"/music"}})
	h.libCmd.Seed(&library.Library{ID: "lib-m", Type: "music", Paths: []string{"/music"}})
	blocking := &blockingDirScanner{
		onCall: func() {
			startedOnce.Do(func() { close(started) })
			<-release
		},
	}
	_ = music
	h.svc.WithMusicScanner(blocking)

	errCh := make(chan error, 1)
	go func() {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-m")
		errCh <- err
	}()

	// Wait deterministically for the first scan to reach ScanDirectory.
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first scan did not reach blocking ScanDirectory")
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-m")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scan already in progress")

	close(release)
	require.NoError(t, <-errCh)
}

type blockingDirScanner struct {
	onCall func()
}

func (b *blockingDirScanner) ScanDirectory(_ context.Context, _ string, _ string) (int, error) {
	if b.onCall != nil {
		b.onCall()
	}
	return 0, nil
}

// --- Scan releases lock: sequential scans on same library succeed ---

func TestScanService_SequentialScansSameLibrary_LockReleased(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.scanner.Results["/m"] = nil

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	// Second run must succeed (lock removed in defer).
	_, _, err = h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
}

// --- Music scanner: nil rejected ---

func TestScanService_MusicLibrary_NilScannerError(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-m", Type: "music", Paths: []string{"/music"}})
	h.libCmd.Seed(&library.Library{ID: "lib-m", Type: "music", Paths: []string{"/music"}})

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-m")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "music scanner not configured")
}

// --- Music scanner: happy path - invoked once per library path ---

func TestScanService_MusicLibrary_InvokesScannerPerPath(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-m", Type: "music", Paths: []string{"/music/a", "/music/b"}})
	h.libCmd.Seed(&library.Library{ID: "lib-m", Type: "music", Paths: []string{"/music/a", "/music/b"}})
	music := newFakeTypedDirScanner()
	music.Results["/music/a"] = 3
	music.Results["/music/b"] = 2
	h.svc.WithMusicScanner(music)

	added, updated, err := h.svc.ScanLibrary(context.Background(), "lib-m")
	require.NoError(t, err)
	assert.Equal(t, 5, added, "added must sum counts from every path")
	assert.Equal(t, 0, updated)
	assert.ElementsMatch(t, []string{"/music/a", "/music/b"}, music.Calls)
}

// --- Music scanner: one path errors, others continue (warning, not fatal) ---

func TestScanService_MusicLibrary_PathErrorContinues(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-m", Type: "music", Paths: []string{"/music/a", "/music/b"}})
	h.libCmd.Seed(&library.Library{ID: "lib-m", Type: "music", Paths: []string{"/music/a", "/music/b"}})
	music := newFakeTypedDirScanner()
	music.Errs["/music/a"] = errors.New("boom")
	music.Results["/music/b"] = 7
	h.svc.WithMusicScanner(music)

	var added int
	logs := captureSlog(t, func() {
		var err error
		added, _, err = h.svc.ScanLibrary(context.Background(), "lib-m")
		require.NoError(t, err)
	})
	assert.Equal(t, 7, added, "failing path must not cancel total")
	assert.Contains(t, logs, "music scan path failed")
}

// A root that cannot be read makes the scan PARTIAL, and says so: the report
// and the library.scan.completed event name the failed root, instead of a
// clean "done" over a library half of which was never looked at.
func TestScanService_MusicLibrary_FailedRootIsReportedAsPartial(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-m", Type: "music", Paths: []string{"/music/a", "/music/b"}})
	h.libCmd.Seed(&library.Library{ID: "lib-m", Type: "music", Paths: []string{"/music/a", "/music/b"}})
	music := newFakeTypedDirScanner()
	music.Errs["/music/a"] = errors.New("share unmounted")
	music.Results["/music/b"] = 7
	h.svc.WithMusicScanner(music)

	report, err := h.svc.ScanLibraryReport(context.Background(), "lib-m")
	require.NoError(t, err)
	assert.Equal(t, 7, report.Added)
	assert.Equal(t, []string{"/music/a"}, report.FailedPaths)

	var completed *library.LibraryScanCompleted
	for _, e := range h.bus.PublishedEvents() {
		if c, ok := e.(library.LibraryScanCompleted); ok {
			completed = &c
		}
	}
	require.NotNil(t, completed)
	assert.Equal(t, []string{"/music/a"}, completed.FailedPaths)

	music.Errs = map[string]error{}
	report, err = h.svc.ScanLibraryReport(context.Background(), "lib-m")
	require.NoError(t, err)
	assert.Empty(t, report.FailedPaths, "every root read: the scan is complete")
}

// --- Photo scanner: nil rejected ---

func TestScanService_PhotoLibrary_NilScannerError(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-p", Type: "photo", Paths: []string{"/photo"}})
	h.libCmd.Seed(&library.Library{ID: "lib-p", Type: "photo", Paths: []string{"/photo"}})

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-p")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "photo scanner not configured")
}

// --- Photo scanner: happy path ---

func TestScanService_PhotoLibrary_InvokesScannerPerPath(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-p", Type: "photo", Paths: []string{"/p/a", "/p/b"}})
	h.libCmd.Seed(&library.Library{ID: "lib-p", Type: "photo", Paths: []string{"/p/a", "/p/b"}})
	photo := newFakeTypedDirScanner()
	photo.Results["/p/a"] = 4
	photo.Results["/p/b"] = 6
	h.svc.WithPhotoScanner(photo)

	added, _, err := h.svc.ScanLibrary(context.Background(), "lib-p")
	require.NoError(t, err)
	assert.Equal(t, 10, added)
	assert.ElementsMatch(t, []string{"/p/a", "/p/b"}, photo.Calls)
}

// --- Photo scanner: path error warning ---

func TestScanService_PhotoLibrary_PathErrorContinues(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-p", Type: "photo", Paths: []string{"/p/a", "/p/b"}})
	h.libCmd.Seed(&library.Library{ID: "lib-p", Type: "photo", Paths: []string{"/p/a", "/p/b"}})
	photo := newFakeTypedDirScanner()
	photo.Errs["/p/b"] = errors.New("permission denied")
	photo.Results["/p/a"] = 1
	h.svc.WithPhotoScanner(photo)

	var added int
	logs := captureSlog(t, func() {
		var err error
		added, _, err = h.svc.ScanLibrary(context.Background(), "lib-p")
		require.NoError(t, err)
	})
	assert.Equal(t, 1, added)
	assert.Contains(t, logs, "photo scan path failed")
}

// --- Music/photo path isolation: music scan must NOT invoke photo scanner ---

func TestScanService_MusicLibrary_DoesNotInvokePhotoScanner(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-m", Type: "music", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-m", Type: "music", Paths: []string{"/m"}})
	music := newFakeTypedDirScanner()
	music.Results["/m"] = 1
	photo := newFakeTypedDirScanner()
	photo.Results["/m"] = 99
	h.svc.WithMusicScanner(music)
	h.svc.WithPhotoScanner(photo)

	added, _, err := h.svc.ScanLibrary(context.Background(), "lib-m")
	require.NoError(t, err)
	assert.Equal(t, 1, added, "music type must route ONLY to music scanner")
	assert.Empty(t, photo.Calls, "photo scanner must not be called for music library")
}

// --- scanner.Scan returns error (movie library) → ScanLibrary propagates error ---

func TestScanService_UnreadableRoot_Reported(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.scanner.Err = errors.New("io error")

	// A root the scanner cannot read is reported as a failed path, not raised as
	// an opaque internal error - the scan completes over whatever it could read.
	report, err := h.svc.ScanLibraryReport(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"/m"}, report.FailedPaths)
}

// --- mediaItems.Create error is logged via worker upsert warn; scan continues ---

func TestScanService_CreateError_LoggedWorkerContinues(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp-a", Type: library.MovieItem, Title: "A"},
		{Path: "/m/b.mkv", Size: 200, Fingerprint: "fp-b", Type: library.MovieItem, Title: "B"},
	}
	h.media.ErrCreate = errors.New("db down")

	var added int
	logs := captureSlog(t, func() {
		var err error
		added, _, err = h.svc.ScanLibrary(context.Background(), "lib-1")
		require.NoError(t, err, "ScanLibrary must not fail when per-file Create errors")
	})
	assert.Equal(t, 0, added)
	assert.Contains(t, logs, "upsert media file")
}

// --- Parallel worker sanity: many files → all processed concurrently ---

func TestScanService_ParallelWorkers_AllFilesProcessed(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	const N = 20
	files := make([]library.ScannedFile, N)
	for i := 0; i < N; i++ {
		files[i] = library.ScannedFile{
			Path:        "/m/file" + string(rune('a'+i)) + ".mkv",
			Size:        int64(100 + i),
			Fingerprint: "fp-" + string(rune('a'+i)),
			Type:        library.MovieItem,
			Title:       "T",
		}
	}
	h.scanner.Results["/m"] = files

	added, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, N, added)
	assert.Len(t, h.media.All(), N)
}

// --- Prober error: ProbeStreams fails → duration stays zero, no fatal error ---

func TestScanService_ProberError_DurationStaysZero(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: []string{"/m"}})
	h.prober.Err = errors.New("ffprobe missing")
	h.scanner.Results["/m"] = []library.ScannedFile{
		{Path: "/m/a.mkv", Size: 100, Fingerprint: "fp", Type: library.MovieItem, Title: "A"},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	items := h.media.All()
	require.Len(t, items, 1)
	require.NotNil(t, items[0].File)
	assert.Equal(t, int64(0), items[0].File.Duration.Milliseconds())
}

// --- ProbeAndStoreStreams short-circuit when prober is nil ---

func TestScanService_ProbeAndStoreStreams_NilProberReturnsZero(t *testing.T) {
	// Rebuild a service with nil prober to exercise the early-return guard.
	h := newScanHarness()
	svc := applib.NewScanService(
		h.libCmd, h.libQuery, h.media, h.streams, h.persons, h.mediaPersons,
		h.scanner, nil /* prober */, h.search, h.bus,
	)
	got, _, probedOK := svc.ProbeAndStoreStreams(context.Background(), "m1", "/unused")
	assert.True(t, got.IsZero())
	assert.False(t, probedOK)
}

// --- Library without Paths: scans nothing but still emits started+completed ---

func TestScanService_EmptyPaths_NoScannerCall(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: nil})
	h.libCmd.Seed(&library.Library{ID: "lib-1", Type: "movie", Paths: nil})

	added, updated, err := h.svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 0, added)
	assert.Equal(t, 0, updated)

	var started, completed int
	for _, e := range h.bus.PublishedEvents() {
		switch e.EventName() {
		case "library.scan.started":
			started++
		case "library.scan.completed":
			completed++
		}
	}
	assert.Equal(t, 1, started)
	assert.Equal(t, 1, completed)
}

// --- Music path with zero paths: still emits scan.completed ---

func TestScanService_MusicLibrary_NoPaths_StillCompletes(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-m", Type: "music", Paths: nil})
	h.libCmd.Seed(&library.Library{ID: "lib-m", Type: "music", Paths: nil})
	music := newFakeTypedDirScanner()
	h.svc.WithMusicScanner(music)

	added, _, err := h.svc.ScanLibrary(context.Background(), "lib-m")
	require.NoError(t, err)
	assert.Equal(t, 0, added)
	assert.Empty(t, music.Calls)
	var completed bool
	for _, e := range h.bus.PublishedEvents() {
		if e.EventName() == "library.scan.completed" {
			completed = true
		}
	}
	assert.True(t, completed)
}
