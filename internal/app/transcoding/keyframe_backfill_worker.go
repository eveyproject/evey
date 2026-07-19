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

// FrameRateAnalyzer classifies a video file's frame-rate mode ("cfr" / "vfr"),
// or "" for unknown (a non-EBML container, or too few frames to classify) -
// never an error for that case. Implemented by the mkvgo adapter's
// AnalyzeFrameRateMode, a structural head-only pass over block headers.
type FrameRateAnalyzer interface {
	AnalyzeFrameRateMode(ctx context.Context, filePath string) (string, error)
}

// FrameRateStore is the persistence slice the cfr/vfr classification needs.
// Kept separate from KeyframeBackfillStore (rather than adding a method to
// it) so the on-demand keyframe path (keyframe_ondemand.go), which shares
// KeyframeBackfillStore but never analyzes frame rate, is unaffected by this
// worker-only need.
type FrameRateStore interface {
	// SetFrameRateMode stores the item's video stream(s) cfr/vfr classification
	// ("" for unknown is a legitimate write, unlike SetKeyframes), stamped with
	// the classifier version. Never overwrites an existing verdict. Returns rows
	// updated.
	SetFrameRateMode(ctx context.Context, mediaItemID shared.MediaItemID, mode string, algoVersion int16) (int, error)
	// ListVideoFrameRateStale returns items whose verdict predates algoVersion.
	ListVideoFrameRateStale(ctx context.Context, limit int, algoVersion int16, exclude []string) ([]string, error)
	// ReclassifyFrameRateMode overwrites a verdict older than algoVersion.
	ReclassifyFrameRateMode(ctx context.Context, mediaItemID shared.MediaItemID, mode string, algoVersion int16) (int, error)
}

// FrameRateAlgoVersion identifies the classifier behind a stored cfr/vfr
// verdict. BUMP IT whenever the classification can change for the same bytes -
// that is the supported way to force a library-wide re-analysis, with no DB
// surgery and no admin action: rows stamped below the current version are
// picked up by the backfill worker's re-classification drain and restamped.
//
// 1 (mkvgo v0.25.1): first versioned classifier. Everything before it reads
// back as 0 and gets re-analyzed, because the older variance test mistook a
// B-frame decode-order artifact for a frame-duration outlier and labelled
// ms-quantised CFR sources 'vfr' - which gates the copy rung off and forces a
// pointless transcode (see CopyRungEligible).
const FrameRateAlgoVersion int16 = 1

// KeyframeBackfillWorker re-probes copy-eligible existing media to fill the
// keyframe index (media_streams.keyframes_ms) the scan never extracts - the
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
// Pacing (mkvgo era): the prober fed to this worker is an mkvgo-fronted
// KeyframeDispatcher (wire.go) that reads the container seek index head-only
// (Cues for mkv, stss for mp4) in milliseconds, and covers even cue-less MKVs
// with an in-process structural scan (0.5-16s, ~1.5% of a real corpus); the
// ffprobe fork only remains for exotic containers (avi/mpegts) and parse
// failures. The defaults are therefore assertive - a multi-thousand-file
// library sweeps in minutes - and back off to politeItemPause whenever
// playback sessions are active (WithActivePlayback), so a weak NAS serving a
// stream never competes with the sweep. The historical ffprobe-era pacing
// (5 / 2s / 1h) remains reachable via the server_config keys.
type KeyframeBackfillWorker struct {
	store  KeyframeBackfillStore
	items  MediaItemReader
	prober KeyframeProber
	logger *slog.Logger

	// frameRate / frameRateStore are optional (nil = frame-rate classification
	// is skipped, the pre-v0.17.0 mkvgo behaviour). Set via WithFrameRateAnalyzer.
	frameRate      FrameRateAnalyzer
	frameRateStore FrameRateStore

	batchSize    int
	idleInterval time.Duration
	itemPause    time.Duration
	// activePlayback, when set, reports live playback sessions; a non-zero
	// count switches the per-item pause to politeItemPause.
	activePlayback func() int

	attempted map[string]struct{} // in-run skip-set (single-goroutine, no lock)
	// frAttempted is the re-classification drain's OWN skip-set. Kept apart from
	// `attempted` so a file whose keyframe probe failed is still re-classified
	// (and vice versa) - the two passes read the same file but answer different
	// questions, and one failing says nothing about the other.
	frAttempted map[string]struct{}

	stop  chan struct{}
	done  chan struct{}
	nudge chan struct{} // wakes an idle run() immediately (buffered 1)

	quietMu    sync.Mutex
	quietTimer *time.Timer // pending NudgeAfterQuiet debounce

	startOnce sync.Once // Start spawns the run goroutine at most once.
	stopOnce  sync.Once // Stop closes the stop channel at most once.
}

// politeItemPause is the per-item gap while playback sessions are active -
// the ffprobe-era pacing, kept as the "someone is watching" mode.
const politeItemPause = 2 * time.Second

// KeyframeBackfillOpts tunes the worker pacing. Zero values fall back to the
// defaults.
type KeyframeBackfillOpts struct {
	BatchSize    int           // items per tick. Default 64.
	IdleInterval time.Duration // sleep when nothing fresh is left. Default 10 min.
	ItemPause    time.Duration // gap between two probes. Default 200 ms.
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
		opts.BatchSize = 64
	}
	if opts.IdleInterval <= 0 {
		opts.IdleInterval = 10 * time.Minute
	}
	if opts.ItemPause <= 0 {
		opts.ItemPause = 200 * time.Millisecond
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
		frAttempted:  make(map[string]struct{}),
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
		nudge:        make(chan struct{}, 1),
	}
}

// WithActivePlayback wires the live playback-session counter: while it
// reports > 0 the per-item pause switches to politeItemPause so the sweep
// never competes with a stream on a weak NAS. Chainable; nil = fixed pacing.
func (w *KeyframeBackfillWorker) WithActivePlayback(count func() int) *KeyframeBackfillWorker {
	w.activePlayback = count
	return w
}

// WithFrameRateAnalyzer wires the mkvgo cfr/vfr classifier: each file this
// worker indexes keyframes for is also, in the SAME per-item iteration (no
// extra library sweep), classified and persisted via store.SetFrameRateMode.
// Chainable; leaving it unset skips frame-rate classification entirely.
func (w *KeyframeBackfillWorker) WithFrameRateAnalyzer(a FrameRateAnalyzer, store FrameRateStore) *KeyframeBackfillWorker {
	w.frameRate = a
	w.frameRateStore = store
	return w
}

// Nudge wakes an idle worker immediately (no-op when it is already working -
// the buffered channel coalesces bursts). Safe from any goroutine.
func (w *KeyframeBackfillWorker) Nudge() {
	select {
	case w.nudge <- struct{}{}:
	default:
	}
}

// NudgeAfterQuiet arms (or re-arms) a debounce: the worker is nudged once no
// further NudgeAfterQuiet call has happened for d. Wired to the library
// scan-completed event so fresh additions are indexed as soon as the library
// settles instead of waiting for the idle cycle.
func (w *KeyframeBackfillWorker) NudgeAfterQuiet(d time.Duration) {
	w.quietMu.Lock()
	defer w.quietMu.Unlock()
	if w.quietTimer != nil {
		w.quietTimer.Stop()
	}
	w.quietTimer = time.AfterFunc(d, w.Nudge)
}

func (w *KeyframeBackfillWorker) Start(ctx context.Context) {
	w.startOnce.Do(func() { go w.run(ctx) })
}

func (w *KeyframeBackfillWorker) Stop() {
	w.stopOnce.Do(func() { close(w.stop) })
	w.quietMu.Lock()
	if w.quietTimer != nil {
		w.quietTimer.Stop()
	}
	w.quietMu.Unlock()
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
			case <-w.nudge:
			case <-w.stop:
				return
			case <-ctx.Done():
				return
			}
		}
	}
}

// pause returns the current inter-item gap: polite while anyone is watching.
func (w *KeyframeBackfillWorker) pause() time.Duration {
	if w.activePlayback != nil && w.activePlayback() > 0 {
		return politeItemPause
	}
	return w.itemPause
}

// processBatch drains the keyframe queue first and only falls through to the
// frame-rate re-classification once it is empty, so the sweep's priority order
// is explicit rather than emergent.
func (w *KeyframeBackfillWorker) processBatch(ctx context.Context) int {
	if n := w.processKeyframeBatch(ctx); n > 0 {
		return n
	}
	return w.processFrameRateBatch(ctx)
}

func (w *KeyframeBackfillWorker) processKeyframeBatch(ctx context.Context) int {
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
		// Pause after EVERY item, including the last of the batch: run() loops
		// straight into the next batch, so skipping the gap on the last item
		// let two probes run back-to-back across the batch boundary (every 64
		// items on a large library), defeating the politeness pacing. The
		// trailing pause when the queue then empties is harmless - it precedes
		// the far longer idle sleep.
		select {
		case <-time.After(w.pause()):
		case <-w.stop:
			return i + 1
		case <-ctx.Done():
			return i + 1
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

	w.backfillFrameRateMode(ctx, mediaItemID, item.File.Path)
	return true
}

// backfillFrameRateMode classifies item.File.Path's cfr/vfr mode and persists
// it, piggybacking on the SAME per-item pass backfillOne already pays for
// (no extra library sweep). Best-effort: a failure here does not fail the
// keyframe write it rides along with, and is not marked into the skip-set -
// the next sweep over this file (should the item resurface for any other
// reason) simply retries it.
func (w *KeyframeBackfillWorker) backfillFrameRateMode(ctx context.Context, mediaItemID, filePath string) {
	if w.frameRate == nil || w.frameRateStore == nil {
		return
	}
	mode, err := w.frameRate.AnalyzeFrameRateMode(ctx, filePath)
	if err != nil {
		w.logger.Warn("keyframe backfill: frame-rate analysis failed",
			slog.String("media_id", mediaItemID), slog.Any("err", err))
		return
	}
	if _, err := w.frameRateStore.SetFrameRateMode(ctx, shared.MediaItemID(mediaItemID), mode, FrameRateAlgoVersion); err != nil {
		w.logger.Warn("keyframe backfill: frame-rate persist failed",
			slog.String("media_id", mediaItemID), slog.Any("err", err))
	}
}

// processFrameRateBatch re-analyzes items whose cfr/vfr verdict predates
// FrameRateAlgoVersion. It runs only once the keyframe queue is drained,
// because a missing keyframe index blocks the copy rung outright while a stale
// verdict merely mis-gates it - the cheaper wrong answer waits its turn.
//
// Returns the number of items handled so run() keeps looping while there is
// work and falls into its idle sleep when there is not.
func (w *KeyframeBackfillWorker) processFrameRateBatch(ctx context.Context) int {
	if w.frameRate == nil || w.frameRateStore == nil {
		return 0
	}
	ids, err := w.frameRateStore.ListVideoFrameRateStale(ctx, w.batchSize, FrameRateAlgoVersion, w.frExcluded())
	if err != nil {
		w.logger.Warn("frame-rate reclassify: list candidates failed", slog.Any("err", err))
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
		if !w.reclassifyOne(ctx, id) {
			w.frAttempted[id] = struct{}{}
		}
		select {
		case <-time.After(w.pause()):
		case <-w.stop:
			return i + 1
		case <-ctx.Done():
			return i + 1
		}
	}
	return len(ids)
}

// reclassifyOne re-runs the analyzer on one item and restamps its verdict.
// Reports whether the item can be considered handled; false puts it in the
// drain's skip-set so a permanently unreadable file is retried at most once
// per run instead of pinning the queue.
func (w *KeyframeBackfillWorker) reclassifyOne(ctx context.Context, mediaItemID string) bool {
	item, err := w.items.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil || item == nil {
		w.logger.Warn("frame-rate reclassify: load item failed",
			slog.String("media_id", mediaItemID), slog.Any("err", err))
		return false
	}
	mode, err := w.frameRate.AnalyzeFrameRateMode(ctx, item.File.Path)
	if err != nil {
		w.logger.Warn("frame-rate reclassify: analysis failed",
			slog.String("media_id", mediaItemID), slog.Any("err", err))
		return false
	}
	if mode == "" {
		// The analyzer cannot speak for this container (non-EBML, too few
		// frames). Restamping to "" would DROP a verdict that may well be
		// right, so leave the row alone and skip it for this run. Logged: this
		// row stays stale forever otherwise, and silence would make a whole
		// library of them indistinguishable from "nothing to do".
		w.logger.Debug("frame-rate reclassify: analyzer returned unknown, verdict left as-is",
			slog.String("media_id", mediaItemID))
		return false
	}
	n, err := w.frameRateStore.ReclassifyFrameRateMode(ctx, shared.MediaItemID(mediaItemID), mode, FrameRateAlgoVersion)
	if err != nil {
		w.logger.Warn("frame-rate reclassify: persist failed",
			slog.String("media_id", mediaItemID), slog.Any("err", err))
		return false
	}
	if n > 0 {
		w.logger.Info("frame-rate reclassified",
			slog.String("media_id", mediaItemID),
			slog.String("title", item.Title),
			slog.String("frame_rate_mode", mode))
	}
	return true
}

func (w *KeyframeBackfillWorker) excluded() []string {
	return keysOf(w.attempted)
}

func (w *KeyframeBackfillWorker) frExcluded() []string {
	return keysOf(w.frAttempted)
}

func keysOf(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	return out
}
