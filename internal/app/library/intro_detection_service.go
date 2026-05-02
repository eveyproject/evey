package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/bits"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
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

// IntroDetectionService orchestrates the two-pass intro detection pipeline.
type IntroDetectionService struct {
	segmentRepo       domainlibrary.MediaSegmentRepository
	trickplayRepo     domainlibrary.TrickplayRepository
	mediaItems        domainlibrary.MediaItemRepository
	configRepo        shared.ServerConfigRepository
	detector          IntroDetector
	trickplayCacheDir string
	shutdownCtx       context.Context
	events            shared.EventBus
}

func NewIntroDetectionService(
	segmentRepo domainlibrary.MediaSegmentRepository,
	trickplayRepo domainlibrary.TrickplayRepository,
	mediaItems domainlibrary.MediaItemRepository,
	configRepo shared.ServerConfigRepository,
	detector IntroDetector,
	cacheDir string,
) *IntroDetectionService {
	return &IntroDetectionService{
		segmentRepo:       segmentRepo,
		trickplayRepo:     trickplayRepo,
		mediaItems:        mediaItems,
		configRepo:        configRepo,
		detector:          detector,
		trickplayCacheDir: cacheDir,
	}
}

func (s *IntroDetectionService) WithShutdownCtx(ctx context.Context) {
	s.shutdownCtx = ctx
}

func (s *IntroDetectionService) WithEventBus(bus shared.EventBus) {
	s.events = bus
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

// detectSeason runs detection on every episode of the given season that
// doesn't already have an intro segment (presence of end_credits alone is not
// sufficient — partial detection happens when not all siblings had their
// trickplay ready, and we must re-run as more become available).
//
// Skips episodes whose existing intro is locked by an operator (manual
// override) since `upsertSegmentRespectingManual` would no-op anyway and we
// don't want to waste the audio extraction.
func (s *IntroDetectionService) detectSeason(ctx context.Context, parentID string) {
	episodeType := string(domainlibrary.Episode)
	siblings, err := s.mediaItems.List(ctx, domainlibrary.ListMediaItemsFilter{
		ParentID: &parentID,
		Type:     &episodeType,
		Limit:    100,
	})
	if err != nil {
		slog.Warn("intro detection: list season episodes", slog.String("parent_id", parentID), slog.Any("err", err))
		return
	}
	for _, ep := range siblings {
		if ctx.Err() != nil {
			return
		}
		existing, _ := s.segmentRepo.GetByMediaItemID(ctx, ep.ID)
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
			// A trickplay+audio confirmed phash intro is "good".
			if sg.Source == "phash" && sg.EndMs-sg.StartMs >= 5_000 {
				hasGoodIntro = true
			}
		}
		if manualLocked || hasGoodIntro {
			continue
		}
		epCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		if err := s.DetectIntro(epCtx, string(ep.ID)); err != nil {
			slog.Warn("intro detection failed", slog.String("media_id", string(ep.ID)), slog.Any("err", err))
		}
		cancel()
	}
}

// DetectIntro is the main entry point. Runs the two-pass pipeline and upserts all detected segments.
func (s *IntroDetectionService) DetectIntro(ctx context.Context, mediaItemID string) error {
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
	}
	return nil
}

// upsertSegmentRespectingManual upserts unless an existing segment for the same
// (media_item_id, type) has source "manual" or "manual_propagated" — operator
// choices must never be silently overwritten by re-runs of auto-detection.
func (s *IntroDetectionService) upsertSegmentRespectingManual(ctx context.Context, seg *domainlibrary.MediaSegment) error {
	existing, err := s.segmentRepo.GetByMediaItemID(ctx, seg.MediaItemID)
	if err == nil {
		for _, e := range existing {
			if e.Type == seg.Type && (e.Source == "manual" || e.Source == "manual_propagated") {
				slog.Info("intro_detection: skipping auto upsert, manual lock present",
					slog.String("media_id", string(seg.MediaItemID)),
					slog.String("type", seg.Type),
					slog.String("existing_source", e.Source))
				return nil
			}
		}
	}
	return s.segmentRepo.Upsert(ctx, seg)
}

// SetManualSegment upserts a single segment with source="manual" and confidence=1.0.
// This is the operator-driven override path. The (media_item_id, type) DB unique
// constraint makes this idempotent: posting the same type updates start/end.
func (s *IntroDetectionService) SetManualSegment(ctx context.Context, mediaItemID, segType string, startMs, endMs int64) error {
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
	refAllHashes, refAllMs, err := s.extractEpisodeHashes(refManifest, manifestTotalFrames(refManifest))
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
	siblings, err := s.mediaItems.List(ctx, domainlibrary.ListMediaItemsFilter{
		ParentID: &refItem.ParentID,
		Type:     &episodeType,
		Limit:    100,
	})
	if err != nil {
		return nil, fmt.Errorf("list siblings: %w", err)
	}

	const maxHamming = 8
	for _, sib := range siblings {
		if sib.ID == refItem.ID {
			continue
		}
		sibSegs, _ := s.segmentRepo.GetByMediaItemID(ctx, sib.ID)
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
		sibHashes, sibMs, hashErr := s.extractEpisodeHashes(sibManifest, manifestTotalFrames(sibManifest))
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
		if upsertErr := s.segmentRepo.Upsert(ctx, seg); upsertErr != nil {
			slog.Warn("propagate: upsert failed",
				slog.String("sibling", string(sib.ID)),
				slog.Any("err", upsertErr))
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
	mediaItemID := string(item.ID)

	if s.isTrickplayEnabled(ctx) {
		result, phashErr := s.pHashPass(ctx, item)
		if phashErr != nil {
			slog.Warn("intro_detection: phash pass failed, falling back",
				slog.String("media_id", mediaItemID),
				slog.Any("err", phashErr),
			)
		} else if result != nil && len(result.candidates) > 0 {
			confirmed := s.confirmAudioCandidates(ctx, item, result)
			if len(confirmed) > 0 {
				// Boundaries from the trickplay+audio stage are already
				// trustworthy (real keyframe ms via showinfo + audio cross-
				// correlation). PrecisionPass used to refine via silencedetect
				// but it picks the *first* silence/black in the input window,
				// which collapses to ~1s when an episode opens with a black
				// studio logo. Skip it.
				return classifyConfirmed(confirmed), nil
			}
			slog.Info("intro_detection.pipeline_decision",
				slog.String("media_id", mediaItemID),
				slog.Int("candidates_evaluated", len(result.candidates)),
				slog.Int("siblings_total", len(result.siblings)),
				slog.String("decision", "fallback_no_candidate_confirmed"),
			)
		}
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
func classifyConfirmed(windows []confirmedWindow) []domainlibrary.MediaSegment {
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
	// Look for an earlier short window (<60s) ending before intro start: that's
	// the recap (a brief "previously on…" before the title sequence).
	const recapMaxMs = 60 * 1000
	for _, w := range windows {
		if w.endMs > intro.startMs {
			continue
		}
		if (w.endMs - w.startMs) >= recapMaxMs {
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
func (s *IntroDetectionService) pHashPass(ctx context.Context, item *domainlibrary.MediaItem) (*pHashResult, error) {
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
	cutoffFrame := frameCutoff(manifest, durationMs)

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
		hashes, _, hashErr := s.extractEpisodeHashes(sibManifest, cutoffFrame)
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
	targetHashes, targetMs, hashErr := s.extractEpisodeHashes(manifest, cutoffFrame)
	if hashErr != nil || len(targetHashes) == 0 {
		return nil, nil
	}

	// FrequencyMap built from target + siblings. At 15s sampling, real intro
	// frames often only land in 50-60% of siblings (sampling phase varies
	// per-episode, fragments the match). We keep the threshold loose here and
	// rely on audio confirmation downstream to reject false positives that
	// pass through.
	allEps := append([][]uint64{targetHashes}, episodeHashes...)
	const maxHamming = 8
	introHashes := FrequencyMapHamming(allEps, 0.50, maxHamming)
	if len(introHashes) == 0 {
		return nil, nil
	}

	candidates := allCandidatesGapTolerant(introHashes, targetHashes, targetMs, maxHamming)
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
func (s *IntroDetectionService) confirmAudioCandidates(ctx context.Context, item *domainlibrary.MediaItem, result *pHashResult) []confirmedWindow {
	mediaID := string(item.ID)
	if item.File == nil || item.File.Path == "" || len(result.siblings) == 0 {
		slog.Debug("intro_detection.candidate_evaluated",
			slog.String("media_id", mediaID),
			slog.Int("siblings_total", len(result.siblings)),
			slog.String("decision", "rejected_no_target_or_siblings"),
		)
		return nil
	}

	const (
		padSec        = 10.0
		searchSec     = 30.0
		binMs         = 250
		numBands      = 12
		corrThreshold = 0.45 // permissive enough to absorb extraction jitter; drift coherence does the real filtering
		driftTolSec   = 6.0
		minMatches    = 3
		minRatio      = 0.6
	)

	confirmed := make([]confirmedWindow, 0, len(result.candidates))
	for _, cand := range result.candidates {
		startSec := float64(cand.startMs) / 1000
		endSec := float64(cand.endMs) / 1000

		tStart := startSec - padSec
		if tStart < 0 {
			tStart = 0
		}
		tDur := endSec + padSec - tStart

		targetFp, err := s.detector.AudioBandedFingerprint(ctx, item.File.Path, tStart, tDur, binMs)
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
			sStart := startSec - searchSec
			if sStart < 0 {
				sStart = 0
			}
			sDur := endSec + searchSec - sStart
			sibFp, err := s.detector.AudioBandedFingerprint(ctx, sib.File.Path, sStart, sDur, binMs)
			if err != nil || len(sibFp) == 0 {
				extractFailed++
				continue
			}
			corr, offBins := ffmpegPkgSlidingBestPearsonBanded(targetFp, sibFp, numBands)
			if corr < corrThreshold {
				continue
			}
			correlated++
			offSec := float64(offBins*binMs) / 1000
			drift := offSec - (tStart - sStart)
			drifts = append(drifts, drift)
		}

		if len(drifts) < minMatches {
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
			if math.Abs(d-median) <= driftTolSec {
				coherent++
			}
		}
		ratio := float64(coherent) / float64(len(result.siblings))
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

// frameCutoff returns the number of frames covering the first 25% of the episode.
func frameCutoff(manifest *domainlibrary.TrickplayManifest, durationMs int64) int {
	if manifest.Interval <= 0 || durationMs <= 0 {
		return manifest.ImageCount * manifest.TileWidth * manifest.TileHeight
	}
	totalFrames := int(durationMs/1000) / manifest.Interval
	return totalFrames / 4
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

	go func() {
		s, e, found, ferr := s.detector.AudioFallback(cancelCtx, item.File.Path, 600)
		if ferr != nil || !found {
			ch <- result{err: ferr}
			return
		}
		ch <- result{startMs: s, endMs: e, source: "audio", confidence: 0.5}
	}()

	go func() {
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
