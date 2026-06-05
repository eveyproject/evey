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
	// Three forward requests within the same sequential stream: ONE ffmpeg.
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 10))
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 50))

	assert.Equal(t, 1, proc.startCount(), "forward requests share the single long-lived encoder")
	assert.Equal(t, 1, p.ActiveSessionCount())
}

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
// Seek = kill + restart the single process (not a second burst)
// ============================================================================

func TestSoftwareProducer_BackwardSeekRestartsSingleProcess(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	p := NewSoftwareProducer(cache, proc, src, swTestOpts(), discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	// Start at segment 100.
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 100))
	require.Equal(t, 1, proc.startCount())

	// Seek BACKWARD to segment 5 — before the live window [100, +inf). The one
	// process is killed and a fresh one started at 5. NOT a second concurrent
	// burst (that's the orchestrator's multi-rafale model).
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 5))
	assert.Equal(t, 2, proc.startCount(), "backward seek restarts the encoder")
	assert.Equal(t, 1, p.ActiveSessionCount(), "still exactly ONE live session after the seek")

	// The restarted process is seeked to segment 5 (StartSec = 5 × 3s).
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
	// A forward jump is still within [0, +inf) — the sequential encoder is
	// already producing toward it, so no restart.
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 500))
	assert.Equal(t, 1, proc.startCount(), "forward seek reuses the running encoder")
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
// Profiles = exactly one rung (single-rendition)
// ============================================================================

func TestSoftwareProducer_ProfilesCollapsesToOneRung(t *testing.T) {
	cache, _ := newTestCache(t)
	p := NewSoftwareProducer(cache, newFakeProcess(), stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	base := transcoding.LadderFilmNormal // 4 rungs up to 1080p
	src := SourceDims{Width: 3840, Height: 2160}
	capability := transcoding.EncodeCapability{SoftwareMaxHeight: 1080}

	profiles := p.Profiles(base, src, transcoding.PlaybackDecision{}, capability)
	require.Len(t, profiles, 1, "software is single-rendition: exactly one rung")
	assert.Equal(t, 1080, profiles[0].Height, "the single rung is the tallest sustainable software rung")
}

func TestSoftwareProducer_ProfilesUsesSoftwareHDRCeilingOnTonemap(t *testing.T) {
	cache, _ := newTestCache(t)
	p := NewSoftwareProducer(cache, newFakeProcess(), stubSource{path: "/m.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	base := transcoding.LadderFilmHighMotion // up to 2160p
	src := SourceDims{Width: 3840, Height: 2160, TenBit: true}
	// 1080p SDR ceiling but a much lower 480p HDR ceiling: a weak NAS cannot
	// hold 1080p HDR software (proven), so the single rung must obey the HDR cap.
	capability := transcoding.EncodeCapability{SoftwareMaxHeight: 1080, SoftwareHDRMaxHeight: 480}
	decision := transcoding.PlaybackDecision{TranscodeReasons: []string{"hdr_tonemapping_needed"}}

	profiles := p.Profiles(base, src, decision, capability)
	require.Len(t, profiles, 1)
	assert.Equal(t, 480, profiles[0].Height,
		"HDR software single rung must obey SoftwareHDRMaxHeight (NOT 1080p) — the honest weak-NAS ceiling")
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
