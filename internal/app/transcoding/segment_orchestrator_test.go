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
	mu      sync.Mutex
	stopped bool
	done    chan struct{}
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
func (h *fakeHandle) Pause() error          { return nil }
func (h *fakeHandle) Resume() error         { return nil }

type stubSource struct {
	path    string
	streams []SourceStream
	err     error
}

func (s stubSource) ResolveSource(_ context.Context, _ DecisionFingerprint) (string, []SourceStream, error) {
	return s.path, s.streams, s.err
}
