package library_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	applib "github.com/eveyproject/evey/internal/app/library"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testDebounceDelay shrinks the watcher's real-time 2s debounce to milliseconds
// so the timing-based tests run fast and don't flake on slower CI runners (the
// retry test does 2× the debounce + scan, which tipped past its 8s deadline on
// the self-hosted box).
const testDebounceDelay = 10 * time.Millisecond

type fakeLibraryScanner struct {
	mu      sync.Mutex
	scanned []string
	// errs is an optional per-call error sequence (consumed front to back);
	// once exhausted (or empty) calls succeed. Lets a test make the first
	// scan report ErrScanInProgress and the retry succeed.
	errs []error
}

func (f *fakeLibraryScanner) ScanLibrary(_ context.Context, libraryID string) (int, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scanned = append(f.scanned, libraryID)
	var err error
	if len(f.errs) > 0 {
		err = f.errs[0]
		f.errs = f.errs[1:]
	}
	return 1, 0, err
}

func (f *fakeLibraryScanner) Scanned() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.scanned))
	copy(out, f.scanned)
	return out
}

func TestWatcherService_Start_WatchesEnabledLibraries(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &fakeLibraryScanner{}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx := context.Background()
	_ = libRepo.Create(ctx, &library.Library{
		ID: "lib-1", Name: "Movies", Type: "movie",
		Paths: []string{"/media/movies"}, Enabled: true, RealtimeMonitoring: true,
	})
	_ = libRepo.Create(ctx, &library.Library{
		ID: "lib-2", Name: "TV", Type: "tvshow",
		Paths: []string{"/media/tv"}, Enabled: true, RealtimeMonitoring: true,
	})
	_ = libRepo.Create(ctx, &library.Library{
		ID: "lib-3", Name: "Disabled", Type: "movie",
		Paths: []string{"/media/disabled"}, Enabled: true, RealtimeMonitoring: false,
	})

	ws := applib.NewWatcherService(watcher, libRepo, scanner, log, applib.WatcherWithDebounce(testDebounceDelay))
	err := ws.Start(ctx)
	require.NoError(t, err)
	defer ws.Stop()

	watched := watcher.WatchedPaths()
	assert.Contains(t, watched, "/media/movies")
	assert.Contains(t, watched, "/media/tv")
	assert.NotContains(t, watched, "/media/disabled")
}

func TestWatcherService_FileEvent_TriggersScan(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &fakeLibraryScanner{}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx := context.Background()
	_ = libRepo.Create(ctx, &library.Library{
		ID: "lib-1", Name: "Movies", Type: "movie",
		Paths: []string{"/media/movies"}, Enabled: true, RealtimeMonitoring: true,
	})

	ws := applib.NewWatcherService(watcher, libRepo, scanner, log, applib.WatcherWithDebounce(testDebounceDelay))
	err := ws.Start(ctx)
	require.NoError(t, err)
	defer ws.Stop()

	watcher.SendEvent(library.FileEvent{
		Path: "/media/movies/new_film.mkv",
		Op:   library.FileCreated,
	})

	// wait for debounce (2s) + processing
	require.Eventually(t, func() bool {
		return len(scanner.Scanned()) > 0
	}, 5*time.Second, 100*time.Millisecond)

	assert.Equal(t, "lib-1", scanner.Scanned()[0])
}

func TestWatcherService_Debounce_MultipleFastEvents(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &fakeLibraryScanner{}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx := context.Background()
	_ = libRepo.Create(ctx, &library.Library{
		ID: "lib-1", Name: "Movies", Type: "movie",
		Paths: []string{"/media/movies"}, Enabled: true, RealtimeMonitoring: true,
	})

	ws := applib.NewWatcherService(watcher, libRepo, scanner, log, applib.WatcherWithDebounce(testDebounceDelay))
	err := ws.Start(ctx)
	require.NoError(t, err)
	defer ws.Stop()

	// rapid-fire events should debounce into a single scan
	for range 10 {
		watcher.SendEvent(library.FileEvent{
			Path: "/media/movies/film.mkv",
			Op:   library.FileModified,
		})
	}

	// wait for debounce + processing
	require.Eventually(t, func() bool {
		return len(scanner.Scanned()) > 0
	}, 5*time.Second, 100*time.Millisecond)

	// a second scan must never appear (debounce) - fail fast if it does, instead
	// of a blind sleep that always pays its full duration.
	assert.Never(t, func() bool {
		return len(scanner.Scanned()) > 1
	}, 500*time.Millisecond, 50*time.Millisecond, "should debounce into single scan")
}

func TestWatcherService_AddLibrary(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &fakeLibraryScanner{}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx := context.Background()
	ws := applib.NewWatcherService(watcher, libRepo, scanner, log, applib.WatcherWithDebounce(testDebounceDelay))
	err := ws.Start(ctx)
	require.NoError(t, err)
	defer ws.Stop()

	lib := &library.Library{
		ID: "lib-new", Name: "New", Type: "movie",
		Paths: []string{"/media/new"}, Enabled: true, RealtimeMonitoring: true,
	}
	ws.AddLibrary(lib)
	assert.Contains(t, watcher.WatchedPaths(), "/media/new")
}

func TestWatcherService_IgnoresDisabledLibrary(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &fakeLibraryScanner{}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx := context.Background()
	ws := applib.NewWatcherService(watcher, libRepo, scanner, log, applib.WatcherWithDebounce(testDebounceDelay))
	err := ws.Start(ctx)
	require.NoError(t, err)
	defer ws.Stop()

	lib := &library.Library{
		ID: "lib-dis", Name: "Disabled", Type: "movie",
		Paths: []string{"/media/dis"}, Enabled: true, RealtimeMonitoring: false,
	}
	ws.AddLibrary(lib)
	assert.NotContains(t, watcher.WatchedPaths(), "/media/dis")
}

func TestWatcherService_UnrelatedPath_NoScan(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &fakeLibraryScanner{}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx := context.Background()
	_ = libRepo.Create(ctx, &library.Library{
		ID: "lib-1", Name: "Movies", Type: "movie",
		Paths: []string{"/media/movies"}, Enabled: true, RealtimeMonitoring: true,
	})

	ws := applib.NewWatcherService(watcher, libRepo, scanner, log, applib.WatcherWithDebounce(testDebounceDelay))
	err := ws.Start(ctx)
	require.NoError(t, err)
	defer ws.Stop()

	watcher.SendEvent(library.FileEvent{
		Path: "/tmp/random/file.mkv",
		Op:   library.FileCreated,
	})

	// An unrelated path must never trigger a scan. assert.Never fails the instant
	// a scan appears and otherwise returns after the (much shorter) window, vs the
	// old blind 3 s sleep that every run paid in full.
	assert.Never(t, func() bool {
		return len(scanner.Scanned()) > 0
	}, time.Second, 50*time.Millisecond, "unrelated path should not trigger scan")
}

func TestWatcherService_RemoveLibraryByID(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &fakeLibraryScanner{}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx := context.Background()
	ws := applib.NewWatcherService(watcher, libRepo, scanner, log, applib.WatcherWithDebounce(testDebounceDelay))
	err := ws.Start(ctx)
	require.NoError(t, err)
	defer ws.Stop()

	lib := &library.Library{
		ID: "lib-rm", Name: "ToRemove", Type: "movie",
		Paths: []string{"/media/rm1", "/media/rm2"}, Enabled: true, RealtimeMonitoring: true,
	}
	ws.AddLibrary(lib)
	assert.Contains(t, watcher.WatchedPaths(), "/media/rm1")
	assert.Contains(t, watcher.WatchedPaths(), "/media/rm2")

	ws.RemoveLibraryByID("lib-rm")
	assert.NotContains(t, watcher.WatchedPaths(), "/media/rm1")
	assert.NotContains(t, watcher.WatchedPaths(), "/media/rm2")
}

func TestWatcherService_MultiplePaths_SingleLibrary(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &fakeLibraryScanner{}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx := context.Background()
	_ = libRepo.Create(ctx, &library.Library{
		ID: "lib-multi", Name: "Multi", Type: "movie",
		Paths: []string{"/media/dir1", "/media/dir2"}, Enabled: true, RealtimeMonitoring: true,
	})

	ws := applib.NewWatcherService(watcher, libRepo, scanner, log, applib.WatcherWithDebounce(testDebounceDelay))
	err := ws.Start(ctx)
	require.NoError(t, err)
	defer ws.Stop()

	watched := watcher.WatchedPaths()
	assert.Contains(t, watched, "/media/dir1")
	assert.Contains(t, watched, "/media/dir2")
}

func TestWatcherService_AddLibrary_NotEnabledIgnored(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &fakeLibraryScanner{}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx := context.Background()
	ws := applib.NewWatcherService(watcher, libRepo, scanner, log, applib.WatcherWithDebounce(testDebounceDelay))
	err := ws.Start(ctx)
	require.NoError(t, err)
	defer ws.Stop()

	ws.AddLibrary(&library.Library{
		ID: "lib-off", Name: "Off", Type: "movie",
		Paths: []string{"/media/off"}, Enabled: false, RealtimeMonitoring: true,
	})
	assert.NotContains(t, watcher.WatchedPaths(), "/media/off")
}

func TestWatcherService_TwoLibraries_EventRoutesToCorrectOne(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &fakeLibraryScanner{}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx := context.Background()
	_ = libRepo.Create(ctx, &library.Library{
		ID: "lib-movies", Name: "Movies", Type: "movie",
		Paths: []string{"/media/movies"}, Enabled: true, RealtimeMonitoring: true,
	})
	_ = libRepo.Create(ctx, &library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow",
		Paths: []string{"/media/tv"}, Enabled: true, RealtimeMonitoring: true,
	})

	ws := applib.NewWatcherService(watcher, libRepo, scanner, log, applib.WatcherWithDebounce(testDebounceDelay))
	err := ws.Start(ctx)
	require.NoError(t, err)
	defer ws.Stop()

	watcher.SendEvent(library.FileEvent{
		Path: "/media/tv/show/episode.mkv",
		Op:   library.FileCreated,
	})

	require.Eventually(t, func() bool {
		return len(scanner.Scanned()) > 0
	}, 5*time.Second, 100*time.Millisecond)

	assert.Equal(t, "lib-tv", scanner.Scanned()[0])
}

// If the debounced scan is rejected because one is already running, the watcher
// must retry (not drop the change), otherwise a file copied during an in-flight
// scan is lost until the next FS event. The first ScanLibrary reports
// ErrScanInProgress; the retry must call it again.
func TestWatcherService_RetriesWhenScanInProgress(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &fakeLibraryScanner{errs: []error{applib.ErrScanInProgress}}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx := context.Background()
	_ = libRepo.Create(ctx, &library.Library{
		ID: "lib-1", Name: "Movies", Type: "movie",
		Paths: []string{"/media/movies"}, Enabled: true, RealtimeMonitoring: true,
	})

	ws := applib.NewWatcherService(watcher, libRepo, scanner, log, applib.WatcherWithDebounce(testDebounceDelay))
	require.NoError(t, ws.Start(ctx))
	defer ws.Stop()

	watcher.SendEvent(library.FileEvent{Path: "/media/movies/new.mkv", Op: library.FileCreated})

	// First debounced scan returns ErrScanInProgress → re-armed → second scan
	// succeeds. Two calls prove the change was retried, not dropped.
	require.Eventually(t, func() bool {
		return len(scanner.Scanned()) >= 2
	}, 8*time.Second, 100*time.Millisecond)
}

func TestWatcherService_StopIsSafe(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &fakeLibraryScanner{}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ws := applib.NewWatcherService(watcher, libRepo, scanner, log, applib.WatcherWithDebounce(testDebounceDelay))
	err := ws.Start(context.Background())
	require.NoError(t, err)
	ws.Stop()
}

func TestWatcherService_RemoveLibraryByID_NoMatchIsNoop(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &fakeLibraryScanner{}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx := context.Background()
	ws := applib.NewWatcherService(watcher, libRepo, scanner, log, applib.WatcherWithDebounce(testDebounceDelay))
	err := ws.Start(ctx)
	require.NoError(t, err)
	defer ws.Stop()

	ws.AddLibrary(&library.Library{
		ID: "lib-keep", Name: "Keep", Type: "movie",
		Paths: []string{"/media/keep"}, Enabled: true, RealtimeMonitoring: true,
	})

	ws.RemoveLibraryByID("nonexistent")
	assert.Contains(t, watcher.WatchedPaths(), "/media/keep")
}

func TestWatcherService_WithEventBus_LibraryCreated(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &fakeLibraryScanner{}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = libRepo.Create(ctx, &library.Library{
		ID: "lib-evt", Name: "EventLib", Type: "movie",
		Paths: []string{"/media/evt"}, Enabled: true, RealtimeMonitoring: true,
	})

	ws := applib.NewWatcherService(watcher, libRepo, scanner, log, applib.WatcherWithEventBus(bus), applib.WatcherWithDebounce(testDebounceDelay))
	err := ws.Start(ctx)
	require.NoError(t, err)
	defer ws.Stop()

	// publish library.created event => watcher should add it
	bus.Publish(ctx, library.LibraryCreated{LibraryID: "lib-evt", Name: "EventLib", Type: "movie"})
	assert.Contains(t, watcher.WatchedPaths(), "/media/evt")
}

func TestWatcherService_WithEventBus_LibraryDeleted(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &fakeLibraryScanner{}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	_ = libRepo.Create(ctx, &library.Library{
		ID: "lib-del", Name: "Del", Type: "movie",
		Paths: []string{"/media/del"}, Enabled: true, RealtimeMonitoring: true,
	})

	ws := applib.NewWatcherService(watcher, libRepo, scanner, log, applib.WatcherWithEventBus(bus), applib.WatcherWithDebounce(testDebounceDelay))
	err := ws.Start(ctx)
	require.NoError(t, err)
	defer ws.Stop()

	assert.Contains(t, watcher.WatchedPaths(), "/media/del")

	bus.Publish(ctx, library.LibraryDeleted{LibraryID: "lib-del"})
	assert.NotContains(t, watcher.WatchedPaths(), "/media/del")
}

func TestWatcherService_WatchFails_FallbackToPeriodicScan(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	watcher.WatchErr = errors.New("inotify: no space")
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &fakeLibraryScanner{}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx := context.Background()
	_ = libRepo.Create(ctx, &library.Library{
		ID: "lib-fail", Name: "Fail", Type: "movie",
		Paths: []string{"/media/fail"}, Enabled: true, RealtimeMonitoring: true,
	})

	ws := applib.NewWatcherService(watcher, libRepo, scanner, log, applib.WatcherWithDebounce(testDebounceDelay))
	err := ws.Start(ctx)
	require.NoError(t, err)
	defer ws.Stop()

	// inotify failed, path should not be in watched list
	assert.NotContains(t, watcher.WatchedPaths(), "/media/fail")
}

func TestWatcherService_FileRemoved_TriggersScan(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &fakeLibraryScanner{}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx := context.Background()
	_ = libRepo.Create(ctx, &library.Library{
		ID: "lib-1", Name: "Movies", Type: "movie",
		Paths: []string{"/media/movies"}, Enabled: true, RealtimeMonitoring: true,
	})

	ws := applib.NewWatcherService(watcher, libRepo, scanner, log, applib.WatcherWithDebounce(testDebounceDelay))
	err := ws.Start(ctx)
	require.NoError(t, err)
	defer ws.Stop()

	watcher.SendEvent(library.FileEvent{
		Path: "/media/movies/old_film.mkv",
		Op:   library.FileRemoved,
	})

	require.Eventually(t, func() bool {
		return len(scanner.Scanned()) > 0
	}, 5*time.Second, 100*time.Millisecond)
	assert.Equal(t, "lib-1", scanner.Scanned()[0])
}

// The adaptive scan cadence trusts this map's POLARITY: only a library it
// names may be demoted to the hourly backstop. It must therefore name ONLY
// libraries with an established, healthy watch - and stay empty when nothing
// is watched (prod 2026-08-26: monitoring off everywhere + the old
// unwatched-set contract read the empty map as "everything healthy" and
// starved every library down to one walk per hour).
func TestWatcherService_AliveWatchedLibraries_NamesOnlyEstablishedWatches(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &fakeLibraryScanner{}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx := context.Background()
	_ = libRepo.Create(ctx, &library.Library{
		ID: "lib-watched", Name: "Movies", Type: "movie",
		Paths: []string{"/media/movies"}, Enabled: true, RealtimeMonitoring: true,
	})
	_ = libRepo.Create(ctx, &library.Library{
		ID: "lib-off", Name: "Musique", Type: "music",
		Paths: []string{"/media/musique"}, Enabled: true, RealtimeMonitoring: false,
	})

	ws := applib.NewWatcherService(watcher, libRepo, scanner, log, applib.WatcherWithDebounce(testDebounceDelay))
	require.NoError(t, ws.Start(ctx))
	defer ws.Stop()

	alive := ws.AliveWatchedLibraries()
	require.True(t, alive["lib-watched"], "an established watch must be vouched for")
	require.False(t, alive["lib-off"], "monitoring OFF must never read as a live watch - it would starve the periodic scan")
}

// Turning real-time monitoring on for an existing library arms its watch at
// once, moving its paths re-points the watch, and turning it off drops it.
// Before, only library.created and library.deleted reached the watcher, so an
// edit changed the database and nothing the server was doing.
func TestWatcherService_WithEventBus_LibraryUpdatedRearmsTheWatch(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	libRepo := libfakes.NewFakeLibraryRepository()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	lib := &library.Library{
		ID: "lib-upd", Name: "Series", Type: "tvshow",
		Paths: []string{"/media/series"}, Enabled: true, RealtimeMonitoring: false,
	}
	require.NoError(t, libRepo.Create(ctx, lib))

	ws := applib.NewWatcherService(watcher, libRepo, &fakeLibraryScanner{}, log, applib.WatcherWithEventBus(bus), applib.WatcherWithDebounce(testDebounceDelay))
	require.NoError(t, ws.Start(ctx))
	defer ws.Stop()
	assert.NotContains(t, watcher.WatchedPaths(), "/media/series")

	lib.RealtimeMonitoring = true
	require.NoError(t, libRepo.Update(ctx, lib))
	require.NoError(t, bus.Publish(ctx, library.LibraryUpdated{LibraryID: "lib-upd"}))
	assert.Contains(t, watcher.WatchedPaths(), "/media/series")
	assert.True(t, ws.AliveWatchedLibraries()["lib-upd"])

	lib.Paths = []string{"/media/shows"}
	require.NoError(t, libRepo.Update(ctx, lib))
	require.NoError(t, bus.Publish(ctx, library.LibraryUpdated{LibraryID: "lib-upd"}))
	assert.NotContains(t, watcher.WatchedPaths(), "/media/series")
	assert.Contains(t, watcher.WatchedPaths(), "/media/shows")

	lib.RealtimeMonitoring = false
	require.NoError(t, libRepo.Update(ctx, lib))
	require.NoError(t, bus.Publish(ctx, library.LibraryUpdated{LibraryID: "lib-upd"}))
	assert.NotContains(t, watcher.WatchedPaths(), "/media/shows")
	assert.False(t, ws.AliveWatchedLibraries()["lib-upd"])
}

// A library whose watch failed and is then fixed by an edit leaves the
// periodic-scan fallback: the failure is the old paths', not the new ones'.
func TestWatcherService_LibraryUpdatedClearsAnOldWatchFailure(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	watcher.WatchErr = errors.New("inotify: no space")
	libRepo := libfakes.NewFakeLibraryRepository()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	bus := sharedfakes.NewFakeEventBus()

	ctx := context.Background()
	lib := &library.Library{
		ID: "lib-fix", Name: "Films", Type: "movie",
		Paths: []string{"/media/films"}, Enabled: true, RealtimeMonitoring: true,
	}
	require.NoError(t, libRepo.Create(ctx, lib))
	ws := applib.NewWatcherService(watcher, libRepo, &fakeLibraryScanner{}, log, applib.WatcherWithEventBus(bus), applib.WatcherWithDebounce(testDebounceDelay))
	require.NoError(t, ws.Start(ctx))
	defer ws.Stop()
	require.True(t, ws.UnwatchedLibraries()["lib-fix"])

	watcher.WatchErr = nil
	require.NoError(t, bus.Publish(ctx, library.LibraryUpdated{LibraryID: "lib-fix"}))
	assert.False(t, ws.UnwatchedLibraries()["lib-fix"])
	assert.True(t, ws.AliveWatchedLibraries()["lib-fix"])
}
