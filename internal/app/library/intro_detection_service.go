package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

// StartWorker subscribes to media.item.added and runs detection on a full
// season once the stream of additions to the same parent stabilises (30s debounce).
func (s *IntroDetectionService) StartWorker(ctx context.Context) {
	if s.events == nil {
		return
	}
	var mu sync.Mutex
	timers := make(map[string]*time.Timer)

	s.events.Subscribe("media.item.added", func(_ context.Context, ev shared.Event) {
		e, ok := ev.(domainlibrary.MediaItemAdded)
		if !ok || e.Type != string(domainlibrary.Episode) {
			return
		}
		item, err := s.mediaItems.GetByID(ctx, e.MediaItemID)
		if err != nil || item.ParentID == "" {
			return
		}
		parentID := item.ParentID

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
	})
}

// detectSeason runs detection on every episode of the given season that has no segments yet.
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
		existing, err := s.segmentRepo.GetByMediaItemID(ctx, ep.ID)
		if err == nil && len(existing) > 0 {
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
	refAllHashes, err := s.extractEpisodeHashes(refManifest, manifestTotalFrames(refManifest))
	if err != nil || len(refAllHashes) == 0 {
		return nil, fmt.Errorf("extract reference hashes: %w", err)
	}
	intervalMs := int64(refManifest.Interval) * 1000
	if intervalMs <= 0 {
		return nil, errors.New("reference manifest has invalid interval")
	}
	startIdx := refSeg.StartMs / intervalMs
	endIdx := refSeg.EndMs / intervalMs
	if endIdx > int64(len(refAllHashes)) {
		endIdx = int64(len(refAllHashes))
	}
	if startIdx < 0 || startIdx >= endIdx {
		return nil, errors.New("reference segment out of trickplay range")
	}
	introHashes := make(map[uint64]bool)
	for _, h := range refAllHashes[startIdx:endIdx] {
		introHashes[h] = true
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
		sibHashes, hashErr := s.extractEpisodeHashes(sibManifest, manifestTotalFrames(sibManifest))
		if hashErr != nil || len(sibHashes) == 0 {
			continue
		}

		windows := topTwoContiguousWindowsHamming(sibManifest, introHashes, sibHashes, maxHamming)
		if len(windows) == 0 {
			continue
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

// runPipeline executes pass 1 (pHash) then pass 2 (precision FFmpeg).
// Returns 0, 1, or 2 segments (intro, possibly with a recap).
func (s *IntroDetectionService) runPipeline(ctx context.Context, item *domainlibrary.MediaItem) ([]domainlibrary.MediaSegment, error) {
	mediaItemID := string(item.ID)

	if s.isTrickplayEnabled(ctx) {
		windows, phashErr := s.pHashPass(ctx, item)
		if phashErr != nil {
			slog.Warn("intro_detection: phash pass failed, falling back",
				slog.String("media_id", mediaItemID),
				slog.Any("err", phashErr),
			)
		} else if len(windows) > 0 {
			segments := classifySegments(windows)
			// Run precision pass on the intro window only.
			for i, seg := range segments {
				if seg.Type != "intro" {
					continue
				}
				startSec := float64(seg.StartMs) / 1000
				endSec := float64(seg.EndMs) / 1000
				pStartMs, pEndMs, precErr := s.detector.PrecisionPass(ctx, item.File.Path, startSec, endSec, 15)
				if precErr != nil {
					slog.Warn("intro_detection: precision pass failed, using coarse window",
						slog.String("media_id", mediaItemID),
						slog.Any("err", precErr),
					)
					seg.Confidence = 0.7
				} else {
					seg.StartMs = pStartMs
					seg.EndMs = pEndMs
					seg.Confidence = 0.9
				}
				seg.Source = "phash"
				segments[i] = seg
			}
			for i, seg := range segments {
				if seg.Type == "recap" {
					seg.Source = "phash"
					seg.Confidence = 0.7
					segments[i] = seg
				}
			}
			return segments, nil
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

// classifySegments converts up to 2 match windows into typed MediaSegments.
// If 2 windows and the earlier one is < 60s → earlier=recap, later=intro.
// Otherwise only the longest window is used as intro.
// Exported for unit tests.
func classifySegments(windows []matchWindow) []domainlibrary.MediaSegment {
	switch len(windows) {
	case 0:
		return nil
	case 1:
		return []domainlibrary.MediaSegment{{
			Type:    "intro",
			StartMs: windows[0].startMs,
			EndMs:   windows[0].endMs,
		}}
	default:
		earlier, later := windows[0], windows[1]
		earlyDur := earlier.endMs - earlier.startMs
		const recapMaxMs = 60 * 1000
		if earlyDur < recapMaxMs {
			return []domainlibrary.MediaSegment{
				{Type: "recap", StartMs: earlier.startMs, EndMs: earlier.endMs},
				{Type: "intro", StartMs: later.startMs, EndMs: later.endMs},
			}
		}
		// Earlier window ≥ 60s: unclear which is the real intro — use only the larger one.
		laterDur := later.endMs - later.startMs
		if earlyDur >= laterDur {
			return []domainlibrary.MediaSegment{{
				Type:    "intro",
				StartMs: earlier.startMs,
				EndMs:   earlier.endMs,
			}}
		}
		return []domainlibrary.MediaSegment{{
			Type:    "intro",
			StartMs: later.startMs,
			EndMs:   later.endMs,
		}}
	}
}

// pHashPass performs the coarse localization via trickplay sprite frequency maps.
// Waits up to 24h for a trickplay manifest if one doesn't exist yet.
// Returns up to 2 non-overlapping match windows in chronological order.
func (s *IntroDetectionService) pHashPass(ctx context.Context, item *domainlibrary.MediaItem) ([]matchWindow, error) {
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

	// Collect hashes for all siblings (including the target episode).
	episodeHashes := make([][]uint64, 0, len(siblings))
	for _, sib := range siblings {
		sibManifest, sibErr := s.trickplayRepo.GetByMediaItemID(ctx, sib.ID)
		if sibErr != nil {
			continue
		}
		hashes, hashErr := s.extractEpisodeHashes(sibManifest, cutoffFrame)
		if hashErr != nil {
			continue
		}
		if len(hashes) > 0 {
			episodeHashes = append(episodeHashes, hashes)
		}
	}

	if len(episodeHashes) < 2 {
		return nil, nil
	}

	const maxHamming = 8
	introHashes := FrequencyMapHamming(episodeHashes, 0.70, maxHamming)
	if len(introHashes) == 0 {
		return nil, nil
	}

	// Find matching frame windows in the target episode's own hash sequence.
	targetHashes, hashErr := s.extractEpisodeHashes(manifest, cutoffFrame)
	if hashErr != nil || len(targetHashes) == 0 {
		return nil, nil
	}

	return topTwoContiguousWindowsHamming(manifest, introHashes, targetHashes, maxHamming), nil
}

// topTwoContiguousWindows returns up to 2 non-overlapping match windows in target hashes,
// sorted by start position (chronological order). The two longest runs are selected.
func topTwoContiguousWindows(manifest *domainlibrary.TrickplayManifest, introHashes map[uint64]bool, targetHashes []uint64) []matchWindow {
	return topTwoContiguousWindowsHamming(manifest, introHashes, targetHashes, 0)
}

// topTwoContiguousWindowsHamming is like topTwoContiguousWindows but matches each
// target hash within maxHamming bits of any introHash (0 = exact match).
func topTwoContiguousWindowsHamming(manifest *domainlibrary.TrickplayManifest, introHashes map[uint64]bool, targetHashes []uint64, maxHamming int) []matchWindow {
	if manifest.Interval <= 0 || len(targetHashes) == 0 {
		return nil
	}
	intervalMs := int64(manifest.Interval * 1000)

	type run struct{ start, length int }
	var runs []run
	runStart, runLen := 0, 0
	for i, h := range targetHashes {
		if matchesIntroHashes(h, introHashes, maxHamming) {
			if runLen == 0 {
				runStart = i
			}
			runLen++
		} else {
			if runLen > 0 {
				runs = append(runs, run{runStart, runLen})
			}
			runLen = 0
		}
	}
	if runLen > 0 {
		runs = append(runs, run{runStart, runLen})
	}
	if len(runs) == 0 {
		return nil
	}

	sort.Slice(runs, func(i, j int) bool { return runs[i].length > runs[j].length })
	if len(runs) > 2 {
		runs = runs[:2]
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].start < runs[j].start })

	out := make([]matchWindow, 0, len(runs))
	for _, r := range runs {
		out = append(out, matchWindow{
			startMs: int64(r.start) * intervalMs,
			endMs:   int64(r.start+r.length) * intervalMs,
		})
	}
	return out
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

// extractEpisodeHashes reads trickplay sprites for a manifest and returns
// hashes for frames in the first cutoffFrame frames only.
func (s *IntroDetectionService) extractEpisodeHashes(manifest *domainlibrary.TrickplayManifest, cutoffFrame int) ([]uint64, error) {
	tilesPerSprite := manifest.TileWidth * manifest.TileHeight
	if tilesPerSprite <= 0 {
		return nil, nil
	}
	tileH := manifest.Height
	if tileH <= 0 {
		tileH = manifest.Width * 9 / 16
	}

	var hashes []uint64
	spriteDir := filepath.Join(s.trickplayCacheDir, string(manifest.MediaItemID))
	frameIdx := 0
	for spriteN := 1; ; spriteN++ {
		spritePath := filepath.Join(spriteDir, fmt.Sprintf("trickplay_%03d.webp", spriteN))
		tileHashes, err := s.detector.ExtractTileHashes(spritePath, manifest.TileWidth, manifest.TileHeight, manifest.Width, tileH)
		if err != nil {
			break
		}
		for _, h := range tileHashes {
			if frameIdx >= cutoffFrame {
				return hashes, nil
			}
			hashes = append(hashes, h)
			frameIdx++
		}
	}
	return hashes, nil
}

// frameCutoff returns the number of frames covering the first 25% of the episode.
func frameCutoff(manifest *domainlibrary.TrickplayManifest, durationMs int64) int {
	if manifest.Interval <= 0 || durationMs <= 0 {
		return manifest.ImageCount * manifest.TileWidth * manifest.TileHeight
	}
	totalFrames := int(durationMs/1000) / manifest.Interval
	return totalFrames / 4
}

// longestContiguousWindow finds the longest contiguous run of frames in targetHashes
// that are all present in introHashes, and returns the time window in ms.
func longestContiguousWindow(manifest *domainlibrary.TrickplayManifest, introHashes map[uint64]bool, targetHashes []uint64) (startMs, endMs int64, found bool) {
	if manifest.Interval <= 0 || len(targetHashes) == 0 {
		return 0, 0, false
	}
	intervalMs := int64(manifest.Interval * 1000)

	bestStart, bestLen := 0, 0
	runStart, runLen := 0, 0
	for i, h := range targetHashes {
		if introHashes[h] {
			if runLen == 0 {
				runStart = i
			}
			runLen++
			if runLen > bestLen {
				bestLen = runLen
				bestStart = runStart
			}
		} else {
			runLen = 0
		}
	}
	if bestLen == 0 {
		return 0, 0, false
	}
	return int64(bestStart) * intervalMs, int64(bestStart+bestLen) * intervalMs, true
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
	hashes, hashErr := s.extractEpisodeHashes(manifest, maxFrames)
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
