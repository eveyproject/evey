package transcoding

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// swTestOpts returns SoftwareProducerOptions with fast GC for tests.
func swTestOpts() SoftwareProducerOptions {
	return SoftwareProducerOptions{
		ReadRate:           1.5,
		SegmentSeconds:     3.0,
		InactivityTimeout:  time.Minute,
		GCInterval:         50 * time.Millisecond,
		PauseIdleThreshold: time.Minute, // don't pause mid-test
		WaitBudgetDur:      60 * time.Second,
	}
}

// reachSegs is the seek-reach window these options actually run with. Tests
// below assert the REUSE RULE ("a request the running encoder can still reach
// shares it"), and the window is derived from the pacing rather than fixed, so
// hardcoding a distance here would silently stop testing the rule the day the
// pacing changes - which is exactly what happened with the old fixed 16.
func reachSegs() int {
	return swTestOpts().withDefaults().SeekReachWindowSegs
}

// ============================================================================
// One process per hash; ReadRate pacing
// ============================================================================

func TestSoftwareProducer_CloseIdempotent(t *testing.T) {
	// A second Close must not panic on a re-close of stopGC.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	p := NewSoftwareProducer(cache, proc, src, swTestOpts(), discardLogger())

	require.NotPanics(t, func() {
		p.Close()
		p.Close()
	})
}

func TestSoftwareProducer_OneSequentialProcessPerHash(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	p := NewSoftwareProducer(cache, proc, src, swTestOpts(), discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	// Forward requests that each stay within the seek-reach window of the running
	// encoder's advancing frontier share ONE ffmpeg (normal sequential playback,
	// where the playhead never jumps far past what has been produced).
	// Each step lands inside the window of the frontier the PREVIOUS step
	// committed, which is what continuous playback looks like from here.
	w := reachSegs()
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, w))
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 2*w))

	assert.Equal(t, 1, proc.startCount(), "in-reach forward requests share the single long-lived encoder")
	assert.Equal(t, 1, p.ActiveSessionCount())
}

// Natural-EOF gauge Dec (the reap goroutine's metricDecOnce) is verified by
// construction against the proven orchestrator pattern, not a bespoke test: a
// gauge-value assertion needs prometheus/testutil, which would promote an indirect
// dep to direct (go.mod churn) for no functional coverage. ActiveSessionCount
// already proves the session is reaped on EOF (TwoTabs / ForwardSeek tests).

func TestSoftwareProducer_ReadRatePaced(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	p := NewSoftwareProducer(cache, proc, src, swTestOpts(), discardLogger())
	defer p.Close()

	require.NoError(t, p.EnsureSegmentRange(context.Background(), minimalFingerprint(t), 0, 0))
	assert.InDelta(t, 1.5, proc.lastOpts().ReadRate, 0.001,
		"the software producer MUST input-pace ffmpeg (-readrate 1.5) - the defining software-flow knob")
}

// Copy-mode session: paced LOOSELY at 8x (NOT the transcode 1.5x, NOT unpaced),
// every-keyframe plan drives StartSec/StartNumber, and the ffmpeg invocation is
// flagged copy + copy-on-seek. 8x bounds the muxer-side A/V skew an unpaced
// copy develops on a weak host (audio re-encode outrun by the ~free video copy
// → interleave force-flush → the prod "son décalé" desync), while keeping
// startup/seek instant via the initial burst.
func TestSoftwareProducer_CopySessionPacedAtKeyframePlan(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	p := NewSoftwareProducer(cache, proc, src, swTestOpts(), discardLogger())
	defer p.Close()

	// Long-GOP plan with a scene-cut: [0,10s) [10,13s) [13,23s) [23,30s).
	streams := &fakeCopyStreams{streams: videoStreamWithKeyframes([]int64{0, 10000, 13000, 23000})}
	items := &fakeCopyItems{item: itemWithDurationMs(30000)}
	p.SetCopyMode(NewCopyPlanner(streams, items), nil)

	fp := minimalFingerprint(t)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 2))

	opts := proc.lastOpts()
	assert.Equal(t, "copy", opts.VideoDecision)
	assert.True(t, opts.CopyOnSeek, "copy session must stay -c:v copy on a mid-stream start")
	assert.InDelta(t, 8.0, opts.ReadRate, 0.001,
		"copy sessions pace at 8x - unpaced lets video outrun the audio re-encode (A/V desync), 1.5x starves the client")
	assert.InDelta(t, 13.0, opts.StartSec, 0.001, "StartSec = the plan keyframe of segment 2, not 2×SegmentSeconds")
	assert.Equal(t, 2, opts.StartNumber, "StartNumber = the absolute plan index")
}

// fakeSeekProber simulates the container seek index: a seek to targetSec lands
// on the configured landing (the preceding cue point).
type fakeSeekProber struct {
	landing float64
	err     error
	calls   int
	target  float64
}

func (f *fakeSeekProber) ProbeSeekLanding(_ context.Context, _ string, targetSec float64) (float64, error) {
	f.calls++
	f.target = targetSec
	return f.landing, f.err
}

// A mid-stream copy session must follow the demuxer's REAL seek landing, not
// the requested keyframe: sparse cue indexes (mkvmerge: one cue per ~5s
// cluster) land a pre-input -ss on the preceding cue point, possibly whole
// GOPs early. If the session keeps the requested StartSec/StartNumber, the
// rebase pump shifts every video timestamp by the gap while the audio (trimmed
// at the exact -ss time) stays true - the measured post-deep-seek A/V desync.
func TestSoftwareProducer_CopySeekFollowsRealLanding(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	p := NewSoftwareProducer(cache, proc, src, swTestOpts(), discardLogger())
	defer p.Close()

	// Plan boundaries: [0,10s) [10,13s) [13,23s) [23,30s).
	streams := &fakeCopyStreams{streams: videoStreamWithKeyframes([]int64{0, 10000, 13000, 23000})}
	items := &fakeCopyItems{item: itemWithDurationMs(30000)}
	// Seek targets segment 2 (13.0s) but the cue index lands at 10.0s (= plan
	// boundary 1, a GOP earlier).
	prober := &fakeSeekProber{landing: 10.0}
	p.SetCopyMode(NewCopyPlanner(streams, items), prober)

	fp := minimalFingerprint(t)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 2))

	require.Equal(t, 1, prober.calls, "mid-stream copy start must probe the real landing")
	assert.InDelta(t, 13.0+transcoding.CopySeekNudgeSec, prober.target, 0.001,
		"the probe must use the SAME nudged target the builder will pass to -ss")
	opts := proc.lastOpts()
	assert.InDelta(t, 10.0, opts.StartSec, 0.001, "StartSec follows the LANDED keyframe, not the requested one")
	assert.Equal(t, 1, opts.StartNumber, "StartNumber follows the landed plan index")
	assert.InDelta(t, 13.0+transcoding.CopySeekNudgeSec, opts.SeekTargetSec, 0.001,
		"-ss must stay the PROBED target - re-deriving it from the landing re-triggers the seek rules one keyframe earlier")
}

// Landing probe failure (or an exact landing) keeps the requested start - the
// pre-fix behavior, correct for dense-cue sources.
func TestSoftwareProducer_CopySeekProbeFallbacks(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	p := NewSoftwareProducer(cache, proc, src, swTestOpts(), discardLogger())
	defer p.Close()

	streams := &fakeCopyStreams{streams: videoStreamWithKeyframes([]int64{0, 10000, 13000, 23000})}
	items := &fakeCopyItems{item: itemWithDurationMs(30000)}
	prober := &fakeSeekProber{err: context.DeadlineExceeded}
	p.SetCopyMode(NewCopyPlanner(streams, items), prober)

	fp := minimalFingerprint(t)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 2))
	opts := proc.lastOpts()
	assert.InDelta(t, 13.0, opts.StartSec, 0.001, "probe failure falls back to the requested keyframe")
	assert.Equal(t, 2, opts.StartNumber)
}

func TestPlanIndexForLanding(t *testing.T) {
	segs := []CopySegment{{0, 10000}, {10000, 13000}, {13000, 23000}, {23000, 30000}}
	cases := []struct {
		landing float64
		want    int
		why     string
	}{
		{landing: 0.0, want: 0},
		{landing: 10.0, want: 1, why: "exact boundary"},
		{landing: 9.95, want: 1, why: "boundary within rounding tolerance"},
		{landing: 16.4, want: 2, why: "mid-segment landing → containing segment"},
		{landing: 22.99, want: 3, why: "within 100ms of the next boundary → snaps forward"},
		{landing: 29.0, want: 3, why: "inside the tail"},
		// The 100 ms tolerance is a real boundary, and it is INCLUSIVE. These two
		// cases sit one millisecond either side of it against the 13 s keyframe:
		// snapping forward one segment too eagerly (or too late) shifts every
		// video timestamp the session emits by a whole GOP while the re-encoded
		// audio stays true - the post-deep-seek A/V desync this function exists
		// to prevent.
		{landing: 12.9, want: 2, why: "landing+tolerance lands exactly ON the boundary: inclusive, so it snaps forward"},
		{landing: 12.8994, want: 1, why: "one millisecond short of the tolerance stays on the previous segment"},
		// Same pair, read as a rounding contract: seconds→milliseconds ROUNDS.
		// Truncating 12.8996 s to 12899 ms would drop it below the tolerance and
		// re-produce a GOP the viewer already has.
		{landing: 12.8996, want: 2, why: "12.8996 s rounds to 12900 ms, it does not truncate to 12899"},
		{landing: 999.0, want: 3, why: "a landing past the plan clamps to the last segment"},
		{landing: -5.0, want: 0, why: "a nonsense landing must not index below zero"},
	}
	for _, tc := range cases {
		if got := planIndexForLanding(segs, tc.landing); got != tc.want {
			t.Fatalf("planIndexForLanding(%v) = %d, want %d - %s", tc.landing, got, tc.want, tc.why)
		}
	}
}

// An empty plan must not panic the session start: a landing probe can race a
// plan rebuild.
func TestPlanIndexForLanding_EmptyPlan(t *testing.T) {
	assert.Equal(t, 0, planIndexForLanding(nil, 12.0))
	assert.Equal(t, 0, planIndexForLanding([]CopySegment{}, 12.0))
}

func TestSoftwareProducer_NeverSetsHWAccel(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	p := NewSoftwareProducer(cache, proc, src, swTestOpts(), discardLogger())
	defer p.Close()

	// Even if a fingerprint carried a HW backend, the software producer never
	// runs the GPU pipeline.
	fp := minimalFingerprint(t)
	fp.HWAccel = "nvenc"
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	assert.Equal(t, transcoding.HWAccelType(""), proc.lastOpts().HWAccel,
		"software producer must never emit a HW encode backend")
}

// ============================================================================
// Uncovered position = spawn ALONGSIDE (never kill a sibling another tab needs)
// ============================================================================

func TestSoftwareProducer_UncoveredBackwardSeekSpawnsAlongside(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	p := NewSoftwareProducer(cache, proc, src, swTestOpts(), discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	// Start at segment 100.
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 100))
	require.Equal(t, 1, proc.startCount())
	hash := fp.Hash()
	p.mu.Lock()
	first := p.sessions[hash][0]
	p.mu.Unlock()
	firstHandle := first.handle.(*fakeHandle)

	// Request segment 5 - before the live window [100, +inf). The producer does
	// NOT kill the running encoder (that would yank a second tab sitting at 100);
	// it spawns a fresh one ALONGSIDE at 5. Two encoders now coexist.
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 5))
	assert.Equal(t, 2, proc.startCount(), "uncovered backward position spawns a second encoder")
	assert.Equal(t, 2, p.ActiveSessionCount(), "both encoders coexist - the first is NOT killed")
	assert.False(t, firstHandle.isStopped(), "the segment-100 encoder must keep running (a sibling tab needs it)")

	// The new process is seeked to segment 5 (StartSec = 5 × 3s).
	assert.InDelta(t, 15.0, proc.lastOpts().StartSec, 0.001)
}

func TestSoftwareProducer_ForwardSeekReusesProcess(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	p := NewSoftwareProducer(cache, proc, src, swTestOpts(), discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	// A SMALL forward jump stays within the seek-reach window - the sequential
	// encoder is already producing toward it - so no restart. (A far jump spawns a
	// fresh seek encoder instead: see ForwardSeekBeyondReachSpawnsSeekEncoder.)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, reachSegs()))
	assert.Equal(t, 1, proc.startCount(), "an in-reach forward seek reuses the running encoder")
}

// ============================================================================
// Never ErrCapacityExceeded - a second media never sheds the first
// ============================================================================

func TestSoftwareProducer_NeverRejectsOnCapacity(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	p := NewSoftwareProducer(cache, proc, src, swTestOpts(), discardLogger())
	defer p.Close()

	// Spawn many distinct-media sessions. The orchestrator would 503 past its
	// MaxActiveSessions cap; the software producer NEVER does (the client waits,
	// the input-rate cap degrades the host gracefully instead of thrashing).
	for i := range 8 {
		fp := minimalFingerprint(t)
		fp.MediaID = "media-" + string(rune('a'+i))
		// require.NoError implicitly proves it's not ErrCapacityExceeded - the
		// software producer has no global session cap, so it never rejects.
		require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0),
			"software producer must never reject on media %d", i)
	}
	assert.Equal(t, 8, p.ActiveSessionCount(), "all eight distinct media run concurrently - no shedding")
}

// ============================================================================
// Multi-tab coexistence: two viewers, same film, different positions
// ============================================================================

func TestSoftwareProducer_TwoTabsDifferentPositionsCoexist(t *testing.T) {
	// The user's scenario: a deep viewer (tab 1, resumed at segment 500) and a
	// second viewer (tab 2) cold-starting the SAME film from segment 0 - identical
	// DecisionHash. The cold start must NOT kill the deep viewer's encoder.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	hash := fp.Hash()

	// Tab 1 resumes deep.
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 500))
	require.Equal(t, 1, p.ActiveSessionCount())
	p.mu.Lock()
	deep := p.sessions[hash][0]
	p.mu.Unlock()
	deepHandle := deep.handle.(*fakeHandle)
	require.Equal(t, 500, deep.startSeg)

	// Tab 2 cold-starts from the beginning.
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))

	assert.Equal(t, 2, proc.startCount(), "tab 2 gets its OWN encoder (a distinct live position)")
	assert.Equal(t, 2, p.ActiveSessionCount(), "both tabs coexist")
	assert.False(t, deepHandle.isStopped(),
		"the deep viewer's encoder must NOT be killed by tab 2 cold-starting - the multi-tab guarantee")

	p.mu.Lock()
	starts := map[int]bool{}
	for _, s := range p.sessions[hash] {
		starts[s.startSeg] = true
	}
	p.mu.Unlock()
	assert.True(t, starts[500] && starts[0], "both the deep (500) and cold-start (0) positions have live encoders")
}

func TestSoftwareProducer_SamePositionViewersShareOneEncoder(t *testing.T) {
	// Viewers at the same position (or a second one BEHIND the live start) are
	// covered by the first's forward-running encoder - they share it, so the weak
	// NAS pays for exactly one encode, not two. The multi-session cost is only
	// incurred for genuinely-distinct uncached positions.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))           // second viewer, same start
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, reachSegs())) // a beat ahead, within the seek-reach window → shared
	assert.Equal(t, 1, proc.startCount(), "viewers at or just ahead of the live start share ONE encoder")
	assert.Equal(t, 1, p.ActiveSessionCount())
}

func TestSoftwareProducer_ForwardSeekBeyondReachSpawnsSeekEncoder(t *testing.T) {
	// A forward seek that lands far past the live encoder's production frontier
	// must NOT reuse the sequential encoder - paced at 1.5× it would take minutes
	// to cross hundreds of segments and the segment Wait would time out (the
	// "buffering forever on seek" bug). A fresh encoder seeks (-ss) straight to the
	// requested segment instead, coexisting with the original (bounded by the cap).
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	hash := fp.Hash()

	// Viewer starts at the beginning; its encoder sits at frontier 0.
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.Equal(t, 1, proc.startCount())

	// Seek to ~8.5 min in (segment 169) - far beyond SeekReachWindowSegs.
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 169))

	assert.Equal(t, 2, proc.startCount(), "a deep forward seek spawns a fresh encoder, not a wait on the from-0 one")
	assert.Equal(t, 2, p.ActiveSessionCount())
	p.mu.Lock()
	starts := map[int]bool{}
	for _, s := range p.sessions[hash] {
		starts[s.startSeg] = true
	}
	p.mu.Unlock()
	assert.True(t, starts[169], "the new encoder seeks straight to the requested segment (169)")
	assert.True(t, starts[0], "the original encoder is left running, not killed mid-stream")
}

func TestSoftwareProducer_DeepSeekSuspendsAbandonedEncoder(t *testing.T) {
	// On a box that fits ONE heavy software transcode, a deep forward seek must
	// hand the CPU to the seek position at once: the abandoned from-0 encoder is
	// SIGSTOPped immediately (not left contending for PauseIdleThreshold seconds -
	// the "small buffering after seeking" symptom, two 4K HDR encoders splitting
	// the CPU at ~0.2× + ~0.9× realtime). Pause, not kill: it resumes if pulled.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	hash := fp.Hash()

	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 169)) // deep forward seek

	p.mu.Lock()
	var atZero, atSeek *swSession
	for _, s := range p.sessions[hash] {
		switch s.startSeg {
		case 0:
			atZero = s
		case 169:
			atSeek = s
		}
	}
	p.mu.Unlock()
	require.NotNil(t, atZero, "the from-0 encoder still exists (paused, not killed)")
	require.NotNil(t, atSeek, "the seek encoder was spawned at 169")

	_, _, zeroPaused := atZero.handle.(*fakeHandle).counts()
	_, _, seekPaused := atSeek.handle.(*fakeHandle).counts()
	assert.True(t, zeroPaused, "the abandoned from-0 encoder is suspended so the seek position gets the full CPU")
	assert.False(t, seekPaused, "the just-started seek encoder keeps running at full speed")
}

// twoRungFingerprint is minimalFingerprint with a real 2-rung ladder so the
// variant-aware session lookup can be exercised (single-rendition uses one rung).
func twoRungFingerprint(t *testing.T) DecisionFingerprint {
	t.Helper()
	fp := minimalFingerprint(t)
	fp.Profiles = []ProfileFingerprint{
		{Name: "360p", Width: 640, Height: 360, VideoBitrate: 800, AudioBitrate: 96},
		{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 2500, AudioBitrate: 128},
	}
	return fp
}

func TestSoftwareProducer_VariantIsolation_NoCrossRungReuse(t *testing.T) {
	// A variant-1 request must NEVER reuse a variant-0 encoder: that encoder writes
	// stream_0_* and would never produce stream_1_*, so the cache Wait (which polls
	// per-variant files) would rebuffer forever. Each rung gets its own encoder.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	fp := twoRungFingerprint(t)
	hash := fp.Hash()

	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0)) // variant 0
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 1, 5)) // variant 1 - must NOT reuse variant 0

	assert.Equal(t, 2, proc.startCount(), "a different rung spawns its own encoder, never reuses another variant's")
	p.mu.Lock()
	variantStart := map[int]int{}
	for _, s := range p.sessions[hash] {
		variantStart[s.variantIdx] = s.startSeg
	}
	p.mu.Unlock()
	assert.Contains(t, variantStart, 0, "variant-0 encoder present")
	assert.Contains(t, variantStart, 1, "variant-1 encoder present")
	assert.Equal(t, 5, variantStart[1], "the variant-1 encoder seeked to the requested segment (5)")
}

func TestSoftwareProducer_VariantAwareReuse_SameRungShares(t *testing.T) {
	// Two in-reach requests on the SAME rung still share one encoder - the variant
	// filter must not break normal same-variant reuse / frontier advance.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	fp := twoRungFingerprint(t)
	hash := fp.Hash()

	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 1, 0))           // variant 1
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 1, reachSegs())) // same rung, in-reach
	assert.Equal(t, 1, proc.startCount(), "same-rung in-reach requests share one encoder")

	p.mu.Lock()
	var s *swSession
	for _, sess := range p.sessions[hash] {
		if sess.variantIdx == 1 {
			s = sess
		}
	}
	p.mu.Unlock()
	require.NotNil(t, s)
	assert.Equal(t, reachSegs(), s.frontier, "the reused variant-1 encoder's frontier advanced to the request")
}

func TestSoftwareProducer_CacheHitTouchesCorrectVariant(t *testing.T) {
	// A cache hit on variant 1 must advance the variant-1 encoder's frontier, not a
	// variant-0 sibling sharing the hash.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	fp := twoRungFingerprint(t)
	hash := fp.Hash()

	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0)) // variant 0 encoder
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 1, 0)) // variant 1 encoder

	writeReadySegment(t, cache, SegmentKey{DecisionHash: hash, VariantIdx: 1, SegmentIdx: 7}, fp.UseFMP4)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 1, 7)) // cache hit on variant 1

	p.mu.Lock()
	var v0, v1 *swSession
	for _, s := range p.sessions[hash] {
		switch s.variantIdx {
		case 0:
			v0 = s
		case 1:
			v1 = s
		}
	}
	p.mu.Unlock()
	require.NotNil(t, v0)
	require.NotNil(t, v1)
	assert.Equal(t, 7, v1.frontier, "the variant-1 cache hit advanced the variant-1 frontier")
	assert.Equal(t, 0, v0.frontier, "the variant-0 frontier is untouched by a variant-1 hit")
}

func threeRungFingerprint(t *testing.T) DecisionFingerprint {
	t.Helper()
	fp := minimalFingerprint(t)
	fp.Profiles = []ProfileFingerprint{
		{Name: "360p", Width: 640, Height: 360, VideoBitrate: 800, AudioBitrate: 96},
		{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 2500, AudioBitrate: 128},
		{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 5000, AudioBitrate: 128},
	}
	return fp
}

func TestSoftwareProducer_LevelSwitchPausesAbandonedRung(t *testing.T) {
	// Switching rungs (a fresh variant starting) must SIGSTOP the old rung at once
	// so two full 4K decodes don't run concurrently on a weak box - pause, not kill,
	// so a co-viewer could still resume it.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	fp := twoRungFingerprint(t)
	hash := fp.Hash()

	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0)) // rung 0 playing
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 1, 0)) // switch to rung 1

	p.mu.Lock()
	var v0, v1 *swSession
	for _, s := range p.sessions[hash] {
		switch s.variantIdx {
		case 0:
			v0 = s
		case 1:
			v1 = s
		}
	}
	p.mu.Unlock()
	require.NotNil(t, v0, "the abandoned rung-0 encoder still exists (paused, not killed)")
	require.NotNil(t, v1)
	_, _, v0paused := v0.handle.(*fakeHandle).counts()
	_, _, v1paused := v1.handle.(*fakeHandle).counts()
	assert.True(t, v0paused, "the abandoned rung-0 encoder is suspended at once on the level switch")
	assert.False(t, v1paused, "the new rung-1 encoder keeps running")
	assert.False(t, v0.handle.(*fakeHandle).isStopped(), "abandoned rung is paused, NOT killed (a co-viewer could resume it)")
}

func TestSoftwareProducer_DistinctVariantCapEvictsLRURung(t *testing.T) {
	// On a weak NAS the number of distinct concurrently-encoding rungs for one film
	// is hard-bounded: a 3rd distinct rung evicts the LRU rung's encoder so a probing
	// client can never stack N concurrent 4K decodes.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := swTestOpts()
	opts.MaxConcurrentVariantsPerHash = 2
	opts.MaxSessionsPerHash = 10 // keep the total cap from masking the variant cap
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, opts, discardLogger())
	defer p.Close()

	fp := threeRungFingerprint(t)
	hash := fp.Hash()

	// The cap's LRU choice is about ESTABLISHED rungs (the newborn shield has
	// its own test below): age each spawn past the shield before the next.
	ageAll := func() {
		p.mu.Lock()
		for _, s := range p.sessions[hash] {
			s.spawnedAt = s.spawnedAt.Add(-time.Hour)
		}
		p.mu.Unlock()
	}
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0)) // rung 0 (oldest)
	ageAll()
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 1, 0)) // rung 1
	ageAll()
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 2, 0)) // rung 2 -> evicts LRU rung 0

	p.mu.Lock()
	variants := map[int]bool{}
	for _, s := range p.sessions[hash] {
		variants[s.variantIdx] = true
	}
	p.mu.Unlock()
	assert.Len(t, variants, 2, "never more than MaxConcurrentVariantsPerHash distinct rungs alive")
	assert.False(t, variants[0], "the LRU rung (0) was evicted")
	assert.True(t, variants[1] && variants[2], "the two most-recent rungs survive")
}

// THE PERPETUAL SPINNER (2026-07-15, 22:42-22:44 prod). The player was blocked
// on variant 3's segment 0; requests for other rungs kept spawning encoders,
// each spawn crossed a cap, and the eviction killed the very session the
// player was waiting on - before it had produced a single byte. The retry
// respawned it, which evicted the next sibling, whose waiter retried... every
// encoder died at half of its first segment and nothing was ever delivered.
//
// A NEWBORN session - one that has not yet delivered its first segment - is
// exactly the thing an active Wait depends on. It is shielded from cap
// eviction for a grace window: keeping it over-cap is safe because the pause
// machinery (pauseSiblingsBehind / pauseOtherVariants) already SIGSTOPs the
// non-requested encoders, so the cap's CPU intent holds; only the kill turns
// overload into livelock.
func TestSoftwareProducer_ANewbornEncoderIsNotEvictedBeforeItsFirstDelivery(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := swTestOpts()
	opts.MaxSessionsPerHash = 2
	opts.MaxConcurrentVariantsPerHash = 2
	opts.NewbornEvictionShield = time.Minute
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, opts, discardLogger())
	defer p.Close()

	fp := threeRungFingerprint(t)
	hash := fp.Hash()

	// The prod shape: three rungs probed within seconds, none has delivered.
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 2, 0))
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 1, 0))
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))

	p.mu.Lock()
	variants := map[int]bool{}
	for _, s := range p.sessions[hash] {
		variants[s.variantIdx] = true
	}
	p.mu.Unlock()
	for v := range 3 {
		assert.True(t, variants[v], "rung %d's newborn encoder must survive its siblings' spawns: killing it is the 2026-07-15 spinner", v)
	}
	assert.Equal(t, 3, p.ActiveSessionCount(), "newborns ride over the cap (paused), they are not killed")

	// Once the grace window has passed, the caps bite again: the shield delays
	// eviction, it never repeals the bound.
	p.mu.Lock()
	for _, s := range p.sessions[hash] {
		s.spawnedAt = time.Now().Add(-2 * opts.NewbornEvictionShield)
		s.lastAccess = time.Now().Add(-2 * opts.NewbornEvictionShield)
	}
	p.mu.Unlock()
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 2, 500))

	assert.Less(t, p.ActiveSessionCount(), 4, "past the grace window the caps evict again")
}

// newbornLocked gates the eviction shield on BOTH conditions: not-yet-delivered
// (frontier <= startSeg) AND within the time window. The eviction tests above
// only ever flip the time condition, so a mutant dropping the frontier check
// would survive. Exercise the frontier axis directly.
func TestSoftwareProducer_NewbornLockedRequiresBothFrontierAndTime(t *testing.T) {
	cache, _ := newTestCache(t)
	opts := swTestOpts()
	opts.NewbornEvictionShield = time.Minute
	p := NewSoftwareProducer(cache, newFakeProcess(), stubSource{path: "/m.mkv"}, opts, discardLogger())
	defer p.Close()
	now := time.Now()

	// Undelivered (frontier == startSeg) AND fresh -> a shielded newborn.
	fresh := &swSession{startSeg: 100, frontier: 100, spawnedAt: now}
	assert.True(t, p.newbornLocked(fresh, now), "undelivered + fresh is a shielded newborn")

	// Delivered its first segment (frontier > startSeg) but still time-fresh ->
	// NOT a newborn: the shield only protects a session an active Wait still
	// depends on, and this is the case the time-only tests never cover.
	delivered := &swSession{startSeg: 100, frontier: 101, spawnedAt: now}
	assert.False(t, p.newbornLocked(delivered, now), "a first delivery ends the shield even while time-fresh")

	// Undelivered but past the window -> the grace has expired.
	old := &swSession{startSeg: 100, frontier: 100, spawnedAt: now.Add(-2 * opts.NewbornEvictionShield)}
	assert.False(t, p.newbornLocked(old, now), "past the shield window the newborn grace ends")
}

func TestSoftwareProducer_MaxSessionsPerHashEvictsLRU(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := swTestOpts()
	opts.MaxSessionsPerHash = 2
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, opts, discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	hash := fp.Hash()
	// ageAll pushes every live session past the newborn shield: this test is
	// about the cap's LRU choice among ESTABLISHED encoders, not about the
	// shield (which has its own test above).
	ageAll := func() {
		p.mu.Lock()
		for _, s := range p.sessions[hash] {
			s.spawnedAt = s.spawnedAt.Add(-time.Hour)
		}
		p.mu.Unlock()
	}
	// Three DESCENDING positions: each is below every existing start, so each
	// spawns its own encoder. The 3rd crosses MaxSessionsPerHash=2 → the LRU
	// (the oldest-touched, segment 300) is evicted.
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 300))
	ageAll()
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 200))
	ageAll()
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 100))

	assert.Equal(t, 3, proc.startCount(), "three distinct lower positions each spawn an encoder")
	assert.Equal(t, 2, p.ActiveSessionCount(), "MaxSessionsPerHash=2 bounds concurrency; the LRU is evicted")

	p.mu.Lock()
	starts := map[int]bool{}
	for _, s := range p.sessions[hash] {
		starts[s.startSeg] = true
	}
	p.mu.Unlock()
	assert.True(t, starts[100], "the freshly-spawned encoder is never the eviction victim")
	assert.False(t, starts[300], "the least-recently-used encoder (segment 300) was evicted")
}

// ============================================================================
// WaitBudget is long
// ============================================================================

func TestSoftwareProducer_WaitBudgetIsLong(t *testing.T) {
	cache, _ := newTestCache(t)
	p := NewSoftwareProducer(cache, newFakeProcess(), stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()
	assert.GreaterOrEqual(t, p.WaitBudget(), 45*time.Second,
		"software WaitBudget must be long - the single paced encoder is slow to land seg 0 and the client waits")
}

func TestSoftwareProducer_WaitBudgetDefault(t *testing.T) {
	cache, _ := newTestCache(t)
	// Zero options → defaults: ReadRate 1.5, 60 s wait budget.
	p := NewSoftwareProducer(cache, newFakeProcess(), stubSource{path: "/m.mkv"}, SoftwareProducerOptions{}, discardLogger())
	defer p.Close()
	assert.Equal(t, 60*time.Second, p.WaitBudget())
}

// ============================================================================
// Profiles = a ladder capped to the sustainable ceiling (auto-degrade + manual)
// ============================================================================

func TestSoftwareProducer_ProfilesReturnsLadderCappedToSustainable(t *testing.T) {
	cache, _ := newTestCache(t)
	p := NewSoftwareProducer(cache, newFakeProcess(), stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	base := transcoding.LadderFilmNormal // [360, 480, 720, 1080]
	src := SourceDims{Width: 3840, Height: 2160}
	capability := transcoding.EncodeCapability{SoftwareMaxHeight: 1080}

	profiles := p.Profiles(base, src, transcoding.PlaybackDecision{}, capability)
	require.Greater(t, len(profiles), 1, "a ladder, not one rung: Auto degrades into it and the viewer picks from it")
	assert.Equal(t, 360, profiles[0].Height, "the lowest rung is the 360p guaranteed-playable floor")
	assert.Equal(t, 1080, profiles[len(profiles)-1].Height, "the top rung is the sustainable ceiling")
	for _, pr := range profiles {
		assert.LessOrEqual(t, pr.Height, 1080, "no rung exceeds the sustainable ceiling")
	}
}

func TestSoftwareProducer_ProfilesCopyModeIsSingleSourceRung(t *testing.T) {
	cache, _ := newTestCache(t)
	p := NewSoftwareProducer(cache, newFakeProcess(), stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	base := transcoding.LadderFilmNormal // [360, 480, 720, 1080]
	src := SourceDims{Width: 1920, Height: 1080}
	capability := transcoding.EncodeCapability{SoftwareMaxHeight: 1080}

	// Transcode mode → a multi-rung ladder.
	require.Greater(t, len(p.Profiles(base, src, transcoding.PlaybackDecision{}, capability)), 1)

	// Copy mode → exactly ONE rung at the source resolution with Copy=true (the
	// paced remux: direct-play quality, no transcode ladder). The planner is
	// never called by Profiles - only its presence flips the branch.
	p.SetCopyMode(NewCopyPlanner(nil, nil), nil)
	copyProfiles := p.Profiles(base, src, transcoding.PlaybackDecision{}, capability)
	require.Len(t, copyProfiles, 1, "copy mode = one direct-play rung, not a ladder")
	assert.True(t, copyProfiles[0].Copy, "the copy rung carries Copy=true (folds into the cache fingerprint)")
	assert.Equal(t, 1080, copyProfiles[0].Height, "rung is at the source resolution")
	assert.Equal(t, 1920, copyProfiles[0].Width)
}

func TestSoftwareProducer_ProfilesUsesSoftwareHDRCeilingOnTonemap(t *testing.T) {
	cache, _ := newTestCache(t)
	p := NewSoftwareProducer(cache, newFakeProcess(), stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	base := transcoding.LadderFilmNormal // [360, 480, 720, 1080]
	src := SourceDims{Width: 3840, Height: 2160, TenBit: true}
	// 1080p SDR ceiling but a much lower 480p HDR ceiling: a weak NAS cannot hold
	// 1080p HDR software, so the whole HDR ladder is capped at the HDR ceiling.
	capability := transcoding.EncodeCapability{SoftwareMaxHeight: 1080, SoftwareHDRMaxHeight: 480}
	decision := transcoding.PlaybackDecision{TranscodeReasons: []string{"hdr_tonemapping_needed"}}

	profiles := p.Profiles(base, src, decision, capability)
	require.NotEmpty(t, profiles)
	for _, pr := range profiles {
		assert.LessOrEqual(t, pr.Height, 480,
			"every HDR rung obeys SoftwareHDRMaxHeight (NOT the 1080p SDR ceiling) - the honest weak-NAS HDR cap")
	}
	assert.Equal(t, 360, profiles[0].Height, "the 360p floor stays for Auto to fall back to")
	assert.Equal(t, 480, profiles[len(profiles)-1].Height, "the HDR top rung is the HDR ceiling")
}

// The prod old-iGPU case: a 4K HDR source software-tonemapped on a box whose
// HDR ceiling is 720p. The 720p rung must inherit the dropped 1080p rung's
// bitrate budget (2800k → 5000k on LadderFilmNormal) so the downscaled grain
// survives - quality the box can afford at the height it can sustain.
func TestSoftwareProducer_ProfilesBoostsTopRungWhenHDRCapped(t *testing.T) {
	cache, _ := newTestCache(t)
	p := NewSoftwareProducer(cache, newFakeProcess(), stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	base := transcoding.LadderFilmNormal // [360@800k, 480@1400k, 720@2800k, 1080@5000k]
	src := SourceDims{Width: 3840, Height: 2160, TenBit: true}
	capability := transcoding.EncodeCapability{SoftwareMaxHeight: 1080, SoftwareHDRMaxHeight: 720}
	decision := transcoding.PlaybackDecision{TranscodeReasons: []string{"hdr_tonemapping_needed"}}

	profiles := p.Profiles(base, src, decision, capability)
	require.Len(t, profiles, 3, "360/480/720 - the 1080p rung is above the HDR ceiling")
	top := profiles[len(profiles)-1]
	assert.Equal(t, 720, top.Height)
	assert.Equal(t, int64(5000), top.VideoBitrate, "720p inherits the dropped 1080p rung's video budget")
	assert.Equal(t, int64(800), profiles[0].VideoBitrate, "lower rungs keep their own budgets")
	assert.Equal(t, int64(1400), profiles[1].VideoBitrate)

	// SDR on the same box reaches its true 1080p ceiling - nothing dropped by
	// capability (only the 4K rung-less ladder's source cap is a no-op here),
	// so no boost fires and the ladder is byte-identical to before.
	sdr := p.Profiles(base, src, transcoding.PlaybackDecision{}, capability)
	assert.Equal(t, int64(5000), sdr[len(sdr)-1].VideoBitrate, "1080p keeps its own 5000k - no boost when nothing was dropped")
	assert.Equal(t, 1080, sdr[len(sdr)-1].Height)
}

// ============================================================================
// Cache hit short-circuit; idle GC reaps
// ============================================================================

func TestSoftwareProducer_NoSpawnOnCacheHit(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	k := SegmentKey{DecisionHash: fp.Hash(), VariantIdx: 0, SegmentIdx: 7}
	writeReadySegment(t, cache, k, fp.UseFMP4)

	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 7))
	assert.Equal(t, 0, proc.startCount(), "a cached segment must not spawn ffmpeg")
}

func TestSoftwareProducer_IdleGCReapsSession(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := swTestOpts()
	opts.InactivityTimeout = 30 * time.Millisecond
	opts.GCInterval = 10 * time.Millisecond
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, opts, discardLogger())
	defer p.Close()

	require.NoError(t, p.EnsureSegmentRange(context.Background(), minimalFingerprint(t), 0, 0))
	require.Equal(t, 1, p.ActiveSessionCount())

	require.Eventually(t, func() bool {
		return p.ActiveSessionCount() == 0
	}, eventuallyWait, 10*time.Millisecond, "idle session must be GC-reaped")
}

// writeReadySegment writes a structurally valid fMP4 segment so IsReady accepts it.
func writeReadySegment(t *testing.T, cache *SegmentCache, k SegmentKey, useFMP4 bool) {
	t.Helper()
	path := cache.Path(k, useFMP4)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, fmp4Seg(), 0o644))
}

// Producers must derive the ffmpeg audio args from the fingerprint's audio
// decision instead of hardcoding "transcode" - the regression that silently
// re-encoded every copyable (client-decodable) audio track to stereo AAC.
func TestSoftwareProducer_HonorsAudioCopy(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	p := NewSoftwareProducer(cache, proc, src, swTestOpts(), discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	fp.AudioCopy = true
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	assert.Equal(t, "copy", proc.lastOpts().AudioDecision)

	fp2 := minimalFingerprint(t)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp2, 0, 0))
	assert.Equal(t, "transcode", proc.lastOpts().AudioDecision, "default stays the historical re-encode")
}

// ============================================================================
// KillPausedAfter: reclaim ffmpeg RSS of long-paused sessions with a
// resume-cushion guard, so the player never feels the kill on resume.
// ============================================================================

// pauseAndBackdate pauses s (via the producer's real pause path, so
// s.pausedSince is set the same way production code sets it) then rewinds
// pausedSince into the past so a KillPausedAfter cutoff test doesn't need to
// sleep in real time.
func pauseAndBackdate(t *testing.T, p *SoftwareProducer, s *swSession, age time.Duration) {
	t.Helper()
	p.pauseEach([]*swSession{s})
	s.mu.Lock()
	require.True(t, s.paused, "pauseEach must have suspended the session")
	s.pausedSince = time.Now().Add(-age)
	s.mu.Unlock()
}

func TestSoftwareProducer_KillPausedAfter_CushionSufficient_KillsAndPreservesSegments(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := swTestOpts()
	opts.KillPausedAfter = time.Minute
	opts.ResumeCushionSegs = 2
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, opts, discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	hash := fp.Hash()
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))

	p.mu.Lock()
	s := p.sessions[hash][0]
	p.mu.Unlock()
	handle := s.handle.(*fakeHandle)

	// The cushion: ResumeCushionSegs (2) segments already sitting on disk beyond
	// the session's frontier (0) - segments 1 and 2.
	writeReadySegment(t, cache, SegmentKey{DecisionHash: hash, VariantIdx: 0, SegmentIdx: 1}, fp.UseFMP4)
	writeReadySegment(t, cache, SegmentKey{DecisionHash: hash, VariantIdx: 0, SegmentIdx: 2}, fp.UseFMP4)

	pauseAndBackdate(t, p, s, 2*time.Minute) // paused well past KillPausedAfter

	p.sweepKillPaused()

	assert.True(t, handle.isStopped(), "a paused session past KillPausedAfter with a full cushion must be killed to free its ffmpeg RSS")
	assert.Equal(t, 0, p.ActiveSessionCount())
	assert.True(t, cache.IsReady(SegmentKey{DecisionHash: hash, VariantIdx: 0, SegmentIdx: 1}, fp.UseFMP4),
		"killing a cushioned paused session must NOT wipe its disk segments (CleanupHash must not run here)")
	assert.True(t, cache.IsReady(SegmentKey{DecisionHash: hash, VariantIdx: 0, SegmentIdx: 2}, fp.UseFMP4),
		"the whole cushion must survive the kill for an instant-feeling resume")
}

func TestSoftwareProducer_KillPausedAfter_CushionInsufficient_ResumesInstead(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := swTestOpts()
	opts.KillPausedAfter = time.Minute
	opts.ResumeCushionSegs = 2
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, opts, discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	hash := fp.Hash()
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))

	p.mu.Lock()
	s := p.sessions[hash][0]
	p.mu.Unlock()
	handle := s.handle.(*fakeHandle)

	// Only 1 of the 2 required cushion segments is on disk.
	writeReadySegment(t, cache, SegmentKey{DecisionHash: hash, VariantIdx: 0, SegmentIdx: 1}, fp.UseFMP4)

	pauseAndBackdate(t, p, s, 2*time.Minute)

	p.sweepKillPaused()

	assert.False(t, handle.isStopped(), "an insufficient cushion must NOT kill the session - the resume would be cold")
	_, resumeN, paused := handle.counts()
	assert.False(t, paused, "the session must be SIGCONTed to build the missing cushion quickly")
	assert.GreaterOrEqual(t, resumeN, 1)
	assert.Equal(t, 1, p.ActiveSessionCount())
}

// ============================================================================
// Shared video-session capacity limiter (VideoSessionLimiter)
// ============================================================================

func TestSoftwareProducer_VideoLimiter_TranscodeGatedAtCapacity(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	limiter := NewVideoSessionLimiter(1)
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	p.WithVideoSessionLimiter(limiter)
	defer p.Close()

	fp1 := minimalFingerprint(t)
	fp2 := minimalFingerprint(t)
	fp2.MediaID = "media-other" // distinct hash => distinct encoder

	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp1, 0, 0))
	assert.Equal(t, 1, limiter.Active())

	err := p.EnsureSegmentRange(context.Background(), fp2, 0, 0)
	require.ErrorIs(t, err, ErrVideoCapacityExhausted)
	assert.Equal(t, 1, p.ActiveSessionCount(), "the refused spawn must not leave a live session")
	assert.Equal(t, 1, limiter.Active())
}

func TestSoftwareProducer_VideoLimiter_ReleasedOnNaturalExit(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	limiter := NewVideoSessionLimiter(1)
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	p.WithVideoSessionLimiter(limiter)
	defer p.Close()

	fp1 := minimalFingerprint(t)
	hash := fp1.Hash()
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp1, 0, 0))
	require.Equal(t, 1, limiter.Active())

	p.mu.Lock()
	handle := p.sessions[hash][0].handle.(*fakeHandle)
	p.mu.Unlock()
	require.NoError(t, handle.Stop()) // simulate the ffmpeg process exiting on its own

	require.Eventually(t, func() bool {
		return limiter.Active() == 0
	}, eventuallyWait, 10*time.Millisecond, "the reap goroutine must free the video slot on natural exit")

	fp2 := minimalFingerprint(t)
	fp2.MediaID = "media-other"
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp2, 0, 0),
		"the freed slot must admit a fresh transcode session")
}

func TestSoftwareProducer_VideoLimiter_ReleasedOnKill(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	limiter := NewVideoSessionLimiter(1)
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	p.WithVideoSessionLimiter(limiter)
	defer p.Close()

	fp1 := minimalFingerprint(t)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp1, 0, 0))
	require.Equal(t, 1, limiter.Active())

	p.RemoveSession(fp1.MediaID)
	assert.Equal(t, 0, limiter.Active(), "an explicit kill must free the video slot")
	assert.Equal(t, 0, p.ActiveSessionCount())
}

func TestSoftwareProducer_VideoLimiter_CopyInstanceNeverAcquires(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	limiter := NewVideoSessionLimiter(1)
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/media/test.mkv"}, swTestOpts(), discardLogger())
	// Defensively wired with the SAME limiter a real deployment would share
	// with the transcode instance - a copy session must still never acquire.
	p.WithVideoSessionLimiter(limiter)
	streams := &fakeCopyStreams{streams: videoStreamWithKeyframes([]int64{0, 10000})}
	items := &fakeCopyItems{item: itemWithDurationMs(20000)}
	p.SetCopyMode(NewCopyPlanner(streams, items), nil)
	defer p.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	assert.Equal(t, 1, p.ActiveSessionCount(), "the copy session did start")
	assert.Equal(t, 0, limiter.Active(), "a video-COPY session never encodes video and must never gate on the limiter")
}

func TestSoftwareProducer_VideoLimiter_Unwired_Unlimited(t *testing.T) {
	// No WithVideoSessionLimiter call at all - byte-identical to pre-feature
	// behaviour: never ErrVideoCapacityExhausted, no matter how many distinct
	// hashes spawn.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	for i := range 5 {
		fp := minimalFingerprint(t)
		fp.MediaID = fmt.Sprintf("media-%d", i)
		require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	}
	assert.Equal(t, 5, p.ActiveSessionCount())
}

func TestSoftwareProducer_VideoLimiter_SpawnRaceLoserReleasesExactlyOnce(t *testing.T) {
	// -race: many goroutines racing to spawn a session for the exact same
	// (hash, variant, segment). Exactly one wins; every loser must release its
	// acquired slot via the "spawn race lost" early-return path, never via the
	// reap goroutine (it never gets that far), and never double-release.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	limiter := NewVideoSessionLimiter(0) // unlimited - isolates the release bookkeeping from acquire refusals
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	p.WithVideoSessionLimiter(limiter)
	defer p.Close()

	fp := minimalFingerprint(t)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			_ = p.EnsureSegmentRange(context.Background(), fp, 0, 0)
		})
	}
	wg.Wait()

	assert.Equal(t, 1, p.ActiveSessionCount(), "exactly one session wins the spawn race")
	assert.Equal(t, 1, limiter.Active(), "every loser released its slot; only the winner still holds one")
}

func TestSoftwareProducer_KillPausedAfterZero_NeverKillsPaused(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := swTestOpts()
	// KillPausedAfter left at its zero value: disabled, current behaviour intact.
	require.Zero(t, opts.KillPausedAfter)
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, opts, discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	hash := fp.Hash()
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))

	p.mu.Lock()
	s := p.sessions[hash][0]
	p.mu.Unlock()
	handle := s.handle.(*fakeHandle)

	// A huge cushion, paused for a very long time - would qualify for a kill
	// under any nonzero KillPausedAfter.
	writeReadySegment(t, cache, SegmentKey{DecisionHash: hash, VariantIdx: 0, SegmentIdx: 1}, fp.UseFMP4)
	writeReadySegment(t, cache, SegmentKey{DecisionHash: hash, VariantIdx: 0, SegmentIdx: 2}, fp.UseFMP4)
	pauseAndBackdate(t, p, s, 24*time.Hour)

	p.sweepKillPaused()

	assert.False(t, handle.isStopped(), "KillPausedAfter=0 must disable the paused-kill sweep entirely")
	assert.Equal(t, 1, p.ActiveSessionCount())
}

// ============================================================================
// Idle sweep vs concurrent session registration (pendingRefs guard)
// ============================================================================

func TestSoftwareProducer_SweepIdle_SparesHashWithLiveCacheRef(t *testing.T) {
	// The race sweepIdle used to lose: a fresh session REGISTERS at the cache
	// (pendingRefs>0) in the window between the sweeper's "no sibling left"
	// check and its disk cleanup. The reclaim must skip such a hash - deleting
	// the directory under the newcomer makes its ffmpeg rename fail (404/stall).
	cache, dir := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	p := NewSoftwareProducer(cache, proc, src, swTestOpts(), discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	hash := fp.Hash()

	hashDir := filepath.Join(dir, "segments", hash)
	require.NoError(t, os.MkdirAll(hashDir, 0o755))
	marker := filepath.Join(hashDir, "stream_0_000.m4s")
	require.NoError(t, os.WriteFile(marker, []byte("seg"), 0o644))

	// Age the producer's session past the inactivity cutoff so the sweep
	// kills it and finds the hash sibling-free.
	p.mu.Lock()
	for _, list := range p.sessions {
		for _, s := range list {
			s.mu.Lock()
			s.lastAccess = time.Now().Add(-2 * p.opts.InactivityTimeout)
			s.mu.Unlock()
		}
	}
	p.mu.Unlock()

	// The concurrent newcomer: registered at the cache, not yet visible in
	// p.sessions (exactly the sweep window).
	release := cache.RegisterSession(hash)
	defer release()

	p.sweepIdle()

	_, err := os.Stat(marker)
	require.NoError(t, err, "sweepIdle must not reclaim a hash a registered session is about to serve")
}

func TestSoftwareProducer_SweepIdle_ReclaimsFullyIdleHash(t *testing.T) {
	// Control arm: with no live cache registration left, the sweep must still
	// reclaim the directory (the fix must not turn the sweeper into a no-op).
	cache, dir := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	p := NewSoftwareProducer(cache, proc, src, swTestOpts(), discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	hash := fp.Hash()

	hashDir := filepath.Join(dir, "segments", hash)
	require.NoError(t, os.MkdirAll(hashDir, 0o755))
	marker := filepath.Join(hashDir, "stream_0_000.m4s")
	require.NoError(t, os.WriteFile(marker, []byte("seg"), 0o644))

	p.mu.Lock()
	for _, list := range p.sessions {
		for _, s := range list {
			s.mu.Lock()
			s.lastAccess = time.Now().Add(-2 * p.opts.InactivityTimeout)
			s.mu.Unlock()
		}
	}
	p.mu.Unlock()

	p.sweepIdle()

	_, err := os.Stat(hashDir)
	require.True(t, os.IsNotExist(err), "a fully idle hash must still be reclaimed by the sweep")
}

// ============================================================================
// Cross-rung spawn guard
// ============================================================================

// An ABR up-switch probe used to SPAWN a second encoder for another rung, hit
// the per-hash cap and evict the rung the viewer was WATCHING - lru_cap
// kill/respawn every ~20s all film long (prod 2026-07-29, 4K source). While
// every live sibling is HOT and none serves the requested rung, the newcomer
// is refused FAST: the player's probe gets its 503 immediately and stays on
// its rung; the viewer's encoder is never yanked.
func TestSoftwareProducer_HotSiblingRungRefusesACrossRungSpawn(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := swTestOpts()
	opts.MaxSessionsPerHash = 1
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, opts, discardLogger())
	defer p.Close()

	fp := threeRungFingerprint(t)
	hash := fp.Hash()
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.Equal(t, 1, proc.startCount())
	// Establish the viewer's rung: past the newborn shield, freshly accessed.
	p.mu.Lock()
	for _, s := range p.sessions[hash] {
		s.mu.Lock()
		s.spawnedAt = s.spawnedAt.Add(-time.Hour)
		s.lastAccess = time.Now()
		s.mu.Unlock()
	}
	p.mu.Unlock()

	// The viewer's rung is HOT and ESTABLISHED: the probe is refused.
	err := p.EnsureSegmentRange(context.Background(), fp, 2, 0)
	require.ErrorIs(t, err, ErrRungBusy, "a cross-rung probe must not evict a hot encoder")
	assert.Equal(t, 1, proc.startCount(), "no second encoder was spawned")
	assert.Equal(t, 1, p.ActiveSessionCount())

	// Once the sibling has gone idle past the protection window, a cross-rung
	// request is a genuine switch: normal spawn (and LRU eviction) applies.
	p.mu.Lock()
	for _, s := range p.sessions[hash] {
		s.mu.Lock()
		s.lastAccess = s.lastAccess.Add(-time.Hour)
		s.spawnedAt = s.spawnedAt.Add(-time.Hour)
		s.mu.Unlock()
	}
	p.mu.Unlock()
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 2, 0))
	assert.Equal(t, 2, proc.startCount(), "idle sibling: the rung switch spawns normally")
}

// The guard is CROSS-rung only: a same-variant deep forward seek keeps
// today's spawn-alongside behaviour even while the sibling is hot.
func TestSoftwareProducer_SameRungSeekStillSpawnsWhileHot(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := swTestOpts()
	opts.MaxSessionsPerHash = 2
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, opts, discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 500))
	assert.Equal(t, 2, proc.startCount(), "a deep same-rung seek spawns alongside, hot or not")
}

// The HDR ceiling only ever LOWERS the software ceiling, and only once benched.
// Both directions matter: a 0 means "never measured" (the SDR ceiling stands),
// and a value at or above the SDR ceiling must not raise it - HDR software is
// strictly heavier than SDR, so it can never sustain more.
func TestSoftwareProducer_ProfilesHDRCeilingOnlyLowers(t *testing.T) {
	cache, _ := newTestCache(t)
	p := NewSoftwareProducer(cache, newFakeProcess(), stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	base := transcoding.LadderFilmNormal // [360, 480, 720, 1080]
	src := SourceDims{Width: 3840, Height: 2160, TenBit: true}
	tonemap := transcoding.PlaybackDecision{TranscodeReasons: []string{"hdr_tonemapping_needed"}}

	topOf := func(caps transcoding.EncodeCapability) int {
		profiles := p.Profiles(base, src, tonemap, caps)
		require.NotEmpty(t, profiles)
		return profiles[len(profiles)-1].Height
	}

	assert.Equal(t, 1080, topOf(transcoding.EncodeCapability{SoftwareMaxHeight: 1080}),
		"an unbenched HDR ceiling (0) must leave the SDR ceiling standing")
	// Documented, not mutation-covered: at the boundary the assignment is a
	// no-op by construction (maxH = maxH), so `<` and `<=` are equivalent here.
	// The case stays because it pins the intent for a future refactor where the
	// assignment stops being a no-op.
	assert.Equal(t, 1080, topOf(transcoding.EncodeCapability{SoftwareMaxHeight: 1080, SoftwareHDRMaxHeight: 1080}),
		"an HDR ceiling equal to the SDR one changes nothing")
	assert.Equal(t, 1080, topOf(transcoding.EncodeCapability{SoftwareMaxHeight: 1080, SoftwareHDRMaxHeight: 2160}),
		"the HDR ceiling must never RAISE the ladder - HDR software is heavier, not lighter")
	assert.Equal(t, 720, topOf(transcoding.EncodeCapability{SoftwareMaxHeight: 1080, SoftwareHDRMaxHeight: 720}),
		"a lower benched HDR ceiling is the one that holds")
}

// The HDR ceiling is gated on the DECISION asking for tone-mapping, not on the
// source being 10-bit: a 10-bit source served to an HDR-capable client is not
// tone-mapped, and capping it at the HDR ceiling would needlessly downgrade it.
func TestSoftwareProducer_ProfilesIgnoresHDRCeilingWithoutTonemap(t *testing.T) {
	cache, _ := newTestCache(t)
	p := NewSoftwareProducer(cache, newFakeProcess(), stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	profiles := p.Profiles(
		transcoding.LadderFilmNormal,
		SourceDims{Width: 3840, Height: 2160, TenBit: true},
		transcoding.PlaybackDecision{}, // no hdr_tonemapping_needed
		transcoding.EncodeCapability{SoftwareMaxHeight: 1080, SoftwareHDRMaxHeight: 480},
	)

	require.NotEmpty(t, profiles)
	assert.Equal(t, 1080, profiles[len(profiles)-1].Height,
		"no tone-mapping requested ⇒ the HDR ceiling does not apply")
}

// The copy rung's Name/Width/Height ride the cache fingerprint. A source whose
// dimensions were never probed must still yield a DETERMINISTIC rung, or the
// manifest and the segment fetch hash differently and the cache misses forever.
func TestSoftwareProducer_ProfilesCopyRungStaysDeterministicWithoutSourceDims(t *testing.T) {
	cache, _ := newTestCache(t)
	p := NewSoftwareProducer(cache, newFakeProcess(), stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()
	p.SetCopyMode(NewCopyPlanner(nil, nil), nil)

	base := transcoding.LadderFilmNormal
	caps := transcoding.EncodeCapability{SoftwareMaxHeight: 1080}

	for _, src := range []SourceDims{
		{Width: 0, Height: 0},
		{Width: 1920, Height: 0},
		{Width: 0, Height: 1080},
		{Width: -1, Height: -1},
	} {
		profiles := p.Profiles(base, src, transcoding.PlaybackDecision{}, caps)
		require.Len(t, profiles, 1)
		assert.Equal(t, 1920, profiles[0].Width, "src %+v must fall back to the safe default", src)
		assert.Equal(t, 1080, profiles[0].Height, "src %+v must fall back to the safe default", src)
		assert.True(t, profiles[0].Copy)
	}
}
