package transcoding

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// ============================================================================
// fakes
// ============================================================================

// fakeAudioProcess implements transcoding.AudioProcess for tests.
// Each StartAudioOnly call returns a handle with its own done channel.
// The process immediately writes the requested segment files via onStart so
// that Wait() returns quickly in most tests.
type fakeAudioProcess struct {
	mu      sync.Mutex
	opts    []transcoding.AudioProcessOpts
	count   atomic.Int32
	Err     error // if set, StartAudioOnly returns this error
	onStart func(opts transcoding.AudioProcessOpts) *audioFakeHandle
}

func newFakeAudioProcess() *fakeAudioProcess {
	return &fakeAudioProcess{}
}

func (f *fakeAudioProcess) StartAudioOnly(_ context.Context, opts transcoding.AudioProcessOpts) (transcoding.HLSProcessHandle, error) {
	f.mu.Lock()
	f.opts = append(f.opts, opts)
	cb := f.onStart
	f.mu.Unlock()
	f.count.Add(1)

	if f.Err != nil {
		return nil, f.Err
	}
	if cb != nil {
		h := cb(opts)
		if h != nil {
			return h, nil
		}
	}
	// Default: never exits on its own (handle whose done is never closed).
	return &audioFakeHandle{done: make(chan struct{})}, nil
}

func (f *fakeAudioProcess) startCount() int { return int(f.count.Load()) }

// optsAt returns the opts captured by the i-th StartAudioOnly call.
func (f *fakeAudioProcess) optsAt(i int) transcoding.AudioProcessOpts {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.opts[i]
}

// audioFakeHandle controls a simulated ffmpeg audio session.
type audioFakeHandle struct {
	mu      sync.Mutex
	stopped bool
	done    chan struct{}
}

func newAudioFakeHandle() *audioFakeHandle {
	return &audioFakeHandle{done: make(chan struct{})}
}

func (h *audioFakeHandle) WaitReady(_ context.Context) error { return nil }
func (h *audioFakeHandle) Done() <-chan struct{}             { return h.done }
func (h *audioFakeHandle) Err() error                        { return nil }
func (h *audioFakeHandle) Failure() transcoding.FailureClass { return transcoding.FailureNone }
func (h *audioFakeHandle) Pause() error                      { return nil }
func (h *audioFakeHandle) Resume() error                     { return nil }
func (h *audioFakeHandle) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.stopped {
		h.stopped = true
		close(h.done)
	}
	return nil
}

// closeHandle signals the handle as done (simulates ffmpeg process exit).
func (h *audioFakeHandle) closeHandle() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.stopped {
		h.stopped = true
		close(h.done)
	}
}

// writeAudioSegment creates the audio_T_NNN.m4s file for the given fingerprint
// and segment index, simulating what ffmpeg would produce.
func writeAudioSegment(t *testing.T, cache *SegmentCache, fp AudioFingerprint, segIdx int) {
	t.Helper()
	key := SegmentKey{
		DecisionHash: fp.Hash(),
		VariantIdx:   fp.SourceTrackIdx,
		SegmentIdx:   segIdx,
		Kind:         KindAudio,
	}
	path := cache.Path(key, true)
	assert.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	// Valid fMP4 (styp+moof+mdat) so SegmentCache.IsReady's content-box check
	// accepts it as a real audio segment (it rejects styp-only/dummy bytes).
	assert.NoError(t, os.WriteFile(path, fmp4Seg(), 0o644))
}

// minimalAudioFp returns an AudioFingerprint for tests.
func minimalAudioFp() AudioFingerprint {
	return AudioFingerprint{
		MediaID:          "mi-audio-test",
		SourcePath:       "/media/test.mkv",
		SourceTrackIdx:   0,
		AudioCodec:       "aac",
		AudioBitrate:     128,
		AudioNormalize:   false,
		MaxAudioChannels: 2,
	}
}

// ============================================================================
// Tests
// ============================================================================

// Test 1: Two successive calls on the same (fp, segIdx) launch exactly one ffmpeg.
func TestAudioOrchestrator_EnsureAudioSegment_SpawnsSessionOnce(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()

	// The fake writes the segment immediately so both calls return without timeout.
	proc := newFakeAudioProcess()
	proc.onStart = func(opts transcoding.AudioProcessOpts) *audioFakeHandle {
		writeAudioSegment(t, cache, fp, 0)
		return newAudioFakeHandle()
	}

	o := NewAudioSegmentOrchestrator(cache, proc)

	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 0))
	// Second call: segment already on disk + session already live → no respawn.
	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 0))

	assert.Equal(t, 1, proc.startCount(), "two calls on same fp must spawn exactly one ffmpeg")

	_ = o.Close()
}

// Test 2: N concurrent goroutines on the same fingerprint trigger exactly one spawn.
func TestAudioOrchestrator_EnsureAudioSegment_ConcurrentSameFp_OneSpawn(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()

	proc := newFakeAudioProcess()
	proc.onStart = func(opts transcoding.AudioProcessOpts) *audioFakeHandle {
		writeAudioSegment(t, cache, fp, 0)
		return newAudioFakeHandle()
	}

	o := NewAudioSegmentOrchestrator(cache, proc)

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			err := o.EnsureAudioSegment(context.Background(), fp, 0)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, proc.startCount(), "concurrent calls on same fp must spawn exactly one ffmpeg")

	_ = o.Close()
}

// Test 3: Two different fingerprints (different source tracks) spawn two processes.
func TestAudioOrchestrator_EnsureAudioSegment_DifferentFp_TwoSpawns(t *testing.T) {
	cache, _ := newTestCache(t)

	fp0 := minimalAudioFp()
	fp0.SourceTrackIdx = 0

	fp1 := minimalAudioFp()
	fp1.SourceTrackIdx = 1

	proc := newFakeAudioProcess()
	proc.onStart = func(opts transcoding.AudioProcessOpts) *audioFakeHandle {
		// Write audio_T_000.m4s directly into the OutputDir that the orchestrator
		// already created - avoids recomputing a fingerprint hash in the callback.
		segFile := filepath.Join(opts.OutputDir,
			SegmentKey{VariantIdx: opts.SourceTrackIdx, SegmentIdx: 0, Kind: KindAudio}.fileName(true))
		_ = os.WriteFile(segFile, fmp4Seg(), 0o644)
		return newAudioFakeHandle()
	}

	o := NewAudioSegmentOrchestrator(cache, proc)

	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp0, 0))
	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp1, 0))

	assert.Equal(t, 2, proc.startCount(), "two distinct fingerprints must spawn two ffmpeg processes")

	_ = o.Close()
}

// Test 4: EnsureAudioSegment blocks until the fake transcoder writes the segment.
func TestAudioOrchestrator_EnsureAudioSegment_BlocksUntilReady(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()

	const writeDelay = 80 * time.Millisecond

	proc := newFakeAudioProcess()
	proc.onStart = func(opts transcoding.AudioProcessOpts) *audioFakeHandle {
		h := newAudioFakeHandle()
		go func() {
			time.Sleep(writeDelay)
			writeAudioSegment(t, cache, fp, 5)
		}()
		return h
	}

	o := NewAudioSegmentOrchestrator(cache, proc)

	start := time.Now()
	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 5))
	elapsed := time.Since(start)

	assert.GreaterOrEqual(t, elapsed, writeDelay/2, "must have waited for segment to appear on disk")
	assert.Less(t, elapsed, 10*time.Second, "must not have timed out")

	_ = o.Close()
}

// Test 5: ActiveCount reflects the session lifecycle: 0 → 1 after spawn → 0 after reap.
func TestAudioOrchestrator_ActiveCount_TracksSessionLifecycle(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()

	var handle *audioFakeHandle
	proc := newFakeAudioProcess()
	proc.onStart = func(opts transcoding.AudioProcessOpts) *audioFakeHandle {
		writeAudioSegment(t, cache, fp, 0)
		h := newAudioFakeHandle()
		handle = h
		return h
	}

	o := NewAudioSegmentOrchestrator(cache, proc)

	assert.Equal(t, 0, o.ActiveCount(), "no sessions initially")

	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 0))
	assert.Equal(t, 1, o.ActiveCount(), "one session after spawn")

	// Simulate ffmpeg exiting normally.
	require.NotNil(t, handle)
	handle.closeHandle()

	require.Eventually(t, func() bool {
		return o.ActiveCount() == 0
	}, eventuallyWait, 20*time.Millisecond, "active count must reach 0 after ffmpeg exits")

	_ = o.Close()
}

// Test 6: Close stops all active sessions and the count drops to 0.
func TestAudioOrchestrator_Close_StopsAllSessions(t *testing.T) {
	cache, _ := newTestCache(t)

	fp0 := minimalAudioFp()
	fp0.SourceTrackIdx = 0

	fp1 := minimalAudioFp()
	fp1.SourceTrackIdx = 1

	proc := newFakeAudioProcess()
	proc.onStart = func(opts transcoding.AudioProcessOpts) *audioFakeHandle {
		// Write segment directly into the orchestrator-allocated output dir.
		segFile := filepath.Join(opts.OutputDir,
			SegmentKey{VariantIdx: opts.SourceTrackIdx, SegmentIdx: 0, Kind: KindAudio}.fileName(true))
		_ = os.WriteFile(segFile, fmp4Seg(), 0o644)
		return newAudioFakeHandle()
	}

	o := NewAudioSegmentOrchestrator(cache, proc)

	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp0, 0))
	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp1, 0))
	assert.Equal(t, 2, o.ActiveCount())

	require.NoError(t, o.Close())
	assert.Equal(t, 0, o.ActiveCount())
}

// A seek must start ffmpeg AT the requested position. The session used to be
// pinned to StartSec 0, so seeking to t=1800s meant re-encoding 30 minutes of
// audio before the segment appeared - the video (packaged by mkvgo, stateless)
// was there in 50 ms while the audio timed out. Measured on the hybrid rung:
// cold seek to segment 600 = HTTP 503 after 30 s.
func TestAudioOrchestrator_DeepSeek_StartsAtRequestedPosition(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()

	proc := newFakeAudioProcess()
	proc.onStart = func(opts transcoding.AudioProcessOpts) *audioFakeHandle {
		// ffmpeg writes from its own start segment onwards.
		writeAudioSegment(t, cache, fp, opts.StartNumber)
		return newAudioFakeHandle()
	}
	o := NewAudioSegmentOrchestrator(cache, proc)
	defer func() { _ = o.Close() }()

	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 600))

	opts := proc.optsAt(0)
	assert.Equal(t, 600, opts.StartNumber, "ffmpeg must start numbering at the seeked segment")
	assert.InDelta(t, 1800.0, opts.StartSec, 0.001, "ffmpeg must -ss to the seeked position (600 * 3s)")
}

// Every spawned audio session must carry the input-pacing rate: unpaced, the
// AC-3/E-AC-3→AAC re-encode bursts a full core per client on start/seek. The
// orchestrator is the single caller that sets it, so the wiring is asserted here.
func TestAudioOrchestrator_SpawnsSessionWithReadRatePacing(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()

	proc := newFakeAudioProcess()
	proc.onStart = func(opts transcoding.AudioProcessOpts) *audioFakeHandle {
		writeAudioSegment(t, cache, fp, opts.StartNumber)
		return newAudioFakeHandle()
	}
	o := NewAudioSegmentOrchestrator(cache, proc)
	defer func() { _ = o.Close() }()

	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 0))

	opts := proc.optsAt(0)
	assert.InDelta(t, float64(audioRenditionReadRate), opts.ReadRate, 1e-9,
		"audio session must be input-paced so the CPU-bound re-encode does not burst a full core per client")
	assert.Positive(t, opts.ReadRate, "pacing must be enabled (non-zero), not left at the old unpaced 0")
}

// A session already covering the requested segment is reused: playback walking
// forward from a seek must not respawn ffmpeg on every segment.
func TestAudioOrchestrator_ForwardPlayback_ReusesSession(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()

	proc := newFakeAudioProcess()
	proc.onStart = func(opts transcoding.AudioProcessOpts) *audioFakeHandle {
		// The live session produces its start segment and the ones just after.
		for i := opts.StartNumber; i <= opts.StartNumber+3; i++ {
			writeAudioSegment(t, cache, fp, i)
		}
		return newAudioFakeHandle()
	}
	o := NewAudioSegmentOrchestrator(cache, proc)
	defer func() { _ = o.Close() }()

	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 600))
	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 601))
	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 602))

	assert.Equal(t, 1, proc.startCount(), "forward playback must ride the live session")
}

// writeAudioInit creates the audio_T_init.mp4 file for the given fingerprint,
// simulating what ffmpeg publishes as soon as it starts - at ANY position.
func writeAudioInit(t *testing.T, cache *SegmentCache, fp AudioFingerprint) {
	t.Helper()
	key := SegmentKey{
		DecisionHash: fp.Hash(),
		VariantIdx:   fp.SourceTrackIdx,
		IsInit:       true,
		Kind:         KindAudio,
	}
	path := cache.Path(key, true)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, fmp4Init(), 0o644))
}

// A viewer reopening a title from another browser resumes at their last position -
// say 1h35, audio segment 1908 - and the player asks for the audio init FIRST.
//
// The init (ftyp+moov, under a kilobyte) describes the codec and is byte-identical
// whatever position the encoder was started at. It must therefore never hinge on
// segment 0, which on a deep resume NO session will ever produce: the one session
// that matters starts at 1908 and runs forward.
//
// It used to hinge on it - the handler asked for segment 0 purely as a pretext to
// spawn a session, then waited for it. On a deep resume that wait timed out, the
// init 503'd, and the browser played the title with a picture but NO SOUND.
func TestAudioOrchestrator_EnsureAudioInit_DoesNotHingeOnSegmentZero(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()

	// ffmpeg publishes its init at startup - and, started at a deep seek, never
	// writes segment 0.
	proc := newFakeAudioProcess()
	proc.onStart = func(_ transcoding.AudioProcessOpts) *audioFakeHandle {
		writeAudioInit(t, cache, fp)
		return newAudioFakeHandle()
	}

	o := NewAudioSegmentOrchestrator(cache, proc)
	defer func() { _ = o.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, o.EnsureAudioInit(ctx, fp),
		"the init must be served off any live session, not off segment 0")
}

// ============================================================================
// Audio track ending before the declared duration (truncated tail)
// ============================================================================
//
// Real-world case (prod 2026-07-26, "Scary Movie"): the selected E-AC-3 track
// ends at 5742 s while the container declares 5991.6 s. The VOD playlist is
// synthesized from the DECLARED duration, so the player asks for segments the
// source cannot yield: ffmpeg -ss past the last audio packet exits ~instantly
// with zero output, the session is reaped, Wait fails, the client retries and
// respawns - 92 ffmpeg spawns for ONE segment were logged before the player
// gave up with a fatal "cannot play at any quality". These tests pin the
// graceful path: mark where the stream really ends, then serve the promised
// tail as SILENCE so playback (and resume) just continues through it.

// naturalDeathNoOutput returns an onStart callback simulating ffmpeg seeked
// past the last audio packet: it exits immediately (naturally - NOT stopped by
// the orchestrator) without producing its start segment.
func naturalDeathNoOutput() func(transcoding.AudioProcessOpts) *audioFakeHandle {
	return func(_ transcoding.AudioProcessOpts) *audioFakeHandle {
		h := newAudioFakeHandle()
		h.closeHandle()
		return h
	}
}

// A session dying naturally without producing its own start segment marks the
// stream end; the next request at/past that point must be served as a bounded
// silence session instead of respawning ffmpeg against the same wall.
func TestAudioOrchestrator_TrackEndingEarly_ServesSilentTail(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()
	fp.MaxAudioChannels = 6
	fp.SourceDurationSec = 5991.611 // container-declared duration

	proc := newFakeAudioProcess()
	proc.onStart = func(opts transcoding.AudioProcessOpts) *audioFakeHandle {
		if opts.SilenceDurationSec > 0 {
			// The silence generator produces its segments like any session.
			for i := opts.StartNumber; i <= opts.StartNumber+2; i++ {
				writeAudioSegment(t, cache, fp, i)
			}
			return newAudioFakeHandle()
		}
		return naturalDeathNoOutput()(opts)
	}

	o := NewAudioSegmentOrchestrator(cache, proc)
	defer func() { _ = o.Close() }()

	// First request: the real session dies against the end-of-stream wall.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	require.Error(t, o.EnsureAudioSegment(ctx, fp, 1914))
	cancel()
	require.Eventually(t, func() bool { return o.ActiveCount() == 0 },
		2*time.Second, 10*time.Millisecond, "dead session must be reaped")

	// Second request: served as silence, not another doomed ffmpeg -ss.
	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 1914),
		"past the recorded stream end the tail must be served as silence")

	last := proc.optsAt(proc.startCount() - 1)
	assert.Positive(t, last.SilenceDurationSec, "tail session must be a silence generator")
	assert.Equal(t, 1914, last.StartNumber, "silence must be numbered at the requested segment")
	assert.InDelta(t, 1914*ManifestSegmentSeconds, last.StartSec, 0.001)
	assert.InDelta(t, 5991.611-1914*ManifestSegmentSeconds, last.SilenceDurationSec, 0.01,
		"silence must be bounded to the declared duration, not run forever")
	assert.Equal(t, 6, last.MaxAudioChannels, "silence must keep the rendition's channel layout")

	// Forward playback rides the silence session - no per-segment respawn.
	spawns := proc.startCount()
	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 1915))
	assert.Equal(t, spawns, proc.startCount(), "the silence session must cover subsequent segments")
}

// Without a declared duration to bound the silence, the orchestrator cannot
// synthesize a tail - but it must STILL not respawn ffmpeg against the wall:
// the recorded stream end short-circuits to a stable sentinel the handler can
// map to a non-retryable status.
func TestAudioOrchestrator_TrackEndingEarly_NoDuration_ReturnsSentinel(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp() // SourceDurationSec zero: unknown

	proc := newFakeAudioProcess()
	proc.onStart = naturalDeathNoOutput()

	o := NewAudioSegmentOrchestrator(cache, proc)
	defer func() { _ = o.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	require.Error(t, o.EnsureAudioSegment(ctx, fp, 1914))
	cancel()
	require.Eventually(t, func() bool { return o.ActiveCount() == 0 },
		2*time.Second, 10*time.Millisecond)

	err := o.EnsureAudioSegment(context.Background(), fp, 1914)
	require.ErrorIs(t, err, ErrAudioPastEndOfStream)
	assert.Equal(t, 1, proc.startCount(), "the wall must be remembered - no second doomed spawn")
}

// A passthrough ("copy") rendition feeds the wasm AC-3 decoder: synthesizing an
// AAC silence tail would hand that decoder segments it cannot read. The copy
// path gets the stable sentinel instead - its client already skips a missing
// tail gracefully.
func TestAudioOrchestrator_TrackEndingEarly_CopyRendition_NoSilence(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()
	fp.AudioCodec = "copy"
	fp.SourceDurationSec = 5991.611

	proc := newFakeAudioProcess()
	proc.onStart = naturalDeathNoOutput()

	o := NewAudioSegmentOrchestrator(cache, proc)
	defer func() { _ = o.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	require.Error(t, o.EnsureAudioSegment(ctx, fp, 1914))
	cancel()
	require.Eventually(t, func() bool { return o.ActiveCount() == 0 },
		2*time.Second, 10*time.Millisecond)

	err := o.EnsureAudioSegment(context.Background(), fp, 1914)
	require.ErrorIs(t, err, ErrAudioPastEndOfStream)
	assert.Equal(t, 1, proc.startCount(), "no AAC silence may be synthesized for a passthrough rendition")
}

// A request BEFORE the recorded end keeps the normal path: the marker only
// gates segments at/after the point the stream proved to end.
func TestAudioOrchestrator_TrackEndingEarly_EarlierSegmentsUnaffected(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()
	fp.SourceDurationSec = 5991.611

	proc := newFakeAudioProcess()
	proc.onStart = func(opts transcoding.AudioProcessOpts) *audioFakeHandle {
		if opts.StartNumber >= 1914 && opts.SilenceDurationSec == 0 {
			return naturalDeathNoOutput()(opts)
		}
		writeAudioSegment(t, cache, fp, opts.StartNumber)
		return newAudioFakeHandle()
	}

	o := NewAudioSegmentOrchestrator(cache, proc)
	defer func() { _ = o.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	require.Error(t, o.EnsureAudioSegment(ctx, fp, 1914))
	cancel()
	require.Eventually(t, func() bool { return o.ActiveCount() == 0 },
		2*time.Second, 10*time.Millisecond)

	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 100),
		"segments before the recorded end must keep spawning real sessions")
	last := proc.optsAt(proc.startCount() - 1)
	assert.Zero(t, last.SilenceDurationSec, "a pre-end request is a REAL session, not silence")
}

// A session the ORCHESTRATOR itself stopped (eviction, Close) must not record a
// stream end: only a natural ffmpeg exit proves the source ran dry there.
func TestAudioOrchestrator_EvictedSession_DoesNotMarkStreamEnd(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()
	fp.SourceDurationSec = 5991.611

	proc := newFakeAudioProcess()
	proc.onStart = func(_ transcoding.AudioProcessOpts) *audioFakeHandle {
		// Sessions never produce anything and never exit on their own: only
		// the eviction cap stops them.
		return newAudioFakeHandle()
	}

	o := NewAudioSegmentOrchestrator(cache, proc)
	defer func() { _ = o.Close() }()

	// Scrub repeatedly: each far-apart seek spawns a session; past the per-hash
	// cap the oldest is evicted (stopped by the orchestrator, start seg unproduced).
	for i, seg := range []int{100, 400, 700, 1000, 1300} {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		_ = o.EnsureAudioSegment(ctx, fp, seg)
		cancel()
		_ = i
	}
	require.Eventually(t, func() bool { return o.ActiveCount() <= maxAudioSessionsPerHash },
		2*time.Second, 10*time.Millisecond)

	// A later request at an evicted session's position must spawn a REAL session
	// (no silence, no sentinel): eviction proved nothing about the stream.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	err := o.EnsureAudioSegment(ctx, fp, 100)
	cancel()
	require.NotErrorIs(t, err, ErrAudioPastEndOfStream,
		"an orchestrator-stopped session must not be read as end-of-stream")
	for i := range proc.startCount() {
		assert.Zero(t, proc.optsAt(i).SilenceDurationSec,
			"no silence session may be spawned off an eviction")
	}
}

// Repeated dead spawns on the SAME start segment - for ANY reason the reap
// cannot classify as end-of-stream - must trip a breaker instead of hammering
// ffmpeg forever (prod log: 92 spawns for one segment).
func TestAudioOrchestrator_RepeatedDeadSpawns_TripBreaker(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()
	fp.SourceDurationSec = 5991.611

	proc := newFakeAudioProcess()
	proc.onStart = func(opts transcoding.AudioProcessOpts) *audioFakeHandle {
		// Even the silence generator keeps dying without output (e.g. broken
		// ffmpeg build): the breaker is the last line against a spawn storm.
		return naturalDeathNoOutput()(opts)
	}

	o := NewAudioSegmentOrchestrator(cache, proc)
	defer func() { _ = o.Close() }()

	for range audioSpawnStrikeLimit + 3 {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		_ = o.EnsureAudioSegment(ctx, fp, 1914)
		cancel()
		require.Eventually(t, func() bool { return o.ActiveCount() == 0 },
			2*time.Second, 5*time.Millisecond)
	}

	err := o.EnsureAudioSegment(context.Background(), fp, 1914)
	require.Error(t, err, "past the strike limit the orchestrator must stop spawning")
	assert.LessOrEqual(t, proc.startCount(), audioSpawnStrikeLimit+1,
		"spawns must be bounded by the breaker, not by the client giving up")
}

// SourceDurationSec bounds the silence tail (-t duration - segment start), so
// the last segment's bytes follow it: a re-probe that moves the duration forks
// the key instead of serving the former tail. Unknown (0) hashes as before.
func TestAudioFingerprint_SourceDurationSec_ForksTheHash(t *testing.T) {
	a := minimalAudioFp()
	b := minimalAudioFp()
	b.SourceDurationSec = 5991.611
	c := minimalAudioFp()
	c.SourceDurationSec = 5992.611
	assert.NotEqual(t, a.Hash(), b.Hash())
	assert.NotEqual(t, b.Hash(), c.Hash(), "another duration, another tail")
}

// On an exact grid the video duration is an encode input too (-frames:v and
// the frame tail): it forks the key there, and only there.
func TestDecisionFingerprint_SourceDurationMs_ForksTheHashOnAnExactGrid(t *testing.T) {
	fp := DecisionFingerprint{MediaID: "m", SourcePath: "/s.mkv", Grid: transcoding.GridForFrameRate(25), SourceDurationMs: 120_000}
	other := fp
	other.SourceDurationMs = 120_640
	assert.NotEqual(t, fp.Hash(), other.Hash())
	legacy, legacyOther := fp, other
	legacy.Grid, legacyOther.Grid = transcoding.SegmentGrid{}, transcoding.SegmentGrid{}
	assert.Equal(t, legacy.Hash(), legacyOther.Hash(), "no tail on the legacy grid: transport only")
}

// And it must ride the session the resume already spawned rather than starting a
// second ffmpeg at t=0 to transcode an hour and a half of audio nobody asked for.
func TestAudioOrchestrator_EnsureAudioInit_RidesTheSeekSession(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()

	proc := newFakeAudioProcess()
	proc.onStart = func(opts transcoding.AudioProcessOpts) *audioFakeHandle {
		writeAudioInit(t, cache, fp)
		writeAudioSegment(t, cache, fp, opts.StartNumber)
		return newAudioFakeHandle()
	}

	o := NewAudioSegmentOrchestrator(cache, proc)
	defer func() { _ = o.Close() }()

	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 1908))
	require.NoError(t, o.EnsureAudioInit(context.Background(), fp))

	assert.Equal(t, 1, proc.startCount(), "the init must ride the live session, not spawn a second ffmpeg")
}

// The stream-end mark is a MUTE: every request at or past it is served as
// silence instead of a real encode. It is TTL'd precisely because that is a
// destructive state to be wrong about - a repaired or replaced file at the same
// path must not stay silent forever. A mark past its TTL must be dropped and a
// real session attempted again.
//
// Both TTL comparisons here (this one and the breaker's cooldown) read a real
// clock, so `>=` vs `>` differs only for a mark aged to the nanosecond - not
// reachable from a test without an injectable clock. These cases pin the
// BEHAVIOUR on each side, not the comparison operator.
func TestAudioOrchestrator_StreamEndMark_ExpiresAndLetsARealSpawnRetry(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()
	fp.SourceDurationSec = 5991.611

	proc := newFakeAudioProcess()
	proc.onStart = func(opts transcoding.AudioProcessOpts) *audioFakeHandle {
		// The file is now repaired: a real session produces its segments.
		writeAudioSegment(t, cache, fp, opts.StartNumber)
		return newAudioFakeHandle()
	}

	o := NewAudioSegmentOrchestrator(cache, proc)
	defer func() { _ = o.Close() }()

	// Plant a stream-end mark older than its TTL, as a long-lived orchestrator
	// would hold after the file was replaced on disk.
	o.mu.Lock()
	o.streamEnd[fp.Hash()] = audioStreamEndMark{seg: 1914, at: time.Now().Add(-audioStreamEndTTL - time.Minute)}
	o.mu.Unlock()

	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 1914))

	last := proc.optsAt(proc.startCount() - 1)
	assert.Zero(t, last.SilenceDurationSec,
		"an expired mark must yield a REAL session, not a silence tail - the file may have been repaired")

	o.mu.Lock()
	_, stillMarked := o.streamEnd[fp.Hash()]
	o.mu.Unlock()
	assert.False(t, stillMarked, "the expired mark must be dropped, not merely bypassed")
}

// A fresh mark is trusted: the same request under a mark INSIDE its TTL is
// served as silence. This is the other side of the expiry boundary, so the two
// together pin it.
func TestAudioOrchestrator_StreamEndMark_WithinTTLStillMutes(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()
	fp.SourceDurationSec = 5991.611

	proc := newFakeAudioProcess()
	proc.onStart = func(opts transcoding.AudioProcessOpts) *audioFakeHandle {
		writeAudioSegment(t, cache, fp, opts.StartNumber)
		return newAudioFakeHandle()
	}

	o := NewAudioSegmentOrchestrator(cache, proc)
	defer func() { _ = o.Close() }()

	o.mu.Lock()
	o.streamEnd[fp.Hash()] = audioStreamEndMark{seg: 1914, at: time.Now()}
	o.mu.Unlock()

	require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, 1914))
	assert.Positive(t, proc.optsAt(proc.startCount()-1).SilenceDurationSec,
		"a fresh mark must still mute - respawning ffmpeg against the same wall is what it prevents")
}

// A request landing EXACTLY on the declared duration has zero silence to
// synthesize. Spawning a zero-length silence generator would be a doomed ffmpeg
// - the same storm the mark exists to stop. It must return the stable sentinel
// the handler maps to a non-retryable status.
func TestAudioOrchestrator_StreamEnd_ZeroLengthTailReturnsSentinel(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()
	const seg = 1914
	fp.SourceDurationSec = seg * ManifestSegmentSeconds // exactly zero tail

	proc := newFakeAudioProcess()
	proc.onStart = func(opts transcoding.AudioProcessOpts) *audioFakeHandle {
		writeAudioSegment(t, cache, fp, opts.StartNumber)
		return newAudioFakeHandle()
	}

	o := NewAudioSegmentOrchestrator(cache, proc)
	defer func() { _ = o.Close() }()

	o.mu.Lock()
	o.streamEnd[fp.Hash()] = audioStreamEndMark{seg: seg, at: time.Now()}
	o.mu.Unlock()

	err := o.EnsureAudioSegment(context.Background(), fp, seg)
	require.ErrorIs(t, err, ErrAudioPastEndOfStream,
		"a zero-length tail is not a silence session, it is the end of the stream")
	assert.Zero(t, proc.startCount(), "no ffmpeg may be spawned for zero seconds of silence")
}

// The breaker trips at EXACTLY audioSpawnStrikeLimit strikes, and its cooldown
// is exactly audioSpawnStrikeTTL. TestAudioOrchestrator_RepeatedDeadSpawns_TripBreaker
// proves it trips at all by really killing sessions; this pins WHERE, by
// planting the strike count directly - the accumulation path is not the thing
// under test here, and driving it through real deaths cannot land on the
// boundary reliably.
//
// One strike too late is another doomed ffmpeg on a box already misbehaving (92
// spawns for one segment, prod 2026-07-26); one too early refuses a stream that
// still had an attempt left.
func TestAudioOrchestrator_Breaker_TripsAtExactlyTheStrikeLimit(t *testing.T) {
	const seg = 1914

	newOrch := func(t *testing.T, strikes audioSpawnStrikes) (*AudioSegmentOrchestrator, *fakeAudioProcess, AudioFingerprint) {
		t.Helper()
		cache, _ := newTestCache(t)
		fp := minimalAudioFp()
		proc := newFakeAudioProcess()
		proc.onStart = func(opts transcoding.AudioProcessOpts) *audioFakeHandle {
			writeAudioSegment(t, cache, fp, opts.StartNumber)
			return newAudioFakeHandle()
		}
		o := NewAudioSegmentOrchestrator(cache, proc)
		t.Cleanup(func() { _ = o.Close() })

		o.mu.Lock()
		o.strikes[audioSessionKey(fp.Hash(), seg)] = strikes
		o.mu.Unlock()
		return o, proc, fp
	}

	t.Run("one strike short still spawns", func(t *testing.T) {
		o, proc, fp := newOrch(t, audioSpawnStrikes{count: audioSpawnStrikeLimit - 1, last: time.Now()})

		require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, seg))
		assert.Positive(t, proc.startCount(), "below the limit the stream still gets its attempt")
	})

	t.Run("at the limit the breaker refuses", func(t *testing.T) {
		o, proc, fp := newOrch(t, audioSpawnStrikes{count: audioSpawnStrikeLimit, last: time.Now()})

		require.ErrorIs(t, o.EnsureAudioSegment(context.Background(), fp, seg), ErrAudioSessionBroken)
		assert.Zero(t, proc.startCount(), "a tripped breaker must spawn nothing at all")
	})

	t.Run("a cooled-down breaker allows a fresh attempt", func(t *testing.T) {
		// TTL'd so a transient cause self-heals: a repaired file must not stay
		// refused for the lifetime of the process.
		o, proc, fp := newOrch(t, audioSpawnStrikes{
			count: audioSpawnStrikeLimit,
			last:  time.Now().Add(-audioSpawnStrikeTTL - time.Second),
		})

		require.NoError(t, o.EnsureAudioSegment(context.Background(), fp, seg))
		assert.Positive(t, proc.startCount(), "past the cooldown the breaker must let one through")

		o.mu.Lock()
		_, stillTripped := o.strikes[audioSessionKey(fp.Hash(), seg)]
		o.mu.Unlock()
		assert.False(t, stillTripped, "the cooled-down entry must be cleared, not merely bypassed")
	})
}

// slowStopAudioHandle is a session whose Stop blocks until release closes,
// like a real ffmpeg handle waiting out its exit grace period.
type slowStopAudioHandle struct {
	*audioFakeHandle
	release  chan struct{}
	stopping chan struct{}
	once     sync.Once
}

func (h *slowStopAudioHandle) Stop() error {
	h.once.Do(func() { close(h.stopping) })
	<-h.release
	return h.audioFakeHandle.Stop()
}

type slowStopAudioProcess struct {
	*fakeAudioProcess
	release chan struct{}
	handles chan *slowStopAudioHandle
}

func (p *slowStopAudioProcess) StartAudioOnly(ctx context.Context, opts transcoding.AudioProcessOpts) (transcoding.HLSProcessHandle, error) {
	_, _ = p.fakeAudioProcess.StartAudioOnly(ctx, opts)
	h := &slowStopAudioHandle{audioFakeHandle: newAudioFakeHandle(), release: p.release, stopping: make(chan struct{})}
	p.handles <- h
	return h, nil
}

// Evicting the oldest session must not hold the orchestrator lock across the
// victim's Stop: that wait lasts up to the exit grace period and froze every
// other audio request behind it.
func TestAudioOrchestrator_EvictionStopsOutsideTheLock(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()
	release := make(chan struct{})
	proc := &slowStopAudioProcess{fakeAudioProcess: newFakeAudioProcess(), release: release, handles: make(chan *slowStopAudioHandle, 8)}
	o := NewAudioSegmentOrchestrator(cache, proc)
	defer func() { _ = o.Close() }()
	defer close(release)

	for _, seg := range []int{100, 400, 700} {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		_ = o.EnsureAudioSegment(ctx, fp, seg)
		cancel()
	}
	first := <-proc.handles
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_ = o.EnsureAudioSegment(ctx, fp, 1000)
	}()
	select {
	case <-first.stopping:
	case <-time.After(2 * time.Second):
		t.Fatal("the oldest session was never evicted")
	}

	got := make(chan int, 1)
	go func() { got <- o.ActiveCount() }()
	select {
	case <-got:
	case <-time.After(time.Second):
		t.Fatal("the orchestrator lock is held while the evicted session stops")
	}
}

// A session being stopped counts in the per-hash cap until its reap: while
// one sits in its stop grace, a further spawn is refused (503, retried)
// instead of stacking ffmpegs; once it is reaped, the next eviction picks
// another victim, never the same one again.
func TestAudioOrchestrator_StoppingSessionsCountInTheCap(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()
	release := make(chan struct{})
	proc := &slowStopAudioProcess{fakeAudioProcess: newFakeAudioProcess(), release: release, handles: make(chan *slowStopAudioHandle, 8)}
	o := NewAudioSegmentOrchestrator(cache, proc)
	defer func() { _ = o.Close() }()
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	ensure := func(seg int) error {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		return o.EnsureAudioSegment(ctx, fp, seg)
	}

	for _, seg := range []int{100, 400, 700, 1000} {
		_ = ensure(seg)
	}
	first, second := <-proc.handles, <-proc.handles
	select {
	case <-first.stopping:
	case <-time.After(2 * time.Second):
		t.Fatal("the oldest session was never evicted")
	}
	if err := ensure(1300); !errors.Is(err, ErrAudioSessionsBusy) {
		t.Fatalf("spawn while one session is still stopping: %v, want ErrAudioSessionsBusy", err)
	}
	if n := proc.startCount(); n != 4 {
		t.Fatalf("%d ffmpegs started, want 4: the cap counts the stopping one", n)
	}
	select {
	case <-second.stopping:
		t.Fatal("a second session was evicted while the first was still stopping")
	default:
	}

	close(release)
	released = true
	require.Eventually(t, func() bool { return o.ActiveCount() == 3 }, eventuallyWait, 10*time.Millisecond, "the stopped session is reaped")
	_ = ensure(1300)
	select {
	case <-second.stopping:
	case <-time.After(2 * time.Second):
		t.Fatal("after the reap, the next eviction must take the next oldest session")
	}
}

// A session being stopped stays listed until its reap but produces nothing
// more: a segment it would have covered, or the init, gets a fresh session
// instead of a wait that can only time out.
func TestAudioOrchestrator_StoppingSessionCoversNothing(t *testing.T) {
	cache, _ := newTestCache(t)
	fp := minimalAudioFp()
	proc := newFakeAudioProcess()
	o := NewAudioSegmentOrchestrator(cache, proc)
	defer func() { _ = o.Close() }()
	ensure := func(seg int) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_ = o.EnsureAudioSegment(ctx, fp, seg)
	}
	stopAll := func() {
		o.mu.Lock()
		for _, sess := range o.byHash[fp.Hash()] {
			sess.stopped = true // as eviction or the idle sweep leaves it until the reap
		}
		o.mu.Unlock()
	}

	ensure(100)
	require.Equal(t, 1, proc.startCount())
	stopAll()
	ensure(101)
	assert.Equal(t, 2, proc.startCount(), "a stopping session must not cover the next segment")

	stopAll()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_ = o.EnsureAudioInit(ctx, fp)
	assert.Equal(t, 3, proc.startCount(), "a stopping session publishes no init")
}
