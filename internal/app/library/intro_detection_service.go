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
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// IntroDetector is the port for the FFmpeg intro-detection adapter.
type IntroDetector interface {
	ExtractTileHashes(spritePath string, tileWidth, tileHeight, tilePixelWidth, tilePixelHeight int) ([]uint64, error)
	PrecisionPass(ctx context.Context, filePath string, startSec, endSec, padSec float64) (int64, int64, error)
	AudioFallback(ctx context.Context, filePath string, maxSec float64) (int64, int64, bool, error)
	DetectEndCredits(ctx context.Context, filePath string, durationSec float64) (int64, int64, bool, error)
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
	concurrencyLimiter chan struct{}

	// activePlayback, when set, reports live playback sessions. The worker defers
	// heavy detection (pHash + trickplay decode) while anyone is watching so it
	// can't starve an active transcode on a weak NAS - the trigger is re-armed
	// until playback goes idle rather than dropped (no periodic catch-up exists).
	activePlayback func() int
}

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

// backfillAllSeasonsLimit is the list-page size the admin backfill uses to
// fetch every season of a library in one query - an "unbounded" sentinel far
// above any real library.
const backfillAllSeasonsLimit = 10000

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
	if n <= 0 {
		s.concurrencyLimiter = nil
		return s
	}
	s.concurrencyLimiter = make(chan struct{}, n)
	return s
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

	// Backfill sweep - runs once at startup, 60s after boot, in the background.
	// Picks up seasons that have episodes without segments (legacy data, or
	// scans that landed while the worker was offline). Each season is detected
	// sequentially to keep CPU usage bounded.
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(60 * time.Second):
		}
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "recovered panic in intro detection backfill", slog.Any("error", r))
			}
		}()
		s.backfillMissingSeasons(ctx)
	}()
}

// backfillMissingSeasons enumerates every season parent that has episodes
// without any media_segments rows and runs detection. Idempotent - episodes
// already covered are skipped by detectSeason.
func (s *IntroDetectionService) backfillMissingSeasons(ctx context.Context) {
	parentIDs, err := s.segmentRepo.ListSeasonsMissingSegments(ctx)
	if err != nil {
		slog.WarnContext(ctx, "intro detection backfill: list seasons", slog.Any("err", err))
		return
	}
	if len(parentIDs) == 0 {
		return
	}
	slog.InfoContext(ctx, "intro detection backfill: starting", slog.Int("seasons", len(parentIDs)))
	s.forEachBackfillTarget(ctx, parentIDs, s.detectSeason)
	slog.InfoContext(ctx, "intro detection backfill: done")
}

// backfillWorkers is how many seasons (or movies) a backfill sweep walks at
// once: the pipeline concurrency budget (wire sizes concurrencyLimiter from
// the trickplay CPU budget), not a second knob. The sweeps used to walk
// sequentially, so that budget was only ever consumed by event triggers.
func (s *IntroDetectionService) backfillWorkers() int {
	if s.concurrencyLimiter == nil {
		return defaultMaxParallelDetections
	}
	return cap(s.concurrencyLimiter)
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
	workers := s.backfillWorkers()
	if workers > len(ids) {
		workers = len(ids)
	}
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
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range work {
				guarded(id)
			}
		}()
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

// BackfillLibrary runs the same detection loop as backfillMissingSeasons but
// scoped to a single library. An empty libraryID is equivalent to calling
// backfillMissingSeasons (all libraries). This is the entry point for the
// admin "run backfill" endpoint - it runs synchronously; callers that want
// non-blocking behaviour should wrap it in a goroutine.
//
// Returns the number of seasons that were scheduled (i.e. passed to
// detectSeason, not necessarily found new segments).
func (s *IntroDetectionService) BackfillLibrary(ctx context.Context, libraryID string) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.BackfillLibrary")
	defer span.End()
	var parentIDs []string
	if libraryID == "" {
		// No filter: use the same missing-segments query as the boot sweep.
		ids, err := s.segmentRepo.ListSeasonsMissingSegments(ctx)
		if err != nil {
			return 0, fmt.Errorf("intro detection admin backfill: list seasons: %w", err)
		}
		parentIDs = ids
	} else {
		seasonType := string(domainlibrary.Season)
		libID := shared.LibraryID(libraryID)
		seasons, err := s.mediaItems.List(ctx, domainlibrary.ListMediaItemsFilter{
			LibraryID: &libID,
			Type:      &seasonType,
			// Effectively "all seasons": no real library approaches this
			// many, and the admin backfill must not silently truncate.
			Limit: backfillAllSeasonsLimit,
		})
		if err != nil {
			return 0, fmt.Errorf("intro detection admin backfill: list library seasons: %w", err)
		}
		for _, s := range seasons {
			parentIDs = append(parentIDs, string(s.ID))
		}
	}

	if len(parentIDs) == 0 && libraryID == "" {
		return 0, nil
	}
	slog.InfoContext(ctx, "intro detection admin backfill: starting",
		slog.String("library_id", libraryID),
		slog.Int("seasons", len(parentIDs)))
	s.forEachBackfillTarget(ctx, parentIDs, s.detectSeason)
	movies := 0
	if libraryID != "" {
		movies = s.backfillMovies(ctx, libraryID)
	}
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
				mCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
				detErr := s.detectIntroWithHint(mCtx, id, nil)
				cancel()
				if detErr != nil {
					slog.WarnContext(ctx, "intro backfill movies: detection failed",
						slog.String("media_id", id), slog.Any("err", detErr))
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
			hasGoodIntro := false
			manualLocked := false
			for _, sg := range existing {
				if sg.Type != "intro" {
					continue
				}
				if sg.Source == "manual" || sg.Source == "manual_propagated" {
					manualLocked = true
					break
				}
				// Treat a segment from a FILE-DERIVED authoritative source as
				// "good": once covered, a re-detection would burn CPU re-running
				// pHash + Chromaprint just to land on the same answer.
				//
				// Excluded on purpose: community DBs ("tidb"/"introdb") describe
				// someone else's copy of the media, and the eval measured them
				// tens of seconds off where the cluster was within a second -
				// keep those episodes eligible so a file-derived detection can
				// replace them (runPipeline ranks it above the provider). Also
				// excluded: the last-resort heuristics ("audio", "blackdetect").
				switch sg.Source {
				case "mkv_chapter", "audio_cluster":
					if sg.EndMs-sg.StartMs >= 5_000 {
						hasGoodIntro = true
					}
				case "phash":
					if sg.EndMs-sg.StartMs >= 5_000 {
						hasGoodIntro = true
					}
				}
			}
			if manualLocked || hasGoodIntro {
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
			epCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
			detErr := s.detectIntroWithHint(epCtx, string(ep.ID), hint)
			cancel()
			if detErr != nil {
				// A real failure (ffmpeg crash, I/O) - don't count it toward the cap
				// so a transient problem doesn't park a detectable episode.
				slog.WarnContext(ctx, "intro detection failed", slog.String("media_id", string(ep.ID)), slog.Any("err", detErr))
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
	if s.concurrencyLimiter != nil {
		select {
		case s.concurrencyLimiter <- struct{}{}:
			defer func() { <-s.concurrencyLimiter }()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
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
	if seg, ecErr := s.detectEndCredits(ctx, item); ecErr != nil {
		slog.WarnContext(ctx, "intro_detection: end credits failed", slog.String("media_id", mediaItemID), slog.Any("err", ecErr))
	} else if seg != nil {
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
		if err := s.upsertSegmentRespectingManual(ctx, &seg); err != nil {
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
