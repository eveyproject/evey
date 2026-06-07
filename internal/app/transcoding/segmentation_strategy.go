package transcoding

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/metrics"
)

// SegmentationStrategy is the port that controls HOW a video-copy rendition's
// segments are materialised in the cache. The CopyProducer holds one strategy
// and delegates to it — swapping the adapter changes the production mechanism
// without touching the producer's interface contract.
//
// Two adapters exist:
//
//   - WholeFileStrategy (implemented here): runs ONE ffmpeg with -c:v copy over
//     the WHOLE source, no -ss, so every segment is a deterministic byte-range
//     of the re-muxed output — segment N is the SAME bytes regardless of which
//     index triggered the run. Disk duplication cost = whole re-muxed file. NOT
//     the product default on small disks; used today to unblock the copy path.
//
//   - ByteRangeStrategy (stub here): Step 4 — serves each segment as a byte-
//     range of the source fMP4 via the mkvgo keyframe pre-pass. Zero disk
//     duplication = the product default. Blocked on the index-time keyframe
//     pre-pass (Cues + SimpleBlock flags + avcC).
type SegmentationStrategy interface {
	// EnsureSegment guarantees segment segIdx (ABSOLUTE index in the copy
	// rendition) is materialised in the cache. Returns immediately; callers
	// block on cache.Wait. DETERMINISTIC: segment N is the same bytes
	// regardless of which index was requested first — the whole point vs the
	// live producer's restart-on-seek model.
	EnsureSegment(ctx context.Context, fp DecisionFingerprint, segIdx int) error

	// Manifest returns the variable-#EXTINF HLS media playlist for fp. Byte-
	// identical across calls for the same fp (same keyframe boundaries) and
	// adapter-independent: swapping WholeFileStrategy for ByteRangeStrategy
	// must not change what the playlist exposes to the player.
	Manifest(ctx context.Context, fp DecisionFingerprint) ([]byte, error)
}

// ============================================================================
// WholeFileStrategy
// ============================================================================

// WholeFileStrategy materialises every segment of a copy rendition by running
// ONE ffmpeg over the WHOLE source file (-c:v copy, -c:a aac, no -ss,
// fMP4 HLS muxer). Single-flight per DecisionHash — concurrent EnsureSegment
// calls wait on the same run, never spawn a second ffmpeg for the same fp.
// After the run completes every segment is on disk; subsequent calls are
// instant cache hits. Manifest() reads the m3u8 ffmpeg wrote.
//
// IMPORTANT: this adapter duplicates the re-muxed source on disk. For a 2 h
// 1080p H.264 file at 5 Mbps that is ~4.5 GB. This is NOT the product default
// for self-hosted boxes with 40 GB disks; it exists to unblock the copy-on-seek
// bug fix while the ByteRangeStrategy (Step 4, zero disk duplication) is
// developed. Do not enable it as the default without an accompanying LRU
// disk-evictor that bounds total cache size.
type WholeFileStrategy struct {
	cache   *SegmentCache
	process transcoding.HLSProcess
	source  SourceResolver
	logger  *slog.Logger

	// shutdownCtx is cancelled only by Close — ffmpeg runs outside the HTTP
	// request context so a client disconnect does not kill the whole-file run.
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc

	mu sync.Mutex
	// runs tracks the background whole-file mux per DecisionHash. The run is
	// started ONCE (single-flight); EnsureSegment returns as soon as the ffmpeg
	// process has STARTED (not finished) so the caller can block on cache.Wait
	// for just the segment it needs — the mux fills the rest in the background at
	// ReadRate=0. This is the whole point vs the old full-mux-blocking model: the
	// first play waits for segment 0 (seconds), not the whole file (minutes).
	runs map[string]*wholeFileRun
}

// wholeFileRun is the state of one background whole-file mux. started is closed
// once ffmpeg's process.Start returns (startErr carries a start failure); done
// is closed when the mux process exits. Segments land on disk between the two.
type wholeFileRun struct {
	started  chan struct{}
	startErr error
	done     chan struct{}
}

// NewWholeFileStrategy wires the strategy to the shared cache + ffmpeg process
// abstraction. source resolves a DecisionFingerprint to an absolute file path.
func NewWholeFileStrategy(
	cache *SegmentCache,
	process transcoding.HLSProcess,
	source SourceResolver,
	logger *slog.Logger,
) *WholeFileStrategy {
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &WholeFileStrategy{
		cache:          cache,
		process:        process,
		source:         source,
		logger:         logger.With("component", "whole_file_strategy"),
		shutdownCtx:    ctx,
		shutdownCancel: cancel,
		runs:           make(map[string]*wholeFileRun),
	}
}

// EnsureSegment guarantees segment segIdx is materialised. The first caller
// for a given fp.Hash() spawns ffmpeg over the full source; every concurrent
// caller waits on the same completion channel. Subsequent calls (hash already
// done) return immediately.
func (s *WholeFileStrategy) EnsureSegment(ctx context.Context, fp DecisionFingerprint, _ int) error {
	run := s.ensureRun(fp)
	// Block only until the mux process has STARTED (fast) so a start failure (bad
	// source, ffmpeg launch error) surfaces here. The caller then blocks on
	// cache.Wait for its specific segment; the mux produces the rest in the
	// background at ReadRate=0.
	select {
	case <-run.started:
		return run.startErr
	case <-ctx.Done():
		return ctx.Err()
	case <-s.shutdownCtx.Done():
		return fmt.Errorf("whole_file: shutdown while starting %s", fp.Hash())
	}
}

// ensureRun returns the in-flight mux run for fp, starting it at most once. The
// runs map holds ONLY active muxes — the DISK is the source of truth, not the
// map. A completed playlist (ENDLIST) short-circuits without storing anything,
// so a later cache eviction is always re-detected; a half-muxed or crashed
// leftover lacks ENDLIST and is re-spawned. The map NEVER caches a terminal
// (done/failed) state — that was the poisoning bug.
func (s *WholeFileStrategy) ensureRun(fp DecisionFingerprint) *wholeFileRun {
	hash := fp.Hash()
	s.mu.Lock()
	defer s.mu.Unlock()
	// An in-flight mux already owns this hash — join it.
	if r, ok := s.runs[hash]; ok {
		return r
	}
	// No in-flight mux: trust the disk. A COMPLETE playlist means every segment is
	// materialised — return a transient done run WITHOUT storing it.
	if s.manifestComplete(hash, fp) {
		return doneRun()
	}
	// Nothing complete on disk (cold, or a half-muxed/evicted leftover): spawn a
	// fresh full mux. The no-ss invariant makes re-running idempotent.
	r := &wholeFileRun{started: make(chan struct{}), done: make(chan struct{})}
	s.runs[hash] = r
	go s.runFFmpegBackground(fp, hash, r)
	return r
}

// doneRun is an already-started, already-finished run with no error — returned
// when the disk already holds a complete playlist so callers go straight to
// cache.Wait (instant hits). Never stored in the runs map.
func doneRun() *wholeFileRun {
	r := &wholeFileRun{started: make(chan struct{}), done: make(chan struct{})}
	close(r.started)
	close(r.done)
	return r
}

// manifestComplete reports whether ffmpeg's playlist for hash exists AND carries
// #EXT-X-ENDLIST — i.e. the whole-file mux reached clean completion. Existence
// alone is insufficient: ffmpeg writes the playlist at segment 0 and appends
// ENDLIST only at the end, so a crashed/partial run lacks it and is re-muxed.
// Disk-as-truth sidesteps the handle.Err() gap (it does not catch mid-mux
// crashes) — a crash simply leaves no ENDLIST.
func (s *WholeFileStrategy) manifestComplete(hash string, fp DecisionFingerprint) bool {
	data, err := os.ReadFile(s.manifestPath(hash, fp))
	return err == nil && bytes.Contains(data, []byte("#EXT-X-ENDLIST"))
}

// runFFmpegBackground launches one ffmpeg session over the whole source file
// (-c:v copy, -c:a aac, no -ss) and runs to completion in the background. It
// closes run.started once process.Start returns (carrying any start error) so
// EnsureSegment can return promptly, then waits on the handle so segments fill
// the cache. Mirrors SoftwareProducer.startSession for the launch pattern.
func (s *WholeFileStrategy) runFFmpegBackground(fp DecisionFingerprint, hash string, run *wholeFileRun) {
	defer close(run.done)
	// Drop the in-flight marker on ANY exit (clean, resolve/start failure,
	// mid-mux crash, shutdown). The NEXT EnsureSegment re-derives from disk —
	// a complete playlist (ENDLIST) short-circuits, anything else re-spawns. The
	// map never retains a terminal state: this is the fix for the poisoning bug.
	defer func() {
		s.mu.Lock()
		delete(s.runs, hash)
		s.mu.Unlock()
	}()

	srcPath, _, err := s.source.ResolveSource(s.shutdownCtx, fp)
	if err != nil {
		run.startErr = fmt.Errorf("whole_file: resolve source: %w", err)
		close(run.started)
		return
	}

	outDir, err := s.cache.DirFor(hash)
	if err != nil {
		run.startErr = fmt.Errorf("whole_file: cache dir: %w", err)
		close(run.started)
		return
	}

	teardown := s.cache.RegisterSession(hash)
	defer teardown()

	// Single copy rung — variant index is always 0 for the copy path. No -ss:
	// that is the whole invariant (segment N is byte-identical regardless of
	// request order). ReadRate=0 lets ffmpeg run as fast as it can — the
	// whole-file mux is a one-time upfront cost, not a paced live session.
	procOpts := transcoding.HLSProcessOpts{
		OutputDir:          outDir,
		FilePath:           srcPath,
		Profiles:           profilesToDomain(fp.Profiles),
		VariantIndex:       0,
		VideoDecision:      "copy",
		AudioDecision:      "transcode",
		AudioTrackIndex:    fp.AudioTrack,
		SubtitleTrackIndex: -1, // subtitle burn-in is a transcode concern, not copy
		HDRSupported:       false,
		ToneMap:            false,
		AudioNormalize:     fp.AudioNormalize,
		StartSec:           0,   // never seek — the defining invariant
		ReadRate:           0,   // no pacing; mux as fast as the disk allows
		VideoCodec:         fp.VideoCodec,
		AudioCodec:         fp.AudioCodec,
		MaxAudioChannels:   fp.MaxAudioChannels,
		ForceSidecarSubs:   fp.ForceSidecarSubs,
		UseFMP4:            fp.UseFMP4,
	}

	sessionCtx, cancel := context.WithCancel(s.shutdownCtx)
	defer cancel()

	handle, err := s.process.Start(sessionCtx, procOpts)
	if err != nil {
		run.startErr = fmt.Errorf("whole_file: ffmpeg start: %w", err)
		close(run.started)
		return
	}

	// Started OK — unblock EnsureSegment callers; the mux now fills the cache.
	close(run.started)
	s.logger.Info("whole_file: started full-file remux",
		slog.String("hash", hash),
		slog.String("source", srcPath),
		slog.String("out_dir", outDir))
	metrics.HLSVODRafaleSpawned.WithLabelValues("copy_whole").Inc()
	metrics.HLSVODSessions.Inc()
	defer metrics.HLSVODSessions.Dec()

	// Wait for the mux to complete (or shutdown). Segments land on disk as it
	// runs; the handler serves them via cache.Wait without waiting for this.
	select {
	case <-handle.Done():
	case <-s.shutdownCtx.Done():
		if stopErr := handle.Stop(); stopErr != nil {
			s.logger.Debug("whole_file: stop on shutdown failed",
				slog.String("hash", hash), slog.Any("err", stopErr))
		}
		<-handle.Done()
		return
	}

	if ferr := handle.Err(); ferr != nil {
		// Partial segments may be cached; later segments will time out on
		// cache.Wait. Log rather than fail — the run already returned to callers.
		s.logger.Warn("whole_file: remux ended with error",
			slog.String("hash", hash), slog.Any("err", ferr))
		return
	}
	s.logger.Info("whole_file: remux complete", slog.String("hash", hash))
}

// Manifest returns the HLS media playlist ffmpeg wrote at the end of the whole-
// file run. EnsureSegment must have been called (and succeeded) before Manifest
// will find the m3u8 on disk; callers should always call EnsureSegment first.
func (s *WholeFileStrategy) Manifest(ctx context.Context, fp DecisionFingerprint) ([]byte, error) {
	// Ensure the background mux is running (surfaces start errors).
	if err := s.EnsureSegment(ctx, fp, 0); err != nil {
		return nil, fmt.Errorf("whole_file: manifest: ensure failed: %w", err)
	}

	// ffmpeg writes the (growing, EVENT-type) playlist once the first segment is
	// committed — NOT when the whole mux completes. Poll until it exists, then
	// serve the current bytes; the player reloads the event playlist until
	// ffmpeg appends ENDLIST at mux completion.
	path := s.manifestPath(fp.Hash(), fp)
	const pollEvery = 100 * time.Millisecond
	for {
		if data, err := os.ReadFile(path); err == nil {
			return data, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("whole_file: manifest: %w", ctx.Err())
		case <-s.shutdownCtx.Done():
			return nil, fmt.Errorf("whole_file: manifest: shutdown for %s", fp.Hash())
		case <-time.After(pollEvery):
		}
	}
}

// manifestPath returns the path where ffmpeg writes the variant playlist for
// the copy rendition. Must match appendHLSOutputArgs (variantIdx=0 →
// stream_0.m3u8) so the file is found after the ffmpeg run.
func (s *WholeFileStrategy) manifestPath(hash string, fp DecisionFingerprint) string {
	dir := filepath.Join(s.cache.baseDir, hash)
	_ = fp // unused beyond hash derivation but kept for future per-fp tuning
	return filepath.Join(dir, "stream_0.m3u8")
}

// Close cancels any in-flight whole-file mux runs. Already-completed runs are
// unaffected (their segments remain on disk for LRU cleanup or explicit
// CleanupHash). Safe to call once at shutdown.
func (s *WholeFileStrategy) Close() {
	s.shutdownCancel()
}

// ============================================================================
// ByteRangeStrategy (Step 4 stub)
// ============================================================================

// errByteRangeNotImplemented is returned by the ByteRangeStrategy stub.
//
// Step 4 — serves segment N as a byte-range of the source fMP4 via the mkvgo
// keyframe pre-pass (Cues + SimpleBlock flags + avcC). Zero disk duplication =
// the product default for self-hosted boxes. Blocked on the index-time keyframe
// pre-pass; implement after mkvgo exposes a ReadMeta early-stop mode.
var errByteRangeNotImplemented = errors.New(
	"segmentation: ByteRangeStrategy not implemented (Step 4 — blocked on mkvgo keyframe pre-pass)",
)

// ByteRangeStrategy is the zero-disk-duplication adapter (Step 4, stub).
// It will serve each copy-rendition segment as a byte-range of the source
// fMP4 file, addressed by the mkvgo Cues index built during library scanning.
// For now both methods return errByteRangeNotImplemented.
type ByteRangeStrategy struct{}

func (ByteRangeStrategy) EnsureSegment(_ context.Context, _ DecisionFingerprint, _ int) error {
	return errByteRangeNotImplemented
}

func (ByteRangeStrategy) Manifest(_ context.Context, _ DecisionFingerprint) ([]byte, error) {
	return nil, errByteRangeNotImplemented
}

// compile-time assertions.
var _ SegmentationStrategy = (*WholeFileStrategy)(nil)
var _ SegmentationStrategy = ByteRangeStrategy{}

