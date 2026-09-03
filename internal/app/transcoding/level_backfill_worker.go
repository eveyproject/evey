package transcoding

import (
	"context"
	"log/slog"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/bgtasks"
)

// LevelBackfillStore is the narrow persistence slice the worker needs: find
// candidates whose copy-rung video metadata is missing, and fill it from a
// fresh probe. Implemented by the media-stream repository.
type LevelBackfillStore interface {
	// ListVideoLevelMissing returns up to `limit` media item IDs with an
	// h264/hevc video stream missing copy-rung metadata (h264: level IS NULL;
	// hevc: level OR color_transfer IS NULL), excluding the given IDs.
	ListVideoLevelMissing(ctx context.Context, limit int, exclude []string) ([]string, error)
	// SetVideoLevels fills the copy-rung metadata (level, color_transfer,
	// dovi_profile, dovi_bl_compat, field_order, sample_aspect_ratio) of the
	// item's video streams from a probe, matched by stream_index, writing only
	// missing values and never deleting. Returns how many rows were updated.
	SetVideoLevels(ctx context.Context, mediaItemID shared.MediaItemID, streams []*library.MediaStream) (int, error)
	// SetIdetInterlaced stores the idet content-detection verdict (a
	// DEFINITIVE true/false - the candidate query keys on NULL, so a negative
	// must persist or the decode-heavy probe re-runs forever). Never
	// overwrites an existing verdict. Returns rows updated.
	SetIdetInterlaced(ctx context.Context, mediaItemID shared.MediaItemID, interlaced bool) (int, error)
}

// InterlaceDetector is the OPTIONAL prober capability the idet fallback needs.
// The worker type-asserts it from its StreamProber (the ffmpeg prober
// implements it); absent, legacy-codec candidates simply skip the idet phase.
type InterlaceDetector interface {
	ProbeInterlaceDetect(ctx context.Context, filePath string) (bool, error)
}

// idetSuspectCodecs are the DVD/DivX-era codecs whose rips routinely carry
// interlaced CONTENT without decoder flags (repacks re-encode the fields as
// "progressive" frames). Only these pay the idet decode - modern codecs trust
// their flags.
var idetSuspectCodecs = map[string]bool{"mpeg2video": true, "mpeg4": true, "msmpeg4v3": true}

// LevelBackfillWorker re-probes existing media in the background to fill the
// copy-rung metadata columns the scanner never back-populates (it skips files
// whose mtime/fingerprint are unchanged): media_streams.level - what gates the
// video-copy (remux) rung, since without it the master manifest can't
// advertise an accurate RFC-6381 CODECS string - and, for HEVC, the HDR
// ground truth (color_transfer + Dolby Vision profile) without which a DoVi
// profile-5 title could be raw-served with broken colours.
//
// It is the only component that hits the (possibly weak, networked) media store
// in a sustained background loop, so it is paced conservatively and every knob is
// tunable - the rate MUST be validated on the real target NAS, not a dev disk.
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

	// activePlayback reports live playback sessions; while > 0 the worker
	// yields entirely. A header probe is far lighter than a full-file walk,
	// but on the weak-NAS target every competing disk read counts, and the
	// backfill is never urgent. Optional (nil = no gating).
	activePlayback func() int

	stop chan struct{}
	done chan struct{}

	startOnce sync.Once // Start spawns the run goroutine at most once.
	stopOnce  sync.Once // Stop closes the stop channel at most once.
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

// WithActivePlayback wires the live-session counter (same signal as the
// keyframe worker's, see wire.go). While it reports > 0 the worker suspends
// instead of probing alongside a viewer.
func (w *LevelBackfillWorker) WithActivePlayback(fn func() int) *LevelBackfillWorker {
	w.activePlayback = fn
	return w
}

// Start kicks off the loop in a goroutine. Stop with Stop(). Idempotent: a
// second Start is a no-op, so a double lifecycle owner can't spawn two loops.
func (w *LevelBackfillWorker) Start(ctx context.Context) {
	w.startOnce.Do(func() { go w.run(ctx) })
}

// Stop signals the worker to finish its current item and exit. Blocks until the
// goroutine returns. Idempotent: a second Stop won't re-close the channel.
func (w *LevelBackfillWorker) Stop() {
	w.stopOnce.Do(func() { close(w.stop) })
	<-w.done
}

// levelBackfillTask is this worker's name in the bgtasks registry.
const levelBackfillTask = "level_backfill"

func (w *LevelBackfillWorker) run(ctx context.Context) {
	bgtasks.RegisterControllable(levelBackfillTask)
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

		processed := w.safeProcessBatch(ctx)
		if processed == 0 {
			// Nothing fresh to do - sleep longer.
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
	// SUSPEND while anyone is watching (2026-07-19 starvation incident family:
	// this worker used to keep forking probes during playback). The backfill
	// is never urgent; a viewer always is.
	if w.activePlayback != nil && w.activePlayback() > 0 {
		return 0
	}
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
		// Re-check between items so a viewer arriving mid-batch waits for at
		// most the probe already in flight, not the batch remainder.
		if w.activePlayback != nil && w.activePlayback() > 0 {
			return i
		}
		// An admin's Stop holds the worker here; a stop that lands mid-item
		// cancels that item, which is not a verdict on the file.
		if bgtasks.WaitResumed(ctx, levelBackfillTask, w.stop) != nil {
			return i
		}
		progressed := w.backfillOne(ctx, id)
		if bgtasks.Paused(levelBackfillTask) {
			return i
		}
		if !progressed {
			// Failed, or resolved no new level - mark so it isn't re-queried this
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
// caller marks it. It NEVER replaces streams - a degraded or empty probe can't
// wipe existing metadata the way a full replace would.
func (w *LevelBackfillWorker) backfillOne(ctx context.Context, mediaItemID string) bool {
	ctx, done := bgtasks.BeginCtx(ctx, levelBackfillTask, mediaItemID)
	defer done()
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

	// idet phase: legacy-codec sources whose flags don't already say
	// interlaced get a CONTENT detection (the flags routinely lie on repacked
	// DVD rips). The verdict - true or false - is persisted so the candidate
	// query (idet_interlaced IS NULL) converges; a probe failure leaves NULL
	// and the in-run skip-set prevents a tight loop.
	updated += w.idetPhase(ctx, mediaItemID, item.File.Path, res.Streams)

	if updated == 0 {
		// Probe succeeded but resolved nothing new (no level, and for hevc no
		// colour/DoVi metadata) - leave NULL; mark to avoid a re-probe loop on a
		// file whose metadata ffprobe never reports.
		return false
	}
	w.logger.Info("level backfill: filled",
		slog.String("media_id", mediaItemID),
		slog.String("title", item.Title),
		slog.Int("streams_updated", updated))
	return true
}

// idetPhase runs the interlace content detection for a legacy-codec video
// stream and persists the verdict. Returns the rows updated (0 when the
// stream isn't a suspect, the prober lacks the capability, the flags already
// say interlaced - bwdif's flag gate handles those - or the probe failed).
func (w *LevelBackfillWorker) idetPhase(ctx context.Context, mediaItemID, path string, streams []*library.MediaStream) int {
	detector, ok := w.prober.(InterlaceDetector)
	if !ok {
		return 0
	}
	var video *library.MediaStream
	for _, s := range streams {
		if s != nil && s.StreamType == library.Video {
			video = s
			break
		}
	}
	if video == nil || !idetSuspectCodecs[strings.ToLower(video.Codec)] {
		return 0
	}
	if fo := video.FieldOrder; fo != nil {
		switch strings.ToLower(*fo) {
		case "tt", "bb", "tb", "bt":
			// Decoder-flagged interlaced: deint=interlaced already covers it, and
			// running deint=all on top would be redundant. Persist FALSE so the
			// candidate query converges without paying the decode.
			n, err := w.store.SetIdetInterlaced(ctx, shared.MediaItemID(mediaItemID), false)
			if err != nil {
				w.logger.Warn("idet backfill: persist flagged verdict failed",
					slog.String("media_id", mediaItemID), slog.Any("err", err))
				return 0
			}
			return n
		}
	}
	interlaced, err := detector.ProbeInterlaceDetect(ctx, path)
	if err != nil {
		w.logger.Warn("idet backfill: probe failed",
			slog.String("media_id", mediaItemID), slog.Any("err", err))
		return 0
	}
	n, err := w.store.SetIdetInterlaced(ctx, shared.MediaItemID(mediaItemID), interlaced)
	if err != nil {
		w.logger.Warn("idet backfill: persist failed",
			slog.String("media_id", mediaItemID), slog.Any("err", err))
		return 0
	}
	if interlaced {
		w.logger.Info("idet backfill: unflagged interlaced content detected",
			slog.String("media_id", mediaItemID))
	}
	return n
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

// safeProcessBatch contains a panic to THAT batch.
//
// The goroutine-level recover wrapping the loop is not enough: it catches the
// panic and then returns, ending the worker for the rest of the process
// lifetime, silently. Whatever this worker is responsible for simply stops
// happening, with one log line as the only trace. One bad batch must cost one
// batch. Same reasoning as WatchProgressBuffer.safeFlush and
// WatcherService.safeProbeRound.
func (w *LevelBackfillWorker) safeProcessBatch(ctx context.Context) (n int) {
	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("batch panicked, continuing with the next one", slog.Any("panic", r))
			n = 0
		}
	}()
	return w.processBatch(ctx)
}
