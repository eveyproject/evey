package transcoding

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// CopyProducer is a StreamProducer for H.264 8-bit SDR content where the video
// stream can be remuxed without re-encoding (-c:v copy). It holds a
// SegmentationStrategy that controls HOW segments are materialised — today
// WholeFileStrategy (one ffmpeg over the whole file, segments are cache hits
// forever after), tomorrow ByteRangeStrategy (byte-range of source fMP4, zero
// disk duplication, Step 4).
//
// Copy-on-seek invariant: because the strategy runs ffmpeg from the START with
// no -ss, every segment is byte-identical regardless of which one the client
// requests first. This is the structural fix for the buildCopyVideoArgs bug
// (StartSec>0 silently re-encoded with libx264 instead of copying).
//
// Single rung: the copy rendition exposes ONE variant (source resolution,
// source bitrate) with no transcode ladder. The player never ABR-switches a
// copy stream — there is nothing to switch to. variantIdx must always be 0.
type CopyProducer struct {
	strategy SegmentationStrategy
	cache    *SegmentCache
	logger   *slog.Logger
	opts     CopyProducerOptions
}

// CopyProducerOptions configures the copy producer's wait budget and lifecycle.
type CopyProducerOptions struct {
	// WaitBudgetDur is how long the handler should block on cache.Wait for a
	// copy-rendition segment. Longer than the HW budget: the whole-file mux is
	// a one-time cost that can take tens of seconds for a 4K H.264 file. After
	// it completes every subsequent request is an instant cache hit. Default 5 min.
	WaitBudgetDur time.Duration
}

func (o CopyProducerOptions) withDefaults() CopyProducerOptions {
	out := o
	if out.WaitBudgetDur <= 0 {
		out.WaitBudgetDur = 5 * time.Minute
	}
	return out
}

// NewCopyProducer wires the copy producer to a SegmentationStrategy, the
// shared SegmentCache, and a logger. opts may be zero for defaults.
func NewCopyProducer(
	strategy SegmentationStrategy,
	cache *SegmentCache,
	opts CopyProducerOptions,
	logger *slog.Logger,
) *CopyProducer {
	if logger == nil {
		logger = slog.Default()
	}
	return &CopyProducer{
		strategy: strategy,
		cache:    cache,
		logger:   logger.With("component", "copy_producer"),
		opts:     opts.withDefaults(),
	}
}

// Profiles returns the single copy rung for the given source. Pure and
// deterministic: the ladder is always one rung at source resolution with
// the source bitrate (or a conservative floor when the source carries no
// bitrate metadata). This feeds DecisionFingerprint.Hash(), so the SAME
// inputs must always yield the SAME rung.
//
// The copy path ignores capacity: no transcode ladder exists to cap, and the
// whole point is that the CPU cost is near zero (mux only, no decode/encode).
func (p *CopyProducer) Profiles(
	_ []transcoding.HLSProfile,
	src SourceDims,
	_ transcoding.PlaybackDecision,
	_ transcoding.EncodeCapability,
) []transcoding.HLSProfile {
	// A sane floor for cases where ffprobe found no bitrate.
	const defaultBitrateKbps int64 = 5_000

	w, h := src.Width, src.Height
	if w <= 0 || h <= 0 {
		// Unknown resolution: use a safe default so the fingerprint stays
		// deterministic (callers may not have source metadata yet).
		w, h = 1920, 1080
	}

	return []transcoding.HLSProfile{
		{
			Name:         fmt.Sprintf("%dp-copy", h),
			Width:        w,
			Height:       h,
			VideoBitrate: defaultBitrateKbps,
			AudioBitrate: 192,
			Copy:         true, // remux rung → folds into the cache fingerprint
		},
	}
}

// EnsureSegmentRange delegates to the strategy for the copy rendition.
// variantIdx MUST be 0 — the copy producer exposes exactly one rendition;
// any other index is a caller error and is rejected immediately.
func (p *CopyProducer) EnsureSegmentRange(ctx context.Context, fp DecisionFingerprint, variantIdx, segmentIdx int) error {
	if variantIdx != 0 {
		return fmt.Errorf("copy: EnsureSegmentRange: variantIdx must be 0, got %d (copy producer has a single rung)", variantIdx)
	}
	if segmentIdx < 0 {
		return fmt.Errorf("copy: EnsureSegmentRange: negative segmentIdx %d", segmentIdx)
	}
	return p.strategy.EnsureSegment(ctx, fp, segmentIdx)
}

// PreWarm kicks the strategy for segment 0 in the background (cold-start
// optimisation: start the whole-file mux before the player requests anything).
// Fire-and-forget; errors are logged but not surfaced.
func (p *CopyProducer) PreWarm(fp DecisionFingerprint, startSegIdx int) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), p.opts.WaitBudgetDur)
		defer cancel()
		if err := p.strategy.EnsureSegment(ctx, fp, startSegIdx); err != nil {
			p.logger.Debug("copy: prewarm failed (non-fatal)",
				slog.String("media_id", fp.MediaID), slog.Any("err", err))
		}
	}()
}

// WaitBudget is how long the handler should block on cache.Wait. Long by
// design: the first request triggers a whole-file mux that can take minutes
// for large files; every subsequent request is an instant cache hit.
func (p *CopyProducer) WaitBudget() time.Duration { return p.opts.WaitBudgetDur }

// NotifyClientHint is a no-op for the copy producer. The whole-file mux is
// not paced and has no burst window to adjust; CMCD hints are irrelevant.
func (p *CopyProducer) NotifyClientHint(_ string, _ int, _ bool, _ int) {}

// ActiveSessionCount reports how many in-flight whole-file mux runs exist.
// The WholeFileStrategy tracks this via the inflight map; for now the copy
// producer does not have direct visibility into the strategy's internals, so
// it reports 0 (the mux is a one-time setup cost, not a per-viewer session).
// Admin metrics use this for throttling decisions; copy-path sessions are
// intentionally not counted in the HW/SW session budget.
func (p *CopyProducer) ActiveSessionCount() int { return 0 }

// RemoveSession is a no-op for the copy producer. The whole-file mux is not
// tied to a specific viewer session — it runs once and its output is shared
// across all viewers. Admin kill-session must call CleanupHash on the cache
// directly (the orchestrating layer handles this).
func (p *CopyProducer) RemoveSession(_ string) {}

// Close shuts down the producer. If the strategy is closeable (WholeFileStrategy),
// it cancels any in-flight whole-file mux runs.
func (p *CopyProducer) Close() {
	type closer interface{ Close() }
	if c, ok := p.strategy.(closer); ok {
		c.Close()
	}
}

// Compile-time assertion: CopyProducer satisfies the StreamProducer seam.
var _ StreamProducer = (*CopyProducer)(nil)
