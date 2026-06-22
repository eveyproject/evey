package transcoding

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/bgtasks"
)

// KeyframeBackfillStore is the narrow persistence slice the keyframe worker
// needs: find copy-eligible candidates missing a keyframe index, and store it.
// Implemented by the media-stream repository.
type KeyframeBackfillStore interface {
	// ListVideoKeyframesMissing returns up to `limit` media item IDs with an h264
	// video stream that has a level but no keyframe index yet, excluding `exclude`.
	ListVideoKeyframesMissing(ctx context.Context, limit int, exclude []string) ([]string, error)
	// SetKeyframes stores the keyframe index (ms) on the item's video stream(s),
	// only where currently NULL, never deleting. Returns rows updated.
	SetKeyframes(ctx context.Context, mediaItemID shared.MediaItemID, keyframesMS []int64) (int, error)
}

// KeyframeProber extracts a video file's keyframe presentation times (ms).
type KeyframeProber interface {
	ProbeKeyframes(ctx context.Context, filePath string) ([]int64, error)
}

// KeyframeBackfillWorker re-probes copy-eligible existing media to fill the
// keyframe index (media_streams.keyframes_ms) the scan never extracts — the
// keyframe probe is far too costly for the hot scan path. The keyframe index is
// what lets the copy/remux rung cut deterministic, keyframe-aligned segments
// addressable by absolute index (it cannot re-key a -c:v copy stream).
//
// Mirrors LevelBackfillWorker exactly: paced + env-tunable, resumable with no
// full re-scan (the candidate query "h264 video, level set, keyframes NULL"
// shrinks as it fills), per-file failure = skip + mark (in-run skip-set so a
// stuck file is attempted at most once per run). It runs AFTER the level
// backfill by construction (the query requires level IS NOT NULL).
//
// Pacing is deliberately gentler than the level worker: the keyframe probe scans
// every packet header (~tens of seconds for a long film over a network mount),
// so this is the heaviest sustained NAS load of the whole pipeline. Defaults are
// conservative; validate on the real NAS before raising. The proper fix for the
// cost is the container seek index (mkvgo OpenCues) — see the memory note.
type KeyframeBackfillWorker struct {
	store  KeyframeBackfillStore
	items  MediaItemReader
	prober KeyframeProber
	logger *slog.Logger

	batchSize    int
	idleInterval time.Duration
	itemPause    time.Duration

	attempted map[string]struct{} // in-run skip-set (single-goroutine, no lock)

	stop chan struct{}
	done chan struct{}

	startOnce sync.Once // Start spawns the run goroutine at most once.
	stopOnce  sync.Once // Stop closes the stop channel at most once.
}

// KeyframeBackfillOpts tunes the worker pacing. Zero values fall back to
// conservative defaults.
type KeyframeBackfillOpts struct {
	BatchSize    int           // items per tick. Default 5.
	IdleInterval time.Duration // sleep when nothing fresh is left. Default 1 h.
	ItemPause    time.Duration // gap between two probes. Default 2 s (on top of a multi-second probe).
}

func NewKeyframeBackfillWorker(
	store KeyframeBackfillStore,
	items MediaItemReader,
	prober KeyframeProber,
	opts KeyframeBackfillOpts,
	logger *slog.Logger,
) *KeyframeBackfillWorker {
	if logger == nil {
		logger = slog.Default()
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 5
	}
	if opts.IdleInterval <= 0 {
		opts.IdleInterval = time.Hour
	}
	if opts.ItemPause <= 0 {
		opts.ItemPause = 2 * time.Second
	}
	return &KeyframeBackfillWorker{
		store:        store,
		items:        items,
		prober:       prober,
		logger:       logger.With("component", "keyframe_backfill_worker"),
		batchSize:    opts.BatchSize,
		idleInterval: opts.IdleInterval,
		itemPause:    opts.ItemPause,
		attempted:    make(map[string]struct{}),
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
	}
}

func (w *KeyframeBackfillWorker) Start(ctx context.Context) {
	w.startOnce.Do(func() { go w.run(ctx) })
}

func (w *KeyframeBackfillWorker) Stop() {
	w.stopOnce.Do(func() { close(w.stop) })
	<-w.done
}

func (w *KeyframeBackfillWorker) run(ctx context.Context) {
	bgtasks.Register("keyframe_backfill")
	defer close(w.done)
	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("keyframe backfill worker panic recovered",
				slog.Any("err", r),
				slog.String("stack", string(debug.Stack())))
		}
	}()
	for {
		select {
		case <-w.stop:
			return
		case <-ctx.Done():
			return
		default:
		}
		if w.processBatch(ctx) == 0 {
			select {
			case <-time.After(w.idleInterval):
			case <-w.stop:
				return
			case <-ctx.Done():
				return
			}
		}
	}
}

func (w *KeyframeBackfillWorker) processBatch(ctx context.Context) int {
	ids, err := w.store.ListVideoKeyframesMissing(ctx, w.batchSize, w.excluded())
	if err != nil {
		w.logger.Warn("keyframe backfill: list candidates failed", slog.Any("err", err))
		return 0
	}
	for i, id := range ids {
		select {
		case <-w.stop:
			return i
		case <-ctx.Done():
			return i
		default:
		}
		if !w.backfillOne(ctx, id) {
			w.attempted[id] = struct{}{}
		}
		if i < len(ids)-1 {
			select {
			case <-time.After(w.itemPause):
			case <-w.stop:
				return i + 1
			case <-ctx.Done():
				return i + 1
			}
		}
	}
	return len(ids)
}

// backfillOne probes one item's keyframes and stores them. Returns true only
// when at least one row was written (progress); false on any skip/failure so the
// caller marks it. NEVER overwrites (SetKeyframes is NULL-guarded).
func (w *KeyframeBackfillWorker) backfillOne(ctx context.Context, mediaItemID string) bool {
	defer bgtasks.Begin("keyframe_backfill", mediaItemID)()
	item, err := w.items.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil {
		w.logger.Warn("keyframe backfill: item lookup failed",
			slog.String("media_id", mediaItemID), slog.Any("err", err))
		return false
	}
	if item == nil || item.File == nil || item.File.Path == "" {
		return false
	}

	kf, err := w.prober.ProbeKeyframes(ctx, item.File.Path)
	if err != nil || len(kf) == 0 {
		w.logger.Warn("keyframe backfill: probe failed or empty",
			slog.String("media_id", mediaItemID),
			slog.String("path", item.File.Path),
			slog.Any("err", err))
		return false
	}

	n, err := w.store.SetKeyframes(ctx, shared.MediaItemID(mediaItemID), kf)
	if err != nil {
		w.logger.Warn("keyframe backfill: persist failed",
			slog.String("media_id", mediaItemID), slog.Any("err", err))
		return false
	}
	if n == 0 {
		return false
	}
	w.logger.Info("keyframe backfill: indexed",
		slog.String("media_id", mediaItemID),
		slog.String("title", item.Title),
		slog.Int("keyframes", len(kf)))
	return true
}

func (w *KeyframeBackfillWorker) excluded() []string {
	if len(w.attempted) == 0 {
		return nil
	}
	out := make([]string, 0, len(w.attempted))
	for id := range w.attempted {
		out = append(out, id)
	}
	return out
}
