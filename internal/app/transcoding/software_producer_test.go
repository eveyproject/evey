package transcoding

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/transcoding"
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

// ============================================================================
// One process per hash; ReadRate pacing
// ============================================================================

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
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 10))
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 20))

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
		"the software producer MUST input-pace ffmpeg (-readrate 1.5) — the defining software-flow knob")
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
		"copy sessions pace at 8x — unpaced lets video outrun the audio re-encode (A/V desync), 1.5x starves the client")
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
// at the exact -ss time) stays true — the measured post-deep-seek A/V desync.
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
		"-ss must stay the PROBED target — re-deriving it from the landing re-triggers the seek rules one keyframe earlier")
}

// Landing probe failure (or an exact landing) keeps the requested start — the
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
	}{
		{0.0, 0},
		{10.0, 1},  // exact boundary
		{9.95, 1},  // boundary within rounding tolerance
		{16.4, 2},  // mid-segment landing → containing segment
		{22.99, 3}, // within 100ms of the next boundary → snaps forward
		{29.0, 3},  // inside the tail
	}
	for _, tc := range cases {
		if got := planIndexForLanding(segs, tc.landing); got != tc.want {
			t.Fatalf("planIndexForLanding(%v) = %d, want %d", tc.landing, got, tc.want)
		}
	}
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

	// Request segment 5 — before the live window [100, +inf). The producer does
	// NOT kill the running encoder (that would yank a second tab sitting at 100);
	// it spawns a fresh one ALONGSIDE at 5. Two encoders now coexist.
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 5))
	assert.Equal(t, 2, proc.startCount(), "uncovered backward position spawns a second encoder")
	assert.Equal(t, 2, p.ActiveSessionCount(), "both encoders coexist — the first is NOT killed")
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
	// A SMALL forward jump stays within the seek-reach window — the sequential
	// encoder is already producing toward it — so no restart. (A far jump spawns a
	// fresh seek encoder instead: see ForwardSeekBeyondReachSpawnsSeekEncoder.)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 8))
	assert.Equal(t, 1, proc.startCount(), "an in-reach forward seek reuses the running encoder")
}

// ============================================================================
// Never ErrCapacityExceeded — a second media never sheds the first
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
		// require.NoError implicitly proves it's not ErrCapacityExceeded — the
		// software producer has no global session cap, so it never rejects.
		require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0),
			"software producer must never reject on media %d", i)
	}
	assert.Equal(t, 8, p.ActiveSessionCount(), "all eight distinct media run concurrently — no shedding")
}

// ============================================================================
// Multi-tab coexistence: two viewers, same film, different positions
// ============================================================================

func TestSoftwareProducer_TwoTabsDifferentPositionsCoexist(t *testing.T) {
	// The user's scenario: a deep viewer (tab 1, resumed at segment 500) and a
	// second viewer (tab 2) cold-starting the SAME film from segment 0 — identical
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
		"the deep viewer's encoder must NOT be killed by tab 2 cold-starting — the multi-tab guarantee")

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
	// covered by the first's forward-running encoder — they share it, so the weak
	// NAS pays for exactly one encode, not two. The multi-session cost is only
	// incurred for genuinely-distinct uncached positions.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))  // second viewer, same start
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 10)) // a beat ahead, within the seek-reach window → shared
	assert.Equal(t, 1, proc.startCount(), "viewers at or just ahead of the live start share ONE encoder")
	assert.Equal(t, 1, p.ActiveSessionCount())
}

func TestSoftwareProducer_ForwardSeekBeyondReachSpawnsSeekEncoder(t *testing.T) {
	// A forward seek that lands far past the live encoder's production frontier
	// must NOT reuse the sequential encoder — paced at 1.5× it would take minutes
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

	// Seek to ~8.5 min in (segment 169) — far beyond SeekReachWindowSegs.
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
	// SIGSTOPped immediately (not left contending for PauseIdleThreshold seconds —
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
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 1, 5)) // variant 1 — must NOT reuse variant 0

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
	// Two in-reach requests on the SAME rung still share one encoder — the variant
	// filter must not break normal same-variant reuse / frontier advance.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	fp := twoRungFingerprint(t)
	hash := fp.Hash()

	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 1, 0))  // variant 1
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 1, 10)) // same rung, in-reach
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
	assert.Equal(t, 10, s.frontier, "the reused variant-1 encoder's frontier advanced to the request")
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
	// so two full 4K decodes don't run concurrently on a weak box — pause, not kill,
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

	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0)) // rung 0 (oldest)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 1, 0)) // rung 1
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

func TestSoftwareProducer_MaxSessionsPerHashEvictsLRU(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	opts := swTestOpts()
	opts.MaxSessionsPerHash = 2
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/m.mkv"}, opts, discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	hash := fp.Hash()
	// Three DESCENDING positions: each is below every existing start, so each
	// spawns its own encoder. The 3rd crosses MaxSessionsPerHash=2 → the LRU
	// (the oldest-touched, segment 300) is evicted.
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 300))
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 200))
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
		"software WaitBudget must be long — the single paced encoder is slow to land seg 0 and the client waits")
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
	// Jellyfin remux: direct-play quality, no transcode ladder). The planner is
	// never called by Profiles — only its presence flips the branch.
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
			"every HDR rung obeys SoftwareHDRMaxHeight (NOT the 1080p SDR ceiling) — the honest weak-NAS HDR cap")
	}
	assert.Equal(t, 360, profiles[0].Height, "the 360p floor stays for Auto to fall back to")
	assert.Equal(t, 480, profiles[len(profiles)-1].Height, "the HDR top rung is the HDR ceiling")
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
	}, time.Second, 10*time.Millisecond, "idle session must be GC-reaped")
}

// writeReadySegment writes a structurally valid fMP4 segment so IsReady accepts it.
func writeReadySegment(t *testing.T, cache *SegmentCache, k SegmentKey, useFMP4 bool) {
	t.Helper()
	path := cache.Path(k, useFMP4)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, fmp4Seg(), 0o644))
}
