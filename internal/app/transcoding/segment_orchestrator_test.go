package transcoding

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// ============================================================================
// EnsureSegmentRange: spawning, cache hit, race
// ============================================================================

func TestOrchestrator_EnsureSegmentRange_SpawnsFFmpegOnMiss(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0))

	assert.Equal(t, 1, o.ActiveCount())
	assert.Equal(t, 1, proc.startCount())
	// StartSec must match segmentIdx × segmentSeconds.
	assert.Equal(t, 0.0, proc.lastOpts().StartSec)
}

func TestOrchestrator_EnsureSegmentRange_NoSpawnOnCacheHit(t *testing.T) {
	// If the segment is already on disk under the right key, the orchestrator
	// must NOT start a fresh ffmpeg — that's the whole point of the cache.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	k := SegmentKey{DecisionHash: fp.Hash(), VariantIdx: 0, SegmentIdx: 5}
	require.NoError(t, os.MkdirAll(filepath.Dir(cache.Path(k, fp.UseFMP4)), 0o755))
	require.NoError(t, os.WriteFile(cache.Path(k, fp.UseFMP4), []byte("seg"), 0o644))

	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 5))
	assert.Equal(t, 0, proc.startCount())
}

func TestOrchestrator_EnsureSegmentRange_SecondCallSameWindowNoRespawn(t *testing.T) {
	// A second request inside the live session's [startSeg, endSeg] window
	// must NOT spawn another ffmpeg — just bump lastAccess.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0))
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 10))
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 29))

	assert.Equal(t, 1, proc.startCount(), "all three calls fall within [0, 29] of the initial rafale")
}

// ============================================================================
// EnsureSegmentRange: cancel-on-seek
// ============================================================================

func TestOrchestrator_EnsureSegmentRange_CancelOnSeek(t *testing.T) {
	// A request OUTSIDE the active window must kill the existing ffmpeg and
	// spawn a fresh one at the new offset (the audit's hls-media-server
	// pattern).
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0))
	require.Equal(t, 1, proc.startCount())

	// Seek to segment 500 → far outside window [0, 29].
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 500))

	// Eventual consistency: the killed session's reaper goroutine deletes the
	// old entry async. Wait until ActiveCount stabilises at 1.
	require.Eventually(t, func() bool {
		return o.ActiveCount() == 1 && proc.startCount() == 2
	}, time.Second, 10*time.Millisecond)

	// The new spawn's StartSec reflects the seek target.
	assert.Equal(t, 500.0*3.0, proc.lastOpts().StartSec)
}

// ============================================================================
// Inactivity GC
// ============================================================================

func TestOrchestrator_InactivityGC_ReapsIdleSessions(t *testing.T) {
	// With a tiny InactivityTimeout, a spawned session is reaped soon after if
	// nothing touches it. Validates the GC sweeper runs and acts.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	opts := testOpts()
	opts.InactivityTimeout = 50 * time.Millisecond
	opts.GCInterval = 20 * time.Millisecond
	o := NewSegmentOrchestrator(cache, proc, src, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0))
	require.Equal(t, 1, o.ActiveCount())

	require.Eventually(t, func() bool {
		return o.ActiveCount() == 0
	}, time.Second, 10*time.Millisecond, "GC must reap the idle session")
}

func TestOrchestrator_InactivityGC_KeepsLiveSessions(t *testing.T) {
	// A session whose lastAccess is bumped within the inactivity window MUST
	// survive the GC sweep. Otherwise actively-streamed sessions would die.
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	opts := testOpts()
	opts.InactivityTimeout = 150 * time.Millisecond
	opts.GCInterval = 30 * time.Millisecond
	o := NewSegmentOrchestrator(cache, proc, src, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0))

	// Touch it repeatedly under the timeout — must stay alive.
	stop := time.After(400 * time.Millisecond)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			assert.Equal(t, 1, o.ActiveCount())
			return
		case <-tick.C:
			require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0))
		}
	}
}

// ============================================================================
// Close
// ============================================================================

// ============================================================================
// Throttle: pause-when-ahead policy
// ============================================================================

func TestOrchestrator_Throttle_PausesWhenClientFarBehind(t *testing.T) {
	cache, dir := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	opts := testOpts()
	opts.ThrottleEnabled = true
	opts.ThrottlePauseAhead = 6
	opts.ThrottleResumeBelow = 2
	opts.ThrottleTickInterval = 50 * time.Millisecond
	o := NewSegmentOrchestrator(cache, proc, src, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0))
	require.Equal(t, 1, o.ActiveCount())

	// Simulate ffmpeg producing 10 segments while the client is still at 0.
	// useFMP4=true (matches minimalFingerprint) → .m4s.
	hashDir := filepath.Join(dir, "segments", fp.Hash())
	require.NoError(t, os.MkdirAll(hashDir, 0o755))
	for i := 0; i < 10; i++ {
		name := filepath.Join(hashDir, formatVariantSegment(0, i, ".m4s"))
		require.NoError(t, os.WriteFile(name, []byte("xx"), 0o644))
	}

	// Reach into the session to retrieve the handle for assertion.
	o.mu.Lock()
	sess := o.sessions[fp.Hash()]
	o.mu.Unlock()
	require.NotNil(t, sess)
	handle := sess.handle.(*fakeHandle)

	// Wait up to 1s for the throttle loop to detect the buffer and pause.
	require.Eventually(t, func() bool {
		p, _, paused := handle.counts()
		return p >= 1 && paused
	}, time.Second, 20*time.Millisecond, "throttle should pause ffmpeg when produced - client >= PauseAhead")

	// Client catches up to within ResumeBelow → throttle must resume.
	o.touchSession(fp.Hash(), 9)

	require.Eventually(t, func() bool {
		_, r, paused := handle.counts()
		return r >= 1 && !paused
	}, time.Second, 20*time.Millisecond, "throttle should resume ffmpeg when client catches up")
}

func TestOrchestrator_NotifyClientHint_ResumesPausedOnStarvation(t *testing.T) {
	cache, dir := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	opts := testOpts()
	opts.ThrottleEnabled = true
	opts.ThrottlePauseAhead = 4
	opts.ThrottleResumeBelow = 2
	opts.ThrottleTickInterval = 50 * time.Millisecond
	o := NewSegmentOrchestrator(cache, proc, src, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0))

	hashDir := filepath.Join(dir, "segments", fp.Hash())
	require.NoError(t, os.MkdirAll(hashDir, 0o755))
	for i := 0; i < 8; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(hashDir, formatVariantSegment(0, i, ".m4s")), []byte("x"), 0o644))
	}
	o.mu.Lock()
	sess := o.sessions[fp.Hash()]
	o.mu.Unlock()
	handle := sess.handle.(*fakeHandle)

	require.Eventually(t, func() bool { _, _, paused := handle.counts(); return paused }, time.Second, 10*time.Millisecond)

	// CMCD bs=1 arrives — we must SIGCONT immediately, not wait the tick.
	o.NotifyClientHint(fp.MediaID, 0, true, 1)

	_, resumes, paused := handle.counts()
	assert.GreaterOrEqual(t, resumes, 1, "starvation hint must resume")
	assert.False(t, paused, "starvation hint must clear paused state")
}

func TestOrchestrator_NotifyClientHint_NoOpOnUnknownMedia(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())
	defer o.Close()

	o.NotifyClientHint("nobody-here", 12000, true, 5)
	assert.Equal(t, 0, o.ActiveCount())
}

func TestOrchestrator_NotifyClientHint_ExtendsEndSegOnFatBuffer(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	opts := testOpts()
	opts.LookAheadSegments = 10
	opts.SegmentSeconds = 3.0
	o := NewSegmentOrchestrator(cache, proc, src, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0))

	o.mu.Lock()
	sess := o.sessions[fp.Hash()]
	o.mu.Unlock()
	sess.mu.Lock()
	endBefore := sess.endSeg
	sess.mu.Unlock()

	// 45s buffer ≥ LookAhead × segmentSeconds (10×3=30s) → stretch endSeg.
	o.NotifyClientHint(fp.MediaID, 45000, false, 0)

	sess.mu.Lock()
	endAfter := sess.endSeg
	sess.mu.Unlock()
	assert.Greater(t, endAfter, endBefore, "fat buffer hint should stretch endSeg")
}

func TestOrchestrator_Throttle_NoFlapWithHysteresis(t *testing.T) {
	cache, dir := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	opts := testOpts()
	opts.ThrottleEnabled = true
	opts.ThrottlePauseAhead = 4
	opts.ThrottleResumeBelow = 2
	opts.ThrottleTickInterval = 30 * time.Millisecond
	o := NewSegmentOrchestrator(cache, proc, src, opts, discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0))

	hashDir := filepath.Join(dir, "segments", fp.Hash())
	require.NoError(t, os.MkdirAll(hashDir, 0o755))
	for i := 0; i < 5; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(hashDir, formatVariantSegment(0, i, ".m4s")), []byte("x"), 0o644))
	}

	o.mu.Lock()
	sess := o.sessions[fp.Hash()]
	o.mu.Unlock()
	handle := sess.handle.(*fakeHandle)

	// Hold steady at buffer=5-0=5: ≥ PauseAhead(4), so we pause exactly once.
	// Many ticks must NOT produce many pauses (hysteresis: stay paused).
	require.Eventually(t, func() bool { p, _, _ := handle.counts(); return p >= 1 }, time.Second, 10*time.Millisecond)
	time.Sleep(200 * time.Millisecond)
	p, r, _ := handle.counts()
	assert.Equal(t, 1, p, "no double-pause while already paused")
	assert.Equal(t, 0, r, "no resume while buffer stays above ResumeBelow")
}

func formatVariantSegment(variantIdx, segIdx int, ext string) string {
	return "stream_" + itoa(variantIdx) + "_" + zeroPad3(segIdx) + ext
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [10]byte
	n := 0
	for i > 0 {
		b[n] = byte('0' + i%10)
		i /= 10
		n++
	}
	out := make([]byte, n)
	for j := 0; j < n; j++ {
		out[j] = b[n-1-j]
	}
	return string(out)
}

func zeroPad3(i int) string {
	if i < 10 {
		return "00" + itoa(i)
	}
	if i < 100 {
		return "0" + itoa(i)
	}
	return itoa(i)
}

func TestOrchestrator_Close_KillsAllSessions(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0))
	require.Equal(t, 1, o.ActiveCount())

	o.Close()
	assert.Equal(t, 0, o.ActiveCount())
}

func TestOrchestrator_Close_CleansDiskSegments(t *testing.T) {
	// The auditor's concern: a graceful shutdown must purge the segments of
	// every live session — cancel-on-seek keeps them warm because rewind
	// might hit them, but Close has no "future" to keep them around for.
	cache, dir := newTestCache(t)
	proc := newFakeProcess()
	src := stubSource{path: "/media/test.mkv"}
	o := NewSegmentOrchestrator(cache, proc, src, testOpts(), discardLogger())

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0))
	hashDir := filepath.Join(dir, "segments", fp.Hash())
	require.NoError(t, os.MkdirAll(hashDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hashDir, "stream_0_000.ts"), []byte("xx"), 0o644))

	o.Close()

	_, err := os.Stat(hashDir)
	assert.True(t, os.IsNotExist(err), "Close must remove the hash directory")
}

// ============================================================================
// fakes
// ============================================================================

func testOpts() OrchestratorOptions {
	return OrchestratorOptions{
		LookAheadSegments: 30,
		InactivityTimeout: 1 * time.Minute,
		GCInterval:        100 * time.Millisecond,
		SegmentSeconds:    3.0,
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeProcess implements transcoding.HLSProcess for tests. Records every Start
// call. The handle it returns blocks on Stop, mimicking a real ffmpeg that
// runs forever until killed.
type fakeProcess struct {
	mu      sync.Mutex
	started []transcoding.HLSProcessOpts
	count   atomic.Int32
}

func newFakeProcess() *fakeProcess { return &fakeProcess{} }

func (p *fakeProcess) Start(_ context.Context, opts transcoding.HLSProcessOpts) (transcoding.HLSProcessHandle, error) {
	p.mu.Lock()
	p.started = append(p.started, opts)
	p.mu.Unlock()
	p.count.Add(1)
	h := &fakeHandle{done: make(chan struct{})}
	return h, nil
}

func (p *fakeProcess) startCount() int { return int(p.count.Load()) }

func (p *fakeProcess) lastOpts() transcoding.HLSProcessOpts {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.started) == 0 {
		return transcoding.HLSProcessOpts{}
	}
	return p.started[len(p.started)-1]
}

type fakeHandle struct {
	mu       sync.Mutex
	stopped  bool
	pauseN   int
	resumeN  int
	paused   bool
	done     chan struct{}
}

func (h *fakeHandle) WaitReady(_ context.Context) error { return nil }
func (h *fakeHandle) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.stopped {
		h.stopped = true
		close(h.done)
	}
	return nil
}
func (h *fakeHandle) Done() <-chan struct{} { return h.done }
func (h *fakeHandle) Err() error            { return nil }
func (h *fakeHandle) Pause() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pauseN++
	h.paused = true
	return nil
}
func (h *fakeHandle) Resume() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.resumeN++
	h.paused = false
	return nil
}
func (h *fakeHandle) counts() (pauseN, resumeN int, paused bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.pauseN, h.resumeN, h.paused
}

type stubSource struct {
	path    string
	streams []SourceStream
	err     error
}

func (s stubSource) ResolveSource(_ context.Context, _ DecisionFingerprint) (string, []SourceStream, error) {
	return s.path, s.streams, s.err
}
