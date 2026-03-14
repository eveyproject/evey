package library_test

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	applib "github.com/gravity-zero/evey/internal/app/library"
	"github.com/gravity-zero/evey/internal/domain/library"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeLibraryScanner struct {
	mu      sync.Mutex
	scanned []string
}

func (f *fakeLibraryScanner) ScanLibrary(_ context.Context, libraryID string) (int, int, error) {
	f.mu.Lock()
	f.scanned = append(f.scanned, libraryID)
	f.mu.Unlock()
	return 1, 0, nil
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

	ws := applib.NewWatcherService(watcher, libRepo, scanner, log)
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

	ws := applib.NewWatcherService(watcher, libRepo, scanner, log)
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

	ws := applib.NewWatcherService(watcher, libRepo, scanner, log)
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

	// wait a bit more to ensure no extra scans
	time.Sleep(500 * time.Millisecond)
	assert.Len(t, scanner.Scanned(), 1, "should debounce into single scan")
}

func TestWatcherService_AddRemoveLibrary(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &fakeLibraryScanner{}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx := context.Background()
	ws := applib.NewWatcherService(watcher, libRepo, scanner, log)
	err := ws.Start(ctx)
	require.NoError(t, err)
	defer ws.Stop()

	lib := &library.Library{
		ID: "lib-new", Name: "New", Type: "movie",
		Paths: []string{"/media/new"}, Enabled: true, RealtimeMonitoring: true,
	}
	ws.AddLibrary(lib)
	assert.Contains(t, watcher.WatchedPaths(), "/media/new")

	ws.RemoveLibrary(lib)
	assert.NotContains(t, watcher.WatchedPaths(), "/media/new")
}

func TestWatcherService_IgnoresDisabledLibrary(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &fakeLibraryScanner{}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx := context.Background()
	ws := applib.NewWatcherService(watcher, libRepo, scanner, log)
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

	ws := applib.NewWatcherService(watcher, libRepo, scanner, log)
	err := ws.Start(ctx)
	require.NoError(t, err)
	defer ws.Stop()

	watcher.SendEvent(library.FileEvent{
		Path: "/tmp/random/file.mkv",
		Op:   library.FileCreated,
	})

	time.Sleep(3 * time.Second)
	assert.Empty(t, scanner.Scanned(), "unrelated path should not trigger scan")
}
