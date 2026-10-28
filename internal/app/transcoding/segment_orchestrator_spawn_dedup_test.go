package transcoding

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// slowSource makes ResolveSource block long enough that every racing goroutine
// is inside EnsureSegmentRange while the first one is still setting its spawn
// up. That is the real shape of the prod incident: three HTTP handlers reached
// the orchestrator within 164 ms, all found no covering session, and all went
// on to exec ffmpeg.
type slowSource struct {
	path  string
	delay time.Duration
	err   error
}

func (s slowSource) ResolveSource(_ context.Context, _ DecisionFingerprint) (string, []SourceStream, error) {
	time.Sleep(s.delay)
	return s.path, nil, s.err
}

// A burst of requests for the SAME position must exec exactly one ffmpeg.
//
// Before the pending-spawn claim the only dedup was the re-check AFTER
// process.Start: every racer resolved the source, took a video slot and started
// a real encoder, then all but one killed theirs. On the 4K HEVC HDR ladder of
// the 2026-09-08 incident that meant three concurrent software decodes on a box
// calibrated for one, during the exact seconds the viewer was waiting.
func TestOrchestrator_ConcurrentSamePosition_StartsOneFFmpeg(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	o := NewSegmentOrchestrator(cache, proc, slowSource{path: "/media/test.mkv", delay: 80 * time.Millisecond}, testOpts(), discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)

	const racers = 3
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, racers)
	for i := range racers {
		wg.Go(func() {
			<-start
			errs[i] = o.EnsureSegmentRange(context.Background(), fp, 0, 0)
		})
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "racer %d", i)
	}
	assert.Equal(t, 1, proc.startCount(), "a burst on one position must exec ONE ffmpeg, not one per racer")
	assert.Equal(t, 1, o.ActiveCount())
}

// The claim is scoped to the window it promises: a racer asking for a segment
// the pending spawn will NOT produce still gets its own rafale, exactly as
// before. Guards against the dedup silently starving a real seek.
func TestOrchestrator_ConcurrentDistantPositions_StillSpawnBoth(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	o := NewSegmentOrchestrator(cache, proc, slowSource{path: "/media/test.mkv", delay: 80 * time.Millisecond}, testOpts(), discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)

	var wg sync.WaitGroup
	start := make(chan struct{})
	// 0 opens [0, 29]; 121 is far outside it - the resume position of the prod
	// incident, which must never be swallowed by the pre-warm's claim.
	segs := []int{0, 121}
	errs := make([]error, len(segs))
	for i, seg := range segs {
		wg.Go(func() {
			<-start
			errs[i] = o.EnsureSegmentRange(context.Background(), fp, 0, seg)
		})
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "segment %d", segs[i])
	}
	assert.Equal(t, 2, proc.startCount(), "disjoint windows are the multi-rafale feature, not a race")
}

// Deduplicating must not swallow the answer. A racer that skips its own spawn
// still has to learn the winner's outcome: returning nil on a spawn that failed
// would park the viewer on cache.Wait for the full budget waiting for segments
// nobody is encoding, instead of letting the handler shed immediately.
func TestOrchestrator_ConcurrentSamePosition_LosersSeeTheWinnersFailure(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := slowSource{path: "/media/test.mkv", delay: 80 * time.Millisecond, err: assert.AnError}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)

	const racers = 3
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, racers)
	for i := range racers {
		wg.Go(func() {
			<-start
			errs[i] = o.EnsureSegmentRange(context.Background(), fp, 0, 0)
		})
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		require.Error(t, err, "racer %d must not be told the spawn succeeded", i)
	}
	assert.Equal(t, 0, proc.startCount())
}

// A claim must never outlive its spawn: when ffmpeg refuses to start, the next
// request has to be able to spawn again instead of waiting forever on a hash
// wedged by a stranded claim.
func TestOrchestrator_FailedSpawn_ReleasesTheClaim(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	proc.startErr = assert.AnError
	o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/media/test.mkv"}, testOpts(), discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.Error(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))

	proc.mu.Lock()
	proc.startErr = nil
	proc.mu.Unlock()

	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
	assert.Equal(t, 2, proc.startCount(), "the retry must reach ffmpeg, not sit behind a dead claim")
	assert.Equal(t, 1, o.ActiveCount())
}

// A rafale nobody ever read is a pre-warm bet that did not pay off: it must be
// retired on the short fuse, while the rafale the viewer is actually draining
// keeps its full inactivity budget. This is the 55 s of double 4K encoding the
// prod incident spent with two ladders running side by side.
func TestOrchestrator_UnreadPreWarm_ReapedBeforeAReadRafale(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := testOpts()
	opts.InactivityTimeout = 1 * time.Minute
	opts.UnreadTimeout = 60 * time.Millisecond
	opts.GCInterval = 20 * time.Millisecond
	o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/media/test.mkv"}, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	// The pre-warm's guess: segment 0, which the player never goes to.
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
	// The real playback, resuming far away.
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 121))
	require.Equal(t, 2, o.ActiveCount())

	// Keep the resume rafale read while the sweeper runs, so only the unread
	// one can qualify.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 122))
		if o.ActiveCount() == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	assert.Equal(t, 1, o.ActiveCount(), "the unread pre-warm must be reaped, the read rafale kept")
	assert.Equal(t, 2, proc.startCount(), "no respawn: the survivor is the original resume rafale")
}

// A spawn that panics must still settle its claim: the settle used to run
// after spawnSession returned, so a panic left every later request inside the
// window parked on a claim nobody would settle.
func TestOrchestrator_PanickingSpawnReleasesItsClaim(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := &panicOnceProcess{fakeProcess: newFakeProcess()}
	o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/media/test.mkv"}, testOpts(), discardLogger())
	defer o.Close()
	fp := minimalFingerprint(t)

	func() {
		defer func() { _ = recover() }()
		_ = o.EnsureSegmentRange(context.Background(), fp, 0, 0)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, o.EnsureSegmentRange(ctx, fp, 0, 0), "the second request must spawn, not wait on a stranded claim")
	assert.Equal(t, 1, o.ActiveCount())
}

// The claimant's own context ending is not an outcome for the requests
// parked on its claim: a waiter whose context is alive spawns for itself.
func TestOrchestrator_ClaimantContextErrorIsNotTheWaitersOutcome(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	claimed := make(chan struct{})
	var once sync.Once
	slowSource := SourceResolverFunc(func(ctx context.Context, _ DecisionFingerprint) (string, []SourceStream, error) {
		once.Do(func() { close(claimed) })
		time.Sleep(50 * time.Millisecond)
		if err := ctx.Err(); err != nil {
			return "", nil, err
		}
		return "/media/test.mkv", nil, nil
	})
	o := NewSegmentOrchestrator(cache, proc, slowSource, testOpts(), discardLogger())
	defer o.Close()
	fp := minimalFingerprint(t)

	claimantCtx, cancelClaimant := context.WithCancel(context.Background())
	claimantErr := make(chan error, 1)
	go func() { claimantErr <- o.EnsureSegmentRange(claimantCtx, fp, 0, 0) }()
	<-claimed
	waiterErr := make(chan error, 1)
	go func() { waiterErr <- o.EnsureSegmentRange(context.Background(), fp, 0, 0) }()
	time.Sleep(10 * time.Millisecond)
	cancelClaimant()

	assert.ErrorIs(t, <-claimantErr, context.Canceled)
	require.NoError(t, <-waiterErr, "the waiter's context is alive: it must not inherit the claimant's cancellation")
	assert.Equal(t, 1, o.ActiveCount())
}
