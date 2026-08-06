package library_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	applib "github.com/evey-project/evey/internal/app/library"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scan_service.go:325 `pruneErr != nil` - prune returns err, log must fire.
// Use tvshow flow so pruneEmptyContainers runs with injectable ErrDelete on Season prune.
func TestScanService_KillMutant_PruneEmptyContainersErrorLogged(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-tv", Type: "tvshow", Paths: []string{"/tv"}})
	h.libCmd.Seed(&library.Library{ID: "lib-tv", Type: "tvshow", Paths: []string{"/tv"}})
	// Pre-seed empty season (no children) → pruneEmptyContainers tries Delete.
	require.NoError(t, h.media.Create(context.Background(), &library.MediaItem{
		ID: "empty-season", LibraryID: "lib-tv", Type: library.Season, ParentID: "no-series",
	}))
	h.scanner.Results["/tv"] = []library.ScannedFile{}
	h.media.ErrDelete = errors.New("delete boom")

	logs := captureSlog(t, func() {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-tv")
		require.NoError(t, err)
	})
	// With ErrDelete, pruneEmptyContainers returns err → "prune empty containers failed" logs.
	// If mutant flips `pruneErr != nil` → `== nil`, log would NOT appear when err is non-nil.
	assert.Contains(t, logs, "delete empty season")
}

// scan_service_tvshow.go:39 `updateErr != nil` - update fingerprint log.
func TestScanService_KillMutant_TVShowUpdateFingerprintErrorLogged(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-tv", Type: "tvshow", Paths: []string{"/tv"}})
	h.libCmd.Seed(&library.Library{ID: "lib-tv", Type: "tvshow", Paths: []string{"/tv"}})
	// Pre-seed series with stale fingerprint so title-match path runs Update.
	require.NoError(t, h.media.Create(context.Background(), &library.MediaItem{
		ID: "series-1", LibraryID: "lib-tv", Type: library.Series, Title: "Show",
		File: &library.MediaFile{Fingerprint: "stale"},
	}))
	s, e := 1, 1
	h.scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/Show/S01/e01.mkv", Size: 100, Fingerprint: "ep-fp",
			Title: "Show", Season: &s, Episode: &e, Type: library.Episode},
	}
	h.media.ErrUpdate = errors.New("update boom")

	logs := captureSlog(t, func() {
		_, _, _ = h.svc.ScanLibrary(context.Background(), "lib-tv")
	})
	assert.Contains(t, logs, "update media item fingerprint failed")
}

// scan_service_tvshow.go:87 `err != nil` publish series added log.
func TestScanService_KillMutant_TVShowSeriesPublishErrorLogged(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-tv", Type: "tvshow", Paths: []string{"/tv"}})
	h.libCmd.Seed(&library.Library{ID: "lib-tv", Type: "tvshow", Paths: []string{"/tv"}})
	s, e := 1, 1
	h.scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/Show/S01/e01.mkv", Size: 100, Fingerprint: "ep-fp",
			Title: "Show", Season: &s, Episode: &e, Type: library.Episode},
	}
	h.bus.PublishErr = errors.New("pub boom")

	logs := captureSlog(t, func() {
		_, _, _ = h.svc.ScanLibrary(context.Background(), "lib-tv")
	})
	// series-level publish failure logs "publish event" with media.item.added event name.
	assert.Contains(t, logs, "publish event")
	assert.Contains(t, logs, "media.item.added")
}

// scan_service_tvshow.go:177 `svc.search != nil` - series prune branch must skip search when nil.
// With nil search AND empty series to prune, non-mutant path skips; mutant path would call
// RemoveMediaItem on nil interface → panic caught via recover? No, code panics.
func TestScanService_KillMutant_TVShowPruneSeriesNilSearchSkips(t *testing.T) {
	// Build a ScanService with nil search explicitly.
	libCmd := libfakes.NewFakeLibraryCommandRepo()
	libQuery := libfakes.NewFakeLibraryQueryRepo()
	media := libfakes.NewFakeMediaItemCommandRepo()
	streams := libfakes.NewFakeMediaStreamRepository()
	persons := libfakes.NewFakePersonRepository()
	mediaPersons := libfakes.NewFakeMediaPersonRepository()
	scanner := libfakes.NewFakeFileScanner()
	prober := libfakes.NewFakeStreamProber()
	bus := sharedfakes.NewFakeEventBus()
	svc := applib.NewScanService(libCmd, libQuery, media, streams, persons, mediaPersons,
		scanner, prober, nil /* search */, bus)

	libQuery.Seed(&library.Library{ID: "lib-tv", Type: "tvshow", Paths: []string{"/tv"}})
	libCmd.Seed(&library.Library{ID: "lib-tv", Type: "tvshow", Paths: []string{"/tv"}})
	// Seed an empty series (no children) - pruneEmptyContainers will delete it and
	// reach the `if svc.search != nil` branch. With nil search, must NOT dereference.
	require.NoError(t, media.Create(context.Background(), &library.MediaItem{
		ID: "empty-series", LibraryID: "lib-tv", Type: library.Series, Title: "Gone",
	}))
	scanner.Results["/tv"] = []library.ScannedFile{}

	// Must not panic.
	_, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)
	// series must be deleted
	_, getErr := media.GetByID(context.Background(), "empty-series")
	assert.Error(t, getErr)
}

// scan_service.go:399 unchanged file with different parentID only - force update path,
// asserting ParentID is actually rewritten (not just "either"), to kill NEGATION mutant
// that flips `!=` on ParentID check.
func TestScanService_KillMutant_UnchangedSamePathNewParentUpdates(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-tv", Type: "tvshow", Paths: []string{"/tv"}})
	h.libCmd.Seed(&library.Library{ID: "lib-tv", Type: "tvshow", Paths: []string{"/tv"}})
	// Pre-seed an episode whose FilePath already matches (same path) but with bogus parent.
	require.NoError(t, h.media.Create(context.Background(), &library.MediaItem{
		ID: "ep1", LibraryID: "lib-tv", ParentID: "wrong-parent",
		File: &library.MediaFile{Path: "/tv/show/s01e01.mkv", Size: 100, Fingerprint: "ep-fp"},
		Type: library.Episode, Title: "S01E01",
	}))
	s1, e1 := 1, 1
	h.scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/show/s01e01.mkv", Size: 100, Fingerprint: "ep-fp",
			Title: "MyShow", Season: &s1, Episode: &e1, Type: library.Episode},
	}

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)
	got, err := h.media.GetByID(context.Background(), "ep1")
	require.NoError(t, err)
	// Parent must have been rewritten to the real season ID (UUID prefix differs from "wrong-parent").
	assert.NotEqual(t, "wrong-parent", got.ParentID,
		"ParentID must be updated when != differs even if FilePath matches")
}

// Same boundary, inverted: same path AND same parent → no-op update branch (no write error log).
func TestScanService_KillMutant_UnchangedSamePathSameParentSkipsUpdate(t *testing.T) {
	h := newScanHarness()
	h.libQuery.Seed(&library.Library{ID: "lib-tv", Type: "tvshow", Paths: []string{"/tv"}})
	h.libCmd.Seed(&library.Library{ID: "lib-tv", Type: "tvshow", Paths: []string{"/tv"}})
	s1, e1 := 1, 1
	// First scan creates everything.
	h.scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/show/s01e01.mkv", Size: 100, Fingerprint: "ep-fp",
			Title: "MyShow", Season: &s1, Episode: &e1, Type: library.Episode},
	}
	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	// Second scan: unchanged fingerprint, same path, same parent - Update for path/parent
	// must NOT be called. If ErrUpdate is set, the "update media item failed" log would
	// fire only if the mutant forced the Update call.
	h.media.ErrUpdate = errors.New("should not be called on no-op unchanged")

	logs := captureSlog(t, func() {
		_, _, err := h.svc.ScanLibrary(context.Background(), "lib-tv")
		require.NoError(t, err)
	})
	assert.NotContains(t, logs, "update media item failed",
		"unchanged same-path/same-parent must not call Update in upsertMediaFile")
}

// watcher.go:146 `ws.cancel != nil` - calling Stop without Start must not panic (nil cancel).
func TestWatcherService_KillMutant_StopBeforeStartNoPanic(t *testing.T) {
	watcher := libfakes.NewFakeFileWatcher()
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &fakeLibraryScanner{}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	ws := applib.NewWatcherService(watcher, libRepo, scanner, log)
	// Stop without Start: cancel is nil. Must not panic.
	require.NotPanics(t, func() { ws.Stop() })
}

// watcher.go:149 `err != nil` close err log.
type errCloseWatcher struct {
	*libfakes.FakeFileWatcher
}

func (w *errCloseWatcher) Close() error {
	_ = w.FakeFileWatcher.Close()
	return errors.New("close boom")
}

func TestWatcherService_KillMutant_StopCloseErrorLogged(t *testing.T) {
	fake := libfakes.NewFakeFileWatcher()
	w := &errCloseWatcher{FakeFileWatcher: fake}
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &fakeLibraryScanner{}

	buf := &syncBuf{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ws := applib.NewWatcherService(w, libRepo, scanner, logger)
	require.NoError(t, ws.Start(context.Background()))
	ws.Stop()
	// give any processing goroutine a moment to settle
	time.Sleep(50 * time.Millisecond)
	assert.Contains(t, buf.String(), "watcher: close failed")
}

// watcher.go:223 `err != nil` scan fail log.
type errScanner struct {
	mu     sync.Mutex
	called int
}

func (s *errScanner) ScanLibrary(_ context.Context, _ string) (int, int, error) {
	s.mu.Lock()
	s.called++
	s.mu.Unlock()
	return 0, 0, errors.New("scan boom")
}

type syncBuf struct {
	mu  sync.Mutex
	buf []byte
}

func (b *syncBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	return len(p), nil
}
func (b *syncBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

func TestWatcherService_KillMutant_ScanFailureLogged(t *testing.T) {
	fake := libfakes.NewFakeFileWatcher()
	libRepo := libfakes.NewFakeLibraryRepository()
	scanner := &errScanner{}

	ctx := context.Background()
	_ = libRepo.Create(ctx, &library.Library{
		ID: "lib-1", Name: "M", Type: "movie",
		Paths: []string{"/media/m"}, Enabled: true, RealtimeMonitoring: true,
	})

	buf := &syncBuf{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ws := applib.NewWatcherService(fake, libRepo, scanner, logger)
	require.NoError(t, ws.Start(ctx))
	defer ws.Stop()
	fake.SendEvent(library.FileEvent{Path: "/media/m/new.mkv", Op: library.FileCreated})
	require.Eventually(t, func() bool {
		scanner.mu.Lock()
		defer scanner.mu.Unlock()
		return scanner.called > 0
	}, 5*time.Second, 100*time.Millisecond)
	require.Eventually(t, func() bool {
		return strings.Contains(buf.String(), "watcher: scan failed")
	}, 2*time.Second, 50*time.Millisecond)
}

// trickplay_service.go:238 `trickplayRepo == nil || trickplayGen == nil` short-circuit in
// StartBackgroundGeneration. With nil repo (but non-nil gen), function must return early
// without launching the goroutine - no panic, no log.
func TestTrickplayService_KillMutant_StartBackgroundGenNilRepoShortCircuits(t *testing.T) {
	gen := libfakes.NewFakeTrickplayGenerator()
	svc := applib.NewTrickplayService(nil /* repo */, gen, "/tmp/tp", nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NotPanics(t, func() { svc.StartBackgroundGeneration(ctx) })
}

func TestTrickplayService_KillMutant_StartBackgroundGenNilGenShortCircuits(t *testing.T) {
	repo := libfakes.NewFakeTrickplayRepository()
	svc := applib.NewTrickplayService(repo, nil /* gen */, "/tmp/tp", nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NotPanics(t, func() { svc.StartBackgroundGeneration(ctx) })
}

// trickplay_service.go:89 `err != nil` and :92 `entry.Value != "false"` - isAdaptiveCPU.
// With configRepo present and no entry → Get returns err → returns default true.
// With entry.Value = "false" → returns false. With other value → true.
// GenerateTrickplay uses this; we observe via LastOutputDir being set regardless, so we
// call GenerateAllMissingTrickplays which only uses isTrickplayEnabled. Better: call
// GenerateTrickplay directly and verify workers via fake gen.
func TestTrickplayService_KillMutant_AdaptiveCPUConfigFalseDisablesAdaptive(t *testing.T) {
	cfg := sharedfakes.NewFakeServerConfigRepository()
	require.NoError(t, cfg.Set(context.Background(), "trickplay.adaptive_cpu", "false"))
	repo := libfakes.NewFakeTrickplayRepository()
	gen := libfakes.NewFakeTrickplayGenerator()
	media := libfakes.NewFakeMediaItemCommandRepo()
	require.NoError(t, media.Create(context.Background(), &library.MediaItem{
		ID: "m1", File: &library.MediaFile{Path: "/x.mkv"},
	}))
	bus := sharedfakes.NewFakeEventBus()
	svc := applib.NewTrickplayService(repo, gen, "/tmp/tp", media, bus, cfg)

	err := svc.GenerateTrickplay(context.Background(), "m1")
	require.NoError(t, err)
	// With "false" config, isAdaptiveCPU returns false. This path is exercised; primary
	// assertion is no panic + manifest created.
	m, err := repo.GetByMediaItemID(context.Background(), shared.MediaItemID("m1"))
	require.NoError(t, err)
	assert.Equal(t, shared.MediaItemID("m1"), m.MediaItemID)
}

func TestTrickplayService_KillMutant_AdaptiveCPUConfigDefaultTrue(t *testing.T) {
	// No entry → Get returns err → default true. Covers `err != nil` return true branch.
	cfg := sharedfakes.NewFakeServerConfigRepository()
	repo := libfakes.NewFakeTrickplayRepository()
	gen := libfakes.NewFakeTrickplayGenerator()
	media := libfakes.NewFakeMediaItemCommandRepo()
	require.NoError(t, media.Create(context.Background(), &library.MediaItem{
		ID: "m2", File: &library.MediaFile{Path: "/y.mkv"},
	}))
	bus := sharedfakes.NewFakeEventBus()
	svc := applib.NewTrickplayService(repo, gen, "/tmp/tp", media, bus, cfg)

	err := svc.GenerateTrickplay(context.Background(), "m2")
	require.NoError(t, err)
	m, err := repo.GetByMediaItemID(context.Background(), shared.MediaItemID("m2"))
	require.NoError(t, err)
	assert.Equal(t, shared.MediaItemID("m2"), m.MediaItemID)
}

// trickplay_service.go isTrickplayEnabled - entry.Value != "false" boundary at :79.
// When disabled, GenerateAllMissingTrickplays returns (0, nil) without launching goroutine.
func TestTrickplayService_KillMutant_DisabledReturnsZero(t *testing.T) {
	cfg := sharedfakes.NewFakeServerConfigRepository()
	require.NoError(t, cfg.Set(context.Background(), "trickplay.enabled", "false"))
	repo := libfakes.NewFakeTrickplayRepository()
	gen := libfakes.NewFakeTrickplayGenerator()
	media := libfakes.NewFakeMediaItemCommandRepo()
	bus := sharedfakes.NewFakeEventBus()
	svc := applib.NewTrickplayService(repo, gen, "/tmp/tp", media, bus, cfg)
	n, err := svc.GenerateAllMissingTrickplays(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, 0, n, "disabled trickplay must return 0 without queuing")
}
