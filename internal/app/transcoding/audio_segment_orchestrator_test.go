package transcoding

import (
	"context"
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
	}, 2*time.Second, 20*time.Millisecond, "active count must reach 0 after ffmpeg exits")

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

// SourceDurationSec is transport for the silence bound, NOT an encode input: it
// must never fork the cache hash (same bytes, same segments, same dir).
func TestAudioFingerprint_SourceDurationSec_DoesNotForkHash(t *testing.T) {
	a := minimalAudioFp()
	b := minimalAudioFp()
	b.SourceDurationSec = 5991.611
	assert.Equal(t, a.Hash(), b.Hash(),
		"SourceDurationSec must not be part of the cache identity")
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
