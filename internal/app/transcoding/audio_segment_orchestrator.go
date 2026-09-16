package transcoding

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// ErrAudioPastEndOfStream reports that the requested segment lies at/after the
// point where the source audio track PROVED to end (a session seeked there
// exited naturally with zero output) and no silence tail can be synthesized
// (unknown duration, or a passthrough rendition whose client decodes the raw
// codec). Handlers map it to a stable, non-retryable status: the prod failure
// mode this replaces was a 503 the player retried, each retry respawning a
// doomed ffmpeg - 92 spawns for one segment (2026-07-26).
var ErrAudioPastEndOfStream = errors.New("audio: segment past the end of the source audio stream")

// ErrAudioSessionBroken reports that consecutive spawns for the same start
// segment all died without producing it, for reasons the reap could not
// classify as end-of-stream. The breaker stops the spawn storm; the entry
// expires (audioSpawnStrikeTTL) so a transient cause self-heals.
var ErrAudioSessionBroken = errors.New("audio: repeated dead sessions for this segment")

// AudioSegmentOrchestrator manages audio-only ffmpeg sessions for HLS EXT-X-MEDIA
// renditions. Sessions are keyed by (AudioFingerprint.Hash(), start segment):
// a seek spawns ffmpeg AT the requested position instead of waiting for a
// session pinned to t=0 to encode its way there. Unlike the video
// SegmentOrchestrator there is no CMCD throttling - audio sessions are
// lightweight - but the same seek semantics apply, because a player that seeks
// needs its audio as promptly as its video.
type AudioSegmentOrchestrator struct {
	cache      *SegmentCache
	transcoder transcoding.AudioProcess
	logger     *slog.Logger

	// shutdownCtx is the long-lived parent context for every ffmpeg audio session.
	// Cancelled only by Close - never the HTTP request context.
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc

	mu       sync.Mutex
	sessions map[string]*audioSession // keyed by sessionKey(hash, startSeg)

	// streamEnd records, per rendition hash, the earliest start segment at
	// which a session died NATURALLY without producing any output - the proof
	// that the source audio track ends there, before the container's declared
	// duration (playlists are synthesized from that declared duration, so the
	// player keeps asking). Requests at/past the mark are served as a bounded
	// silence tail instead of respawning ffmpeg against the same wall.
	// TTL'd: a repaired/replaced file at the same path must not stay muted.
	streamEnd map[string]audioStreamEndMark

	// strikes counts consecutive spawns per (hash, startSeg) that died without
	// producing their start segment - the generic breaker behind the specific
	// streamEnd handling (covers crash loops the reap cannot classify).
	strikes map[string]audioSpawnStrikes

	// Idle reaping; fields so tests run the sweep in milliseconds.
	inactivityTimeout time.Duration
	gcInterval        time.Duration
	stopGC            chan struct{}
	gcDone            chan struct{}
	gcOnce            sync.Once
}

// audioInactivityTimeout: a rendition nobody has asked for in this long is
// killed. Without it a session only ended with ffmpeg, reading the whole film
// at 8x - an audio switch or the wasm handover left one behind (prod 2026-09-09).
const audioInactivityTimeout = 60 * time.Second

// audioGCInterval is the idle sweep period.
const audioGCInterval = 15 * time.Second

type audioStreamEndMark struct {
	seg int
	at  time.Time
}

type audioSpawnStrikes struct {
	count int
	last  time.Time
}

// audioSpawnStrikeLimit is how many consecutive no-output session deaths on the
// same start segment are tolerated before the breaker refuses further spawns.
const audioSpawnStrikeLimit = 3

// audioSpawnStrikeTTL is how long a tripped breaker (and its strike count)
// stays in force; past it the next request may try a fresh spawn again.
const audioSpawnStrikeTTL = 5 * time.Minute

// audioStreamEndTTL bounds how long a recorded stream end is trusted. The mark
// lives on the rendition hash, which does not change when the FILE's content
// does (same path, same encode params) - so a repaired source re-proves its
// end at most this much later.
const audioStreamEndTTL = 30 * time.Minute

// audioSessionLookahead is how far AHEAD of a session's start segment that
// session is still considered the right one to wait on. ffmpeg audio-only runs
// far faster than realtime, so a request a little ahead of the live frontier is
// served in well under a second - but a request FAR ahead (a seek) must not wait
// for the encoder to walk there. 40 segments = 2 minutes of audio.
const audioSessionLookahead = 40

// maxAudioSessionsPerHash bounds concurrent ffmpeg processes for one rendition
// (a viewer scrubbing repeatedly would otherwise spawn one per seek). The oldest
// is evicted past the cap.
const maxAudioSessionsPerHash = 3

// audioRenditionReadRate paces the audio-only ffmpeg input to N× real-time.
// UNPACED (the old 0), each session decodes AC-3/E-AC-3 and re-encodes AAC for
// the WHOLE remaining track flat-out the instant it starts - and again on every
// seek - spiking ~a full core per client on a weak NAS (measured ~25-30% CPU for
// audio alone). 8× is the SAME loose pace the video copy rung uses
// (SoftwareProducer.copyReadRate): fast enough to keep "far faster than realtime"
// true - the audioSessionLookahead promise that a request just ahead of the
// frontier lands in well under a second, and that seeks fill instantly - yet
// bounded so the CPU-bound re-encode spreads across playback instead of a startup
// spike. Do NOT drop toward ~1.5×: that would break the lookahead assumption and
// re-introduce the buffering-spinner regression documented in copyReadRate.
// `-readrate` gates INPUT reading only, so segments stay byte-identical and the
// audio cache/fingerprint is untouched. 4x: ffmpeg demuxes the WHOLE file even
// with -vn, and 8x on a 70 Mb/s UHD remux was 70 MB/s of disk for audio alone
// (prod 2026-09-10); a 3 s segment still lands in under a second.
const audioRenditionReadRate = 4

// sessionKey namespaces a session by its start segment so a seek's session and
// the cold-start one coexist instead of aliasing.
func audioSessionKey(hash string, startSeg int) string {
	return fmt.Sprintf("%s:%d", hash, startSeg)
}

type audioSession struct {
	mu         sync.Mutex // guards teardown nil-out between Close and reap goroutine
	fp         AudioFingerprint
	handle     transcoding.HLSProcessHandle
	teardown   func() // idempotent; release SegmentCache pending refcount
	startedAt  time.Time
	startSeg   int       // segment this ffmpeg was started at (-ss / -start_number)
	hash       string    // AudioFingerprint.Hash(), without the start-segment suffix
	silence    bool      // bounded anullsrc tail, not a real source read
	lastAccess time.Time // last request at or past startSeg (guarded by o.mu)

	// stopped marks a session the ORCHESTRATOR killed (eviction, Close), set
	// under o.mu BEFORE Stop() is called. The reap goroutine reads it (under
	// o.mu) to tell an orchestrator kill from a NATURAL ffmpeg exit - only the
	// latter, with zero output, proves the source audio stream ended.
	stopped bool
}

// NewAudioSegmentOrchestrator wires the segment cache and audio process together.
func NewAudioSegmentOrchestrator(
	cache *SegmentCache,
	t transcoding.AudioProcess,
) *AudioSegmentOrchestrator {
	return newAudioSegmentOrchestrator(cache, t, audioInactivityTimeout, audioGCInterval)
}

// newAudioSegmentOrchestrator exposes the sweep timing for tests. The sweep
// goroutine starts here, so nothing may be tuned on the value afterwards.
func newAudioSegmentOrchestrator(cache *SegmentCache, t transcoding.AudioProcess, inactivity, gcInterval time.Duration) *AudioSegmentOrchestrator {
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	o := &AudioSegmentOrchestrator{
		cache:             cache,
		transcoder:        t,
		logger:            slog.Default().With("component", "audio_segment_orchestrator"),
		shutdownCtx:       shutdownCtx,
		shutdownCancel:    shutdownCancel,
		sessions:          make(map[string]*audioSession),
		streamEnd:         make(map[string]audioStreamEndMark),
		strikes:           make(map[string]audioSpawnStrikes),
		inactivityTimeout: inactivity,
		gcInterval:        gcInterval,
		stopGC:            make(chan struct{}),
		gcDone:            make(chan struct{}),
	}
	go o.gcLoop()
	return o
}

// gcLoop sweeps idle sessions until Close.
func (o *AudioSegmentOrchestrator) gcLoop() {
	defer close(o.gcDone)
	ticker := time.NewTicker(o.gcInterval)
	defer ticker.Stop()
	for {
		select {
		case <-o.stopGC:
			return
		case <-ticker.C:
			o.sweepIdle()
		}
	}
}

// sweepIdle kills idle sessions. stopped is set under the lock first so the
// reap goroutine never reads the kill as a natural end of stream.
func (o *AudioSegmentOrchestrator) sweepIdle() {
	cutoff := time.Now().Add(-o.inactivityTimeout)
	var idle []*audioSession
	o.mu.Lock()
	for _, sess := range o.sessions {
		if sess.stopped || sess.handle == nil || !sess.lastAccess.Before(cutoff) {
			continue
		}
		sess.stopped = true
		idle = append(idle, sess)
	}
	o.mu.Unlock()

	for _, sess := range idle {
		o.logger.Info("audio_orchestrator: session killed",
			slog.String("hash", sess.hash),
			slog.String("media_id", sess.fp.MediaID),
			slog.Int("track", sess.fp.SourceTrackIdx),
			slog.Int("start_seg", sess.startSeg),
			slog.String("reason", "inactivity"))
		_ = sess.handle.Stop() //nolint:errcheck // best-effort: the reap goroutine unblocks on Done() and cleans up
	}
}

// touchLocked marks the sessions started at or before segIdx as live; one
// started after it is an abandoned forward seek. Called on cache hits too: a
// session runs ahead of the player, so steady playback is all cache hits.
func (o *AudioSegmentOrchestrator) touchLocked(audioHash string, segIdx int) {
	now := time.Now()
	for _, sess := range o.sessions {
		if sess.hash == audioHash && sess.startSeg <= segIdx {
			sess.lastAccess = now
		}
	}
}

// touch is touchLocked under the lock.
func (o *AudioSegmentOrchestrator) touch(audioHash string, segIdx int) {
	o.mu.Lock()
	o.touchLocked(audioHash, segIdx)
	o.mu.Unlock()
}

// EnsureAudioSegment guarantees that the audio segment (fp.SourceTrackIdx, segIdx)
// is available on disk. Spawns an ffmpeg session if one does not already exist for
// fp.Hash(). Concurrent calls for the same fingerprint share the single session -
// only one ffmpeg is spawned regardless of how many goroutines call in parallel.
// Blocks (with ctx timeout) until the segment is ready or the session fails.
func (o *AudioSegmentOrchestrator) EnsureAudioSegment(ctx context.Context, fp AudioFingerprint, segIdx int) error {
	if segIdx < 0 {
		return fmt.Errorf("EnsureAudioSegment: negative segIdx %d", segIdx)
	}
	audioHash := fp.Hash()

	key := SegmentKey{
		DecisionHash: audioHash,
		VariantIdx:   fp.SourceTrackIdx,
		SegmentIdx:   segIdx,
		Kind:         KindAudio,
	}

	// Fast path: segment already written to disk.
	if o.cache.IsReady(key, true) {
		o.touch(audioHash, segIdx)
		return nil
	}

	// Ensure an ffmpeg session is running that will actually reach this segment.
	if err := o.ensureSession(ctx, fp, audioHash, segIdx); err != nil {
		return err
	}

	// Wait for the segment to materialise, propagating ctx cancellation.
	const audioSegmentTimeout = 60 * time.Second
	return o.cache.Wait(ctx, key, true, audioSegmentTimeout)
}

// audioInitTimeout bounds the wait for the init. ffmpeg publishes it as soon as
// the muxer opens, so this only fires on a broken session.
const audioInitTimeout = 30 * time.Second

// EnsureAudioInit guarantees the rendition's fMP4 init (ftyp+moov) is on disk.
// The init describes the CODEC, not a position - every session publishes the same
// bytes at startup - so any live session will do.
//
// It must NOT route through EnsureAudioSegment(fp, 0), as it once did: a deep
// resume's session starts at the viewer's position and never writes segment 0, so
// the wait timed out, the init 503'd, and the title played with no sound.
func (o *AudioSegmentOrchestrator) EnsureAudioInit(ctx context.Context, fp AudioFingerprint) error {
	return o.EnsureAudioInitAt(ctx, fp, 0)
}

// EnsureAudioInitAt is EnsureAudioInit with the segment a fresh session should
// start at when none is live: the viewer's position, so the session that writes
// the init is the one the segment requests ride (a session at 0 was pure waste
// on every resume and track switch, prod 2026-09-10).
func (o *AudioSegmentOrchestrator) EnsureAudioInitAt(ctx context.Context, fp AudioFingerprint, startSeg int) error {
	audioHash := fp.Hash()
	key := SegmentKey{
		DecisionHash: audioHash,
		VariantIdx:   fp.SourceTrackIdx,
		IsInit:       true,
		Kind:         KindAudio,
	}

	if o.cache.IsReady(key, true) {
		o.touch(audioHash, max(startSeg, 0))
		return nil
	}
	if err := o.ensureSessionForInit(fp, audioHash, max(startSeg, 0)); err != nil {
		return err
	}
	return o.cache.Wait(ctx, key, true, audioInitTimeout)
}

// PreWarmAudio starts the rendition's session at segIdx in the background, so
// the first segment a player asks for after a switch is already encoding.
func (o *AudioSegmentOrchestrator) PreWarmAudio(fp AudioFingerprint, segIdx int) {
	if o.shutdownCtx.Err() != nil || segIdx < 0 {
		return
	}
	go func() {
		if err := o.ensureSession(o.shutdownCtx, fp, fp.Hash(), segIdx); err != nil {
			o.logger.Debug("audio_orchestrator: prewarm failed (non-fatal)",
				slog.String("media_id", fp.MediaID), slog.Any("err", err))
		}
	}()
}

// ensureSession guarantees a live ffmpeg session that will reach segIdx. A
// session started at or just before segIdx (within audioSessionLookahead) is
// ridden as-is; anything else - notably a seek far ahead of the live frontier -
// spawns its own ffmpeg AT that position. Concurrent callers serialise on mu and
// find the session already populated when they proceed.
func (o *AudioSegmentOrchestrator) ensureSession(ctx context.Context, fp AudioFingerprint, audioHash string, segIdx int) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.touchLocked(audioHash, segIdx)
	if o.coveringSessionLocked(audioHash, segIdx) {
		return nil
	}

	// Past the proven end of the source audio stream, a real spawn is doomed:
	// serve the playlist's promised tail as bounded silence instead. The copy
	// (passthrough) rendition cannot - its client decodes the raw AC-3/E-AC-3
	// and would choke on AAC silence - so it gets the stable sentinel; the wasm
	// engine already skips a missing tail gracefully.
	if mark, ok := o.streamEnd[audioHash]; ok {
		if time.Since(mark.at) >= audioStreamEndTTL {
			delete(o.streamEnd, audioHash)
		} else if segIdx >= mark.seg {
			silenceDur := fp.SourceDurationSec - float64(segIdx)*ManifestSegmentSeconds
			if fp.AudioCodec == "copy" || silenceDur <= 0 {
				return fmt.Errorf("audio session: seg %d >= proven stream end %d: %w",
					segIdx, mark.seg, ErrAudioPastEndOfStream)
			}
			return o.spawnSessionLocked(fp, audioHash, segIdx, silenceDur)
		}
	}

	return o.spawnSessionLocked(fp, audioHash, segIdx, 0)
}

// ensureSessionForInit guarantees SOME live session for this rendition, without
// caring where it starts: every ffmpeg publishes the same init at startup. Only
// when the rendition has no session at all is one started, at t=0.
func (o *AudioSegmentOrchestrator) ensureSessionForInit(fp AudioFingerprint, audioHash string, startSeg int) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.touchLocked(audioHash, startSeg)
	for _, sess := range o.sessions {
		if sess.hash == audioHash {
			return nil
		}
	}
	return o.spawnSessionLocked(fp, audioHash, startSeg, 0)
}

// spawnSessionLocked starts one ffmpeg audio session at segIdx and registers it.
// silenceDur > 0 makes it a bounded silence generator (see ensureSession's
// stream-end handling) instead of a source read. Caller holds o.mu and has
// already decided no existing session fits.
func (o *AudioSegmentOrchestrator) spawnSessionLocked(fp AudioFingerprint, audioHash string, segIdx int, silenceDur float64) error {
	// Breaker: consecutive spawns at this start segment that died without
	// producing it stop here instead of storming ffmpeg (92 spawns for one
	// segment in prod 2026-07-26). Checked at THE spawn choke point so every
	// caller - segment, init, silence tail - is bounded; the entry expires so
	// a transient cause self-heals.
	key := audioSessionKey(audioHash, segIdx)
	if s, ok := o.strikes[key]; ok && s.count >= audioSpawnStrikeLimit {
		if time.Since(s.last) < audioSpawnStrikeTTL {
			return fmt.Errorf("audio session: %d consecutive dead spawns at seg %d: %w",
				s.count, segIdx, ErrAudioSessionBroken)
		}
		delete(o.strikes, key) // cooled down - allow a fresh attempt
	}

	o.evictOldestLocked(audioHash)

	outputDir, err := o.cache.DirFor(audioHash)
	if err != nil {
		return fmt.Errorf("audio session: cache dir: %w", err)
	}

	teardown := o.cache.RegisterSession(audioHash)

	opts := transcoding.AudioProcessOpts{
		FilePath:            fp.SourcePath,
		OutputDir:           outputDir,
		SourceTrackIdx:      fp.SourceTrackIdx,
		AudioCodec:          fp.AudioCodec,
		AudioEncoder:        fp.AudioEncoder,
		AudioBitrate:        int(fp.AudioBitrate),
		MaxAudioChannels:    fp.MaxAudioChannels,
		SourceAudioChannels: fp.SourceAudioChannels,
		// Seek: ffmpeg both decodes from here (-ss) and numbers its output from
		// here (-start_number), so segment N on disk is always source second
		// N*ManifestSegmentSeconds - the invariant the playlist addresses.
		StartSec:       float64(segIdx) * ManifestSegmentSeconds,
		StartNumber:    segIdx,
		AudioNormalize: fp.AudioNormalize,
		// Bound the CPU-bound AC-3/E-AC-3→AAC re-encode so it doesn't burst a full
		// core per client on start/seek. See audioRenditionReadRate.
		ReadRate:           audioRenditionReadRate,
		SilenceDurationSec: silenceDur,
		// Land on the video's timeline. Non-zero only when the video is packaged
		// by the ffmpeg copy fallback rather than mkvgo; the fingerprint carries
		// it (and hashes it) so a session and its cache entry always agree.
		VideoCompositionDelay: fp.VideoCompositionDelay,
	}
	if silenceDur > 0 {
		// Encoding a few minutes of silence is trivial - no pacing.
		opts.ReadRate = 0
	}

	// Use the orchestrator's long-lived context so the ffmpeg process survives
	// past the requesting HTTP context being cancelled on response send.
	sessionCtx, cancel := context.WithCancel(o.shutdownCtx)
	handle, err := o.transcoder.StartAudioOnly(sessionCtx, opts)
	if err != nil {
		cancel()
		teardown()
		return fmt.Errorf("audio session: start audio: %w", err)
	}

	sess := &audioSession{
		fp:         fp,
		handle:     handle,
		teardown:   teardown,
		startedAt:  time.Now(),
		lastAccess: time.Now(),
		startSeg:   segIdx,
		hash:       audioHash,
		silence:    silenceDur > 0,
	}
	sessKey := audioSessionKey(audioHash, segIdx)
	o.sessions[sessKey] = sess

	o.logger.Info("audio_orchestrator: session spawned",
		slog.String("hash", audioHash),
		slog.String("media_id", fp.MediaID),
		slog.Int("track", fp.SourceTrackIdx),
		slog.Int("start_seg", segIdx),
		slog.String("codec", fp.AudioCodec),
		slog.Bool("silence_tail", sess.silence))

	// Reap goroutine: remove the session when ffmpeg exits (naturally or via cancel).
	// cancel() is idempotent - the goroutine owns it from here.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(sessionCtx, "audio_segment_orchestrator: detached reap goroutine panic recovered", slog.Any("panic", r), slog.String("stack", string(debug.Stack())))
			}
		}()
		select {
		case <-handle.Done():
		case <-sessionCtx.Done():
		}

		// Disk stat outside the lock: did this session produce its own start
		// segment before exiting?
		startKey := SegmentKey{
			DecisionHash: audioHash,
			VariantIdx:   fp.SourceTrackIdx,
			SegmentIdx:   segIdx,
			Kind:         KindAudio,
		}
		producedStart := o.cache.IsReady(startKey, true)

		o.mu.Lock()
		delete(o.sessions, sessKey)
		if producedStart {
			// A healthy run clears the strike history for this start segment.
			delete(o.strikes, sessKey)
		} else {
			s := o.strikes[sessKey]
			s.count++
			s.last = time.Now()
			o.strikes[sessKey] = s

			// Natural exit + zero output + a real source read = the audio
			// stream ends at/before this session's start. Record the earliest
			// such point; requests past it become the silence tail. An
			// orchestrator kill (eviction/Close) or a classified failure
			// proves nothing about the source.
			natural := !sess.stopped && o.shutdownCtx.Err() == nil &&
				handle.Err() == nil && handle.Failure() == transcoding.FailureNone
			if natural && !sess.silence {
				if mark, ok := o.streamEnd[audioHash]; !ok || segIdx < mark.seg {
					o.streamEnd[audioHash] = audioStreamEndMark{seg: segIdx, at: time.Now()}
					o.logger.Warn("audio_orchestrator: source audio stream ends before declared duration",
						slog.String("hash", audioHash),
						slog.String("media_id", fp.MediaID),
						slog.Int("track", fp.SourceTrackIdx),
						slog.Int("end_seg", segIdx))
				}
			}
		}
		o.mu.Unlock()

		sess.mu.Lock()
		td := sess.teardown
		sess.teardown = nil
		sess.mu.Unlock()
		if td != nil {
			td()
		}
		cancel()

		o.logger.Debug("audio_orchestrator: session reaped",
			slog.String("hash", audioHash))
	}()

	return nil
}

// coveringSessionLocked reports whether a live session for this hash will reach
// segIdx soon enough to just wait on it: it must have started AT or BEFORE the
// segment (ffmpeg only moves forward) and no more than audioSessionLookahead
// behind it. A session further back is a cold-start ffmpeg grinding towards a
// seek target minutes away - waiting on it is what made a seek take 30 s.
// Caller holds o.mu.
func (o *AudioSegmentOrchestrator) coveringSessionLocked(audioHash string, segIdx int) bool {
	for _, sess := range o.sessions {
		if sess.hash != audioHash || sess.startSeg > segIdx {
			continue
		}
		if segIdx-sess.startSeg <= audioSessionLookahead {
			return true
		}
	}
	return false
}

// evictOldestLocked stops the oldest session for this hash once the per-hash cap
// is reached, so repeated scrubbing can't pile up ffmpeg processes. Caller holds
// o.mu; the reap goroutine removes the map entry when the process exits.
func (o *AudioSegmentOrchestrator) evictOldestLocked(audioHash string) {
	var oldest *audioSession
	live := 0
	for _, sess := range o.sessions {
		if sess.hash != audioHash {
			continue
		}
		live++
		if oldest == nil || sess.startedAt.Before(oldest.startedAt) {
			oldest = sess
		}
	}
	if live < maxAudioSessionsPerHash || oldest == nil || oldest.handle == nil {
		return
	}
	o.logger.Debug("audio_orchestrator: evicting oldest session (per-hash cap)",
		slog.String("hash", audioHash),
		slog.Int("start_seg", oldest.startSeg))
	// Set under o.mu (the caller holds it) BEFORE Stop() so the reap goroutine
	// never mistakes this kill for a natural end-of-stream exit.
	oldest.stopped = true
	_ = oldest.handle.Stop() //nolint:errcheck // best-effort: the reap goroutine unblocks on Done() and cleans up
}

// ActiveCount returns the number of live audio ffmpeg sessions. Useful for
// metrics and tests.
func (o *AudioSegmentOrchestrator) ActiveCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.sessions)
}

// Close stops all active audio sessions and releases their resources. Idempotent.
func (o *AudioSegmentOrchestrator) Close() error {
	// Stop the idle sweep first so it cannot race the teardown below.
	o.gcOnce.Do(func() {
		close(o.stopGC)
		<-o.gcDone
	})
	// Cancel the long-lived context - all session reap goroutines unblock via Done().
	o.shutdownCancel()

	// Snapshot live sessions under the lock, then stop them outside. stopped is
	// set here (under the lock) so no reap racing the shutdown reads a natural
	// exit out of a kill.
	o.mu.Lock()
	toStop := make([]*audioSession, 0, len(o.sessions))
	for _, sess := range o.sessions {
		sess.stopped = true
		toStop = append(toStop, sess)
	}
	o.sessions = make(map[string]*audioSession)
	o.mu.Unlock()

	for _, sess := range toStop {
		if sess.handle != nil {
			if err := sess.handle.Stop(); err != nil {
				o.logger.Warn("audio_orchestrator: stop failed on close",
					slog.String("media_id", sess.fp.MediaID),
					slog.Any("err", err))
			}
		}
		sess.mu.Lock()
		td := sess.teardown
		sess.teardown = nil
		sess.mu.Unlock()
		if td != nil {
			td()
		}
	}
	return nil
}
