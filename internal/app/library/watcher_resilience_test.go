package library

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type resScanner struct {
	mu      sync.Mutex
	scanned []string
}

func (s *resScanner) ScanLibrary(_ context.Context, id string) (int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scanned = append(s.scanned, id)
	return 0, 0, nil
}

func (s *resScanner) count(id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, x := range s.scanned {
		if x == id {
			n++
		}
	}
	return n
}

// gatedWatcher blocks Watch on a gate (or fails it) so a test controls how
// long registration takes. It records Unwatch calls.
type gatedWatcher struct {
	*libfakes.FakeFileWatcher
	gate     chan struct{}
	failPath map[string]bool
	errs     chan error

	mu        sync.Mutex
	unwatched []string
}

func newGatedWatcher() *gatedWatcher {
	return &gatedWatcher{
		FakeFileWatcher: libfakes.NewFakeFileWatcher(),
		failPath:        map[string]bool{},
		errs:            make(chan error, 4),
	}
}

func (g *gatedWatcher) Watch(p string) error {
	if g.gate != nil {
		<-g.gate
	}
	if g.failPath[p] {
		return errors.New("no space left on device")
	}
	return g.FakeFileWatcher.Watch(p)
}

func (g *gatedWatcher) Unwatch(p string) error {
	g.mu.Lock()
	g.unwatched = append(g.unwatched, p)
	g.mu.Unlock()
	return g.FakeFileWatcher.Unwatch(p)
}

func (g *gatedWatcher) Errors() <-chan error { return g.errs }

func (g *gatedWatcher) unwatchedPaths() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.unwatched...)
}

// ctxWatcher is a context-aware watcher that hangs until its context ends,
// like a walk stuck on a dead mount.
type ctxWatcher struct{ *gatedWatcher }

func (c ctxWatcher) WatchContext(ctx context.Context, _ string) error {
	<-ctx.Done()
	return ctx.Err()
}

type overflowErr struct{}

func (overflowErr) Error() string  { return "queue overflow" }
func (overflowErr) Overflow() bool { return true }

func resLib(id string, paths ...string) *library.Library {
	return &library.Library{ID: shared.LibraryID(id), Name: id, Paths: paths, Enabled: true, RealtimeMonitoring: true}
}

func newResService(t *testing.T, w library.FileWatcher, libs []*library.Library, opts ...WatcherOption) (*WatcherService, *resScanner) {
	t.Helper()
	repo := libfakes.NewFakeLibraryRepository()
	for _, l := range libs {
		require.NoError(t, repo.Create(context.Background(), l))
	}
	sc := &resScanner{}
	opts = append([]WatcherOption{WatcherWithDebounce(5 * time.Millisecond)}, opts...)
	ws := NewWatcherService(w, repo, sc, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})), opts...)
	return ws, sc
}

func TestStart_DoesNotWaitForTheWalk_AndLibraryIsNotAliveUntilWatched(t *testing.T) {
	w := newGatedWatcher()
	w.gate = make(chan struct{})
	ws, _ := newResService(t, w, []*library.Library{resLib("l1", "/m/a")})

	done := make(chan error, 1)
	go func() { done <- ws.Start(context.Background()) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Start blocked on the watch walk")
	}
	defer ws.Stop()

	assert.Empty(t, ws.AliveWatchedLibraries(), "a library whose walk has not finished must keep the frequent scan cadence")

	close(w.gate)
	ws.WaitInitialWatch()
	assert.True(t, ws.AliveWatchedLibraries()["l1"])
}

func TestWatchLibrary_WalkDoesNotHoldTheLock(t *testing.T) {
	w := newGatedWatcher()
	w.gate = make(chan struct{})
	ws, _ := newResService(t, w, []*library.Library{resLib("l1", "/m/a")})
	require.NoError(t, ws.Start(context.Background()))
	defer ws.Stop()

	got := make(chan struct{})
	go func() {
		ws.AliveWatchedLibraries()
		ws.RemoveLibraryByID("l1")
		close(got)
	}()
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("reads and removal were blocked by a walk in progress")
	}

	// The walk finishes after the removal: its result is stale and must be
	// dropped, not recorded as a live watch of a library that no longer exists.
	close(w.gate)
	ws.WaitInitialWatch()
	assert.Empty(t, ws.AliveWatchedLibraries())
	assert.Contains(t, w.unwatchedPaths(), "/m/a")
}

func TestWatchLibrary_RootExceedingTheWalkTimeoutFailsTheLibrary(t *testing.T) {
	w := ctxWatcher{newGatedWatcher()}
	ws, _ := newResService(t, w, []*library.Library{resLib("l1", "/m/a")}, WatcherWithWalkTimeout(30*time.Millisecond))
	require.NoError(t, ws.Start(context.Background()))
	defer ws.Stop()

	done := make(chan struct{})
	go func() { ws.WaitInitialWatch(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("a hung root was never given up on")
	}
	assert.True(t, ws.UnwatchedLibraries()["l1"])
	assert.Empty(t, ws.AliveWatchedLibraries())
}

func TestWatchLibrary_OneFailedRootMakesTheLibraryUnhealthy(t *testing.T) {
	w := newGatedWatcher()
	w.failPath["/m/b"] = true
	ws, _ := newResService(t, w, []*library.Library{resLib("l1", "/m/a", "/m/b")})
	require.NoError(t, startForTest(ws))
	defer ws.Stop()

	assert.True(t, ws.UnwatchedLibraries()["l1"], "a library with a blind root must keep the frequent scan")
	assert.Empty(t, ws.AliveWatchedLibraries())
}

func startForTest(ws *WatcherService) error {
	if err := ws.Start(context.Background()); err != nil {
		return err
	}
	ws.WaitInitialWatch()
	return nil
}

func TestEventQueueOverflow_RescansTheWatchedLibraries(t *testing.T) {
	w := newGatedWatcher()
	ws, sc := newResService(t, w, []*library.Library{resLib("l1", "/m/a"), resLib("l2", "/m/b")})
	require.NoError(t, startForTest(ws))
	defer ws.Stop()

	w.errs <- overflowErr{}

	require.Eventually(t, func() bool { return sc.count("l1") == 1 && sc.count("l2") == 1 }, 2*time.Second, 5*time.Millisecond)
}

func TestEventQueueOverflow_PlainErrorsDoNotRescan(t *testing.T) {
	w := newGatedWatcher()
	ws, sc := newResService(t, w, []*library.Library{resLib("l1", "/m/a")})
	require.NoError(t, startForTest(ws))
	defer ws.Stop()

	w.errs <- errors.New("something else")
	time.Sleep(60 * time.Millisecond)
	assert.Zero(t, sc.count("l1"))
}

func TestProbeRound_EveryRootIsProbed(t *testing.T) {
	good, bad := t.TempDir(), t.TempDir()
	ws := &WatcherService{
		log:       slog.Default(),
		probes:    newProbeState(),
		pathToLib: map[string]string{good: "lib-two", bad: "lib-two"},
		probeWait: 150 * time.Millisecond,
	}
	// Only the first root answers; the second stays deaf.
	go func() {
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			entries, _ := os.ReadDir(good)
			for _, e := range entries {
				if library.IsProbeFile(e.Name()) {
					ws.probes.deliver(filepath.Join(good, e.Name()))
					return
				}
			}
			time.Sleep(time.Millisecond)
		}
	}()
	ws.probeRound(context.Background())

	assert.True(t, ws.probes.UnhealthyLibraries()["lib-two"], "one deaf root must make the library unhealthy")
}

func TestProbeRound_NetworkRootAnywhereIsNotVouchedFor(t *testing.T) {
	local, remote := t.TempDir(), t.TempDir()
	ws := &WatcherService{
		log:       slog.Default(),
		probes:    newProbeState(),
		pathToLib: map[string]string{local: "lib-mix", remote: "lib-mix"},
		probeWait: 20 * time.Millisecond,
		networkFS: func(p string) (string, bool) { return "nfs", p == remote },
	}
	ws.probeRound(context.Background())
	assert.True(t, ws.probes.UnhealthyLibraries()["lib-mix"])
}

func TestProbeState_BlindNoticeIsOncePerLibraryUntilStateChanges(t *testing.T) {
	p := newProbeState()
	assert.True(t, p.noteBlind("l1"))
	assert.False(t, p.noteBlind("l1"))
	assert.True(t, p.noteBlind("l2"))
	p.clearBlind("l1")
	assert.True(t, p.noteBlind("l1"), "a state change re-arms the notice")
}

func TestProbeRound_UnwritableRootWarnsOnlyOnce(t *testing.T) {
	var buf lockedBuf
	ws := &WatcherService{
		log:       slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})),
		probes:    newProbeState(),
		pathToLib: map[string]string{filepath.Join(t.TempDir(), "missing"): "lib-ro"},
		probeWait: 20 * time.Millisecond,
	}
	ws.probeRound(context.Background())
	ws.probeRound(context.Background())
	ws.probeRound(context.Background())
	assert.Equal(t, 1, countOccurrences(buf.String(), "cannot write a probe file"))
}

type lockedBuf struct {
	mu sync.Mutex
	b  []byte
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.b = append(l.b, p...)
	return len(p), nil
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return string(l.b)
}

func countOccurrences(s, sub string) int {
	n := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			n++
		}
	}
	return n
}
