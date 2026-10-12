package transcoding

import (
	"context"
	"log/slog"
	"os"
	"runtime/debug"
	"sync"
	"time"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/eveyproject/evey/internal/pkg/bgtasks"
)

// MediaItemReader is the minimal slice of the library API we need: fetch
// one item by ID so we can hand the file path to the classifier. Defined
// here narrowly to keep the worker test-friendly without dragging the
// whole library service in.
type MediaItemReader interface {
	GetByID(ctx context.Context, id shared.MediaItemID) (*domainlibrary.MediaItem, error)
}

// VideoDimsReader hands back the item's video resolution. Narrow on purpose:
// the worker only needs it to spare the classifier an ffprobe fork, and the
// resolution is one of the few source facts the scan reliably persists.
// Optional - a nil reader simply leaves the classifier to probe.
type VideoDimsReader interface {
	VideoDims(ctx context.Context, mediaItemID shared.MediaItemID) (width, height int, err error)
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
	dims         VideoDimsReader // optional
	logger       *slog.Logger
	batchSize    int
	idleInterval time.Duration
	itemPause    time.Duration

	// activePlayback reports how many streams are being served right now. Nil
	// means "no gate", which is what this worker used to be - and the doc
	// comment on startTranscodingWorkers already claimed EVERY worker yielded to
	// playback, so a reader had no way to know this one did not.
	activePlayback func() int

	stop chan struct{}
	done chan struct{}

	startOnce sync.Once // Start spawns the run goroutine at most once.
	stopOnce  sync.Once // Stop closes the stop channel at most once.
}

// WithVideoDims installs the resolution reader that lets the classifier skip
// its ffprobe pre-probe.
func (w *ComplexityWorker) WithVideoDims(d VideoDimsReader) *ComplexityWorker {
	w.dims = d
	return w
}

// ComplexityWorkerOpts tunes the worker pacing.
type ComplexityWorkerOpts struct {
	// BatchSize is the max number of items pulled per tick. Default 5.
	BatchSize int
	// IdleInterval is how long the worker sleeps between batches when
	// there's nothing left to classify. Default 15 min - picks up new
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
	registerComplexityCoverage(repo)
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

// Start kicks off the loop in a goroutine. Stop with Stop(). Idempotent: a
// second Start is a no-op, so a double lifecycle owner can't spawn two loops.
func (w *ComplexityWorker) Start(ctx context.Context) {
	w.startOnce.Do(func() { go w.run(ctx) })
}

// Stop signals the worker to finish its current item and exit. Blocks
// until the goroutine returns. Idempotent: a second Stop won't re-close.
func (w *ComplexityWorker) Stop() {
	w.stopOnce.Do(func() { close(w.stop) })
	<-w.done
}

// complexityTask is this worker's name in the bgtasks registry.
const complexityTask = "complexity_classifier"

func (w *ComplexityWorker) run(ctx context.Context) {
	bgtasks.RegisterControllable(complexityTask)
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

		// Step aside entirely while someone is watching. The classifier forks
		// nice -19 ffmpeg bursts that only get SIGSTOPped once the shaper
		// notices them, so up to a second of decode still lands on a box that
		// is trying to serve a stream. Its two sibling backfill workers already
		// skip their batch this way; one of those gates exists because an
		// ungated pass stalled a Chromecast in production on 2026-07-19.
		if w.playbackActive() {
			select {
			case <-time.After(w.idleInterval):
			case <-w.stop:
				return
			case <-ctx.Done():
				return
			}
			continue
		}

		processed := w.safeProcessBatch(ctx)
		if processed == 0 {
			// Nothing to do - sleep longer.
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

// WithActivePlayback wires the live playback-session counter: while it reports
// more than zero, the worker skips its batch instead of competing with a
// stream. Chainable; nil leaves the worker ungated. Mirrors the identically
// named option on KeyframeBackfillWorker and LevelBackfillWorker.
func (w *ComplexityWorker) WithActivePlayback(count func() int) *ComplexityWorker {
	w.activePlayback = count
	return w
}

// playbackActive reports whether a stream is being served. Nil-safe: an unwired
// counter means "cannot tell", and this worker is background work, so it keeps
// running rather than stalling forever on a missing dependency.
func (w *ComplexityWorker) playbackActive() bool {
	return w.activePlayback != nil && w.activePlayback() > 0
}

// processBatch pulls one batch and classifies each item. Returns the count
// of items processed (0 ⇒ nothing left, sleep deeper).
func (w *ComplexityWorker) processBatch(ctx context.Context) int {
	ids, err := w.repo.ListUnclassified(ctx, w.batchSize)
	if err != nil {
		w.logger.Warn("complexity: list unclassified failed", slog.Any("err", err))
		return 0
	}
	bgtasks.Queue(complexityTask, len(ids))
	for i, id := range ids {
		select {
		case <-w.stop:
			return i
		case <-ctx.Done():
			return i
		default:
		}
		// An admin's Stop holds the worker here.
		if bgtasks.WaitResumed(ctx, complexityTask, w.stop) != nil {
			return i
		}
		w.classifyOne(ctx, id)
		// Throttle: don't sleep after the last item - keeps the worker
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
	ctx, done := bgtasks.BeginCtx(ctx, complexityTask, mediaItemID)
	defer done()
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
		// Duration unknown - the classifier needs it to spread samples.
		// Skip; the scanner will eventually re-probe.
		return
	}

	// Everything the classifier would otherwise fork ffprobe for. Bitrate comes
	// off the item (persisted at scan, derived from size/duration when the probe
	// gave none); the resolution needs the one extra read below.
	known := domaintranscoding.KnownSourceFacts{BitrateBps: item.File.BitrateBps()}
	if w.dims != nil {
		if wd, ht, dErr := w.dims.VideoDims(ctx, shared.MediaItemID(mediaItemID)); dErr == nil {
			known.Width, known.Height = wd, ht
		}
	}

	metrics, err := w.classifier.Classify(ctx, item.File.Path, durationSec, known)
	if err != nil && metrics.FramesSampled == 0 {
		if ctx.Err() != nil {
			// Cancelled (an admin stopped the worker, or shutdown): a verdict on
			// nothing, so no fallback class may be persisted for the file.
			return
		}
		// Transient vs permanent (same doctrine as mkvgo AnalyzeFrameRateMode):
		// a file the OS can't even stat may heal (unmounted share) - leave the
		// row NULL so the next sweep retries. A present file that still
		// hard-fails would fail identically every sweep; ListUnclassified would
		// re-serve it forever (the 2026-07-26 prod loop). Settle it with the
		// sample-free heuristic on the known source facts instead.
		if _, statErr := os.Stat(item.File.Path); statErr != nil {
			w.logger.Warn("complexity: classify failed, source unavailable - will retry",
				slog.String("media_id", mediaItemID),
				slog.String("path", item.File.Path),
				slog.Any("err", err))
			return
		}
		w.logger.Warn("complexity: classify failed on a readable file, persisting sample-free fallback",
			slog.String("media_id", mediaItemID),
			slog.String("path", item.File.Path),
			slog.Any("err", err))
		metrics = domaintranscoding.ComplexityMetrics{
			SourceBitrateKbps: known.BitrateBps / 1000,
			SourceHeight:      known.Height,
			MetricVersion:     domaintranscoding.CanonicalMetricVersion, // no YDIF sampled, but the row is on the current definition (the class comes from bitrate/height only)
		}
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
		slog.Float64("motion_mean", metrics.MotionMean),
		slog.Float64("scenecuts_per_min", metrics.ScenecutsPerMinute),
		slog.Int64("src_bitrate_kbps", metrics.SourceBitrateKbps),
		slog.Int("frames_sampled", metrics.FramesSampled))
}

// safeProcessBatch contains a panic to THAT batch.
//
// The goroutine-level recover wrapping the loop is not enough: it catches the
// panic and then returns, ending the worker for the rest of the process
// lifetime, silently. Whatever this worker is responsible for simply stops
// happening, with one log line as the only trace. One bad batch must cost one
// batch. Same reasoning as WatchProgressBuffer.safeFlush and
// WatcherService.safeProbeRound.
func (w *ComplexityWorker) safeProcessBatch(ctx context.Context) (n int) {
	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("batch panicked, continuing with the next one", slog.Any("panic", r))
			n = 0
		}
	}()
	return w.processBatch(ctx)
}
