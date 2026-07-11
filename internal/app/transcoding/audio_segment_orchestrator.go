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
// renditions. One session per AudioFingerprint (keyed by fp.Hash()). Unlike the
// video SegmentOrchestrator it does not support multi-rafale per hash, CMCD
// throttling, or LRU eviction - audio sessions are lightweight and short-lived.
type AudioSegmentOrchestrator struct {
	cache      *SegmentCache
	transcoder transcoding.AudioProcess
	logger     *slog.Logger

	// shutdownCtx is the long-lived parent context for every ffmpeg audio session.
	// Cancelled only by Close - never the HTTP request context.
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc

	mu       sync.Mutex
	sessions map[string]*audioSession // keyed by AudioFingerprint.Hash()
}

type audioSession struct {
	mu        sync.Mutex // guards teardown nil-out between Close and reap goroutine
	fp        AudioFingerprint
	handle    transcoding.HLSProcessHandle
	teardown  func() // idempotent; release SegmentCache pending refcount
	startedAt time.Time
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

	// Ensure exactly one ffmpeg is running for this fingerprint.
	if err := o.ensureSession(ctx, fp, audioHash); err != nil {
		return err
	}

	// Wait for the segment to materialise, propagating ctx cancellation.
	const audioSegmentTimeout = 60 * time.Second
	return o.cache.Wait(ctx, key, true, audioSegmentTimeout)
}

// ensureSession spawns a new audio ffmpeg session for audioHash if none exists.
// Concurrent callers that arrive while a spawn is in progress all wait on the
// same mu lock and will find the session already populated when they proceed.
func (o *AudioSegmentOrchestrator) ensureSession(ctx context.Context, fp AudioFingerprint, audioHash string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if _, ok := o.sessions[audioHash]; ok {
		// Session already live - nothing to spawn.
		return nil
	}

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
		StartSec:         0,
		StartNumber:      0,
		AudioNormalize:   fp.AudioNormalize,
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
	}
	o.sessions[audioHash] = sess

	o.logger.Info("audio_orchestrator: session spawned",
		slog.String("hash", audioHash),
		slog.String("media_id", fp.MediaID),
		slog.Int("track", fp.SourceTrackIdx),
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
		delete(o.sessions, audioHash)
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
