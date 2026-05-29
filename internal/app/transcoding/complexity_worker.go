package transcoding

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// MediaItemReader is the minimal slice of the library API we need: fetch
// one item by ID so we can hand the file path to the classifier. Defined
// here narrowly to keep the worker test-friendly without dragging the
// whole library service in.
type MediaItemReader interface {
	GetByID(ctx context.Context, id shared.MediaItemID) (*domainlibrary.MediaItem, error)
}

// ComplexityWorker classifies media items in the background, one at a
// time, paced so it never crowds the playback path. Each tick pulls a
// small batch of unclassified IDs from the repo, runs ffmpeg signalstats
// on each, persists the resulting class + metrics.
//
// Idempotent and safe to restart: any item already classified is filtered
// out by the repo's ListUnclassified query.
type ComplexityWorker struct {
	repo         domaintranscoding.ComplexityRepo
	classifier   domaintranscoding.Classifier
	items        MediaItemReader
	logger       *slog.Logger
	batchSize    int
	idleInterval time.Duration
	itemPause    time.Duration

	stop chan struct{}
	done chan struct{}
}

// ComplexityWorkerOpts tunes the worker pacing.
type ComplexityWorkerOpts struct {
	// BatchSize is the max number of items pulled per tick. Default 5.
	BatchSize int
	// IdleInterval is how long the worker sleeps between batches when
	// there's nothing left to classify. Default 15 min — picks up new
	// items dropped by the scanner within a reasonable window without
	// burning cycles when the library is steady.
	IdleInterval time.Duration
	// ItemPause separates two consecutive classification runs inside a
	// batch so the worker doesn't peg a CPU. Default 5 s.
	ItemPause time.Duration
}

func NewComplexityWorker(
	repo domaintranscoding.ComplexityRepo,
	classifier domaintranscoding.Classifier,
	items MediaItemReader,
	opts ComplexityWorkerOpts,
	logger *slog.Logger,
) *ComplexityWorker {
	if logger == nil {
		logger = slog.Default()
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 5
	}
	if opts.IdleInterval <= 0 {
		opts.IdleInterval = 15 * time.Minute
	}
	if opts.ItemPause <= 0 {
		opts.ItemPause = 5 * time.Second
	}
	return &ComplexityWorker{
		repo:         repo,
		classifier:   classifier,
		items:        items,
		logger:       logger.With("component", "complexity_worker"),
		batchSize:    opts.BatchSize,
		idleInterval: opts.IdleInterval,
		itemPause:    opts.ItemPause,
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
	}
}

// Start kicks off the loop in a goroutine. Stop with Stop().
func (w *ComplexityWorker) Start(ctx context.Context) {
	go w.run(ctx)
}

// Stop signals the worker to finish its current item and exit. Blocks
// until the goroutine returns.
func (w *ComplexityWorker) Stop() {
	close(w.stop)
	<-w.done
}

func (w *ComplexityWorker) run(ctx context.Context) {
	defer close(w.done)
	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("complexity worker panic recovered",
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

		processed := w.processBatch(ctx)
		if processed == 0 {
			// Nothing to do — sleep longer.
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

// processBatch pulls one batch and classifies each item. Returns the count
// of items processed (0 ⇒ nothing left, sleep deeper).
func (w *ComplexityWorker) processBatch(ctx context.Context) int {
	ids, err := w.repo.ListUnclassified(ctx, w.batchSize)
	if err != nil {
		w.logger.Warn("complexity: list unclassified failed", slog.Any("err", err))
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
		w.classifyOne(ctx, id)
		// Throttle: don't sleep after the last item — keeps the worker
		// snappy when ListUnclassified came back full and another batch
		// is waiting.
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

func (w *ComplexityWorker) classifyOne(ctx context.Context, mediaItemID string) {
	item, err := w.items.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil {
		w.logger.Warn("complexity: media item lookup failed",
			slog.String("media_id", mediaItemID), slog.Any("err", err))
		return
	}
	if item == nil || item.File == nil || item.File.Path == "" {
		// No file = nothing to sample. Silently skip.
		return
	}
	durationSec := item.File.Duration.Seconds()
	if durationSec <= 0 {
		// Duration unknown — the classifier needs it to spread samples.
		// Skip; the scanner will eventually re-probe.
		return
	}

	metrics, err := w.classifier.Classify(ctx, item.File.Path, durationSec)
	if err != nil && metrics.FramesSampled == 0 {
		w.logger.Warn("complexity: classify failed",
			slog.String("media_id", mediaItemID),
			slog.String("path", item.File.Path),
			slog.Any("err", err))
		return
	}
	class := domaintranscoding.Classify(metrics)
	if setErr := w.repo.Set(ctx, mediaItemID, class, metrics, time.Now()); setErr != nil {
		w.logger.Warn("complexity: persist failed",
			slog.String("media_id", mediaItemID),
			slog.String("class", string(class)),
			slog.Any("err", setErr))
		return
	}
	w.logger.Info("complexity: classified",
		slog.String("media_id", mediaItemID),
		slog.String("title", item.Title),
		slog.String("class", string(class)),
		slog.Float64("ydev_mean", metrics.YDEVMean),
		slog.Float64("scenecuts_per_min", metrics.ScenecutsPerMinute),
		slog.Int64("src_bitrate_kbps", metrics.SourceBitrateKbps),
		slog.Int("frames_sampled", metrics.FramesSampled))
}
