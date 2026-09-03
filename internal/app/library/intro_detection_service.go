package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/bgtasks"
	"github.com/evey-project/evey/internal/pkg/procfork"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// IntroDetector is the port for the FFmpeg intro-detection adapter.
type IntroDetector interface {
	ExtractTileHashes(spritePath string, tileWidth, tileHeight, tilePixelWidth, tilePixelHeight int) ([]uint64, error)
	PrecisionPass(ctx context.Context, filePath string, startSec, endSec, padSec float64) (int64, int64, error)
	AudioFallback(ctx context.Context, filePath string, maxSec float64) (int64, int64, bool, error)
	DetectEndCredits(ctx context.Context, filePath string, durationSec float64) (int64, int64, bool, error)
	DetectLeadingLogoBlock(ctx context.Context, filePath string, durationSec float64) (int64, int64, bool, error)
	DetectStudioLogo(hashes []uint64, intervalSec int) (int64, int64, bool)
	AudioBandedFingerprint(ctx context.Context, filePath string, startSec, durationSec float64, binMs int) ([]float64, error)
	SubtitleCueStarts(ctx context.Context, filePath string) ([]int64, error)
	SceneCutTimestamps(ctx context.Context, filePath string, startSec, durationSec, threshold float64) ([]int64, error)
}

// Chromaprinter is an optional adapter wrapping fpcalc - when set, the
// confirmAudioCandidates pass uses Chromaprint fingerprints (industry
// standard, robust across mix/codec/language differences) instead of the
// in-house banded Pearson. Falls back to banded when the adapter is nil
// or returns an error.
type Chromaprinter interface {
	Available(ctx context.Context) error
	Compute(ctx context.Context, filePath string, startSec, lengthSec float64) ([]uint32, error)
}

// IntroDetectionService orchestrates the two-pass intro detection pipeline.
type IntroDetectionService struct {
	segmentRepo       domainlibrary.MediaSegmentRepository
	trickplayRepo     domainlibrary.TrickplayRepository
	mediaItems        domainlibrary.MediaItemRepository
	configRepo        shared.ServerConfigRepository
	fpCache           domainlibrary.IntroFingerprintCacheRepository
	audioFPCache      domainlibrary.IntroAudioFingerprintCacheRepository
	chapterRepo       domainlibrary.ChapterQueryRepo
	chromaprinter     Chromaprinter
	metadataProvider  domainlibrary.IntroMetadataProvider
	contributor       domainlibrary.SegmentContributor
	detector          IntroDetector
	trickplayCacheDir string
	shutdownCtx       context.Context
	events            shared.EventBus
	// concurrencyLimiter caps the number of pipeline runs in flight at once.
	// Each pipeline run is CPU + IO heavy (sprite decode, ffmpeg audio
	// extraction across all siblings) so unbounded parallelism - what the
	// /api/v1/admin/media/{id}/detect-segments endpoint allowed before - pegs
	// the box and triggers context deadline exceeded on individual queries.
	// Guarded by limiterMu: limiterForRun swaps the channel when the admin
	// use-all-cores toggle changes the desired capacity, without a restart.
	concurrencyLimiter chan struct{}
	limiterMu          sync.Mutex
	// maxParallelBase is the wire-time cap (trickplay CPU budget or env
	// override); maxParallelAllCores is the full CPU budget the admin toggle
	// escalates to (0 = toggle inert).
	maxParallelBase     int
	maxParallelAllCores int

	// activePlayback, when set, reports live playback sessions. The worker defers
	// heavy detection (pHash + trickplay decode) while anyone is watching so it
	// can't starve an active transcode on a weak NAS - the trigger is re-armed
	// until playback goes idle rather than dropped (no periodic catch-up exists).
	activePlayback func() int

	// episodeBudget bounds one episode's detection run (0 = the 30-minute
	// default). Tests shrink it; production keeps the default.
	episodeBudget time.Duration

	// backfillInFlight tracks the library IDs with a BackfillLibrary sweep
	// currently running, so a duplicate admin trigger is rejected instead of
	// stacking a concurrent sweep over the same seasons.
	backfillMu       sync.Mutex
	backfillInFlight map[string]struct{}

	// backfillBootDelay and backfillIdle pace the resumable sweep (0 = the
	// defaults). Tests shrink them; production keeps the defaults.
	backfillBootDelay time.Duration
	backfillIdle      time.Duration
}

// ErrIntroBackfillAlreadyRunning is returned by BackfillLibrary when a sweep
// for the same library is still in flight.
var ErrIntroBackfillAlreadyRunning = errors.New("intro backfill already running for this library")

// backfillAllLibraries is the guard key of the catalogue-wide sweep. It is
// EXCLUSIVE with every per-library key in both directions: it covers all the
// libraries, so letting an admin trigger run underneath it would be exactly the
// "two concurrent sweeps over the same library" the guard exists to prevent -
// and that only became reachable once the sweep learned to start itself.
const backfillAllLibraries = ""

// acquireBackfill claims the sweep slot for libraryID, or answers
// ErrIntroBackfillAlreadyRunning. Every sweep - admin trigger or automatic
// resume - goes through here so there is ONE answer to "may this sweep start?".
func (s *IntroDetectionService) acquireBackfill(libraryID string) error {
	s.backfillMu.Lock()
	defer s.backfillMu.Unlock()
	if _, running := s.backfillInFlight[libraryID]; running {
		return ErrIntroBackfillAlreadyRunning
	}
	if libraryID == backfillAllLibraries {
		if len(s.backfillInFlight) > 0 {
			return ErrIntroBackfillAlreadyRunning
		}
	} else if _, all := s.backfillInFlight[backfillAllLibraries]; all {
		return ErrIntroBackfillAlreadyRunning
	}
	if s.backfillInFlight == nil {
		s.backfillInFlight = make(map[string]struct{})
	}
	s.backfillInFlight[libraryID] = struct{}{}
	return nil
}

func (s *IntroDetectionService) releaseBackfill(libraryID string) {
	s.backfillMu.Lock()
	delete(s.backfillInFlight, libraryID)
	s.backfillMu.Unlock()
}

// IntroDetectionUseAllCoresKey is the server_config toggle that lets the admin
// run intro detection on the full CPU budget instead of the gentle trickplay
// share. Read at every run start, so flipping it needs no restart: it applies
// to the next episode/movie entering detection (a running sweep keeps the
// worker count it started with).
const IntroDetectionUseAllCoresKey = "intro_detection.use_all_cores"

// currentPHashAlgoVersion identifies the hash function used to populate the
// cache. Bump it whenever ExtractTileHashes' output distribution changes
// (e.g. aHash → pHash DCT) so the cache invalidates en masse without per-row
// surgery.
//
//	v1 = aHash 8×8 average over 5×5 tiles at 320×180 (pre-2026-05-03).
//	v2 = pHash DCT 8×8 of a 32×32 grayscale grid, DC dropped, median split
//	     over 63 AC coefs (current).
const currentPHashAlgoVersion int16 = 2

// currentAudioAlgoVersion versions the Chromaprint fingerprint cache.
// Bump whenever windowSec or the Chromaprint method changes so cached rows
// are invalidated en masse.
//
//	v1 = raw fpcalc []uint32 over discoverWindowSec (current).
const currentAudioAlgoVersion int16 = 1

// introDetectionAlgoVersion versions the OVERALL detection pipeline (pHash +
// Chromaprint + blackdetect + audio-primary), distinct from currentPHashAlgoVersion
// (the hash function alone). The backfill records a per-episode attempt counter
// stamped with this version and skips an episode once it has yielded no usable
// intro maxIntroDetectionAttempts times AT this version - instead of re-decoding
// the file (a full software 4K HEVC decode) on every restart forever.
//
// BUMP THIS whenever any part of detection improves: every episode that
// previously produced no usable intro becomes eligible again exactly once at the
// new version. This is the supported way to force a global re-detection after a
// quality improvement - no DB surgery, no admin action.
// v2: season-level audio clustering (intro_season_cluster.go) added to the
// pipeline - every episode that previously had no/weak intro becomes eligible
// for re-detection once at this version.
// v3: combine-by-type pipeline (a partial provider/chapter match no longer
// suppresses local detection of the other types) + heuristic-floor guard
// (sub-5s heuristic intros are dropped instead of surfacing as broken skip
// buttons). Every previously-uncovered/weak episode becomes eligible once more.
// v4: community DBs (tidb/introdb) demoted below every file-derived pass in
// runPipeline AND no longer count as a "good" cover in detectSeason - their
// timestamps ignore the local edition. Episodes parked with a provider-only
// intro become eligible for file-derived re-detection.
// v5: late title sequences become visible - fingerprint window 480s→720s,
// MaxIntroStartMs 240s→660s (gold-refuted cap), cluster hints capped too.
// Episodes parked at v4 with no detectable intro (long cold opens) re-enter.
const introDetectionAlgoVersion = 5

// maxIntroDetectionAttempts bounds the backfill's re-decodes of an episode that
// keeps yielding no usable intro at the same algo version (one retry for the
// detector's sampling variance, then park until introDetectionAlgoVersion bumps).
const maxIntroDetectionAttempts = 2

// introDetectionEpisodeBudget bounds one episode's detection run - decode,
// fingerprinting, credits scan - counted from the moment a concurrency slot is
// held, never from the wait for one. Generous enough for a software 4K decode
// on a weak NAS.
const introDetectionEpisodeBudget = 30 * time.Minute

// failureConsumesAttempt reports whether a detection failure counts toward
// maxIntroDetectionAttempts. Two failures reproduce on the same file and must
// not be handed back by every sweep: a decode the kernel killed (the OOM
// killer, 2026-08-24) and a spent episode budget (2026-08-29: three seasons
// timed out at 30 minutes on every 90-minute idle sweep, for a day). A
// canceled context is the sweep being stopped, not the episode - it stays
// uncounted, like an I/O error or memory pressure on the host (transient).
func failureConsumesAttempt(err error) bool {
	return errors.Is(err, domainlibrary.ErrDetectorKilled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, procfork.ErrMemoryCapExceeded)
}

// introPersistBudget bounds the segment writes after detection, on a context
// detached from the episode budget so an exhausted budget can't void the
// found results at the write.
const introPersistBudget = 2 * time.Minute

// introDeferPoll is how often a detection trigger re-checks for idle while
// playback is active - it defers (re-arms) rather than decoding on top of a
// live transcode on a weak NAS.
const introDeferPoll = 30 * time.Second

// minHeuristicIntroMs is the shortest intro any in-house heuristic (silence,
// audio, pHash, cluster) is allowed to emit. Below this the result is almost
// always degenerate - the sub-second studio-logo silence at 0:00, or a pHash
// match that confirmed only a sliver - not a real title sequence. It sits below
// the "good intro" floor used by detectSeason's skip check anyway, so a
// non-result beats a junk stub the user sees as a broken skip button.
const minHeuristicIntroMs = 5_000

// isHeuristicSegmentSource reports whether a segment was produced by an in-house
// heuristic (pHash, audio clustering/discovery, silence) rather than an
// authoritative source (an uploader-labelled MKV chapter, a community DB, or a
// manual lock). Only heuristic results are subject to the minHeuristicIntroMs
// floor - an authoritative short intro is intentional (e.g. 9-1-1's ~2s title
// card), not noise.
func isHeuristicSegmentSource(source string) bool {
	switch source {
	case "mkv_chapter", "tidb", "introdb", "manual", "manual_propagated":
		return false
	default:
		return true
	}
}

// defaultMaxParallelDetections caps how many pipeline runs may execute
// concurrently. 2 is a safe default for shared boxes - pHashPass extracts
// audio fingerprints across every sibling via ffmpeg, so each in-flight
// detection burns ~1 CPU core for several minutes. Override via
// WithMaxParallelDetections if the host has CPU headroom.
const defaultMaxParallelDetections = 2

func NewIntroDetectionService(
	segmentRepo domainlibrary.MediaSegmentRepository,
	trickplayRepo domainlibrary.TrickplayRepository,
	mediaItems domainlibrary.MediaItemRepository,
	configRepo shared.ServerConfigRepository,
	detector IntroDetector,
	cacheDir string,
) *IntroDetectionService {
	return &IntroDetectionService{
		segmentRepo:        segmentRepo,
		trickplayRepo:      trickplayRepo,
		mediaItems:         mediaItems,
		configRepo:         configRepo,
		detector:           detector,
		trickplayCacheDir:  cacheDir,
		concurrencyLimiter: make(chan struct{}, defaultMaxParallelDetections),
	}
}

// WithMaxParallelDetections overrides the default concurrency cap. n ≤ 0 is
// treated as "no limit" and disables the semaphore (the previous behaviour).
// Returns the service so the call chains cleanly with the other With...
// helpers in wire.go.
func (s *IntroDetectionService) WithMaxParallelDetections(n int) *IntroDetectionService {
	s.maxParallelBase = n
	if n <= 0 {
		s.concurrencyLimiter = nil
		return s
	}
	s.concurrencyLimiter = make(chan struct{}, n)
	return s
}

// WithAllCoresParallelDetections sets the cap the use-all-cores admin toggle
// escalates to (wire passes the full sysinfo.CPUBudget). 0 leaves the toggle
// inert.
func (s *IntroDetectionService) WithAllCoresParallelDetections(n int) *IntroDetectionService {
	s.maxParallelAllCores = n
	return s
}

// limiterForRun returns the semaphore for one detection run, first resizing it
// when the admin use-all-cores toggle changed the desired capacity. Callers
// must release into the returned channel (not the field): in-flight holders of
// a swapped-out channel drain into it harmlessly.
func (s *IntroDetectionService) limiterForRun(ctx context.Context) chan struct{} {
	s.limiterMu.Lock()
	defer s.limiterMu.Unlock()
	if s.concurrencyLimiter == nil {
		return nil
	}
	want := s.maxParallelBase
	if s.useAllCores(ctx) && s.maxParallelAllCores > want {
		want = s.maxParallelAllCores
	}
	if want <= 0 {
		want = cap(s.concurrencyLimiter)
	}
	if want != cap(s.concurrencyLimiter) {
		slog.InfoContext(ctx, "intro detection: max parallel resized",
			slog.Int("from", cap(s.concurrencyLimiter)),
			slog.Int("to", want))
		s.concurrencyLimiter = make(chan struct{}, want)
	}
	return s.concurrencyLimiter
}

func (s *IntroDetectionService) useAllCores(ctx context.Context) bool {
	if s.configRepo == nil {
		return false
	}
	entry, err := s.configRepo.Get(ctx, IntroDetectionUseAllCoresKey)
	return err == nil && entry != nil && entry.Value == "true"
}

// WithActivePlayback wires the live playback-session counter. While any session
// is running, StartWorker defers detection (re-arming the trigger) instead of
// decoding on top of the active stream.
func (s *IntroDetectionService) WithActivePlayback(count func() int) *IntroDetectionService {
	s.activePlayback = count
	return s
}

func (s *IntroDetectionService) WithShutdownCtx(ctx context.Context) {
	s.shutdownCtx = ctx
}

func (s *IntroDetectionService) WithEventBus(bus shared.EventBus) {
	s.events = bus
}

// WithFingerprintCache enables persistent caching of pHash tile hashes.
// When set, pHashPass reads from / writes to the cache; when nil, hashes are
// always recomputed from the trickplay sprites (no behavior change vs. v0).
func (s *IntroDetectionService) WithFingerprintCache(cache domainlibrary.IntroFingerprintCacheRepository) {
	s.fpCache = cache
}

// WithAudioFingerprintCache enables persistent caching of Chromaprint fingerprints.
// When set, discoverIntroByAudio reads from / writes to the cache; when nil,
// fingerprints are always recomputed via fpcalc (no behavior change vs. v0).
func (s *IntroDetectionService) WithAudioFingerprintCache(cache domainlibrary.IntroAudioFingerprintCacheRepository) {
	s.audioFPCache = cache
}

// WithChapterRepo enables intro detection via Matroska chapter names.
// Many MKV containers ship with chapters explicitly labelled "Intro",
// "Opening Credits", "Generique", etc. - those are authoritative
// (encoded by the uploader) and should win over any heuristic.
func (s *IntroDetectionService) WithChapterRepo(repo domainlibrary.ChapterQueryRepo) {
	s.chapterRepo = repo
}

// WithChromaprinter wires fpcalc as the primary audio confirmation
// signal. When set, confirmAudioCandidates uses Chromaprint sliding-XOR
// match instead of the in-house banded Pearson; banded stays as fallback
// when fpcalc is unavailable on the host or returns an error for a given
// segment.
func (s *IntroDetectionService) WithChromaprinter(c Chromaprinter) {
	s.chromaprinter = c
}

// WithIntroMetadataProvider plugs an external source of intro/recap segments
// (e.g. TheIntroDB/TIDB via the introproviders chain, wired when
// intro_providers.enabled). Provider segments rank BELOW every file-derived
// pass (MKV chapters, season cluster, pHash): community timestamps describe
// someone else's copy of the media, not the local edition. They fill only the
// segment types the local pipeline left empty.
//
// nil disables the pass; the rest of the pipeline runs unchanged.
func (s *IntroDetectionService) WithIntroMetadataProvider(p domainlibrary.IntroMetadataProvider) *IntroDetectionService {
	s.metadataProvider = p
	return s
}

// WithSegmentContributor plugs an optional community contribution adapter.
// When set, every phash-detected segment with Confidence >= 0.95 is submitted
// fire-and-forget so other instances can benefit from our local detection.
// nil is safe and disables contributions.
func (s *IntroDetectionService) WithSegmentContributor(c domainlibrary.SegmentContributor) *IntroDetectionService {
	s.contributor = c
	return s
}

func (s *IntroDetectionService) isTrickplayEnabled(ctx context.Context) bool {
	if s.configRepo == nil {
		return true
	}
	entry, err := s.configRepo.Get(ctx, "trickplay.enabled")
	if err != nil {
		return true
	}
	return entry.Value != "false"
}

// StartWorker subscribes to media.item.added (new scans) and trickplay.generated
// (rescans / regenerations) and runs detection on a full season once the stream
// of triggers to the same parent stabilises (30s debounce). For movies, which
// have no parent, trickplay.generated triggers DetectIntro directly.
//
// Also schedules a background backfill sweep at startup so seasons that were
// scanned before the feature existed (or while the worker was down) get covered.
func (s *IntroDetectionService) StartWorker(ctx context.Context) {
	bgtasks.Register("intro_detection")
	// Started before the event-bus guard: the sweep is what covers the catalogue
	// (99% of the detections in the 2026-08-22 prod sample came from it, against
	// ~8 a day from the event path), so it must not be hostage to a bus.
	s.StartResumableBackfill(ctx)
	if s.events == nil {
		return
	}
	var mu sync.Mutex
	timers := make(map[string]*time.Timer)

	// runDeferred fires run after delay, de-duped by key. When the timer fires
	// WHILE playback is active it re-arms (introDeferPoll) instead of running:
	// heavy detection (pHash + trickplay decode) waits until nobody is watching
	// rather than decoding on top of a live transcode on a weak NAS. Deferring,
	// not dropping - there is no periodic catch-up, so a skipped trigger would be
	// lost until the next scan/regeneration.
	var runDeferred func(key string, delay time.Duration, run func())
	runDeferred = func(key string, delay time.Duration, run func()) {
		mu.Lock()
		if t, exists := timers[key]; exists {
			t.Stop()
		}
		timers[key] = time.AfterFunc(delay, func() {
			if s.activePlayback != nil && s.activePlayback() > 0 {
				runDeferred(key, introDeferPoll, run) // still watching - try again later
				return
			}
			mu.Lock()
			delete(timers, key)
			mu.Unlock()
			defer func() {
				if r := recover(); r != nil {
					slog.ErrorContext(ctx, "recovered panic in intro detection worker", slog.Any("error", r))
				}
			}()
			run()
		})
		mu.Unlock()
	}

	scheduleSeason := func(parentID string) {
		if parentID == "" {
			return
		}
		runDeferred("season:"+parentID, 30*time.Second, func() { s.detectSeason(ctx, parentID) })
	}

	s.events.Subscribe("media.item.added", func(_ context.Context, ev shared.Event) {
		e, ok := ev.(domainlibrary.MediaItemAdded)
		if !ok || e.Type != domainlibrary.Episode {
			return
		}
		// When trickplay is enabled, defer to the trickplay.generated trigger:
		// on a fresh add the sprites don't exist yet, and the pHash + trickplay-
		// based passes (visual intro, studio logo, end credits) need them. Running
		// now would detect on audio-only signals and re-run on trickplay.generated
		// anyway. When trickplay is disabled, no such event is coming, so detect now.
		if s.isTrickplayEnabled(ctx) {
			return
		}
		item, err := s.mediaItems.GetByID(ctx, e.MediaItemID)
		if err != nil || item.ParentID == "" {
			return
		}
		scheduleSeason(item.ParentID)
	})

	// trickplay.generated fires for both fresh and regenerated trickplays,
	// covering the case where existing media's sprites get rebuilt (e.g. after
	// a format change). Episodes share the season debounce; movies trigger
	// detection immediately since they have no siblings to wait for.
	s.events.Subscribe("trickplay.generated", func(_ context.Context, ev shared.Event) {
		e, ok := ev.(domainlibrary.TrickplayGenerated)
		if !ok {
			return
		}
		switch e.Type {
		case domainlibrary.Episode:
			scheduleSeason(string(e.ParentID))
		case domainlibrary.MovieItem:
			id := string(e.MediaItemID)
			runDeferred("movie:"+id, 0, func() {
				if err := s.DetectIntro(ctx, id); err != nil {
					slog.WarnContext(ctx, "intro detection on trickplay.generated movie failed",
						slog.String("media_id", id), slog.Any("err", err))
				}
			})
		}
	})

}

// StartResumableBackfill launches the sweep that survives a redeployment. It is
// started from StartWorker and needs no event bus: its intention lives in the
// database, not in this process.
func (s *IntroDetectionService) StartResumableBackfill(ctx context.Context) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "recovered panic in intro detection backfill", slog.Any("error", r))
			}
		}()
		s.runResumableBackfill(ctx)
	}()
}

// introBackfillBootDelay lets the boot settle before the first drain.
const introBackfillBootDelay = 60 * time.Second

// introBackfillIdleInterval is how long the sweep sleeps once the work list
// comes back empty. It then asks again, so newly scanned seasons and an
// introDetectionAlgoVersion bump are both picked up without a restart.
const introBackfillIdleInterval = 30 * time.Minute

// introBackfillBatchSize bounds one batch of the work list.
const introBackfillBatchSize = 100

// runResumableBackfill drains the work list, sleeps when it is empty, and asks
// again - forever. The previous design fired ONCE, 60s after boot, off a
// predicate that matched almost nothing; everything else came from an admin
// button whose intention lived in a map in memory. A redeployment on 2026-08-09
// killed a 42h sweep and nothing restarted it: 31 detection decisions in the
// following 11 days.
//
// Nothing here is remembered across a restart on purpose - the next boot asks
// the database the same question and gets the remaining seasons.
func (s *IntroDetectionService) runResumableBackfill(ctx context.Context) {
	delay := s.backfillBootDelay
	if delay <= 0 {
		delay = introBackfillBootDelay
	}
	idle := s.backfillIdle
	if idle <= 0 {
		idle = introBackfillIdleInterval
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		s.sweepPendingSeasons(ctx)
		delay = idle
	}
}

// sweepPendingSeasons runs one drain cycle over the seasons whose intro
// detection is still to do, and returns how many it walked. Guarded like an
// admin trigger, and holding the guard only while it works: an idle sweep must
// not lock the admin out of its own library.
func (s *IntroDetectionService) sweepPendingSeasons(ctx context.Context) int {
	if err := s.acquireBackfill(backfillAllLibraries); err != nil {
		slog.InfoContext(ctx, "intro detection backfill: another sweep is running, skipping this cycle")
		return 0
	}
	defer s.releaseBackfill(backfillAllLibraries)
	return s.drainPendingSeasons(ctx)
}

// drainPendingSeasons walks the work list batch by batch until it is empty.
// Caller holds the guard.
//
// seen is the in-run skip set (the keyframe backfill's `attempted`): a season
// the sweep just walked WITHOUT making progress - nothing detectable, no
// attempt recorded - is still pending, and would otherwise be handed back by
// every batch forever. Resetting it each cycle is what lets the next wake-up
// try again.
func (s *IntroDetectionService) drainPendingSeasons(ctx context.Context) int {
	var seen []string
	swept := 0
	for {
		if ctx.Err() != nil {
			return swept
		}
		ids, err := s.segmentRepo.ListSeasonsPendingIntroDetection(
			ctx, introDetectionAlgoVersion, maxIntroDetectionAttempts, introBackfillBatchSize, seen)
		if err != nil {
			slog.WarnContext(ctx, "intro detection backfill: list pending seasons", slog.Any("err", err))
			return swept
		}
		if len(ids) == 0 {
			if swept > 0 {
				slog.InfoContext(ctx, "intro detection backfill: work list drained", slog.Int("seasons", swept))
			}
			return swept
		}
		slog.InfoContext(ctx, "intro detection backfill: batch", slog.Int("seasons", len(ids)))
		seen = append(seen, ids...)
		s.forEachBackfillTarget(ctx, ids, s.detectSeason)
		swept += len(ids)
	}
}

// backfillWorkers is how many seasons (or movies) a backfill sweep walks at
// once: the pipeline concurrency budget (wire sizes concurrencyLimiter from
// the trickplay CPU budget, the use-all-cores toggle escalates it), not a
// second knob. The sweeps used to walk sequentially, so that budget was only
// ever consumed by event triggers.
func (s *IntroDetectionService) backfillWorkers(ctx context.Context) int {
	lim := s.limiterForRun(ctx)
	if lim == nil {
		return defaultMaxParallelDetections
	}
	return cap(lim)
}

// forEachBackfillTarget fans ids out to backfillWorkers goroutines running fn.
// Every run is recover-guarded: pool goroutines escape the caller's recover,
// and one poisoned target must neither kill the process nor abort the sweep.
func (s *IntroDetectionService) forEachBackfillTarget(ctx context.Context, ids []string, fn func(context.Context, string)) {
	guarded := func(id string) {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "intro detection backfill: recovered panic",
					slog.String("id", id), slog.Any("error", r))
			}
		}()
		fn(ctx, id)
	}
	workers := min(s.backfillWorkers(ctx), len(ids))
	if workers <= 1 {
		for _, id := range ids {
			if ctx.Err() != nil {
				return
			}
			guarded(id)
		}
		return
	}
	work := make(chan string)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for id := range work {
				guarded(id)
			}
		})
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		work <- id
	}
	close(work)
	wg.Wait()
}

// BackfillLibrary sweeps one library's seasons and movies. An empty libraryID
// means the whole catalogue and runs the SAME resumable drain as the automatic
// sweep - the admin button and the boot now share one work list, so pressing it
// after a restart re-enters the queue where the killed process left it. This is
// the entry point for the admin "run backfill" endpoint - it runs synchronously;
// callers that want non-blocking behaviour should wrap it in a goroutine.
//
// Returns the number of seasons that were scheduled (i.e. passed to
// detectSeason, not necessarily found new segments).
func (s *IntroDetectionService) BackfillLibrary(ctx context.Context, libraryID string) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.BackfillLibrary")
	defer span.End()
	if err := s.acquireBackfill(libraryID); err != nil {
		slog.InfoContext(ctx, "intro detection admin backfill: duplicate trigger ignored",
			slog.String("library_id", libraryID))
		return 0, err
	}
	defer s.releaseBackfill(libraryID)
	if libraryID == backfillAllLibraries {
		return s.drainPendingSeasons(ctx), nil
	}
	var parentIDs []string
	seasonType := string(domainlibrary.Season)
	libID := shared.LibraryID(libraryID)
	// Paged like backfillMovies: the repo clamps ANY single List to its
	// own cap (postgres DefaultListLimit=500), so one big request
	// silently truncated 500+-season libraries - prod logged exactly
	// seasons=500 on two consecutive sweeps (2026-08-08) and everything
	// past the cap was never detected.
	const seasonPageSize = 200
	for offset := 0; ; offset += seasonPageSize {
		seasons, err := s.mediaItems.List(ctx, domainlibrary.ListMediaItemsFilter{
			LibraryID: &libID,
			Type:      &seasonType,
			Limit:     seasonPageSize,
			Offset:    offset,
		})
		if err != nil {
			return 0, fmt.Errorf("intro detection admin backfill: list library seasons: %w", err)
		}
		for _, se := range seasons {
			parentIDs = append(parentIDs, string(se.ID))
		}
		if len(seasons) < seasonPageSize {
			break
		}
	}

	slog.InfoContext(ctx, "intro detection admin backfill: starting",
		slog.String("library_id", libraryID),
		slog.Int("seasons", len(parentIDs)))
	s.forEachBackfillTarget(ctx, parentIDs, s.detectSeason)
	movies := s.backfillMovies(ctx, libraryID)
	slog.InfoContext(ctx, "intro detection admin backfill: done",
		slog.String("library_id", libraryID),
		slog.Int("seasons", len(parentIDs)),
		slog.Int("movies", movies))
	return len(parentIDs) + movies, nil
}

// backfillMovies runs detection over every movie of the library. Movies have
// no season path (detectSeason walks episodes), yet they carry credits, logo
// and provider/chapter intro segments - without this they had NO supported
// re-detection route after an algo fix. Mirrors detectSeason's skip rules:
// manual locks are sticky, file-derived intro sources count as covered,
// attempts are version-bounded, and skipped movies still get their suspect
// credits refreshed. Returns the number of movies that entered detection.
func (s *IntroDetectionService) backfillMovies(ctx context.Context, libraryID string) int {
	movieType := string(domainlibrary.MovieItem)
	libID := shared.LibraryID(libraryID)
	ran := 0
	const pageSize = 200
	for offset := 0; ; offset += pageSize {
		page, err := s.mediaItems.List(ctx, domainlibrary.ListMediaItemsFilter{
			LibraryID: &libID, Type: &movieType, Limit: pageSize, Offset: offset,
		})
		if err != nil || len(page) == 0 {
			break
		}
		ids := make([]shared.MediaItemID, len(page))
		for i, m := range page {
			ids[i] = m.ID
		}
		existingByID, batchErr := s.segmentRepo.GetByMediaItemIDs(ctx, ids)
		if batchErr != nil {
			slog.WarnContext(ctx, "intro backfill movies: batch get segments", slog.Any("err", batchErr))
			existingByID = nil
		}
		var candidates []*domainlibrary.MediaItem
		for _, m := range page {
			if ctx.Err() != nil {
				return ran
			}
			skip := false
			for _, sg := range existingByID[m.ID] {
				if sg.Type != "intro" {
					continue
				}
				switch sg.Source {
				case "manual", "manual_propagated", "mkv_chapter":
					skip = true
				}
			}
			if skip {
				s.refreshSuspectCredits(ctx, m, existingByID[m.ID])
				continue
			}
			candidates = append(candidates, m)
		}
		if len(candidates) > 0 {
			candidateIDs := make([]string, len(candidates))
			for i, m := range candidates {
				candidateIDs[i] = string(m.ID)
			}
			exhaustedByID, exErr := s.segmentRepo.IntroDetectionExhaustedBatch(ctx, candidateIDs, introDetectionAlgoVersion, maxIntroDetectionAttempts)
			if exErr != nil {
				slog.WarnContext(ctx, "intro backfill movies: batch exhausted check", slog.Any("err", exErr))
				exhaustedByID = nil
			}
			var runnable []string
			for _, m := range candidates {
				if ctx.Err() != nil {
					return ran
				}
				if exhaustedByID[string(m.ID)] {
					s.refreshSuspectCredits(ctx, m, existingByID[m.ID])
					continue
				}
				runnable = append(runnable, string(m.ID))
			}
			var ranPage atomic.Int64
			s.forEachBackfillTarget(ctx, runnable, func(ctx context.Context, id string) {
				detErr := s.detectIntroWithHint(ctx, id, nil)
				if detErr != nil {
					slog.WarnContext(ctx, "intro backfill movies: detection failed",
						slog.String("media_id", id), slog.Any("err", detErr))
					// Same rule as the season sweep: a killed decode or a spent budget
					// is repeatable, so it must consume an attempt or the backfill
					// keeps re-picking it.
					if failureConsumesAttempt(detErr) {
						if recErr := s.segmentRepo.RecordIntroDetectionAttempt(ctx, id, introDetectionAlgoVersion); recErr != nil {
							slog.WarnContext(ctx, "intro backfill movies: record attempt after kill",
								slog.String("media_id", id), slog.Any("err", recErr))
						}
					}
					return
				}
				ranPage.Add(1)
				if recErr := s.segmentRepo.RecordIntroDetectionAttempt(ctx, id, introDetectionAlgoVersion); recErr != nil {
					slog.WarnContext(ctx, "intro backfill movies: record attempt",
						slog.String("media_id", id), slog.Any("err", recErr))
				}
			})
			ran += int(ranPage.Load())
		}
		if len(page) < pageSize {
			break
		}
	}
	return ran
}

// detectSeason runs detection on every episode of the given season that
// doesn't already have an intro segment (presence of end_credits alone is not
// sufficient - partial detection happens when not all siblings had their
// trickplay ready, and we must re-run as more become available).
//
// Skips episodes whose existing intro is locked by an operator (manual
// override) since `upsertSegmentRespectingManual` would no-op anyway and we
// don't want to waste the audio extraction.
func (s *IntroDetectionService) detectSeason(ctx context.Context, parentID string) {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.detectSeason")
	defer span.End()

	episodeType := string(domainlibrary.Episode)

	// Season-level clustering hint (v2): one pass over the whole season locates
	// the recurring intro per episode at its own offset (cached fingerprints, no
	// per-candidate re-extraction). nil when disabled or no cluster is found -
	// episodes then fall through to the per-episode pipeline unchanged.
	clusterHints := s.clusterSeasonIntro(ctx, parentID)

	// Page through the whole season. A single Limit:100 (or even the repo's 500
	// cap) would silently skip episodes 101+ of a long single-season (anime,
	// daily soaps), leaving them without intro markers forever - this is the only
	// detection path (MediaItemAdded schedules the whole SEASON, not per episode).
	const pageSize = 200
	for offset := 0; ; offset += pageSize {
		siblings, err := s.mediaItems.List(ctx, domainlibrary.ListMediaItemsFilter{
			ParentID: &parentID,
			Type:     &episodeType,
			Limit:    pageSize,
			Offset:   offset,
		})
		if err != nil {
			slog.WarnContext(ctx, "intro detection: list season episodes", slog.String("parent_id", parentID), slog.Any("err", err))
			return
		}
		if len(siblings) == 0 {
			break
		}
		// Batch the existing-segments lookup for the whole page in one query
		// instead of one GetByMediaItemID call per episode (N+1: a single-
		// season sweep of 200 episodes was issuing 200 round trips just to
		// check which ones already have a covering segment).
		pageIDs := make([]shared.MediaItemID, len(siblings))
		for i, ep := range siblings {
			pageIDs[i] = ep.ID
		}
		existingByID, batchErr := s.segmentRepo.GetByMediaItemIDs(ctx, pageIDs)
		if batchErr != nil {
			// Couldn't read existing segments; treat the whole page as
			// uncovered and let detection re-run rather than skip it silently.
			slog.WarnContext(ctx, "intro detection: batch get existing segments", slog.String("parent_id", parentID), slog.Any("err", batchErr))
			existingByID = nil
		}
		// Pass 1: filter the page down to candidates needing detection (not
		// manual-locked, no good intro yet already covered above).
		var candidates []*domainlibrary.MediaItem
		for _, ep := range siblings {
			if ctx.Err() != nil {
				return
			}
			existing := existingByID[ep.ID]
			// One answer to "is this episode's intro settled?", shared with the
			// SQL that rebuilds the sweep's work list after a restart: an
			// operator lock, or a long-enough file-derived intro. See
			// domainlibrary.IntroDetectionSettled for what is deliberately NOT
			// coverage (community DBs, last-resort heuristics, sub-5s stubs).
			settled := false
			for _, sg := range existing {
				if sg.Type == "intro" && domainlibrary.IntroDetectionSettled(sg.Source, sg.EndMs-sg.StartMs) {
					settled = true
					break
				}
			}
			if settled {
				// The episode skips the intro pipeline, but its stored credits
				// may come from the old first-black parser - refresh the
				// suspect ones so a parser fix reaches covered episodes too.
				s.refreshSuspectCredits(ctx, ep, existing)
				continue
			}
			candidates = append(candidates, ep)
		}
		if len(candidates) == 0 {
			if len(siblings) < pageSize {
				break
			}
			continue
		}
		// Batch the exhausted-attempts check for every candidate on the page in
		// one query instead of one IntroDetectionExhausted call per episode (the
		// N+1 this restructure closes: a single-season sweep of 200 candidates
		// was issuing 200 round trips just to check the retry cap).
		candidateIDs := make([]string, len(candidates))
		for i, ep := range candidates {
			candidateIDs[i] = string(ep.ID)
		}
		exhaustedByID, exErr := s.segmentRepo.IntroDetectionExhaustedBatch(ctx, candidateIDs, introDetectionAlgoVersion, maxIntroDetectionAttempts)
		if exErr != nil {
			// Fail open, matching the previous per-episode semantics (exErr != nil
			// meant "don't skip"): treat nobody as exhausted rather than silently
			// parking the whole page.
			slog.WarnContext(ctx, "intro detection: batch exhausted check", slog.String("parent_id", parentID), slog.Any("err", exErr))
			exhaustedByID = nil
		}

		// Pass 2: run detection on the non-exhausted candidates.
		for _, ep := range candidates {
			if ctx.Err() != nil {
				return
			}
			// Already re-decoded to the cap at the current algo version with no
			// usable intro? Skip - it lands on the same answer and a 4K HEVC decode
			// is expensive. Bumping introDetectionAlgoVersion re-enables it (the
			// row's older version no longer matches).
			if exhaustedByID[string(ep.ID)] {
				continue
			}
			var hint *domainlibrary.MediaSegment
			if h, ok := clusterHints[string(ep.ID)]; ok {
				hh := h
				hint = &hh
			}
			detErr := s.detectIntroWithHint(ctx, string(ep.ID), hint)
			if detErr != nil {
				// A transient failure (I/O, shutdown) is not counted toward the cap,
				// so it doesn't park an episode that is perfectly detectable. A decode
				// the kernel KILLED or a spent budget is not transient - see
				// failureConsumesAttempt - and the work list is rebuilt from the
				// database, so an uncounted one comes back after every sweep.
				slog.WarnContext(ctx, "intro detection failed", slog.String("media_id", string(ep.ID)), slog.Any("err", detErr))
				if failureConsumesAttempt(detErr) {
					if recErr := s.segmentRepo.RecordIntroDetectionAttempt(ctx, string(ep.ID), introDetectionAlgoVersion); recErr != nil {
						slog.WarnContext(ctx, "intro detection: record attempt after kill", slog.String("media_id", string(ep.ID)), slog.Any("err", recErr))
					}
				}
				continue
			}
			// Clean run: count it. If it found a good intro the episode is skipped
			// next time via hasGoodIntro; if it found nothing/weak, this bounds the
			// retries until introDetectionAlgoVersion bumps.
			if recErr := s.segmentRepo.RecordIntroDetectionAttempt(ctx, string(ep.ID), introDetectionAlgoVersion); recErr != nil {
				slog.WarnContext(ctx, "intro detection: record attempt", slog.String("media_id", string(ep.ID)), slog.Any("err", recErr))
			}
		}
		if len(siblings) < pageSize {
			break
		}
	}

	// Season-wide duration prior: shrink over-long outliers (absorbed recap /
	// music-bleed) back to the season's median intro duration. Runs last so it
	// sees the season's final mix of sources.
	s.reconcileSeasonIntroDurations(ctx, parentID)
}

// DetectIntro is the main entry point. Runs the two-pass pipeline and upserts
// all detected segments. Acquires a slot from concurrencyLimiter so a flood
// of admin /detect-segments triggers (or worker auto-runs) don't pile up
// ffmpeg processes that thrash the host.
func (s *IntroDetectionService) DetectIntro(ctx context.Context, mediaItemID string) error {
	return s.detectIntroWithHint(ctx, mediaItemID, nil)
}

// DetectSeason runs the full season pipeline (cluster hint included) on one
// season - the admin intro-test tool's "apply" granularity. detectSeason
// recovers its own panics and logs per-episode failures, so there is no
// error to surface beyond scheduling.
func (s *IntroDetectionService) DetectSeason(ctx context.Context, seasonID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.DetectSeason")
	defer span.End()
	s.detectSeason(ctx, seasonID)
	return nil
}

// DetectSeries runs DetectSeason over every season of the series, paged past
// the repo's list cap like the library backfill. Returns the season count.
func (s *IntroDetectionService) DetectSeries(ctx context.Context, seriesID string) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.DetectSeries")
	defer span.End()
	seasonType := string(domainlibrary.Season)
	var seasonIDs []string
	const pageSize = 200
	for offset := 0; ; offset += pageSize {
		seasons, err := s.mediaItems.List(ctx, domainlibrary.ListMediaItemsFilter{
			ParentID: &seriesID,
			Type:     &seasonType,
			Limit:    pageSize,
			Offset:   offset,
		})
		if err != nil {
			return 0, fmt.Errorf("intro detection: list series seasons: %w", err)
		}
		for _, se := range seasons {
			seasonIDs = append(seasonIDs, string(se.ID))
		}
		if len(seasons) < pageSize {
			break
		}
	}
	s.forEachBackfillTarget(ctx, seasonIDs, s.detectSeason)
	return len(seasonIDs), nil
}

// DetectIntroDryRun runs the full per-episode pipeline (season-cluster hint
// included, for parity with the real detectSeason path) and RETURNS the segments
// it would store - applying the same heuristic-floor guard - WITHOUT persisting
// anything. It is the read-only path behind the admin ?dry_run tooling: it lets
// an operator measure the detector against a known ground truth (an authoritative
// tidb/introdb/chapter intro already stored for the episode) without clobbering
// that truth via upsert. No community contribution, no end-credit/studio side
// effects beyond the returned list.
func (s *IntroDetectionService) DetectIntroDryRun(ctx context.Context, mediaItemID string) ([]domainlibrary.MediaSegment, error) {
	item, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil {
		return nil, fmt.Errorf("get media item: %w", err)
	}
	if item.File == nil || item.File.Path == "" {
		return nil, errors.New("media item has no file path")
	}

	// Season-cluster hint for parity with detectSeason. Cached audio fingerprints
	// keep re-clustering mostly in-memory. Deliberately NOT guarded by the
	// concurrencyLimiter: clusterSeasonIntro extracts sibling fingerprints and a
	// shared limiter slot could deadlock; the eval driver issues requests serially.
	var hint *domainlibrary.MediaSegment
	if item.ParentID != "" {
		if hints := s.clusterSeasonIntro(ctx, item.ParentID); hints != nil {
			if h, ok := hints[mediaItemID]; ok {
				hh := h
				hint = &hh
			}
		}
	}

	// Intro pipeline only: the eval harness scores intro start/end, and the
	// end-credits blackdetect scans the WHOLE file (a full-length 2160p black
	// scan dominates the call), so it is intentionally skipped here.
	segments, err := s.runPipeline(ctx, item, hint)
	if err != nil {
		return nil, fmt.Errorf("detection pipeline: %w", err)
	}

	out := make([]domainlibrary.MediaSegment, 0, len(segments))
	for _, seg := range segments {
		if seg.Type == "intro" && isHeuristicSegmentSource(seg.Source) &&
			seg.EndMs-seg.StartMs < minHeuristicIntroMs {
			continue
		}
		out = append(out, seg)
	}
	return out, nil
}

// detectIntroWithHint runs the per-episode pipeline. hint, when non-nil, is the
// season-cluster intro window for this episode - authoritative over the visual
// heuristics (applied in runPipeline, below explicit metadata/chapters).
func (s *IntroDetectionService) detectIntroWithHint(ctx context.Context, mediaItemID string, hint *domainlibrary.MediaSegment) error {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.DetectIntro")
	defer span.End()
	if lim := s.limiterForRun(ctx); lim != nil {
		select {
		case lim <- struct{}{}:
			defer func() { <-lim }()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	// The budget starts once a slot is held: under limiter contention (several
	// sweeps in flight) episodes used to burn the whole allowance waiting in
	// line, then every pass soft-failed on the already-expired context.
	budget := s.episodeBudget
	if budget <= 0 {
		budget = introDetectionEpisodeBudget
	}
	ctx, cancelBudget := context.WithTimeout(ctx, budget)
	defer cancelBudget()
	item, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil {
		return fmt.Errorf("get media item: %w", err)
	}
	if item.File == nil || item.File.Path == "" {
		return errors.New("media item has no file path")
	}

	segments, err := s.runPipeline(ctx, item, hint)
	if err != nil {
		return fmt.Errorf("detection pipeline: %w", err)
	}

	// End credits detection runs independently of the intro pipeline so that
	// films (which produce no intro windows) also get a credits segment.
	if seg := s.endCreditsPass(ctx, item); seg != nil {
		segments = append(segments, *seg)
	}

	// Studio logo detection also runs independently for both films and episodes.
	// It requires a trickplay manifest but does not wait if one is absent.
	if seg := s.detectStudioLogo(ctx, item); seg != nil {
		segments = append(segments, *seg)
	}

	if len(segments) == 0 {
		slog.InfoContext(ctx, "intro_detection: no segments found", slog.String("media_id", mediaItemID))
		return nil
	}

	// Persist on a context detached from the detection budget: a slow scan
	// exhausting the allowance must not void the results found before it - the
	// write costs milliseconds where re-detection costs another full decode.
	persistCtx, cancelPersist := context.WithTimeout(context.WithoutCancel(ctx), introPersistBudget)
	defer cancelPersist()
	for _, seg := range segments {
		// Heuristic-floor guard: a sub-5s intro from a heuristic source is almost
		// always a degenerate result - the silence pass locking onto the
		// studio-logo silence at 0:00, or a pHash match that confirmed only a
		// sliver. It surfaces to the user as a broken skip button. Drop it; a
		// non-result is strictly better. Authoritative sources are exempt: a
		// short intro from an uploader chapter or a community DB is intentional
		// (e.g. 9-1-1's ~2s title card), not noise.
		if seg.Type == "intro" && isHeuristicSegmentSource(seg.Source) &&
			seg.EndMs-seg.StartMs < minHeuristicIntroMs {
			slog.InfoContext(ctx, "intro_detection: dropping degenerate heuristic intro",
				slog.String("media_id", mediaItemID),
				slog.String("source", seg.Source),
				slog.Int64("start_ms", seg.StartMs),
				slog.Int64("end_ms", seg.EndMs))
			continue
		}
		seg.ID = uuid.New().String()
		seg.MediaItemID = shared.MediaItemID(mediaItemID)
		if err := s.upsertSegmentRespectingManual(persistCtx, &seg); err != nil {
			return fmt.Errorf("upsert segment %s: %w", seg.Type, err)
		}
		slog.InfoContext(ctx, "intro_detection: segment stored",
			slog.String("media_id", mediaItemID),
			slog.String("type", seg.Type),
			slog.Int64("start_ms", seg.StartMs),
			slog.Int64("end_ms", seg.EndMs),
		)
		// Contribute phash segments with high confidence back to the community
		// database so other instances can skip local detection for the same item.
		// Only phash segments are submitted: tidb/introdb/mkv_chapter results
		// either came from the community already or represent third-party metadata.
		if s.contributor != nil && seg.Source == "phash" && seg.Confidence >= 0.95 {
			segCopy := seg
			itemCopy := item
			// ADR-0007: detached ctx with bounded timeout + defer recover.
			// 30s is generous for a single outbound POST.
			go func() {
				defer func() {
					if r := recover(); r != nil {
						// ADR-0007: deliberately detached from the request ctx, so no
						// request id survives to correlate here.
						slog.Error("panic recovered in intro_detection: contribute",
							slog.Any("panic", r),
							slog.String("stack", string(debug.Stack())))
					}
				}()
				cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if cErr := s.contributor.Contribute(cctx, itemCopy, segCopy); cErr != nil {
					slog.WarnContext(cctx, "intro_detection: segment contribution failed",
						slog.String("type", segCopy.Type), slog.Any("err", cErr))
				}
			}()
		}
	}
	return nil
}
