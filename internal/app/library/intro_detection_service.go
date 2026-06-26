package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/bits"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
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
}

// Chromaprinter is an optional adapter wrapping fpcalc — when set, the
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
	fpCache      domainlibrary.IntroFingerprintCacheRepository
	audioFPCache domainlibrary.IntroAudioFingerprintCacheRepository
	chapterRepo  domainlibrary.ChapterQueryRepo
	chromaprinter     Chromaprinter
	metadataProvider  domainlibrary.IntroMetadataProvider
	contributor       domainlibrary.SegmentContributor
	detector          IntroDetector
	trickplayCacheDir string
	shutdownCtx       context.Context
	events            shared.EventBus
	// concurrencyLimiter caps the number of pipeline runs in flight at once.
	// Each pipeline run is CPU + IO heavy (sprite decode, ffmpeg audio
	// extraction across all siblings) so unbounded parallelism — what the
	// /api/v1/admin/media/{id}/detect-segments endpoint allowed before — pegs
	// the box and triggers context deadline exceeded on individual queries.
	concurrencyLimiter chan struct{}
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
// intro maxIntroDetectionAttempts times AT this version — instead of re-decoding
// the file (a full software 4K HEVC decode) on every restart forever.
//
// BUMP THIS whenever any part of detection improves: every episode that
// previously produced no usable intro becomes eligible again exactly once at the
// new version. This is the supported way to force a global re-detection after a
// quality improvement — no DB surgery, no admin action.
const introDetectionAlgoVersion = 1

// maxIntroDetectionAttempts bounds the backfill's re-decodes of an episode that
// keeps yielding no usable intro at the same algo version (one retry for the
// detector's sampling variance, then park until introDetectionAlgoVersion bumps).
const maxIntroDetectionAttempts = 2

// defaultMaxParallelDetections caps how many pipeline runs may execute
// concurrently. 2 is a safe default for shared boxes — pHashPass extracts
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
// "Opening Credits", "Generique", etc. — those are authoritative
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
// (e.g. AniDB, simkl, a community plugin) ahead of the heuristic pipeline.
// When the provider returns segments for a given item, those win over MKV
// chapters and over the pHash/audio fallback chain — that's the contract
// described in domainlibrary.IntroMetadataProvider.
//
// nil disables the pass; the rest of the pipeline runs unchanged. There is
// no wired adapter today.
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

	scheduleSeason := func(parentID string) {
		if parentID == "" {
			return
		}
		mu.Lock()
		if t, exists := timers[parentID]; exists {
			t.Stop()
		}
		timers[parentID] = time.AfterFunc(30*time.Second, func() {
			mu.Lock()
			delete(timers, parentID)
			mu.Unlock()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("recovered panic in intro detection worker", slog.Any("error", r))
				}
			}()
			s.detectSeason(ctx, parentID)
		})
		mu.Unlock()
	}

	s.events.Subscribe("media.item.added", func(_ context.Context, ev shared.Event) {
		e, ok := ev.(domainlibrary.MediaItemAdded)
		if !ok || e.Type != string(domainlibrary.Episode) {
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
		case string(domainlibrary.Episode):
			scheduleSeason(string(e.ParentID))
		case string(domainlibrary.MovieItem):
			go func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("recovered panic in intro detection (movie)", slog.Any("error", r))
					}
				}()
				if err := s.DetectIntro(ctx, string(e.MediaItemID)); err != nil {
					slog.Warn("intro detection on trickplay.generated movie failed",
						slog.String("media_id", string(e.MediaItemID)),
						slog.Any("err", err))
				}
			}()
		}
	})

	// Backfill sweep — runs once at startup, 60s after boot, in the background.
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
				slog.Error("recovered panic in intro detection backfill", slog.Any("error", r))
			}
		}()
		s.backfillMissingSeasons(ctx)
	}()
}

// backfillMissingSeasons enumerates every season parent that has episodes
// without any media_segments rows and runs detection. Idempotent — episodes
// already covered are skipped by detectSeason.
func (s *IntroDetectionService) backfillMissingSeasons(ctx context.Context) {
	parentIDs, err := s.segmentRepo.ListSeasonsMissingSegments(ctx)
	if err != nil {
		slog.Warn("intro detection backfill: list seasons", slog.Any("err", err))
		return
	}
	if len(parentIDs) == 0 {
		return
	}
	slog.Info("intro detection backfill: starting", slog.Int("seasons", len(parentIDs)))
	for _, pid := range parentIDs {
		if ctx.Err() != nil {
			return
		}
		s.detectSeason(ctx, pid)
	}
	slog.Info("intro detection backfill: done")
}

// BackfillLibrary runs the same detection loop as backfillMissingSeasons but
// scoped to a single library. An empty libraryID is equivalent to calling
// backfillMissingSeasons (all libraries). This is the entry point for the
// admin "run backfill" endpoint — it runs synchronously; callers that want
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
			Limit:     10000,
		})
		if err != nil {
			return 0, fmt.Errorf("intro detection admin backfill: list library seasons: %w", err)
		}
		for _, s := range seasons {
			parentIDs = append(parentIDs, string(s.ID))
		}
	}

	if len(parentIDs) == 0 {
		return 0, nil
	}
	slog.Info("intro detection admin backfill: starting",
		slog.String("library_id", libraryID),
		slog.Int("seasons", len(parentIDs)))
	for _, pid := range parentIDs {
		if ctx.Err() != nil {
			break
		}
		s.detectSeason(ctx, pid)
	}
	slog.Info("intro detection admin backfill: done",
		slog.String("library_id", libraryID),
		slog.Int("seasons", len(parentIDs)))
	return len(parentIDs), nil
}

// detectSeason runs detection on every episode of the given season that
// doesn't already have an intro segment (presence of end_credits alone is not
// sufficient — partial detection happens when not all siblings had their
// trickplay ready, and we must re-run as more become available).
//
// Skips episodes whose existing intro is locked by an operator (manual
// override) since `upsertSegmentRespectingManual` would no-op anyway and we
// don't want to waste the audio extraction.
func (s *IntroDetectionService) detectSeason(ctx context.Context, parentID string) {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.detectSeason")
	defer span.End()

	episodeType := string(domainlibrary.Episode)
	// Page through the whole season. A single Limit:100 (or even the repo's 500
	// cap) would silently skip episodes 101+ of a long single-season (anime,
	// daily soaps), leaving them without intro markers forever — this is the only
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
			slog.Warn("intro detection: list season episodes", slog.String("parent_id", parentID), slog.Any("err", err))
			return
		}
		if len(siblings) == 0 {
			break
		}
		for _, ep := range siblings {
			if ctx.Err() != nil {
				return
			}
			existing, getErr := s.segmentRepo.GetByMediaItemID(ctx, ep.ID)
			if getErr != nil {
				// Couldn't read existing segments; treat as uncovered and let
				// detection re-run rather than skip the episode silently.
				slog.Warn("intro detection: get existing segments", slog.String("media_id", string(ep.ID)), slog.Any("err", getErr))
			}
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
				// Treat any segment from an authoritative source as "good": once
				// covered, a re-detection would burn CPU re-running pHash +
				// Chromaprint just to land on the same answer (or worse, a
				// noisier heuristic answer). Sources are ordered by authority
				// in runPipeline; skipping any of them is safe.
				//
				// Excluded on purpose: the audio fallback ("audio") and the
				// blackdetect end-credits source ("blackdetect") are last-
				// resort heuristics — keep them eligible for re-detection so
				// a future TIDB/IntroDB hit can replace them.
				switch sg.Source {
				case "mkv_chapter", "tidb", "introdb":
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
				continue
			}
			// Already re-decoded to the cap at the current algo version with no
			// usable intro? Skip — it lands on the same answer and a 4K HEVC decode
			// is expensive. Bumping introDetectionAlgoVersion re-enables it (the
			// row's older version no longer matches).
			if exhausted, exErr := s.segmentRepo.IntroDetectionExhausted(ctx, string(ep.ID), introDetectionAlgoVersion, maxIntroDetectionAttempts); exErr == nil && exhausted {
				continue
			}
			epCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
			detErr := s.DetectIntro(epCtx, string(ep.ID))
			cancel()
			if detErr != nil {
				// A real failure (ffmpeg crash, I/O) — don't count it toward the cap
				// so a transient problem doesn't park a detectable episode.
				slog.Warn("intro detection failed", slog.String("media_id", string(ep.ID)), slog.Any("err", detErr))
				continue
			}
			// Clean run: count it. If it found a good intro the episode is skipped
			// next time via hasGoodIntro; if it found nothing/weak, this bounds the
			// retries until introDetectionAlgoVersion bumps.
			if recErr := s.segmentRepo.RecordIntroDetectionAttempt(ctx, string(ep.ID), introDetectionAlgoVersion); recErr != nil {
				slog.Warn("intro detection: record attempt", slog.String("media_id", string(ep.ID)), slog.Any("err", recErr))
			}
		}
		if len(siblings) < pageSize {
			break
		}
	}
}

// DetectIntro is the main entry point. Runs the two-pass pipeline and upserts
// all detected segments. Acquires a slot from concurrencyLimiter so a flood
// of admin /detect-segments triggers (or worker auto-runs) don't pile up
// ffmpeg processes that thrash the host.
func (s *IntroDetectionService) DetectIntro(ctx context.Context, mediaItemID string) error {
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

	segments, err := s.runPipeline(ctx, item)
	if err != nil {
		return fmt.Errorf("detection pipeline: %w", err)
	}

	// End credits detection runs independently of the intro pipeline so that
	// films (which produce no intro windows) also get a credits segment.
	if seg, ecErr := s.detectEndCredits(ctx, item); ecErr != nil {
		slog.Warn("intro_detection: end credits failed", slog.String("media_id", mediaItemID), slog.Any("err", ecErr))
	} else if seg != nil {
		segments = append(segments, *seg)
	}

	// Studio logo detection also runs independently for both films and episodes.
	// It requires a trickplay manifest but does not wait if one is absent.
	if seg := s.detectStudioLogo(ctx, item); seg != nil {
		segments = append(segments, *seg)
	}

	if len(segments) == 0 {
		slog.Info("intro_detection: no segments found", slog.String("media_id", mediaItemID))
		return nil
	}

	for _, seg := range segments {
		seg.ID = uuid.New().String()
		seg.MediaItemID = shared.MediaItemID(mediaItemID)
		if err := s.upsertSegmentRespectingManual(ctx, &seg); err != nil {
			return fmt.Errorf("upsert segment %s: %w", seg.Type, err)
		}
		slog.Info("intro_detection: segment stored",
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
						slog.Error("panic recovered in intro_detection: contribute",
							slog.Any("panic", r),
							slog.String("stack", string(debug.Stack())))
					}
				}()
				cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if cErr := s.contributor.Contribute(cctx, itemCopy, segCopy); cErr != nil {
					slog.Warn("intro_detection: segment contribution failed",
						slog.String("type", segCopy.Type), slog.Any("err", cErr))
				}
			}()
		}
	}
	return nil
}

// upsertSegmentRespectingManual upserts unless an existing segment for the same
// (media_item_id, type) has source "manual" or "manual_propagated" — operator
// choices must never be silently overwritten by re-runs of auto-detection.
//
// The manual-lock check and the write are done in ONE atomic statement
// (UpsertUnlessManual) rather than read-then-write: a separate
// GetByMediaItemID + Upsert let a concurrent detection (or an operator
// SetManualSegment) slip a manual mark into the window between the read and
// the write, which the auto-upsert would then clobber.
func (s *IntroDetectionService) upsertSegmentRespectingManual(ctx context.Context, seg *domainlibrary.MediaSegment) error {
	applied, err := s.segmentRepo.UpsertUnlessManual(ctx, seg)
	if err != nil {
		return fmt.Errorf("upsert segment with manual-lock guard: %w", err)
	}
	if !applied {
		slog.Info("intro_detection: skipping auto upsert, manual lock present",
			slog.String("media_id", string(seg.MediaItemID)),
			slog.String("type", seg.Type))
	}
	return nil
}

// SetManualSegment upserts a single segment with source="manual" and confidence=1.0.
// This is the operator-driven override path. The (media_item_id, type) DB unique
// constraint makes this idempotent: posting the same type updates start/end.
func (s *IntroDetectionService) SetManualSegment(ctx context.Context, mediaItemID, segType string, startMs, endMs int64) error {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.SetManualSegment")
	defer span.End()
	if err := validateSegmentInput(segType, startMs, endMs); err != nil {
		return err
	}
	item, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil {
		return fmt.Errorf("get media item: %w", err)
	}
	if item.File != nil && !item.File.Duration.IsZero() {
		durMs := item.File.Duration.Milliseconds()
		if endMs > durMs {
			return fmt.Errorf("end_ms %d exceeds media duration %d", endMs, durMs)
		}
	}
	seg := &domainlibrary.MediaSegment{
		ID:          uuid.New().String(),
		MediaItemID: shared.MediaItemID(mediaItemID),
		Type:        segType,
		StartMs:     startMs,
		EndMs:       endMs,
		Confidence:  1.0,
		Source:      "manual",
	}
	return s.segmentRepo.Upsert(ctx, seg)
}

// DeleteSegment removes a segment of the given type for the given media item.
func (s *IntroDetectionService) DeleteSegment(ctx context.Context, mediaItemID, segType string) error {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.DeleteSegment")
	defer span.End()
	return s.segmentRepo.DeleteByMediaItemIDAndType(ctx, shared.MediaItemID(mediaItemID), segType)
}

// PropagateSegment takes a manually-defined segment on a reference episode and
// re-runs pHash detection across the season, using the reference's frame hashes
// (covering [startMs, endMs]) as the introHashes set. Each sibling that finds
// a matching contiguous window gets a "manual_propagated" segment. Siblings
// that already have a "manual" lock are skipped (operator choice wins).
//
// Only works on episodes; films return an error.
func (s *IntroDetectionService) PropagateSegment(ctx context.Context, refMediaItemID, segType string) (propagatedTo []string, err error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.PropagateSegment")
	defer span.End()
	refItem, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(refMediaItemID))
	if err != nil {
		return nil, fmt.Errorf("get reference item: %w", err)
	}
	if refItem.Type != domainlibrary.Episode {
		return nil, fmt.Errorf("propagate only works on episodes, got %q", refItem.Type)
	}
	if refItem.ParentID == "" {
		return nil, errors.New("episode has no parent (season) to propagate within")
	}

	refSegments, err := s.segmentRepo.GetByMediaItemID(ctx, refItem.ID)
	if err != nil {
		return nil, fmt.Errorf("load reference segments: %w", err)
	}
	var refSeg *domainlibrary.MediaSegment
	for _, sg := range refSegments {
		if sg.Type == segType {
			refSeg = sg
			break
		}
	}
	if refSeg == nil {
		return nil, fmt.Errorf("no %s segment on reference episode", segType)
	}

	refManifest, err := s.trickplayRepo.GetByMediaItemID(ctx, refItem.ID)
	if err != nil {
		return nil, fmt.Errorf("reference trickplay missing: %w", err)
	}
	refFingerprint := ""
	if refItem.File != nil {
		refFingerprint = refItem.File.Fingerprint
	}
	refAllHashes, refAllMs, err := s.extractEpisodeHashesCached(ctx, string(refItem.ID), refFingerprint, refManifest, manifestTotalFrames(refManifest))
	if err != nil || len(refAllHashes) == 0 {
		return nil, fmt.Errorf("extract reference hashes: %w", err)
	}
	// Pick frames whose real timestamp falls inside [refSeg.StartMs, refSeg.EndMs).
	introHashes := make(map[uint64]bool)
	for i, ms := range refAllMs {
		if ms >= refSeg.StartMs && ms < refSeg.EndMs {
			introHashes[refAllHashes[i]] = true
		}
	}
	if len(introHashes) == 0 {
		return nil, errors.New("reference segment out of trickplay range")
	}

	episodeType := string(domainlibrary.Episode)
	// Page through the whole season — a fixed Limit:100 would silently skip
	// episodes 101+ of a long single-season (anime, daily soaps), leaving them
	// without the propagated segment. Mirrors detectSeason's pagination.
	var siblings []*domainlibrary.MediaItem
	const pageSize = 200
	for offset := 0; ; offset += pageSize {
		page, err := s.mediaItems.List(ctx, domainlibrary.ListMediaItemsFilter{
			ParentID: &refItem.ParentID,
			Type:     &episodeType,
			Limit:    pageSize,
			Offset:   offset,
		})
		if err != nil {
			return nil, fmt.Errorf("list siblings: %w", err)
		}
		if len(page) == 0 {
			break
		}
		siblings = append(siblings, page...)
		if len(page) < pageSize {
			break
		}
	}

	cfg := loadIntroConfig(ctx, s.configRepo)
	maxHamming := cfg.MaxHamming
	for _, sib := range siblings {
		if sib.ID == refItem.ID {
			continue
		}
		sibSegs, sibSegErr := s.segmentRepo.GetByMediaItemID(ctx, sib.ID)
		if sibSegErr != nil {
			// Cannot determine lock state — skip the sibling to protect any
			// operator "manual" mark we cannot read right now.
			slog.Warn("intro_detection: propagate skipping sibling, cannot read segments for manual-lock guard",
				slog.String("sibling_id", string(sib.ID)),
				slog.Any("err", sibSegErr))
			continue
		}
		manualLock := false
		for _, sg := range sibSegs {
			if sg.Type == segType && sg.Source == "manual" {
				manualLock = true
				break
			}
		}
		if manualLock {
			continue
		}

		sibManifest, sibErr := s.trickplayRepo.GetByMediaItemID(ctx, sib.ID)
		if sibErr != nil {
			continue
		}
		sibFingerprint := ""
		if sib.File != nil {
			sibFingerprint = sib.File.Fingerprint
		}
		sibHashes, sibMs, hashErr := s.extractEpisodeHashesCached(ctx, string(sib.ID), sibFingerprint, sibManifest, manifestTotalFrames(sibManifest))
		if hashErr != nil || len(sibHashes) == 0 {
			continue
		}

		cands := allCandidatesGapTolerant(introHashes, sibHashes, sibMs, maxHamming)
		if len(cands) == 0 {
			continue
		}
		windows := make([]matchWindow, 0, len(cands))
		for _, c := range cands {
			windows = append(windows, matchWindow{startMs: c.startMs, endMs: c.endMs})
		}
		refDur := refSeg.EndMs - refSeg.StartMs
		chosen := pickWindowMatchingDuration(windows, refDur)
		if chosen == nil {
			continue
		}
		seg := &domainlibrary.MediaSegment{
			ID:          uuid.New().String(),
			MediaItemID: sib.ID,
			Type:        segType,
			StartMs:     chosen.startMs,
			EndMs:       chosen.endMs,
			Confidence:  0.85,
			Source:      "manual_propagated",
		}
		// Atomic manual-lock guard closes the TOCTOU: the manualLock read-check
		// above can go stale while we hash + match (expensive), so re-check the
		// operator 'manual' mark in the same statement as the write. Operator-only
		// (not 'manual_propagated') so a re-run can still refresh our own prior
		// propagation.
		applied, upsertErr := s.segmentRepo.UpsertUnlessOperatorManual(ctx, seg)
		if upsertErr != nil {
			slog.Warn("propagate: upsert failed",
				slog.String("sibling", string(sib.ID)),
				slog.Any("err", upsertErr))
			continue
		}
		if !applied {
			slog.Info("intro_detection: propagate skipped, operator manual lock won the race",
				slog.String("sibling", string(sib.ID)))
			continue
		}
		propagatedTo = append(propagatedTo, string(sib.ID))
	}
	return propagatedTo, nil
}

// pickWindowMatchingDuration returns the window whose duration is closest to ref,
// within a 50% tolerance, or nil if none qualifies.
func pickWindowMatchingDuration(windows []matchWindow, refDurationMs int64) *matchWindow {
	var best *matchWindow
	var bestDelta int64 = -1
	for i, w := range windows {
		dur := w.endMs - w.startMs
		delta := dur - refDurationMs
		if delta < 0 {
			delta = -delta
		}
		if dur*2 < refDurationMs || dur > refDurationMs*2 {
			continue
		}
		if bestDelta < 0 || delta < bestDelta {
			bestDelta = delta
			best = &windows[i]
		}
	}
	return best
}

// manifestTotalFrames returns the upper bound on extractable frames for a manifest.
func manifestTotalFrames(m *domainlibrary.TrickplayManifest) int {
	return m.ImageCount * m.TileWidth * m.TileHeight
}

// validateSegmentInput rejects unknown types and bad ranges.
func validateSegmentInput(segType string, startMs, endMs int64) error {
	switch segType {
	case "intro", "recap", "end_credits", "studio_logo":
	default:
		return fmt.Errorf("invalid segment type %q", segType)
	}
	if startMs < 0 {
		return errors.New("start_ms must be >= 0")
	}
	if endMs <= startMs {
		return errors.New("end_ms must be > start_ms")
	}
	return nil
}

// runPipeline runs the multi-stage intro detection pipeline:
//
//  1. pHash trickplay matching produces a list of candidate windows.
//  2. Audio fingerprint cross-correlation against siblings confirms which
//     candidate is the real recurring intro and rejects coincidental scene
//     transitions that share visual hashes (establishing shots, recurring
//     locations) but not audio content.
//  3. PrecisionPass tunes the boundaries via silence/black detection.
//
// Without sibling audio confirmation the visual-pHash signal alone often picks
// false positives (e.g. recurring landscape inserts), because runs of matched
// frames inside an episode can outscore the real intro when intro frames are
// fragmented by unique transition frames.
func (s *IntroDetectionService) runPipeline(ctx context.Context, item *domainlibrary.MediaItem) ([]domainlibrary.MediaSegment, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.runPipeline")
	defer span.End()
	defer bgtasks.Begin("intro_detection", string(item.ID))()

	mediaItemID := string(item.ID)
	cfg := loadIntroConfig(ctx, s.configRepo)

	// External metadata provider first: when an adapter is wired (e.g. an
	// AniDB / simkl / community-DB lookup) and it knows this item, those
	// segments are the most authoritative. Provider errors are non-fatal —
	// fall through to chapters/heuristics.
	if s.metadataProvider != nil {
		mdSegs, mdErr := s.metadataProvider.GetSegments(ctx, item)
		if mdErr != nil {
			slog.Warn("intro_detection: metadata provider failed, falling back",
				slog.String("media_id", mediaItemID), slog.Any("err", mdErr))
		} else if len(mdSegs) > 0 {
			slog.Info("intro_detection.pipeline_decision",
				slog.String("media_id", mediaItemID),
				slog.String("decision", "metadata_provider_match"),
				slog.Int("segments", len(mdSegs)),
			)
			return mdSegs, nil
		}
	}

	// MKV chapters next: authoritative when present (uploader labelled
	// the chapter "Intro" / "Opening" / "Generique" etc.). Skip the rest
	// of the pipeline if we got something — no heuristic beats explicit
	// metadata.
	if chSegs := s.chapterPass(ctx, item); len(chSegs) > 0 {
		slog.Info("intro_detection.pipeline_decision",
			slog.String("media_id", mediaItemID),
			slog.String("decision", "mkv_chapter_match"),
			slog.Int("segments", len(chSegs)),
		)
		return chSegs, nil
	}

	if s.isTrickplayEnabled(ctx) {
		result, phashErr := s.pHashPass(ctx, item, cfg)
		if phashErr != nil {
			slog.Warn("intro_detection: phash pass failed, falling back",
				slog.String("media_id", mediaItemID),
				slog.Any("err", phashErr),
			)
		} else if result != nil && len(result.candidates) > 0 {
			var confirmed []confirmedWindow
			if s.chromaprinter != nil {
				confirmed = s.confirmAudioCandidatesChromaprint(ctx, item, result, cfg)
			}
			if len(confirmed) == 0 {
				confirmed = s.confirmAudioCandidates(ctx, item, result, cfg)
			}
			if len(confirmed) > 0 {
				// pHash boundaries cover only the *matched* frames, which
				// can be a strict subset of the real intro: title-card
				// frames stable across episodes match, but the establishing
				// shots and intertitles around them are unique-per-episode
				// and don't. Use silencedetect+blackdetect on a wider
				// window around each pHash window to widen the boundaries
				// out to the real start/end (silence before/after the
				// theme music). Anchored at the pHash window, so the
				// "first silence in the file" failure mode (studio logo
				// at 0s) doesn't apply.
				refined := s.refineWithPrecisionPass(ctx, item.File.Path, confirmed)
				return classifyConfirmed(refined, cfg), nil
			}
			slog.Info("intro_detection.pipeline_decision",
				slog.String("media_id", mediaItemID),
				slog.Int("candidates_evaluated", len(result.candidates)),
				slog.Int("siblings_total", len(result.siblings)),
				slog.String("decision", "fallback_no_candidate_confirmed"),
			)
		}
	}

	// Audio-primary DISCOVERY: the visual pass produced nothing confirmable
	// (poor SD trickplay, or a cold-open show whose title card isn't visually
	// stable). Before the weak silencedetect fallback, let chromaprint discover
	// a recurring intro directly across siblings at a free per-episode offset —
	// the variable-teaser case the visual+confirm path structurally misses.
	if seg, ok := s.discoverIntroByAudio(ctx, item); ok {
		slog.Info("intro_detection.pipeline_decision",
			slog.String("media_id", mediaItemID),
			slog.String("decision", "chromaprint_discovery"),
		)
		return []domainlibrary.MediaSegment{seg}, nil
	}

	// Fallback: concurrent audio/subtitle passes.
	startMs, endMs, source, confidence, err := s.concurrentFallback(ctx, item)
	if err != nil || (startMs == 0 && endMs == 0) {
		return nil, err
	}
	return []domainlibrary.MediaSegment{{
		Type:       "intro",
		StartMs:    startMs,
		EndMs:      endMs,
		Source:     source,
		Confidence: confidence,
	}}, nil
}

// refineWithPrecisionPass widens each pHash window to the actual intro
// boundaries. The pHash window is the anchor (it survived audio
// confirmation, so we trust it points at a real intro), but its raw
// boundaries only cover the matched frames — the actual intro can extend
// before the first match and after the last (frames unique per episode
// don't match across siblings).
//
// Strategy:
//  1. Run silencedetect+blackdetect on a +/-padSec window around the pHash
//     anchor. If a usable transition is found, use it (most accurate).
//  2. Otherwise apply a fixed +/-fallbackPadSec extension to the pHash
//     window. Series with continuous music (Robin Hood, action shows)
//     don't have detectable silence around their intro, so silencedetect
//     finds nothing — but the actual intro is still wider than the
//     matched-frame window. The fallback pad gives the user a usable
//     skip button rather than a too-tight 16s window.
//
// PrecisionPass used to be called on the whole episode and would lock onto
// the first studio-logo silence at 0s — that failure mode doesn't apply
// here because we anchor on the pHash window position.
func (s *IntroDetectionService) refineWithPrecisionPass(ctx context.Context, filePath string, windows []confirmedWindow) []confirmedWindow {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.refineWithPrecisionPass")
	defer span.End()

	if filePath == "" || len(windows) == 0 {
		return windows
	}
	const (
		padSec = 15.0
		// Asymmetric fallback pad: pHash matches the visually-stable parts of
		// an intro (title cards, logos), which typically sit at the END of
		// the intro. The START is more often a cold-open transition unique
		// per episode that doesn't match. So expand more on the start side
		// than the end side to cover the real intro span without
		// over-shooting into the post-intro content.
		fallbackPadStartMs = 10_000
		fallbackPadEndMs   = 2_000
	)
	out := make([]confirmedWindow, 0, len(windows))
	for _, w := range windows {
		startSec := float64(w.startMs) / 1000
		endSec := float64(w.endMs) / 1000
		refStart, refEnd, err := s.detector.PrecisionPass(ctx, filePath, startSec, endSec, padSec)

		usePrecision := err == nil &&
			refStart > 0 && refEnd > refStart &&
			refEnd >= w.startMs-int64(2*padSec*1000) &&
			refStart <= w.endMs+int64(2*padSec*1000) &&
			// PrecisionPass returns the *input* (startMs, endMs) when it
			// finds nothing — detect the no-op to fall through to padding.
			(refStart != w.startMs || refEnd != w.endMs)

		var newStart, newEnd int64
		var source string
		if usePrecision {
			newStart, newEnd, source = refStart, refEnd, "silencedetect"
		} else {
			newStart = w.startMs - fallbackPadStartMs
			if newStart < 0 {
				newStart = 0
			}
			newEnd = w.endMs + fallbackPadEndMs
			source = "fixed_pad"
		}

		slog.Debug("intro_detection.refined",
			slog.Int64("phash_start_ms", w.startMs),
			slog.Int64("phash_end_ms", w.endMs),
			slog.Int64("refined_start_ms", newStart),
			slog.Int64("refined_end_ms", newEnd),
			slog.String("refine_source", source),
		)
		out = append(out, confirmedWindow{
			startMs:    newStart,
			endMs:      newEnd,
			matchCount: w.matchCount,
			confidence: w.confidence,
		})
	}
	return out
}

// chapterPass looks for Matroska chapters whose title matches a known
// intro/recap pattern (Intro, Opening, Theme, Generique, Title Sequence,
// Previously, Recap, ...). Returns one MediaSegment per matched chapter
// with source="mkv_chapter", confidence 1.0 — chapters are authoritative
// since they were encoded by the uploader.
//
// Special cases:
//   - "Intro End" alone is not an intro; it's the END marker. Pair with a
//     preceding chapter (typically the cold open or studio logo) by using
//     [start_of_episode, intro_end_chapter.start_ms] as the intro window.
//     Heuristic: if "Intro End" is found and no plain "Intro" chapter
//     exists, treat the previous chapter's start (or 0) as the intro start.
func (s *IntroDetectionService) chapterPass(ctx context.Context, item *domainlibrary.MediaItem) []domainlibrary.MediaSegment {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.chapterPass")
	defer span.End()

	if s.chapterRepo == nil {
		return nil
	}
	chapters, err := s.chapterRepo.GetByMediaItemID(ctx, item.ID)
	if err != nil || len(chapters) == 0 {
		return nil
	}
	// Sort by start in case the repo doesn't guarantee order.
	sort.Slice(chapters, func(i, j int) bool { return chapters[i].StartTimeMs < chapters[j].StartTimeMs })

	// MKV chapters often record only the start timestamp; end_ms is then 0
	// (or equal to start). Standard convention: a chapter ends where the
	// next one begins. Fill missing ends in-place so the segment builders
	// below can rely on EndTimeMs > StartTimeMs.
	for i := range chapters {
		if chapters[i].EndTimeMs > chapters[i].StartTimeMs {
			continue
		}
		if i+1 < len(chapters) && chapters[i+1].StartTimeMs > chapters[i].StartTimeMs {
			chapters[i].EndTimeMs = chapters[i+1].StartTimeMs
		}
	}

	classify := func(title string) string {
		t := strings.ToLower(strings.TrimSpace(title))
		switch {
		case t == "intro end" || t == "end of intro" || t == "intro ends":
			return "intro_end_marker"
		case strings.Contains(t, "intro") ||
			strings.Contains(t, "opening") ||
			strings.Contains(t, "theme") ||
			strings.Contains(t, "title sequence") ||
			strings.Contains(t, "generique") || strings.Contains(t, "générique"):
			return "intro"
		case strings.Contains(t, "previously") ||
			strings.Contains(t, "recap") ||
			strings.HasPrefix(t, "previously on") ||
			strings.Contains(t, "résumé") || strings.Contains(t, "resume des"):
			return "recap"
		case strings.Contains(t, "end credits") ||
			strings.Contains(t, "closing credits") ||
			strings.Contains(t, "ending"):
			return "end_credits"
		}
		return ""
	}

	var (
		out         []domainlibrary.MediaSegment
		hasIntro    bool
		introEndIdx = -1
	)
	for i, ch := range chapters {
		kind := classify(ch.Title)
		switch kind {
		case "intro":
			out = append(out, domainlibrary.MediaSegment{
				Type: "intro", Source: "mkv_chapter", Confidence: 1.0,
				StartMs: ch.StartTimeMs, EndMs: ch.EndTimeMs,
			})
			hasIntro = true
		case "recap":
			out = append(out, domainlibrary.MediaSegment{
				Type: "recap", Source: "mkv_chapter", Confidence: 1.0,
				StartMs: ch.StartTimeMs, EndMs: ch.EndTimeMs,
			})
		case "end_credits":
			out = append(out, domainlibrary.MediaSegment{
				Type: "end_credits", Source: "mkv_chapter", Confidence: 1.0,
				StartMs: ch.StartTimeMs, EndMs: ch.EndTimeMs,
			})
		case "intro_end_marker":
			introEndIdx = i
		}
	}

	// "Intro End" without a paired "Intro" chapter: synthesise an intro
	// segment from [previous chapter start, intro_end start]. Skip if a
	// plain Intro chapter was already added (avoid duplicates).
	if introEndIdx >= 0 && !hasIntro {
		var startMs int64
		if introEndIdx > 0 {
			startMs = chapters[introEndIdx-1].StartTimeMs
		}
		endMs := chapters[introEndIdx].StartTimeMs
		if endMs > startMs {
			out = append(out, domainlibrary.MediaSegment{
				Type: "intro", Source: "mkv_chapter", Confidence: 1.0,
				StartMs: startMs, EndMs: endMs,
			})
		}
	}
	return out
}

// matchWindow represents a contiguous time range of matching hashes.
type matchWindow struct {
	startMs, endMs int64
}

// pHashCandidate is a candidate intro/recap window in target ms (real frame
// timestamps from the trickplay sidecar — not interval×index, which lies when
// keyframes are sparse). matchCount counts the matched frames inside, used as
// a tie-breaker; the audio stage filters by signal coherence not by count.
type pHashCandidate struct {
	startMs, endMs int64
	matchCount     int
}

// pHashResult bundles the trickplay-stage output with the data needed for the
// audio confirmation stage (sibling files for cross-correlation).
type pHashResult struct {
	candidates []pHashCandidate
	siblings   []*domainlibrary.MediaItem
}

// confirmedWindow is a pHash candidate that passed audio cross-correlation
// confirmation against siblings.
type confirmedWindow struct {
	startMs, endMs int64
	matchCount     int
	confidence     float64
}

// classifyConfirmed converts audio-confirmed windows into typed MediaSegments.
// At most one intro and one preceding recap (<60s, occurs before intro) are
// emitted. Source is set to "phash" with confidence inherited from the
// confirmation stage (typically 0.95 for audio-confirmed, 0.7 for visual-only
// fallback).
func classifyConfirmed(windows []confirmedWindow, cfg introConfig) []domainlibrary.MediaSegment {
	if len(windows) == 0 {
		return nil
	}
	// Pick the longest as intro.
	intro := windows[0]
	for _, w := range windows[1:] {
		if (w.endMs - w.startMs) > (intro.endMs - intro.startMs) {
			intro = w
		}
	}
	out := []domainlibrary.MediaSegment{{
		Type:       "intro",
		StartMs:    intro.startMs,
		EndMs:      intro.endMs,
		Source:     "phash",
		Confidence: intro.confidence,
	}}
	// Look for an earlier short window (< RecapMaxMs) ending before intro start:
	// that's the recap (a brief "previously on…" before the title sequence).
	for _, w := range windows {
		if w.endMs > intro.startMs {
			continue
		}
		if (w.endMs - w.startMs) >= cfg.RecapMaxMs {
			continue
		}
		out = append([]domainlibrary.MediaSegment{{
			Type:       "recap",
			StartMs:    w.startMs,
			EndMs:      w.endMs,
			Source:     "phash",
			Confidence: 0.7,
		}}, out...)
		break
	}
	return out
}

// pHashPass performs the coarse localization via trickplay sprite frequency
// maps. Waits up to 24h for a trickplay manifest. Returns all candidate
// windows (gap-tolerant, ≥2 matched frames) along with the sibling MediaItems
// needed for the audio-confirmation stage.
func (s *IntroDetectionService) pHashPass(ctx context.Context, item *domainlibrary.MediaItem, cfg introConfig) (*pHashResult, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.pHashPass")
	defer span.End()

	mediaID := shared.MediaItemID(item.ID)

	manifest, err := s.waitForManifest(ctx, mediaID)
	if err != nil {
		return nil, err
	}

	episodeType := string(domainlibrary.Episode)
	siblings, listErr := s.mediaItems.List(ctx, domainlibrary.ListMediaItemsFilter{
		ParentID: &item.ParentID,
		Type:     &episodeType,
		Limit:    100,
	})
	if listErr != nil {
		return nil, fmt.Errorf("list siblings: %w", listErr)
	}

	var durationMs int64
	if item.File != nil {
		durationMs = item.File.Duration.Milliseconds()
	}
	cutoffFrame := frameCutoff(manifest, durationMs, cfg.FrameCutoffPercent)

	// Collect hashes for siblings that have a trickplay manifest. Keep MediaItems
	// in parallel so the audio-confirmation stage can read their file paths.
	episodeHashes := make([][]uint64, 0, len(siblings))
	siblingItems := make([]*domainlibrary.MediaItem, 0, len(siblings))
	for _, sib := range siblings {
		if sib.ID == item.ID {
			continue
		}
		if sib.File == nil || sib.File.Path == "" {
			continue
		}
		sibManifest, sibErr := s.trickplayRepo.GetByMediaItemID(ctx, sib.ID)
		if sibErr != nil {
			continue
		}
		sibFingerprint := ""
		if sib.File != nil {
			sibFingerprint = sib.File.Fingerprint
		}
		hashes, _, hashErr := s.extractEpisodeHashesCached(ctx, string(sib.ID), sibFingerprint, sibManifest, cutoffFrame)
		if hashErr != nil || len(hashes) == 0 {
			continue
		}
		episodeHashes = append(episodeHashes, hashes)
		siblingItems = append(siblingItems, sib)
	}

	// Need at least 2 episodes total (target + 1 sibling) to compute frequencies.
	if len(episodeHashes) < 1 {
		return nil, nil
	}

	// Find matching frame windows in the target episode's own hash sequence.
	targetFingerprint := ""
	if item.File != nil {
		targetFingerprint = item.File.Fingerprint
	}
	targetHashes, targetMs, hashErr := s.extractEpisodeHashesCached(ctx, string(item.ID), targetFingerprint, manifest, cutoffFrame)
	if hashErr != nil || len(targetHashes) == 0 {
		return nil, nil
	}

	// FrequencyMap built from target + siblings. At 15s sampling, real intro
	// frames often only land in 50-60% of siblings (sampling phase varies
	// per-episode, fragments the match). We keep the threshold loose here and
	// rely on audio confirmation downstream to reject false positives that
	// pass through.
	allEps := append([][]uint64{targetHashes}, episodeHashes...)
	introHashes := FrequencyMapHamming(allEps, cfg.FreqMapThreshold, cfg.MaxHamming)
	if len(introHashes) == 0 {
		return nil, nil
	}

	candidates := allCandidatesGapTolerant(introHashes, targetHashes, targetMs, cfg.MaxHamming)
	// Drop candidates that start past MaxIntroStartMs — those are not intros
	// but recurring in-episode visual elements (chapter cards, flashbacks,
	// previously-on segments) that happen to share visual hashes across
	// episodes. Without this filter we surface false positives like a
	// "Skip Intro" button at 9 minutes into an episode.
	if cfg.MaxIntroStartMs > 0 {
		filtered := candidates[:0]
		for _, c := range candidates {
			if c.startMs > cfg.MaxIntroStartMs {
				slog.Debug("intro_detection.candidate_dropped_too_late",
					slog.String("media_id", string(item.ID)),
					slog.Int64("candidate_start_ms", c.startMs),
					slog.Int64("max_intro_start_ms", cfg.MaxIntroStartMs),
				)
				continue
			}
			filtered = append(filtered, c)
		}
		candidates = filtered
	}
	return &pHashResult{
		candidates: candidates,
		siblings:   siblingItems,
	}, nil
}

// allCandidatesGapTolerant scans the target hash sequence for runs of
// matching frames, merges runs separated by ≤2 non-matching frames into
// single spans (real intros sampled at 15s/frame often have 1-2 unique
// transition frames inside them), and returns spans with ≥2 matched frames.
//
// targetMs[i] is the source-video timestamp of targetHashes[i]; candidate
// boundaries are returned in real ms, not in frame-index×interval (which
// would be a lie when keyframes are sparse).
func allCandidatesGapTolerant(introHashes map[uint64]bool, targetHashes []uint64, targetMs []int64, maxHamming int) []pHashCandidate {
	if len(targetHashes) == 0 || len(targetMs) != len(targetHashes) {
		return nil
	}

	type rawRun struct {
		start, end int // endFrame exclusive (frame index)
		matchCount int
	}
	var raw []rawRun
	runStart, runLen := 0, 0
	for i, h := range targetHashes {
		if matchesIntroHashes(h, introHashes, maxHamming) {
			if runLen == 0 {
				runStart = i
			}
			runLen++
		} else if runLen > 0 {
			raw = append(raw, rawRun{runStart, runStart + runLen, runLen})
			runLen = 0
		}
	}
	if runLen > 0 {
		raw = append(raw, rawRun{runStart, runStart + runLen, runLen})
	}
	if len(raw) == 0 {
		return nil
	}

	const maxGap = 2
	type span struct{ start, end, matchCount int }
	merged := []span{{raw[0].start, raw[0].end, raw[0].matchCount}}
	for _, r := range raw[1:] {
		last := &merged[len(merged)-1]
		gap := r.start - last.end
		if gap <= maxGap {
			last.end = r.end
			last.matchCount += r.matchCount
		} else {
			merged = append(merged, span{r.start, r.end, r.matchCount})
		}
	}

	out := make([]pHashCandidate, 0, len(merged))
	for _, m := range merged {
		if m.matchCount < 2 {
			continue
		}
		// Map frame range → real ms via the sidecar timestamps. The end ms
		// uses the next frame's timestamp when available (so the cue covers
		// the time *up to* the next sample); otherwise it falls back to the
		// last matched frame's ms.
		startMs := targetMs[m.start]
		var endMs int64
		if m.end < len(targetMs) {
			endMs = targetMs[m.end]
		} else {
			endMs = targetMs[m.end-1]
		}
		out = append(out, pHashCandidate{startMs: startMs, endMs: endMs, matchCount: m.matchCount})
	}
	return out
}

// confirmAudioCandidatesChromaprint is the Chromaprint variant of
// confirmAudioCandidates — anchored on the same pHash trickplay
// candidates (so we never scan a "big portion" of the file: only the
// short window around each candidate).
//
// For each candidate window:
//  1. Compute the target's Chromaprint fingerprint on [start-padSec,
//     end+padSec].
//  2. For every sibling, compute the fingerprint on a wider search
//     window [start-searchSec, end+searchSec] and slide-match the
//     target into it. Similarity ≥ chromaprintSimThreshold counts.
//  3. Drift coherence (siblings clustering around the median offset)
//     enforces that the match is actually the same intro and not a
//     coincidence of similar audio between two unrelated moments.
//
// Returns confirmedWindow{} for accepted candidates. Empty result lets
// the caller fall through to confirmAudioCandidates (banded Pearson)
// — that guard handles the "fpcalc broken on this episode" edge case
// without losing detection coverage.
func (s *IntroDetectionService) confirmAudioCandidatesChromaprint(ctx context.Context, item *domainlibrary.MediaItem, result *pHashResult, cfg introConfig) []confirmedWindow {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.confirmAudioCandidatesChromaprint")
	defer span.End()

	mediaID := string(item.ID)
	if s.chromaprinter == nil || item.File == nil || item.File.Path == "" || len(result.siblings) == 0 {
		return nil
	}
	const (
		// Chromaprint frames are ~123ms each at 8 fps. AcoustID's "same
		// recording" cutoff is 0.85, but that's for exact takes (mp3
		// re-encodings of the same master). Cross-episode intro detection
		// is different: same musical content, slightly different mixing /
		// mastering / dub track per episode, so legitimate matches
		// observed live drop to ~0.55-0.70 even on aligned windows.
		// 0.65 keeps random-pair noise (~0.50) out while accepting real
		// recurring intros. Drift coherence + min ratio downstream filter
		// any false positive that slips through.
		chromaprintSimThreshold = 0.65
		framesPerSecond         = 8.0 // fpcalc default rate
	)

	confirmed := make([]confirmedWindow, 0, len(result.candidates))
	for _, cand := range result.candidates {
		startSec := float64(cand.startMs) / 1000
		endSec := float64(cand.endMs) / 1000

		tStart := startSec - cfg.PadSec
		if tStart < 0 {
			tStart = 0
		}
		tDur := endSec + cfg.PadSec - tStart

		targetFp, err := s.chromaprinter.Compute(ctx, item.File.Path, tStart, tDur)
		if err != nil || len(targetFp) == 0 {
			slog.Debug("intro_detection.candidate_evaluated",
				slog.String("media_id", mediaID),
				slog.Int64("candidate_start_ms", cand.startMs),
				slog.Int64("candidate_end_ms", cand.endMs),
				slog.String("audio_engine", "chromaprint"),
				slog.String("decision", "rejected_target_extract_failed"),
				slog.Any("err", err),
			)
			continue
		}

		drifts := make([]float64, 0, len(result.siblings))
		matched := 0
		extractFailed := 0
		for _, sib := range result.siblings {
			sStart := startSec - cfg.SearchSec
			if sStart < 0 {
				sStart = 0
			}
			sDur := endSec + cfg.SearchSec - sStart
			sibFp, err := s.chromaprinter.Compute(ctx, sib.File.Path, sStart, sDur)
			if err != nil || len(sibFp) == 0 {
				extractFailed++
				continue
			}
			offsetFrames, sim := slidingChromaprint(targetFp, sibFp)
			if sim < chromaprintSimThreshold {
				continue
			}
			matched++
			offsetSec := float64(offsetFrames) / framesPerSecond
			drift := offsetSec - (tStart - sStart)
			drifts = append(drifts, drift)
		}

		if len(drifts) < cfg.MinMatches {
			slog.Debug("intro_detection.candidate_evaluated",
				slog.String("media_id", mediaID),
				slog.Int64("candidate_start_ms", cand.startMs),
				slog.Int64("candidate_end_ms", cand.endMs),
				slog.Int("siblings_total", len(result.siblings)),
				slog.Int("siblings_matched", matched),
				slog.Int("siblings_extract_failed", extractFailed),
				slog.String("audio_engine", "chromaprint"),
				slog.String("decision", "rejected_low_matches"),
			)
			continue
		}
		median := medianFloat(drifts)
		coherent := 0
		for _, d := range drifts {
			if math.Abs(d-median) <= cfg.DriftTolSec {
				coherent++
			}
		}
		ratio := float64(coherent) / float64(len(result.siblings))
		minRatio := effectiveMinRatio(cfg, len(result.siblings))
		if ratio < minRatio {
			slog.Debug("intro_detection.candidate_evaluated",
				slog.String("media_id", mediaID),
				slog.Int64("candidate_start_ms", cand.startMs),
				slog.Int64("candidate_end_ms", cand.endMs),
				slog.Int("siblings_total", len(result.siblings)),
				slog.Int("siblings_matched", matched),
				slog.Float64("drift_median", median),
				slog.Any("drift_values", drifts),
				slog.Int("coherent_count", coherent),
				slog.Float64("ratio", ratio),
				slog.String("audio_engine", "chromaprint"),
				slog.String("decision", "rejected_low_ratio"),
			)
			continue
		}

		slog.Debug("intro_detection.candidate_evaluated",
			slog.String("media_id", mediaID),
			slog.Int64("candidate_start_ms", cand.startMs),
			slog.Int64("candidate_end_ms", cand.endMs),
			slog.Int("siblings_total", len(result.siblings)),
			slog.Int("siblings_matched", matched),
			slog.Float64("drift_median", median),
			slog.Any("drift_values", drifts),
			slog.Int("coherent_count", coherent),
			slog.Float64("ratio", ratio),
			slog.String("audio_engine", "chromaprint"),
			slog.String("decision", "accepted"),
		)
		confirmed = append(confirmed, confirmedWindow{
			startMs:    cand.startMs,
			endMs:      cand.endMs,
			matchCount: cand.matchCount,
			confidence: 0.97, // higher than banded Pearson 0.95 — Chromaprint signal is more reliable
		})
	}
	return confirmed
}

// slidingChromaprint slides target over reference and returns the best
// (offset_in_frames, similarity_in_[0,1]). similarity = 1 - hamming/
// (32 * len(target)).
//
// Inlined here (mirrors ffmpeg.SlidingBestMatch) to avoid pulling the
// adapter package into the app layer; the adapter exports the same
// algorithm under SlidingBestMatch for consumers that don't go through
// this service (e.g. intro-diag CLI).
func slidingChromaprint(target, reference []uint32) (offsetFrames int, similarity float64) {
	if len(target) == 0 || len(reference) < len(target) {
		return 0, 0
	}
	maxBits := 32 * len(target)
	bestOffset, bestBits := 0, maxBits
	for offset := 0; offset <= len(reference)-len(target); offset++ {
		var diffBits int
		for i := range target {
			diffBits += bits.OnesCount32(target[i] ^ reference[offset+i])
		}
		if diffBits < bestBits {
			bestBits = diffBits
			bestOffset = offset
		}
	}
	return bestOffset, 1.0 - float64(bestBits)/float64(maxBits)
}

// confirmAudioCandidates filters pHash candidates by audio cross-correlation
// against siblings.
//
// For each candidate, an audio fingerprint of the target file is extracted on
// [start-padSec, end+padSec], and for every sibling on a wider search window
// [start-searchSec, end+searchSec] (which absorbs intro position drift across
// episodes). A sliding Pearson correlation finds the best alignment and its
// drift relative to target.
//
// A candidate is CONFIRMED when ≥3 siblings show correlation ≥0.5 AND their
// drifts cluster within ±6s of the median drift — coincidental matches scatter
// drifts uniformly across the search range, while a true recurring intro pins
// them tightly. This is what distinguishes a real intro from a recurring
// landscape/transition shot that happens to share visual hashes.
func (s *IntroDetectionService) confirmAudioCandidates(ctx context.Context, item *domainlibrary.MediaItem, result *pHashResult, cfg introConfig) []confirmedWindow {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.confirmAudioCandidates")
	defer span.End()

	mediaID := string(item.ID)
	if item.File == nil || item.File.Path == "" || len(result.siblings) == 0 {
		slog.Debug("intro_detection.candidate_evaluated",
			slog.String("media_id", mediaID),
			slog.Int("siblings_total", len(result.siblings)),
			slog.String("decision", "rejected_no_target_or_siblings"),
		)
		return nil
	}

	confirmed := make([]confirmedWindow, 0, len(result.candidates))
	for _, cand := range result.candidates {
		startSec := float64(cand.startMs) / 1000
		endSec := float64(cand.endMs) / 1000

		tStart := startSec - cfg.PadSec
		if tStart < 0 {
			tStart = 0
		}
		tDur := endSec + cfg.PadSec - tStart

		targetFp, err := s.detector.AudioBandedFingerprint(ctx, item.File.Path, tStart, tDur, cfg.BinMs)
		if err != nil || len(targetFp) == 0 {
			slog.Debug("intro_detection.candidate_evaluated",
				slog.String("media_id", mediaID),
				slog.Int64("candidate_start_ms", cand.startMs),
				slog.Int64("candidate_end_ms", cand.endMs),
				slog.Int("match_count", cand.matchCount),
				slog.Int("siblings_total", len(result.siblings)),
				slog.String("decision", "rejected_target_extract_failed"),
				slog.Any("err", err),
			)
			continue
		}

		drifts := make([]float64, 0, len(result.siblings))
		correlated := 0
		extractFailed := 0
		for _, sib := range result.siblings {
			sStart := startSec - cfg.SearchSec
			if sStart < 0 {
				sStart = 0
			}
			sDur := endSec + cfg.SearchSec - sStart
			sibFp, err := s.detector.AudioBandedFingerprint(ctx, sib.File.Path, sStart, sDur, cfg.BinMs)
			if err != nil || len(sibFp) == 0 {
				extractFailed++
				continue
			}
			corr, offBins := ffmpegPkgSlidingBestPearsonBanded(targetFp, sibFp, cfg.NumBands)
			if corr < cfg.CorrThreshold {
				continue
			}
			correlated++
			offSec := float64(offBins*cfg.BinMs) / 1000
			drift := offSec - (tStart - sStart)
			drifts = append(drifts, drift)
		}

		if len(drifts) < cfg.MinMatches {
			slog.Debug("intro_detection.candidate_evaluated",
				slog.String("media_id", mediaID),
				slog.Int64("candidate_start_ms", cand.startMs),
				slog.Int64("candidate_end_ms", cand.endMs),
				slog.Int("match_count", cand.matchCount),
				slog.Int("siblings_total", len(result.siblings)),
				slog.Int("siblings_correlated", correlated),
				slog.Int("siblings_extract_failed", extractFailed),
				slog.String("decision", "rejected_low_matches"),
			)
			continue
		}
		median := medianFloat(drifts)
		coherent := 0
		for _, d := range drifts {
			if math.Abs(d-median) <= cfg.DriftTolSec {
				coherent++
			}
		}
		ratio := float64(coherent) / float64(len(result.siblings))
		minRatio := effectiveMinRatio(cfg, len(result.siblings))
		if ratio < minRatio {
			slog.Debug("intro_detection.candidate_evaluated",
				slog.String("media_id", mediaID),
				slog.Int64("candidate_start_ms", cand.startMs),
				slog.Int64("candidate_end_ms", cand.endMs),
				slog.Int("match_count", cand.matchCount),
				slog.Int("siblings_total", len(result.siblings)),
				slog.Int("siblings_correlated", correlated),
				slog.Int("siblings_extract_failed", extractFailed),
				slog.Float64("drift_median", median),
				slog.Any("drift_values", drifts),
				slog.Int("coherent_count", coherent),
				slog.Float64("ratio", ratio),
				slog.String("decision", "rejected_low_ratio"),
			)
			continue
		}

		slog.Debug("intro_detection.candidate_evaluated",
			slog.String("media_id", mediaID),
			slog.Int64("candidate_start_ms", cand.startMs),
			slog.Int64("candidate_end_ms", cand.endMs),
			slog.Int("match_count", cand.matchCount),
			slog.Int("siblings_total", len(result.siblings)),
			slog.Int("siblings_correlated", correlated),
			slog.Int("siblings_extract_failed", extractFailed),
			slog.Float64("drift_median", median),
			slog.Any("drift_values", drifts),
			slog.Int("coherent_count", coherent),
			slog.Float64("ratio", ratio),
			slog.String("decision", "accepted"),
		)
		confirmed = append(confirmed, confirmedWindow{
			startMs:    cand.startMs,
			endMs:      cand.endMs,
			matchCount: cand.matchCount,
			confidence: 0.95,
		})
	}
	return confirmed
}

// effectiveMinRatio returns MinRatio adapted to the sibling sample size.
// On large series (>=6 siblings) we keep cfg.MinRatio (default 0.7) to
// reject noisy false positives; on smaller seasons (Robin Hood with 5
// siblings, indie shows with 3-4) we drop to 0.6 since each sibling
// "vote" carries more relative weight and the strict 0.7 cuts out real
// matches with one outlier.
func effectiveMinRatio(cfg introConfig, siblingsTotal int) float64 {
	if siblingsTotal < 6 && cfg.MinRatio > 0.6 {
		return 0.6
	}
	return cfg.MinRatio
}

// medianFloat returns the median of vs (sorts a copy in place).
func medianFloat(vs []float64) float64 {
	n := len(vs)
	if n == 0 {
		return 0
	}
	cp := make([]float64, n)
	copy(cp, vs)
	sort.Float64s(cp)
	return cp[n/2]
}

// ffmpegPkgSlidingBestPearsonBanded indirects via the package-level helper so
// the service tests can stub it via the IntroDetector port. In practice it
// always resolves to ffmpeg.SlidingBestPearsonBanded.
var ffmpegPkgSlidingBestPearsonBanded = func(target, ref []float64, vec int) (float64, int) {
	return slidingBestPearsonBandedImpl(target, ref, vec)
}

// slidingBestPearsonBandedImpl is a copy of the adapter helper kept inline to
// avoid a circular dep between domain/app and the ffmpeg adapter (the service
// only depends on the IntroDetector port). Sliding Pearson over banded
// fingerprints with vectorSize floats per time bin.
func slidingBestPearsonBandedImpl(target, reference []float64, vectorSize int) (bestCorr float64, bestOffsetBins int) {
	if vectorSize <= 0 {
		return 0, 0
	}
	if len(target)%vectorSize != 0 || len(reference)%vectorSize != 0 {
		return 0, 0
	}
	if len(reference) < len(target) || len(target) == 0 {
		return 0, 0
	}
	maxOffBins := (len(reference) - len(target)) / vectorSize
	bestCorr = -1
	for offBins := 0; offBins <= maxOffBins; offBins++ {
		offFloats := offBins * vectorSize
		c := pearsonImpl(target, reference[offFloats:offFloats+len(target)])
		if c > bestCorr {
			bestCorr = c
			bestOffsetBins = offBins
		}
	}
	return bestCorr, bestOffsetBins
}

func pearsonImpl(a, b []float64) float64 {
	n := len(a)
	if n == 0 || n != len(b) {
		return 0
	}
	var sumA, sumB float64
	for i := 0; i < n; i++ {
		sumA += a[i]
		sumB += b[i]
	}
	meanA := sumA / float64(n)
	meanB := sumB / float64(n)
	var num, dA, dB float64
	for i := 0; i < n; i++ {
		da := a[i] - meanA
		db := b[i] - meanB
		num += da * db
		dA += da * da
		dB += db * db
	}
	if dA == 0 || dB == 0 {
		return 0
	}
	return num / math.Sqrt(dA*dB)
}

// waitForManifest polls for the trickplay manifest, waiting up to 24h.
func (s *IntroDetectionService) waitForManifest(ctx context.Context, mediaID shared.MediaItemID) (*domainlibrary.TrickplayManifest, error) {
	manifest, err := s.trickplayRepo.GetByMediaItemID(ctx, mediaID)
	if err == nil {
		return manifest, nil
	}
	if !errors.Is(err, shared.ErrNotFound) {
		return nil, fmt.Errorf("get trickplay manifest: %w", err)
	}

	deadline := time.Now().Add(24 * time.Hour)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return nil, errors.New("trickplay manifest not available after 24h")
			}
			m, e := s.trickplayRepo.GetByMediaItemID(ctx, mediaID)
			if e == nil {
				return m, nil
			}
			if !errors.Is(e, shared.ErrNotFound) {
				return nil, fmt.Errorf("get trickplay manifest: %w", e)
			}
		}
	}
}

// extractEpisodeHashesCached returns hashes/timestamps for a media item,
// short-circuiting via the fingerprint cache when the cached file_fingerprint
// matches the current MediaFile.Fingerprint and algo_version is current.
// Falls back to extractEpisodeHashes on cache miss / invalidation, and
// writes the FULL manifest hash sequence back when the cache is wired.
// Truncation to cutoffFrame is applied to the returned slice but the cache
// stores the unsliced version, so a later call with a larger cutoff still
// hits the cache (e.g. PropagateSegment uses manifestTotalFrames while
// pHashPass uses 25%).
//
// fileFingerprint can be empty (pre-fingerprint media files); in that case
// the cache is skipped (read AND write).
func (s *IntroDetectionService) extractEpisodeHashesCached(ctx context.Context, mediaItemID, fileFingerprint string, manifest *domainlibrary.TrickplayManifest, cutoffFrame int) ([]uint64, []int64, error) {
	truncate := func(h []uint64, t []int64) ([]uint64, []int64) {
		if cutoffFrame > 0 && cutoffFrame < len(h) {
			return h[:cutoffFrame], t[:cutoffFrame]
		}
		return h, t
	}
	if s.fpCache == nil || fileFingerprint == "" {
		return s.extractEpisodeHashes(manifest, cutoffFrame)
	}
	cached, err := s.fpCache.Get(ctx, mediaItemID)
	if err == nil && cached.FileFingerprint == fileFingerprint && cached.AlgoVersion == currentPHashAlgoVersion {
		h, t := truncate(cached.PhashTiles, cached.PhashTimestamps)
		return h, t, nil
	}
	// Miss / stale / version mismatch / read error: recompute the FULL
	// manifest (so a larger-cutoff caller later doesn't have to recompute)
	// and store before truncating for this caller.
	allHashes, allTimestamps, extractErr := s.extractEpisodeHashes(manifest, manifestTotalFrames(manifest))
	if extractErr != nil || len(allHashes) == 0 {
		return allHashes, allTimestamps, extractErr
	}
	if upsertErr := s.fpCache.Upsert(ctx, &domainlibrary.IntroFingerprintCacheEntry{
		MediaItemID:     mediaItemID,
		FileFingerprint: fileFingerprint,
		PhashTiles:      allHashes,
		PhashTimestamps: allTimestamps,
		AlgoVersion:     currentPHashAlgoVersion,
	}); upsertErr != nil {
		// Cache write failures are non-fatal: log and proceed with the freshly
		// computed hashes so the pipeline still produces a result.
		slog.Warn("intro_detection: cache upsert failed",
			slog.String("media_id", mediaItemID),
			slog.Any("err", upsertErr),
		)
	}
	h, t := truncate(allHashes, allTimestamps)
	return h, t, nil
}

// extractEpisodeAudioCached returns the Chromaprint fingerprint for a media item,
// reading from the audio fingerprint cache when available and valid.
//
// Cache is skipped (read and write) when:
//   - s.audioFPCache is nil, OR
//   - item.File is nil, OR
//   - item.File.Fingerprint is empty (pre-fingerprint media files).
//
// On cache miss or invalidation (FileFingerprint, AlgoVersion, or WindowSec
// mismatch) the fingerprint is recomputed via Chromaprint and the result is
// stored back. Upsert failures are non-fatal (slog.Warn); the freshly computed
// fingerprint is still returned so the pipeline produces a result.
func (s *IntroDetectionService) extractEpisodeAudioCached(ctx context.Context, item *domainlibrary.MediaItem, windowSec float64) ([]uint32, error) {
	if s.audioFPCache == nil || item.File == nil || item.File.Fingerprint == "" {
		return s.chromaprinter.Compute(ctx, item.File.Path, 0, windowSec)
	}
	cached, err := s.audioFPCache.Get(ctx, string(item.ID))
	if err == nil &&
		cached.FileFingerprint == item.File.Fingerprint &&
		cached.AlgoVersion == currentAudioAlgoVersion &&
		cached.WindowSec == int(windowSec) {
		return cached.Fingerprint, nil
	}
	fp, computeErr := s.chromaprinter.Compute(ctx, item.File.Path, 0, windowSec)
	if computeErr != nil || len(fp) == 0 {
		return fp, computeErr
	}
	if upsertErr := s.audioFPCache.Upsert(ctx, &domainlibrary.IntroAudioFingerprintCacheEntry{
		MediaItemID:     string(item.ID),
		FileFingerprint: item.File.Fingerprint,
		WindowSec:       int(windowSec),
		Fingerprint:     fp,
		AlgoVersion:     currentAudioAlgoVersion,
	}); upsertErr != nil {
		// Cache write failures are non-fatal: log and proceed with the freshly
		// computed fingerprint so the pipeline still produces a result.
		slog.Warn("intro_detection: audio cache upsert failed",
			slog.String("media_id", string(item.ID)),
			slog.Any("err", upsertErr),
		)
	}
	return fp, nil
}

// extractEpisodeHashes reads trickplay sprites for a manifest and returns the
// per-frame pHash plus the real per-frame timestamp (in ms) read from the
// sidecar file written at generation time. Both slices are aligned: index i in
// hashes corresponds to ms[i] in the source video.
//
// Returns an error if the sidecar is missing — there is no fallback to
// interval-based timing.
func (s *IntroDetectionService) extractEpisodeHashes(manifest *domainlibrary.TrickplayManifest, cutoffFrame int) ([]uint64, []int64, error) {
	tilesPerSprite := manifest.TileWidth * manifest.TileHeight
	if tilesPerSprite <= 0 {
		return nil, nil, nil
	}
	tileH := manifest.Height
	if tileH <= 0 {
		tileH = manifest.Width * 9 / 16
	}

	spriteDir := filepath.Join(s.trickplayCacheDir, string(manifest.MediaItemID))
	sidecar, err := readTrickplaySidecar(spriteDir)
	if err != nil {
		return nil, nil, fmt.Errorf("read trickplay sidecar for %s: %w", manifest.MediaItemID, err)
	}

	var hashes []uint64
	var ms []int64
	frameIdx := 0
	for spriteN := 1; ; spriteN++ {
		spritePath := filepath.Join(spriteDir, fmt.Sprintf("trickplay_%03d.webp", spriteN))
		tileHashes, err := s.detector.ExtractTileHashes(spritePath, manifest.TileWidth, manifest.TileHeight, manifest.Width, tileH)
		if err != nil {
			break
		}
		for _, h := range tileHashes {
			if frameIdx >= cutoffFrame || frameIdx >= len(sidecar.FrameTimestamps) {
				return hashes, ms, nil
			}
			hashes = append(hashes, h)
			ms = append(ms, sidecar.FrameTimestamps[frameIdx])
			frameIdx++
		}
	}
	return hashes, ms, nil
}

// frameCutoff returns the number of frames covering the first cutoffPercent of
// the episode (0.25 by default — searches the first quarter for the intro).
// If cutoffPercent is ≤ 0 or > 1, falls back to 0.25.
func frameCutoff(manifest *domainlibrary.TrickplayManifest, durationMs int64, cutoffPercent float64) int {
	if manifest.Interval <= 0 || durationMs <= 0 {
		return manifest.ImageCount * manifest.TileWidth * manifest.TileHeight
	}
	if cutoffPercent <= 0 || cutoffPercent > 1 {
		cutoffPercent = 0.25
	}
	totalFrames := int(durationMs/1000) / manifest.Interval
	return int(float64(totalFrames) * cutoffPercent)
}

// FrequencyMap builds a set of hashes that appear in at least threshold fraction of episodes.
// Exported for unit tests.
func FrequencyMap(episodeHashes [][]uint64, threshold float64) map[uint64]bool {
	if len(episodeHashes) == 0 {
		return nil
	}
	counts := make(map[uint64]int)
	for _, ep := range episodeHashes {
		seen := make(map[uint64]struct{})
		for _, h := range ep {
			if _, ok := seen[h]; !ok {
				seen[h] = struct{}{}
				counts[h]++
			}
		}
	}
	total := len(episodeHashes)
	result := make(map[uint64]bool)
	for h, cnt := range counts {
		if float64(cnt)/float64(total) >= threshold {
			result[h] = true
		}
	}
	return result
}

// hammingDist returns the number of differing bits between two 64-bit hashes.
func hammingDist(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}

// FrequencyMapHamming is like FrequencyMap but counts an episode as containing
// a candidate hash if any of its hashes is within maxHamming bits.
// Trickplay aHash at 1 frame/15s rarely has identical bytes across episodes
// (frame timing drift ±15s), so exact-match misses real intros — Hamming ≤8
// catches the same logo/title-card frame even when offset by a few seconds.
// Candidates come from the first episode's unique hashes.
func FrequencyMapHamming(episodeHashes [][]uint64, threshold float64, maxHamming int) map[uint64]bool {
	if len(episodeHashes) == 0 {
		return nil
	}
	if maxHamming <= 0 {
		return FrequencyMap(episodeHashes, threshold)
	}
	seen := make(map[uint64]struct{}, len(episodeHashes[0]))
	candidates := make([]uint64, 0, len(episodeHashes[0]))
	for _, h := range episodeHashes[0] {
		if _, ok := seen[h]; !ok {
			seen[h] = struct{}{}
			candidates = append(candidates, h)
		}
	}
	total := len(episodeHashes)
	result := make(map[uint64]bool)
	for _, c := range candidates {
		count := 0
		for _, ep := range episodeHashes {
			for _, h := range ep {
				if hammingDist(c, h) <= maxHamming {
					count++
					break
				}
			}
		}
		if float64(count)/float64(total) >= threshold {
			result[c] = true
		}
	}
	return result
}

// matchesIntroHashes returns true if h is within maxHamming bits of any introHash.
// Falls back to exact map lookup when maxHamming is 0.
func matchesIntroHashes(h uint64, introHashes map[uint64]bool, maxHamming int) bool {
	if introHashes[h] {
		return true
	}
	if maxHamming <= 0 {
		return false
	}
	for ih := range introHashes {
		if hammingDist(ih, h) <= maxHamming {
			return true
		}
	}
	return false
}

// concurrentFallback runs audio and subtitle-gap detection concurrently.
// First valid result wins; both are cancelled when one succeeds.
func (s *IntroDetectionService) concurrentFallback(ctx context.Context, item *domainlibrary.MediaItem) (startMs, endMs int64, source string, confidence float64, err error) {
	type result struct {
		startMs, endMs int64
		source         string
		confidence     float64
		err            error
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch := make(chan result, 2)

	// Each child carries its own recover: a parent-level recover does NOT
	// cover goroutines it spawned, and a panicking child that never sends
	// would stall the receive loop below until the caller's ctx expires. The
	// recovered child reports an error result instead, so the other detector
	// still gets its chance.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("intro: audio fallback panic recovered",
					slog.Any("err", r), slog.String("stack", string(debug.Stack())))
				ch <- result{err: fmt.Errorf("audio fallback panic: %v", r)}
			}
		}()
		s, e, found, ferr := s.detector.AudioFallback(cancelCtx, item.File.Path, 600)
		if ferr != nil || !found {
			ch <- result{err: ferr}
			return
		}
		ch <- result{startMs: s, endMs: e, source: "audio", confidence: 0.5}
	}()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("intro: subtitle-gap fallback panic recovered",
					slog.Any("err", r), slog.String("stack", string(debug.Stack())))
				ch <- result{err: fmt.Errorf("subtitle-gap fallback panic: %v", r)}
			}
		}()
		s, e, found := subtitleGapFallback(item)
		if !found {
			ch <- result{}
			return
		}
		ch <- result{startMs: s, endMs: e, source: "subtitle", confidence: 0.4}
	}()

	received := 0
	for received < 2 {
		select {
		case <-ctx.Done():
			return 0, 0, "", 0, ctx.Err()
		case r := <-ch:
			received++
			if r.endMs > r.startMs {
				cancel()
				return r.startMs, r.endMs, r.source, r.confidence, nil
			}
		}
	}
	return 0, 0, "", 0, nil
}

// subtitleGapFallback looks for a gap >30s in external subtitle data.
// Returns (0,0,false) when no subtitle info is available.
func subtitleGapFallback(item *domainlibrary.MediaItem) (startMs, endMs int64, found bool) {
	// Subtitle gap detection requires parsed SRT/ASS cues. Without a subtitle
	// repo dependency in this service, we return not-found. The audio fallback
	// path covers this case in practice.
	_ = item
	return 0, 0, false
}

// detectStudioLogo attempts to match the first ~90 seconds of trickplay frames
// against known studio logo fingerprints. Returns nil when no manifest is available,
// when no patterns match, or when the catalog has no fingerprints yet.
// Does NOT wait for a manifest — skips immediately if not present.
func (s *IntroDetectionService) detectStudioLogo(ctx context.Context, item *domainlibrary.MediaItem) *domainlibrary.MediaSegment {
	manifest, err := s.trickplayRepo.GetByMediaItemID(ctx, item.ID)
	if err != nil {
		return nil
	}
	if manifest.Interval <= 0 {
		return nil
	}

	// Extract only enough frames to cover 90 seconds.
	maxFrames := 90 / manifest.Interval
	if maxFrames < 1 {
		maxFrames = 1
	}
	hashes, _, hashErr := s.extractEpisodeHashes(manifest, maxFrames)
	if hashErr != nil || len(hashes) == 0 {
		return nil
	}

	startMs, endMs, found := s.detector.DetectStudioLogo(hashes, manifest.Interval)
	if !found {
		return nil
	}
	return &domainlibrary.MediaSegment{
		Type:       "studio_logo",
		StartMs:    startMs,
		EndMs:      endMs,
		Confidence: 0.7,
		Source:     "logo_match",
	}
}

// detectEndCredits runs the credits detector on the file and returns a
// MediaSegment if found. Caller is responsible for setting ID and MediaItemID
// before persisting.
func (s *IntroDetectionService) detectEndCredits(ctx context.Context, item *domainlibrary.MediaItem) (*domainlibrary.MediaSegment, error) {
	if item.File == nil || item.File.Path == "" {
		return nil, nil
	}
	durationSec := item.File.Duration.Seconds()
	if durationSec < 60 {
		return nil, nil
	}
	startMs, endMs, found, err := s.detector.DetectEndCredits(ctx, item.File.Path, durationSec)
	if err != nil {
		return nil, fmt.Errorf("detect end credits: %w", err)
	}
	if !found {
		return nil, nil
	}
	return &domainlibrary.MediaSegment{
		Type:       "end_credits",
		StartMs:    startMs,
		EndMs:      endMs,
		Confidence: 0.6,
		Source:     "blackdetect",
	}, nil
}
