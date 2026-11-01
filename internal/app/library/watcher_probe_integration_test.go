package library_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/filesystem"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
)

// stubScanner records ScanLibrary calls so the test can prove the probe does -
// or does not - trigger recovery scans.
type stubScanner struct{ calls chan string }

func (s *stubScanner) ScanLibrary(_ context.Context, libraryID string) (int, int, error) {
	select {
	case s.calls <- libraryID:
	default:
	}
	return 0, 0, nil
}

// End-to-end against the REAL fsnotify adapter and the REAL watcher event loop.
//
// Everything else about the probe is tested with a stubbed event delivery,
// which cannot catch the failure that would actually hurt: if the real adapter
// reports a path the probe does not recognise as its own - a symlinked temp
// dir, a trailing separator, a different cleaning - then every probe times out,
// every healthy library is declared dead, and the server falls back to walking
// all of them constantly. That is strictly worse than the behaviour this whole
// change set replaced, so it gets a real-kernel test.
func TestProbe_AgainstRealWatcher_ReportsAlive(t *testing.T) {
	// macOS/Linux temp dirs can be symlinks (/tmp -> /private/tmp); resolve so
	// the comparison exercises the same shape production sees.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}

	fsWatcher, err := filesystem.NewFSWatcher()
	if err != nil {
		t.Skipf("inotify unavailable in this environment: %v", err)
	}

	libs := libfakes.NewFakeLibraryQueryRepo()
	libs.Seed(&library.Library{
		ID: "lib-real", Name: "Real", Type: "movie", Paths: []string{dir},
		Enabled: true, RealtimeMonitoring: true,
	})
	scanner := &stubScanner{calls: make(chan string, 8)}

	ws := applibrary.NewWatcherService(fsWatcher, libs, scanner, slog.Default(),
		applibrary.WatcherWithProbeTimeout(5*time.Second),
	)
	ctx := t.Context()
	if startErr := startSync(ws, ctx); startErr != nil {
		t.Fatalf("watcher start: %v", startErr)
	}
	defer ws.Stop()

	// Let the watch settle before probing.
	time.Sleep(200 * time.Millisecond)

	ws.ProbeRoundForTest(ctx)

	if unhealthy := ws.UnwatchedLibraries(); unhealthy["lib-real"] {
		t.Fatal("a healthy inotify watch must answer the probe - a false 'dead' verdict " +
			"would make the server walk every library constantly")
	}

	// And the probe must not have triggered a recovery scan.
	select {
	case libID := <-scanner.calls:
		t.Fatalf("a passing probe must not trigger a scan, got one for %s", libID)
	case <-time.After(300 * time.Millisecond):
	}

	// No probe file may survive in the user's library.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "evey-watch-probe") {
			t.Fatalf("probe file left behind in a real library root: %s", e.Name())
		}
	}
}
