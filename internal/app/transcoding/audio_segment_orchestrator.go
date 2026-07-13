package transcoding

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

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
}

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

// sessionKey namespaces a session by its start segment so a seek's session and
// the cold-start one coexist instead of aliasing.
func audioSessionKey(hash string, startSeg int) string {
	return fmt.Sprintf("%s:%d", hash, startSeg)
}

type audioSession struct {
	mu        sync.Mutex // guards teardown nil-out between Close and reap goroutine
	fp        AudioFingerprint
	handle    transcoding.HLSProcessHandle
	teardown  func() // idempotent; release SegmentCache pending refcount
	startedAt time.Time
	startSeg  int    // segment this ffmpeg was started at (-ss / -start_number)
	hash      string // AudioFingerprint.Hash(), without the start-segment suffix
}

// NewAudioSegmentOrchestrator wires the segment cache and audio process together.
func NewAudioSegmentOrchestrator(
	cache *SegmentCache,
	t transcoding.AudioProcess,
) *AudioSegmentOrchestrator {
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	return &AudioSegmentOrchestrator{
		cache:          cache,
		transcoder:     t,
		logger:         slog.Default().With("component", "audio_segment_orchestrator"),
		shutdownCtx:    shutdownCtx,
		shutdownCancel: shutdownCancel,
		sessions:       make(map[string]*audioSession),
	}
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

// ensureSession guarantees a live ffmpeg session that will reach segIdx. A
// session started at or just before segIdx (within audioSessionLookahead) is
// ridden as-is; anything else - notably a seek far ahead of the live frontier -
// spawns its own ffmpeg AT that position. Concurrent callers serialise on mu and
// find the session already populated when they proceed.
func (o *AudioSegmentOrchestrator) ensureSession(ctx context.Context, fp AudioFingerprint, audioHash string, segIdx int) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.coveringSessionLocked(audioHash, segIdx) {
		return nil
	}
	o.evictOldestLocked(audioHash)

	outputDir, err := o.cache.DirFor(audioHash)
	if err != nil {
		return fmt.Errorf("EnsureAudioSegment: cache dir: %w", err)
	}

	teardown := o.cache.RegisterSession(audioHash)

	opts := transcoding.AudioProcessOpts{
		FilePath:         fp.SourcePath,
		OutputDir:        outputDir,
		SourceTrackIdx:   fp.SourceTrackIdx,
		AudioCodec:       fp.AudioCodec,
		AudioBitrate:     int(fp.AudioBitrate),
		MaxAudioChannels: fp.MaxAudioChannels,
		// Seek: ffmpeg both decodes from here (-ss) and numbers its output from
		// here (-start_number), so segment N on disk is always source second
		// N*ManifestSegmentSeconds - the invariant the playlist addresses.
		StartSec:       float64(segIdx) * ManifestSegmentSeconds,
		StartNumber:    segIdx,
		AudioNormalize: fp.AudioNormalize,
	}

	// Use the orchestrator's long-lived context so the ffmpeg process survives
	// past the requesting HTTP context being cancelled on response send.
	sessionCtx, cancel := context.WithCancel(o.shutdownCtx)
	handle, err := o.transcoder.StartAudioOnly(sessionCtx, opts)
	if err != nil {
		cancel()
		teardown()
		return fmt.Errorf("EnsureAudioSegment: start audio: %w", err)
	}

	sess := &audioSession{
		fp:        fp,
		handle:    handle,
		teardown:  teardown,
		startedAt: time.Now(),
		startSeg:  segIdx,
		hash:      audioHash,
	}
	sessKey := audioSessionKey(audioHash, segIdx)
	o.sessions[sessKey] = sess

	o.logger.Info("audio_orchestrator: session spawned",
		slog.String("hash", audioHash),
		slog.String("media_id", fp.MediaID),
		slog.Int("track", fp.SourceTrackIdx),
		slog.Int("start_seg", segIdx),
		slog.String("codec", fp.AudioCodec))

	// Reap goroutine: remove the session when ffmpeg exits (naturally or via cancel).
	// cancel() is idempotent - the goroutine owns it from here.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("audio_segment_orchestrator: detached reap goroutine panic recovered", slog.Any("panic", r), slog.String("stack", string(debug.Stack())))
			}
		}()
		select {
		case <-handle.Done():
		case <-sessionCtx.Done():
		}

		o.mu.Lock()
		delete(o.sessions, sessKey)
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
	_ = oldest.handle.Stop() // reap goroutine unblocks on Done() and cleans up
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
	// Cancel the long-lived context - all session reap goroutines unblock via Done().
	o.shutdownCancel()

	// Snapshot live sessions under the lock, then stop them outside.
	o.mu.Lock()
	toStop := make([]*audioSession, 0, len(o.sessions))
	for _, sess := range o.sessions {
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
