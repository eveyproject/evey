package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
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

// DetectIntro is the main entry point. Runs the two-pass pipeline and upserts a MediaSegment.
func (s *IntroDetectionService) DetectIntro(ctx context.Context, mediaItemID string) error {
	item, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil {
		return fmt.Errorf("get media item: %w", err)
	}
	if item.File == nil || item.File.Path == "" {
		return errors.New("media item has no file path")
	}

	startMs, endMs, source, confidence, err := s.runPipeline(ctx, item)
	if err != nil {
		return fmt.Errorf("detection pipeline: %w", err)
	}
	if startMs == 0 && endMs == 0 {
		slog.Info("intro_detection: no intro found", slog.String("media_id", mediaItemID))
		return nil
	}

	seg := &domainlibrary.MediaSegment{
		ID:          uuid.New().String(),
		MediaItemID: shared.MediaItemID(mediaItemID),
		Type:        "intro",
		StartMs:     startMs,
		EndMs:       endMs,
		Confidence:  confidence,
		Source:      source,
	}
	if err := s.segmentRepo.Upsert(ctx, seg); err != nil {
		return fmt.Errorf("upsert segment: %w", err)
	}
	slog.Info("intro_detection: segment stored",
		slog.String("media_id", mediaItemID),
		slog.Int64("start_ms", startMs),
		slog.Int64("end_ms", endMs),
		slog.String("source", source),
	)
	return nil
}

// runPipeline executes pass 1 (pHash) then pass 2 (precision FFmpeg).
// Returns (startMs, endMs, source, confidence, error). Returns (0,0,...) when no intro found.
func (s *IntroDetectionService) runPipeline(ctx context.Context, item *domainlibrary.MediaItem) (startMs, endMs int64, source string, confidence float64, err error) {
	mediaItemID := string(item.ID)

	if s.isTrickplayEnabled(ctx) {
		startApprox, endApprox, found, phashErr := s.pHashPass(ctx, item)
		if phashErr != nil {
			slog.Warn("intro_detection: phash pass failed, falling back",
				slog.String("media_id", mediaItemID),
				slog.Any("err", phashErr),
			)
		} else if found {
			// Pass 2: precision
			startSec := float64(startApprox) / 1000
			endSec := float64(endApprox) / 1000
			pStartMs, pEndMs, precErr := s.detector.PrecisionPass(ctx, item.File.Path, startSec, endSec, 15)
			if precErr != nil {
				slog.Warn("intro_detection: precision pass failed, using coarse window",
					slog.String("media_id", mediaItemID),
					slog.Any("err", precErr),
				)
				return startApprox, endApprox, "phash", 0.7, nil
			}
			return pStartMs, pEndMs, "phash", 0.9, nil
		}
	}

	// Fallback: concurrent audio/subtitle passes
	return s.concurrentFallback(ctx, item)
}

// pHashPass performs the coarse localization via trickplay sprite frequency maps.
// Waits up to 24h for a trickplay manifest if one doesn't exist yet.
func (s *IntroDetectionService) pHashPass(ctx context.Context, item *domainlibrary.MediaItem) (startMs, endMs int64, found bool, err error) {
	mediaID := shared.MediaItemID(item.ID)

	manifest, err := s.waitForManifest(ctx, mediaID)
	if err != nil {
		return 0, 0, false, err
	}

	episodeType := string(domainlibrary.Episode)
	siblings, listErr := s.mediaItems.List(ctx, domainlibrary.ListMediaItemsFilter{
		ParentID: &item.ParentID,
		Type:     &episodeType,
		Limit:    100,
	})
	if listErr != nil {
		return 0, 0, false, fmt.Errorf("list siblings: %w", listErr)
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
		return 0, 0, false, nil
	}

	introHashes := FrequencyMap(episodeHashes, 0.70)
	if len(introHashes) == 0 {
		return 0, 0, false, nil
	}

	// Find the matching frame window in the target episode's own hash sequence.
	targetHashes, hashErr := s.extractEpisodeHashes(manifest, cutoffFrame)
	if hashErr != nil || len(targetHashes) == 0 {
		return 0, 0, false, nil
	}

	startMs, endMs, found = longestContiguousWindow(manifest, introHashes, targetHashes)
	return startMs, endMs, found, nil
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
