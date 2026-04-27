package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	if len(segments) == 0 {
		slog.Info("intro_detection: no segments found", slog.String("media_id", mediaItemID))
		return nil
	}

	for _, seg := range segments {
		seg.ID = uuid.New().String()
		seg.MediaItemID = shared.MediaItemID(mediaItemID)
		if err := s.segmentRepo.Upsert(ctx, &seg); err != nil {
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

	introHashes := FrequencyMap(episodeHashes, 0.70)
	if len(introHashes) == 0 {
		return nil, nil
	}

	// Find matching frame windows in the target episode's own hash sequence.
	targetHashes, hashErr := s.extractEpisodeHashes(manifest, cutoffFrame)
	if hashErr != nil || len(targetHashes) == 0 {
		return nil, nil
	}

	return topTwoContiguousWindows(manifest, introHashes, targetHashes), nil
}

// topTwoContiguousWindows returns up to 2 non-overlapping match windows in target hashes,
// sorted by start position (chronological order). The two longest runs are selected.
func topTwoContiguousWindows(manifest *domainlibrary.TrickplayManifest, introHashes map[uint64]bool, targetHashes []uint64) []matchWindow {
	if manifest.Interval <= 0 || len(targetHashes) == 0 {
		return nil
	}
	intervalMs := int64(manifest.Interval * 1000)

	type run struct{ start, length int }
	var runs []run
	runStart, runLen := 0, 0
	for i, h := range targetHashes {
		if introHashes[h] {
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

	// Select top 2 by length, then restore chronological order.
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
