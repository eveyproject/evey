package library

import (
	"context"
	"fmt"
	"log/slog"
	"math/bits"
	"path/filepath"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// Perceptual-hash (pHash) candidate extraction and matching for intro detection.

// matchWindow represents a contiguous time range of matching hashes.
type matchWindow struct {
	startMs, endMs int64
}

// pHashCandidate is a candidate intro/recap window in target ms (real frame
// timestamps from the trickplay sidecar - not interval×index, which lies when
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

// pHashPass performs the coarse localization via trickplay sprite frequency
// maps. Waits up to 24h for a trickplay manifest. Returns all candidate
// windows (gap-tolerant, ≥2 matched frames) along with the sibling MediaItems
// needed for the audio-confirmation stage.
func (s *IntroDetectionService) pHashPass(ctx context.Context, item *domainlibrary.MediaItem, cfg introConfig) (*pHashResult, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.pHashPass")
	defer span.End()

	mediaID := item.ID

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
	maxFrames := frameCutoff(manifest, durationMs, cfg.FrameCutoffPercent)

	// Batch the trickplay manifest read for the whole page instead of one query
	// per sibling - this loop hits the same N+1 as PropagateSegment's sibling
	// sweep above.
	candidateIDs := make([]shared.MediaItemID, 0, len(siblings))
	for _, sib := range siblings {
		if sib.ID == item.ID {
			continue
		}
		if sib.File == nil || sib.File.Path == "" {
			continue
		}
		candidateIDs = append(candidateIDs, sib.ID)
	}
	// A sibling absent from the map (no manifest, or the batch read failed
	// outright) is skipped below exactly like the previous per-sibling error
	// handling.
	trickByID, trickErr := s.trickplayRepo.GetByMediaItemIDs(ctx, candidateIDs)
	if trickErr != nil {
		slog.WarnContext(ctx, "intro_detection: pHashPass batch trickplay read failed, siblings without a cached manifest will be skipped",
			slog.Any("err", trickErr))
		trickByID = nil
	}

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
		sibManifest, ok := trickByID[sib.ID]
		if !ok {
			continue
		}
		sibFingerprint := ""
		if sib.File != nil {
			sibFingerprint = sib.File.Fingerprint
		}
		hashes, _, hashErr := s.extractEpisodeHashesCached(ctx, string(sib.ID), sibFingerprint, sibManifest, maxFrames)
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
	targetHashes, targetMs, hashErr := s.extractEpisodeHashesCached(ctx, string(item.ID), targetFingerprint, manifest, maxFrames)
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
	// Drop candidates that start past MaxIntroStartMs - those are not intros
	// but recurring in-episode visual elements (chapter cards, flashbacks,
	// previously-on segments) that happen to share visual hashes across
	// episodes. Without this filter we surface false positives like a
	// "Skip Intro" button at 9 minutes into an episode.
	if cfg.MaxIntroStartMs > 0 {
		filtered := candidates[:0]
		for _, c := range candidates {
			if c.startMs > cfg.MaxIntroStartMs {
				slog.DebugContext(ctx, "intro_detection.candidate_dropped_too_late",
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
			merged = append(merged, span(r))
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

// extractEpisodeHashesCached returns hashes/timestamps for a media item,
// short-circuiting via the fingerprint cache when the cached file_fingerprint
// matches the current MediaFile.Fingerprint and algo_version is current.
// Falls back to extractEpisodeHashes on cache miss / invalidation, and
// writes the FULL manifest hash sequence back when the cache is wired.
// Truncation to maxFrames is applied to the returned slice but the cache
// stores the unsliced version, so a later call with a larger cutoff still
// hits the cache (e.g. PropagateSegment uses manifestTotalFrames while
// pHashPass uses 25%).
//
// fileFingerprint can be empty (pre-fingerprint media files); in that case
// the cache is skipped (read AND write).
func (s *IntroDetectionService) extractEpisodeHashesCached(ctx context.Context, mediaItemID, fileFingerprint string, manifest *domainlibrary.TrickplayManifest, maxFrames int) ([]uint64, []int64, error) {
	truncate := func(h []uint64, t []int64) ([]uint64, []int64) {
		if maxFrames > 0 && maxFrames < len(h) {
			return h[:maxFrames], t[:maxFrames]
		}
		return h, t
	}
	if s.fpCache == nil || fileFingerprint == "" {
		return s.extractEpisodeHashes(manifest, maxFrames)
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
		slog.WarnContext(ctx, "intro_detection: cache upsert failed",
			slog.String("media_id", mediaItemID),
			slog.Any("err", upsertErr),
		)
	}
	h, t := truncate(allHashes, allTimestamps)
	return h, t, nil
}

// extractEpisodeHashes reads trickplay sprites for a manifest and returns the
// per-frame pHash plus the real per-frame timestamp (in ms) read from the
// sidecar file written at generation time. Both slices are aligned: index i in
// hashes corresponds to ms[i] in the source video.
//
// Returns an error if the sidecar is missing - there is no fallback to
// interval-based timing.
func (s *IntroDetectionService) extractEpisodeHashes(manifest *domainlibrary.TrickplayManifest, maxFrames int) ([]uint64, []int64, error) {
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
			break // no more sprite files: the collected prefix is the result
		}
		for _, h := range tileHashes {
			if frameIdx >= maxFrames || frameIdx >= len(sidecar.FrameTimestamps) {
				return hashes, ms, nil
			}
			hashes = append(hashes, h)
			ms = append(ms, sidecar.FrameTimestamps[frameIdx])
			frameIdx++
		}
	}
	return hashes, ms, nil //nolint:nilerr // a missing next sprite ends the walk; the prefix is the result
}

// frameCutoff returns the number of frames covering the first cutoffPercent of
// the episode (0.25 by default - searches the first quarter for the intro).
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
// (frame timing drift ±15s), so exact-match misses real intros - Hamming ≤8
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
