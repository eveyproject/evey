package library_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
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

// missingIDsRepo wraps FakeTrickplayRepository to expose injectable missing IDs
// and an optional error from ListMissingMediaItemIDs.
type missingIDsRepo struct {
	*libfakes.FakeTrickplayRepository
	missing []string
	listErr error
	// lastMinAlgoVersion records what the sweep asked for, pinning the
	// boot-vs-admin contract (0 = missing-only, current version = outdated too).
	lastMinAlgoVersion int
}

func (r *missingIDsRepo) ListMissingMediaItemIDs(_ context.Context, minAlgoVersion int) ([]string, error) {
	r.lastMinAlgoVersion = minAlgoVersion
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.missing, nil
}

func newMissingIDsRepo(missing []string) *missingIDsRepo {
	return &missingIDsRepo{
		FakeTrickplayRepository: libfakes.NewFakeTrickplayRepository(),
		missing:                 missing,
	}
}

// countingGen counts how many times Generate is called and which file paths.
type countingGen struct {
	mu        sync.Mutex
	callCount int32
	calls     []string
	failOn    map[string]error
	delay     time.Duration
}

func (g *countingGen) Generate(_ context.Context, filePath, outputDir string, _, _, _, _, _ int, _ int64) (*library.TrickplayResult, error) {
	atomic.AddInt32(&g.callCount, 1)
	g.mu.Lock()
	g.calls = append(g.calls, filePath)
	g.mu.Unlock()
	if g.delay > 0 {
		time.Sleep(g.delay)
	}
	if err, ok := g.failOn[filePath]; ok {
		return nil, err
	}
	return &library.TrickplayResult{ImageDir: outputDir, ImageCount: 1}, nil
}

func (g *countingGen) Calls() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	cp := make([]string, len(g.calls))
	copy(cp, g.calls)
	return cp
}

// --- GetTrickplayManifest ---

func TestTrickplay_GetTrickplayManifest_NotConfiguredReturnsError(t *testing.T) {
	t.Parallel()
	gen := libfakes.NewFakeTrickplayGenerator()
	svc := applib.NewTrickplayService(nil /* repo */, gen, "/cache", nil, nil, nil)

	m, err := svc.GetTrickplayManifest(context.Background(), "media-1")
	require.Error(t, err)
	assert.Nil(t, m)
	assert.Contains(t, err.Error(), "not configured")
}

func TestTrickplay_GetTrickplayManifest_ReturnsExisting(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeTrickplayRepository()
	manifest := &library.TrickplayManifest{
		ID:          "tp-1",
		MediaItemID: shared.MediaItemID("media-1"),
		Interval:    15,
		Width:       320,
		ImageCount:  4,
		ImagePath:   "/cache/media-1",
	}
	require.NoError(t, repo.Create(context.Background(), manifest))

	gen := libfakes.NewFakeTrickplayGenerator()
	svc := applib.NewTrickplayService(repo, gen, "/cache", nil, nil, nil)

	got, err := svc.GetTrickplayManifest(context.Background(), "media-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, shared.MediaItemID("media-1"), got.MediaItemID)
	assert.Equal(t, 4, got.ImageCount)
}

func TestTrickplay_GetTrickplayManifest_PropagatesRepoError(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeTrickplayRepository()
	repo.Err = errors.New("db boom")
	gen := libfakes.NewFakeTrickplayGenerator()
	svc := applib.NewTrickplayService(repo, gen, "/cache", nil, nil, nil)

	m, err := svc.GetTrickplayManifest(context.Background(), "media-1")
	require.Error(t, err)
	assert.Nil(t, m)
	assert.Contains(t, err.Error(), "db boom")
}

// --- GenerateAllMissingTrickplays ---

func TestTrickplay_GenerateAllMissingTrickplays_NotConfiguredReturnsError(t *testing.T) {
	t.Parallel()
	svc := applib.NewTrickplayService(nil, nil, "/cache", nil, nil, nil)

	n, err := svc.GenerateAllMissingTrickplays(context.Background(), false)
	require.Error(t, err)
	assert.Equal(t, 0, n)
	assert.Contains(t, err.Error(), "not configured")
}

func TestTrickplay_GenerateAllMissingTrickplays_EmptyListNoop(t *testing.T) {
	t.Parallel()
	repo := newMissingIDsRepo(nil)
	gen := &countingGen{}
	media := libfakes.NewFakeMediaItemCommandRepo()
	bus := sharedfakes.NewFakeEventBus()
	svc := applib.NewTrickplayService(repo, gen, "/cache", media, bus, nil)

	n, err := svc.GenerateAllMissingTrickplays(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	// Give async goroutine a moment to confirm no work happens
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&gen.callCount))
}

func TestTrickplay_GenerateAllMissingTrickplays_ListErrorPropagated(t *testing.T) {
	t.Parallel()
	repo := newMissingIDsRepo(nil)
	repo.listErr = errors.New("list boom")
	gen := &countingGen{}
	media := libfakes.NewFakeMediaItemCommandRepo()
	bus := sharedfakes.NewFakeEventBus()
	svc := applib.NewTrickplayService(repo, gen, "/cache", media, bus, nil)

	n, err := svc.GenerateAllMissingTrickplays(context.Background(), false)
	require.Error(t, err)
	assert.Equal(t, 0, n)
	assert.Contains(t, err.Error(), "list missing trickplays")
}

func TestTrickplay_GenerateAllMissingTrickplays_GeneratesForMissingItems(t *testing.T) {
	t.Parallel()
	// Two missing items; generator should be invoked for both.
	repo := newMissingIDsRepo([]string{"m1", "m2"})
	gen := &countingGen{}
	media := libfakes.NewFakeMediaItemCommandRepo()
	require.NoError(t, media.Create(context.Background(), &library.MediaItem{
		ID: "m1", File: &library.MediaFile{Path: "/movies/a.mkv"}, Title: "A",
	}))
	require.NoError(t, media.Create(context.Background(), &library.MediaItem{
		ID: "m2", File: &library.MediaFile{Path: "/movies/b.mkv"}, Title: "B",
	}))
	bus := sharedfakes.NewFakeEventBus()
	// Use a shutdown ctx so the inner throttle (10s sleep) is cut on cancel.
	ctx, cancel := context.WithCancel(context.Background())
	svc := applib.NewTrickplayService(repo, gen, "/cache", media, bus, nil)
	svc.WithShutdownCtx(ctx)

	n, err := svc.GenerateAllMissingTrickplays(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	// Wait for first generation, then cancel to skip the 10s throttle and stop loop.
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&gen.callCount) >= 1
	}, 3*time.Second, 20*time.Millisecond, "first generate must run")
	cancel()
	// Allow goroutine to exit
	time.Sleep(50 * time.Millisecond)
	calls := gen.Calls()
	assert.NotEmpty(t, calls, "at least one item generated")
}

func TestTrickplay_GenerateAllMissingTrickplays_ContinuesAfterItemError(t *testing.T) {
	t.Parallel()
	// First item fails generation; loop must continue (we observe by cancelling
	// after callCount reaches >=1 and asserting no error returned upfront).
	repo := newMissingIDsRepo([]string{"m1", "m2"})
	gen := &countingGen{
		failOn: map[string]error{
			"/movies/a.mkv": errors.New("ffmpeg crash"),
		},
	}
	media := libfakes.NewFakeMediaItemCommandRepo()
	require.NoError(t, media.Create(context.Background(), &library.MediaItem{
		ID: "m1", File: &library.MediaFile{Path: "/movies/a.mkv"}, Title: "A",
	}))
	require.NoError(t, media.Create(context.Background(), &library.MediaItem{
		ID: "m2", File: &library.MediaFile{Path: "/movies/b.mkv"}, Title: "B",
	}))
	bus := sharedfakes.NewFakeEventBus()
	ctx, cancel := context.WithCancel(context.Background())
	svc := applib.NewTrickplayService(repo, gen, "/cache", media, bus, nil)
	svc.WithShutdownCtx(ctx)

	n, err := svc.GenerateAllMissingTrickplays(context.Background(), false)
	require.NoError(t, err, "GenerateAllMissingTrickplays returns synchronously without surfacing per-item errors")
	assert.Equal(t, 2, n)

	// First call happens on m1 (failing); without cancel the loop sleeps 10s before m2.
	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&gen.callCount) >= 1
	}, 3*time.Second, 20*time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)
}

// --- GenerateTrickplay failure persistence ---

func TestTrickplay_GenerateTrickplay_PermanentFailureIsRecorded(t *testing.T) {
	t.Parallel()
	// The file is present on disk and the generator still failed (the prod
	// 2026-07-26 case: ffprobe exit 1 on the same two files at every boot).
	// The failure must be recorded against the file's size+mod time so the
	// missing-list stops re-serving it.
	f, err := os.CreateTemp(t.TempDir(), "*.mkv")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	repo := newMissingIDsRepo(nil)
	gen := &countingGen{failOn: map[string]error{f.Name(): errors.New("ffprobe: exit status 1")}}
	media := libfakes.NewFakeMediaItemCommandRepo()
	mod := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	require.NoError(t, media.Create(context.Background(), &library.MediaItem{
		ID: "m1", File: &library.MediaFile{Path: f.Name(), Size: 12345, ModTime: mod}, Title: "A",
	}))
	svc := applib.NewTrickplayService(repo, gen, t.TempDir(), media, sharedfakes.NewFakeEventBus(), nil)

	require.Error(t, svc.GenerateTrickplay(context.Background(), "m1"))
	failures := repo.Failures()
	require.Len(t, failures, 1, "a permanent generation failure must be recorded")
	assert.Equal(t, shared.MediaItemID("m1"), failures[0].MediaItemID)
	assert.Equal(t, int64(12345), failures[0].FileSize)
	assert.Equal(t, mod, failures[0].ModTime)
	assert.Contains(t, failures[0].Reason, "ffprobe")
}

func TestTrickplay_GenerateTrickplay_MissingFileFailureNotRecorded(t *testing.T) {
	t.Parallel()
	// The file is gone (unmounted share): transient, the next sweep may find it
	// healed. Recording now would freeze a failure the bytes never caused.
	repo := newMissingIDsRepo(nil)
	gen := &countingGen{failOn: map[string]error{"/gone/a.mkv": errors.New("no such file")}}
	media := libfakes.NewFakeMediaItemCommandRepo()
	require.NoError(t, media.Create(context.Background(), &library.MediaItem{
		ID: "m1", File: &library.MediaFile{Path: "/gone/a.mkv", Size: 1}, Title: "A",
	}))
	svc := applib.NewTrickplayService(repo, gen, t.TempDir(), media, sharedfakes.NewFakeEventBus(), nil)

	require.Error(t, svc.GenerateTrickplay(context.Background(), "m1"))
	assert.Empty(t, repo.Failures(), "missing file ⇒ transient, nothing recorded")
}

func TestTrickplay_GenerateTrickplay_StoppedSweepNotRecorded(t *testing.T) {
	t.Parallel()
	// An admin's Stop cancels the item's context and kills the ffmpeg: the
	// generator then reports "signal: killed", a plain error that does not
	// wrap context.Canceled. The context says what happened; the file must not
	// be blacklisted for it.
	f, err := os.CreateTemp(t.TempDir(), "*.mkv")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	repo := newMissingIDsRepo(nil)
	gen := &countingGen{failOn: map[string]error{f.Name(): errors.New("ffmpeg trickplay single-pass: signal: killed")}}
	media := libfakes.NewFakeMediaItemCommandRepo()
	require.NoError(t, media.Create(context.Background(), &library.MediaItem{
		ID: "m1", File: &library.MediaFile{Path: f.Name(), Size: 1}, Title: "A",
	}))
	svc := applib.NewTrickplayService(repo, gen, t.TempDir(), media, sharedfakes.NewFakeEventBus(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Error(t, svc.GenerateTrickplay(ctx, "m1"))
	assert.Empty(t, repo.Failures(), "a stopped sweep must not record a permanent failure")
}

func TestTrickplay_GenerateTrickplay_ContextErrorNotRecorded(t *testing.T) {
	t.Parallel()
	// A cancelled/timed-out generation says nothing about the file: a shutdown
	// mid-batch or a 30-min budget blown by a busy NAS must not permanently
	// blacklist a healthy item.
	f, err := os.CreateTemp(t.TempDir(), "*.mkv")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	repo := newMissingIDsRepo(nil)
	gen := &countingGen{failOn: map[string]error{f.Name(): fmt.Errorf("ffmpeg: %w", context.Canceled)}}
	media := libfakes.NewFakeMediaItemCommandRepo()
	require.NoError(t, media.Create(context.Background(), &library.MediaItem{
		ID: "m1", File: &library.MediaFile{Path: f.Name(), Size: 1}, Title: "A",
	}))
	svc := applib.NewTrickplayService(repo, gen, t.TempDir(), media, sharedfakes.NewFakeEventBus(), nil)

	require.Error(t, svc.GenerateTrickplay(context.Background(), "m1"))
	assert.Empty(t, repo.Failures(), "a context error is never a verdict on the file")
}

// --- GenerateTrickplay concurrency ---

func TestTrickplay_GenerateTrickplay_DedupsConcurrent(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeTrickplayRepository()
	// delay keeps the first generation "running" (holding the per-media claim)
	// long enough for the second concurrent call to observe it.
	gen := &countingGen{delay: 300 * time.Millisecond}
	media := libfakes.NewFakeMediaItemCommandRepo()
	require.NoError(t, media.Create(context.Background(), &library.MediaItem{
		ID: "m1", File: &library.MediaFile{Path: "/movies/a.mkv"}, Title: "A",
	}))
	bus := sharedfakes.NewFakeEventBus()
	svc := applib.NewTrickplayService(repo, gen, t.TempDir(), media, bus, nil)

	// A holds the generation claim through its delayed Generate.
	doneA := make(chan struct{})
	go func() { _ = svc.GenerateTrickplay(context.Background(), "m1"); close(doneA) }()

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&gen.callCount) >= 1
	}, 2*time.Second, 10*time.Millisecond, "first generation must start")

	// B: a concurrent request for the same item must be skipped - two ffmpeg
	// runs writing the same sprite dir would corrupt each other's output.
	_ = svc.GenerateTrickplay(context.Background(), "m1")
	assert.Equal(t, int32(1), atomic.LoadInt32(&gen.callCount), "concurrent generation should be deduped")

	<-doneA
	// After the first finishes, the claim is released: a later call generates again.
	_ = svc.GenerateTrickplay(context.Background(), "m1")
	assert.Equal(t, int32(2), atomic.LoadInt32(&gen.callCount), "after release a new generation runs")
}

// --- StartTrickplayWorker ---

func TestTrickplay_StartTrickplayWorker_NilRepoShortCircuits(t *testing.T) {
	t.Parallel()
	gen := libfakes.NewFakeTrickplayGenerator()
	bus := sharedfakes.NewFakeEventBus()
	svc := applib.NewTrickplayService(nil, gen, "/cache", nil, bus, nil)

	require.NotPanics(t, func() {
		svc.StartTrickplayWorker(context.Background())
	})
	// No subscription should have been registered (no handler list to verify, but
	// publishing must not crash).
	require.NoError(t, bus.Publish(context.Background(), library.MediaItemAdded{
		MediaItemID: shared.MediaItemID("m1"), Type: library.MovieItem,
	}))
}

func TestTrickplay_StartTrickplayWorker_NilGenShortCircuits(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeTrickplayRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := applib.NewTrickplayService(repo, nil, "/cache", nil, bus, nil)

	require.NotPanics(t, func() {
		svc.StartTrickplayWorker(context.Background())
	})
	require.NoError(t, bus.Publish(context.Background(), library.MediaItemAdded{
		MediaItemID: shared.MediaItemID("m1"), Type: library.MovieItem,
	}))
}

func TestTrickplay_StartTrickplayWorker_ConsumesAddedMovieEvent(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeTrickplayRepository()
	gen := &countingGen{}
	media := libfakes.NewFakeMediaItemCommandRepo()
	require.NoError(t, media.Create(context.Background(), &library.MediaItem{
		ID: "m1", File: &library.MediaFile{Path: "/movies/a.mkv"}, Title: "A",
	}))
	bus := sharedfakes.NewFakeEventBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc := applib.NewTrickplayService(repo, gen, "/cache", media, bus, nil)
	svc.StartTrickplayWorker(ctx)

	// Publish a movie-added event; worker enqueues and runs in a goroutine.
	require.NoError(t, bus.Publish(context.Background(), library.MediaItemAdded{
		MediaItemID: shared.MediaItemID("m1"),
		Type:        library.MovieItem,
	}))

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&gen.callCount) == 1
	}, 3*time.Second, 20*time.Millisecond, "worker must invoke generator for movie")
}

func TestTrickplay_StartTrickplayWorker_IgnoresNonPlayableTypes(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeTrickplayRepository()
	gen := &countingGen{}
	media := libfakes.NewFakeMediaItemCommandRepo()
	bus := sharedfakes.NewFakeEventBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc := applib.NewTrickplayService(repo, gen, "/cache", media, bus, nil)
	svc.StartTrickplayWorker(ctx)

	// Series / season / track should be skipped.
	for _, ty := range []library.MediaItemType{library.Series, library.Season, library.Track, library.PhotoItem} {
		require.NoError(t, bus.Publish(context.Background(), library.MediaItemAdded{
			MediaItemID: shared.MediaItemID("m1"), Type: ty,
		}))
	}
	// Give workers a moment; none should fire.
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&gen.callCount))
}

func TestTrickplay_StartTrickplayWorker_SkipsWhenManifestExists(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeTrickplayRepository()
	// Pre-existing manifest → worker GetTrickplayManifest returns nil error → early return.
	require.NoError(t, repo.Create(context.Background(), &library.TrickplayManifest{
		ID: "tp-existing", MediaItemID: shared.MediaItemID("m1"),
	}))
	gen := &countingGen{}
	media := libfakes.NewFakeMediaItemCommandRepo()
	require.NoError(t, media.Create(context.Background(), &library.MediaItem{
		ID: "m1", File: &library.MediaFile{Path: "/movies/a.mkv"}, Title: "A",
	}))
	bus := sharedfakes.NewFakeEventBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc := applib.NewTrickplayService(repo, gen, "/cache", media, bus, nil)
	svc.StartTrickplayWorker(ctx)

	require.NoError(t, bus.Publish(context.Background(), library.MediaItemAdded{
		MediaItemID: shared.MediaItemID("m1"),
		Type:        library.MovieItem,
	}))
	// Wait a bit; generator must NOT be called.
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&gen.callCount))
}

func TestTrickplay_StartTrickplayWorker_DisabledByConfigSkips(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeTrickplayRepository()
	gen := &countingGen{}
	media := libfakes.NewFakeMediaItemCommandRepo()
	require.NoError(t, media.Create(context.Background(), &library.MediaItem{
		ID: "m1", File: &library.MediaFile{Path: "/movies/a.mkv"},
	}))
	bus := sharedfakes.NewFakeEventBus()
	cfg := sharedfakes.NewFakeServerConfigRepository()
	require.NoError(t, cfg.Set(context.Background(), "trickplay.enabled", "false"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc := applib.NewTrickplayService(repo, gen, "/cache", media, bus, cfg)
	svc.StartTrickplayWorker(ctx)

	require.NoError(t, bus.Publish(context.Background(), library.MediaItemAdded{
		MediaItemID: shared.MediaItemID("m1"),
		Type:        library.MovieItem,
	}))
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&gen.callCount))
}

// --- StartBackgroundGeneration ---

func TestTrickplay_StartBackgroundGeneration_CtxCancelExitsCleanly(t *testing.T) {
	t.Parallel()
	repo := newMissingIDsRepo(nil)
	gen := &countingGen{}
	media := libfakes.NewFakeMediaItemCommandRepo()
	bus := sharedfakes.NewFakeEventBus()
	svc := applib.NewTrickplayService(repo, gen, "/cache", media, bus, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel BEFORE starting → goroutine must exit on the 60s wait select
	require.NotPanics(t, func() { svc.StartBackgroundGeneration(ctx) })
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(0), atomic.LoadInt32(&gen.callCount))
}

func TestTrickplay_StartBackgroundGeneration_NilRepoShortCircuits(t *testing.T) {
	t.Parallel()
	gen := libfakes.NewFakeTrickplayGenerator()
	svc := applib.NewTrickplayService(nil, gen, "/cache", nil, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NotPanics(t, func() { svc.StartBackgroundGeneration(ctx) })
}

func TestTrickplay_StartBackgroundGeneration_NilGenShortCircuits(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeTrickplayRepository()
	svc := applib.NewTrickplayService(repo, nil, "/cache", nil, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NotPanics(t, func() { svc.StartBackgroundGeneration(ctx) })
}

// The boot sweep must stay missing-only (minAlgoVersion 0): after a params
// bump, a mere restart would otherwise silently queue a library-wide
// regeneration. The admin trigger opts into outdated manifests explicitly.
func TestTrickplay_GenerateAllMissing_BootIsMissingOnly_AdminSweepsOutdated(t *testing.T) {
	t.Parallel()
	repo := newMissingIDsRepo(nil)
	gen := &countingGen{}
	media := libfakes.NewFakeMediaItemCommandRepo()
	bus := sharedfakes.NewFakeEventBus()
	svc := applib.NewTrickplayService(repo, gen, "/cache", media, bus, nil)

	_, err := svc.GenerateAllMissingTrickplays(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, 0, repo.lastMinAlgoVersion, "boot path: missing-only (version clause disabled)")

	_, err = svc.GenerateAllMissingTrickplays(context.Background(), true)
	require.NoError(t, err)
	assert.Positive(t, repo.lastMinAlgoVersion, "admin path: outdated manifests below the current version are re-listed")
}
