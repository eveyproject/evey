package transcoding

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// LevelBackfillStore is the narrow persistence slice the worker needs: find
// candidates whose H.264 video level is missing, and fill it from a fresh probe.
// Implemented by the media-stream repository.
type LevelBackfillStore interface {
	// ListVideoLevelMissing returns up to `limit` media item IDs with an h264
	// video stream whose level IS NULL, excluding the given IDs.
	ListVideoLevelMissing(ctx context.Context, limit int, exclude []string) ([]string, error)
	// SetVideoLevels fills the level of the item's video streams from a probe,
	// matched by stream_index, writing only missing levels and never deleting.
	// Returns how many rows were updated.
	SetVideoLevels(ctx context.Context, mediaItemID shared.MediaItemID, streams []*library.MediaStream) (int, error)
}

// LevelBackfillWorker re-probes existing media in the background to fill the
// media_streams.level column the scanner never back-populates (it skips files
// whose mtime/fingerprint are unchanged). Level is what gates the H.264
// video-copy (remux) rung: without it the master manifest can't advertise an
// accurate avc1.PPCCLL, so a copyable title is transcoded instead.
//
// It is the only component that hits the (possibly weak, networked) media store
// in a sustained background loop, so it is paced conservatively and every knob is
// tunable — the rate MUST be validated on the real target NAS, not a dev disk.
//
// Three invariants:
//   - Paced: a small batch per tick, a pause between probes, a long idle sleep.
//   - Resumable after a crash with no full re-scan: the candidate query itself
//     ("h264 video, level IS NULL") shrinks as levels fill, so a restart simply
//     re-queries the remainder; there is no cursor to lose.
//   - Per-file failure = skip + mark (no loop): a probe that errors, or that
//     resolves no new level, is added to an in-run skip-set excluded from the
//     query, so it is attempted at most once per run instead of spinning. The set
//     is in-memory by design: a transient failure (e.g. a briefly unmounted disk)
//     is retried once on the next restart, which is the wanted behavior.
type LevelBackfillWorker struct {
	store  LevelBackfillStore
	items  MediaItemReader
	prober library.StreamProber
	logger *slog.Logger

	batchSize    int
	idleInterval time.Duration
	itemPause    time.Duration

	// attempted is the in-run skip-set. The run loop is single-goroutine, so no
	// lock is needed.
	attempted map[string]struct{}

	stop chan struct{}
	done chan struct{}
}

// LevelBackfillOpts tunes the worker pacing. All zero values fall back to
// conservative defaults; override them (e.g. from env) once measured on the NAS.
type LevelBackfillOpts struct {
	BatchSize    int           // items pulled per tick. Default 5.
	IdleInterval time.Duration // sleep when nothing fresh is left. Default 30 min.
	ItemPause    time.Duration // gap between two probes. Default 3 s.
}

func NewLevelBackfillWorker(
	store LevelBackfillStore,
	items MediaItemReader,
	prober library.StreamProber,
	opts LevelBackfillOpts,
	logger *slog.Logger,
) *LevelBackfillWorker {
	if logger == nil {
		logger = slog.Default()
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 5
	}
	if opts.IdleInterval <= 0 {
		opts.IdleInterval = 30 * time.Minute
	}
	if opts.ItemPause <= 0 {
		opts.ItemPause = 3 * time.Second
	}
	return &LevelBackfillWorker{
		store:        store,
		items:        items,
		prober:       prober,
		logger:       logger.With("component", "level_backfill_worker"),
		batchSize:    opts.BatchSize,
		idleInterval: opts.IdleInterval,
		itemPause:    opts.ItemPause,
		attempted:    make(map[string]struct{}),
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
	}
}

// Start kicks off the loop in a goroutine. Stop with Stop().
func (w *LevelBackfillWorker) Start(ctx context.Context) {
	go w.run(ctx)
}

// Stop signals the worker to finish its current item and exit. Blocks until the
// goroutine returns.
func (w *LevelBackfillWorker) Stop() {
	close(w.stop)
	<-w.done
}

func (w *LevelBackfillWorker) run(ctx context.Context) {
	defer close(w.done)
	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("level backfill worker panic recovered",
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
			// Nothing fresh to do — sleep longer.
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

// processBatch pulls one batch of candidates (excluding the in-run skip-set) and
// backfills each. Returns the count of items pulled (0 ⇒ nothing fresh left,
// sleep deeper).
func (w *LevelBackfillWorker) processBatch(ctx context.Context) int {
	ids, err := w.store.ListVideoLevelMissing(ctx, w.batchSize, w.excluded())
	if err != nil {
		w.logger.Warn("level backfill: list candidates failed", slog.Any("err", err))
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
			// Failed, or resolved no new level — mark so it isn't re-queried this
			// run (skip + mark, no loop).
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

// backfillOne probes one item and fills its video level. Returns true only when
// at least one level was written (progress); false on any skip/failure so the
// caller marks it. It NEVER replaces streams — a degraded or empty probe can't
// wipe existing metadata the way a full replace would.
func (w *LevelBackfillWorker) backfillOne(ctx context.Context, mediaItemID string) bool {
	item, err := w.items.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil {
		w.logger.Warn("level backfill: item lookup failed",
			slog.String("media_id", mediaItemID), slog.Any("err", err))
		return false
	}
	if item == nil || item.File == nil || item.File.Path == "" {
		return false // no file to probe
	}

	res, err := w.prober.ProbeStreams(ctx, item.File.Path)
	if err != nil || res == nil || len(res.Streams) == 0 || !hasVideoStream(res.Streams) {
		// Skip: never persist a degraded/empty probe.
		w.logger.Warn("level backfill: probe failed or empty",
			slog.String("media_id", mediaItemID),
			slog.String("path", item.File.Path),
			slog.Any("err", err))
		return false
	}

	updated, err := w.store.SetVideoLevels(ctx, shared.MediaItemID(mediaItemID), res.Streams)
	if err != nil {
		w.logger.Warn("level backfill: persist failed",
			slog.String("media_id", mediaItemID), slog.Any("err", err))
		return false
	}
	if updated == 0 {
		// Probe succeeded but no h264 level resolved — leave NULL; mark to avoid a
		// re-probe loop on a file whose level ffprobe never reports.
		return false
	}
	w.logger.Info("level backfill: filled",
		slog.String("media_id", mediaItemID),
		slog.String("title", item.Title),
		slog.Int("streams_updated", updated))
	return true
}

// excluded snapshots the in-run skip-set for the candidate query.
func (w *LevelBackfillWorker) excluded() []string {
	if len(w.attempted) == 0 {
		return nil
	}
	out := make([]string, 0, len(w.attempted))
	for id := range w.attempted {
		out = append(out, id)
	}
	return out
}

func hasVideoStream(streams []*library.MediaStream) bool {
	for _, s := range streams {
		if s != nil && s.StreamType == library.Video {
			return true
		}
	}
	return false
}
