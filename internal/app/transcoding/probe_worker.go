package transcoding

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/bgtasks"
)

// probe_worker.go - the P3 per-title probe worker: it decides WHICH titles earn
// a probe and runs them one at a time in the background, never on the read
// path. It owns only triggering and lifecycle; the probe itself (scenes, CRF
// escalation, early-stop, scan priority, SIGSTOP, cache) lives in the ffmpeg
// TitleProber, and the ground truth it produces is read by the cost model.

// ProbeCandidateRepo lists the titles whose PERSISTED metrics make them a probe
// candidate (missing metrics, motion out of range, or the animation weak spot),
// excluding any already probed at the current model version. Ceiling-hit
// candidates come from the in-memory signal store, not here.
type ProbeCandidateRepo interface {
	ListProbeCandidates(ctx context.Context, modelVersion, limit int) ([]string, error)
	Get(ctx context.Context, mediaItemID string) (transcoding.ComplexityEntry, bool, error)
	Set(ctx context.Context, mediaItemID string, class transcoding.ContentClass, metrics transcoding.ComplexityMetrics, computedAt time.Time) error
}

// TitleProber runs one probe and returns the ground truth. Implemented by the
// ffmpeg adapter (ProbeTitle).
type TitleProber interface {
	Probe(ctx context.Context, sourcePath string, cost transcoding.TitleCost) (transcoding.TitleProbe, ProbeStats, error)
}

// ProbeStats is the observability of one probe run.
type ProbeStats struct {
	Points    int
	CacheHits int
	Wall      time.Duration
}

// ceilingHitSource is the in-memory signal store the worker consumes.
type ceilingHitSource interface {
	CeilingHitCandidates() []string
	Signals(mediaID string) transcoding.ProbeSignals
	Clear(mediaID string)
}

// ProbeWorker is background-only: a batch of candidates per tick, one probe at
// a time, paused while a playback is served.
type ProbeWorker struct {
	repo    ProbeCandidateRepo
	prober  TitleProber
	items   MediaItemReader
	signals ceilingHitSource
	model   transcoding.CostModel
	logger  *slog.Logger

	batchSize      int
	idleInterval   time.Duration
	itemPause      time.Duration
	activePlayback func() int

	// inFlight dedups: a title being probed (or queued this tick) is never
	// started again concurrently.
	mu       sync.Mutex
	inFlight map[string]bool

	stop      chan struct{}
	done      chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

// ProbeWorkerOpts tunes pacing.
type ProbeWorkerOpts struct {
	BatchSize    int
	IdleInterval time.Duration
	ItemPause    time.Duration
}

const probeTask = "title_probe"

// NewProbeWorker builds the worker. signals may be nil (only the persisted
// triggers then fire).
func NewProbeWorker(repo ProbeCandidateRepo, prober TitleProber, items MediaItemReader, signals ceilingHitSource, model transcoding.CostModel, opts ProbeWorkerOpts, logger *slog.Logger) *ProbeWorker {
	if logger == nil {
		logger = slog.Default()
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 3
	}
	if opts.IdleInterval <= 0 {
		opts.IdleInterval = 30 * time.Minute
	}
	if opts.ItemPause <= 0 {
		opts.ItemPause = 10 * time.Second
	}
	return &ProbeWorker{
		repo: repo, prober: prober, items: items, signals: signals, model: model,
		logger:       logger.With("component", "probe_worker"),
		batchSize:    opts.BatchSize,
		idleInterval: opts.IdleInterval,
		itemPause:    opts.ItemPause,
		inFlight:     map[string]bool{},
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
	}
}

// WithActivePlayback wires the live-stream counter; the worker skips a tick
// while a playback is being served (the probe's own ffmpeg also SIGSTOPs, this
// just avoids even starting one).
func (w *ProbeWorker) WithActivePlayback(count func() int) *ProbeWorker {
	w.activePlayback = count
	return w
}

func (w *ProbeWorker) playbackActive() bool {
	return w.activePlayback != nil && w.activePlayback() > 0
}

// Start / Stop mirror the complexity worker.
func (w *ProbeWorker) Start(ctx context.Context) { w.startOnce.Do(func() { go w.run(ctx) }) }
func (w *ProbeWorker) Stop() {
	w.stopOnce.Do(func() { close(w.stop) })
	<-w.done
}

func (w *ProbeWorker) run(ctx context.Context) {
	defer close(w.done)
	for {
		n := w.safeProcessBatch(ctx)
		wait := w.idleInterval
		if n > 0 {
			wait = w.itemPause
		}
		select {
		case <-w.stop:
			return
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

func (w *ProbeWorker) safeProcessBatch(ctx context.Context) (n int) {
	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("probe: batch panic recovered", slog.Any("recover", r))
			n = 0
		}
	}()
	return w.processBatch(ctx)
}

// candidates gathers this tick's IDs: the ceiling-hit set (from memory) first,
// then the persisted-metric candidates, deduped and bounded to the batch.
func (w *ProbeWorker) candidates(ctx context.Context) []string {
	seen := map[string]bool{}
	var out []string
	add := func(ids []string) {
		for _, id := range ids {
			if len(out) >= w.batchSize {
				return
			}
			if id != "" && !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	if w.signals != nil {
		add(w.signals.CeilingHitCandidates())
	}
	if len(out) < w.batchSize {
		if ids, err := w.repo.ListProbeCandidates(ctx, w.model.Version, w.batchSize); err == nil {
			add(ids)
		} else {
			w.logger.Warn("probe: list candidates failed", slog.Any("err", err))
		}
	}
	return out
}

func (w *ProbeWorker) processBatch(ctx context.Context) int {
	if w.playbackActive() {
		return 0 // a viewer is being served: don't even start a probe this tick
	}
	ids := w.candidates(ctx)
	done := 0
	for i, id := range ids {
		select {
		case <-w.stop:
			return done
		case <-ctx.Done():
			return done
		default:
		}
		if bgtasks.WaitResumed(ctx, probeTask, w.stop) != nil {
			return done
		}
		if w.playbackActive() {
			return done // a playback arrived mid-batch: yield
		}
		if w.probeOne(ctx, id) {
			done++
		}
		if i < len(ids)-1 {
			select {
			case <-time.After(w.itemPause):
			case <-w.stop:
				return done
			case <-ctx.Done():
				return done
			}
		}
	}
	return done
}

// claim marks a title in-flight; false when another goroutine already holds it.
func (w *ProbeWorker) claim(id string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.inFlight[id] {
		return false
	}
	w.inFlight[id] = true
	return true
}

func (w *ProbeWorker) release(id string) {
	w.mu.Lock()
	delete(w.inFlight, id)
	w.mu.Unlock()
}

// probeOne confirms the trigger, runs the probe and persists the ground truth.
// Returns true when a probe actually ran.
func (w *ProbeWorker) probeOne(ctx context.Context, mediaID string) (ran bool) {
	if !w.claim(mediaID) {
		return false // already being probed: strict de-dup
	}
	defer w.release(mediaID)

	ctx, endTask := bgtasks.BeginCtx(ctx, probeTask, mediaID)
	defer endTask()

	entry, _, err := w.repo.Get(ctx, mediaID)
	if err != nil {
		w.logger.Warn("probe: metrics lookup failed", slog.String("media_id", mediaID), slog.Any("err", err))
		return false
	}
	sig := transcoding.ProbeSignals{}
	if w.signals != nil {
		sig = w.signals.Signals(mediaID)
	}
	sig.Animation = entry.Class == transcoding.ClassAnimation
	reason, want := transcoding.ProbeCandidate(entry.Metrics, sig, w.model)
	if !want {
		// The signal or the metrics no longer justify it (already probed,
		// re-classified, count decayed): drop the stale ceiling-hit and move on.
		if w.signals != nil {
			w.signals.Clear(mediaID)
		}
		return false
	}
	item, err := w.items.GetByID(ctx, shared.MediaItemID(mediaID))
	if err != nil || item == nil || item.File == nil || item.File.Path == "" {
		return false
	}

	cost, _ := w.model.Predict(entry.Metrics) // may be !ok (no_metrics): the probe measures the truth regardless
	start := time.Now()
	probe, stats, perr := w.prober.Probe(ctx, item.File.Path, cost)
	if perr != nil {
		if ctx.Err() != nil {
			return false // cancelled: no verdict persisted
		}
		w.logger.Warn("probe: run failed", slog.String("media_id", mediaID), slog.String("reason", string(reason)), slog.Any("err", perr))
		// Record the failure so the title leaves the candidate set instead of
		// retrying every idle cycle - a probe that fails on a readable file
		// fails identically each time.
		entry.Metrics.Probe = &transcoding.TitleProbe{Protocol: transcoding.TitleProbeProtocol, ModelVersion: w.model.Version, Failed: true, MeasuredAt: time.Now().UTC()}
		if setErr := w.repo.Set(ctx, mediaID, entry.Class, entry.Metrics, entry.ComputedAt); setErr != nil {
			w.logger.Warn("probe: persist failure marker failed", slog.String("media_id", mediaID), slog.Any("err", setErr))
		}
		if w.signals != nil {
			w.signals.Clear(mediaID)
		}
		return false
	}

	entry.Metrics.Probe = &probe
	if setErr := w.repo.Set(ctx, mediaID, entry.Class, entry.Metrics, entry.ComputedAt); setErr != nil {
		w.logger.Warn("probe: persist failed", slog.String("media_id", mediaID), slog.Any("err", setErr))
		return false
	}
	if w.signals != nil {
		w.signals.Clear(mediaID)
	}
	scenes := probe.Scenes(transcoding.StaticSoftwareCRF)
	w.logger.Info("probe: measured",
		slog.String("media_id", mediaID),
		slog.String("title", item.Title),
		slog.String("reason", string(reason)),
		slog.Bool("confirmed", probe.Confirmed),
		slog.Bool("early_stop", probe.Confirmed && scenes == 1),
		slog.Int("scenes", scenes),
		slog.Int("points", stats.Points),
		slog.Int("cache_hits", stats.CacheHits),
		slog.Float64("predicted_kbps", probe.PredictedKbps),
		slog.Float64("measured_kbps", probe.MeasuredKbps),
		slog.Float64("ratio", probe.Ratio),
		slog.Bool("overrides_model", probe.Usable(w.model.Version, transcoding.StaticSoftwareCRF)),
		slog.Duration("wall", time.Since(start)))
	return true
}
