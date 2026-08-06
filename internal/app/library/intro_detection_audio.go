package library

import (
	"context"
	"log/slog"
	"math"
	"math/bits"
	"sort"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// Chromaprint/audio-based confirmation of pHash intro candidates.

// confirmAudioCandidatesChromaprint is the Chromaprint variant of
// confirmAudioCandidates - anchored on the same pHash trickplay
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
// - that guard handles the "fpcalc broken on this episode" edge case
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
		// recurring intros. Drift coherence + min coherenceRatio downstream filter
		// any false positive that slips through.
		chromaprintSimThreshold = 0.65
		framesPerSecond         = 8.0 // fpcalc default rate
	)

	confirmed := make([]confirmedWindow, 0, len(result.candidates))
	for _, cand := range result.candidates {
		startSec := float64(cand.startMs) / 1000
		endSec := float64(cand.endMs) / 1000

		targetStartSec := startSec - cfg.PadSec
		if targetStartSec < 0 {
			targetStartSec = 0
		}
		targetDurSec := endSec + cfg.PadSec - targetStartSec

		targetFp, err := s.chromaprinter.Compute(ctx, item.File.Path, targetStartSec, targetDurSec)
		if err != nil || len(targetFp) == 0 {
			slog.DebugContext(ctx, "intro_detection.candidate_evaluated",
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
			sibStartSec := startSec - cfg.SearchSec
			if sibStartSec < 0 {
				sibStartSec = 0
			}
			sibDurSec := endSec + cfg.SearchSec - sibStartSec
			sibFp, err := s.chromaprinter.Compute(ctx, sib.File.Path, sibStartSec, sibDurSec)
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
			// Remove the window-start offset (target and sibling are extracted on
			// differently-shifted windows) to get the intro's real drift.
			drift := offsetSec - (targetStartSec - sibStartSec)
			drifts = append(drifts, drift)
		}

		if len(drifts) < cfg.MinMatches {
			slog.DebugContext(ctx, "intro_detection.candidate_evaluated",
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
		coherenceRatio := float64(coherent) / float64(len(result.siblings))
		minRatio := effectiveMinRatio(cfg, len(result.siblings))
		if coherenceRatio < minRatio {
			slog.DebugContext(ctx, "intro_detection.candidate_evaluated",
				slog.String("media_id", mediaID),
				slog.Int64("candidate_start_ms", cand.startMs),
				slog.Int64("candidate_end_ms", cand.endMs),
				slog.Int("siblings_total", len(result.siblings)),
				slog.Int("siblings_matched", matched),
				slog.Float64("drift_median", median),
				slog.Any("drift_values", drifts),
				slog.Int("coherent_count", coherent),
				slog.Float64("coherenceRatio", coherenceRatio),
				slog.String("audio_engine", "chromaprint"),
				slog.String("decision", "rejected_low_ratio"),
			)
			continue
		}

		slog.DebugContext(ctx, "intro_detection.candidate_evaluated",
			slog.String("media_id", mediaID),
			slog.Int64("candidate_start_ms", cand.startMs),
			slog.Int64("candidate_end_ms", cand.endMs),
			slog.Int("siblings_total", len(result.siblings)),
			slog.Int("siblings_matched", matched),
			slog.Float64("drift_median", median),
			slog.Any("drift_values", drifts),
			slog.Int("coherent_count", coherent),
			slog.Float64("coherenceRatio", coherenceRatio),
			slog.String("audio_engine", "chromaprint"),
			slog.String("decision", "accepted"),
		)
		confirmed = append(confirmed, confirmedWindow{
			startMs:    cand.startMs,
			endMs:      cand.endMs,
			matchCount: cand.matchCount,
			confidence: 0.97, // higher than banded Pearson 0.95 - Chromaprint signal is more reliable
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
	const bitsPerSubfingerprint = 32 // each Chromaprint frame is a uint32 (bits.OnesCount32 below)
	maxBits := bitsPerSubfingerprint * len(target)
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
// drifts cluster within ±6s of the median drift - coincidental matches scatter
// drifts uniformly across the search range, while a true recurring intro pins
// them tightly. This is what distinguishes a real intro from a recurring
// landscape/transition shot that happens to share visual hashes.
func (s *IntroDetectionService) confirmAudioCandidates(ctx context.Context, item *domainlibrary.MediaItem, result *pHashResult, cfg introConfig) []confirmedWindow {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.confirmAudioCandidates")
	defer span.End()

	mediaID := string(item.ID)
	if item.File == nil || item.File.Path == "" || len(result.siblings) == 0 {
		slog.DebugContext(ctx, "intro_detection.candidate_evaluated",
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

		targetStartSec := startSec - cfg.PadSec
		if targetStartSec < 0 {
			targetStartSec = 0
		}
		targetDurSec := endSec + cfg.PadSec - targetStartSec

		targetFp, err := s.detector.AudioBandedFingerprint(ctx, item.File.Path, targetStartSec, targetDurSec, cfg.BinMs)
		if err != nil || len(targetFp) == 0 {
			slog.DebugContext(ctx, "intro_detection.candidate_evaluated",
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
			sibStartSec := startSec - cfg.SearchSec
			if sibStartSec < 0 {
				sibStartSec = 0
			}
			sibDurSec := endSec + cfg.SearchSec - sibStartSec
			sibFp, err := s.detector.AudioBandedFingerprint(ctx, sib.File.Path, sibStartSec, sibDurSec, cfg.BinMs)
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
			// Remove the window-start offset (target and sibling are extracted on
			// differently-shifted windows) to get the intro's real drift.
			drift := offSec - (targetStartSec - sibStartSec)
			drifts = append(drifts, drift)
		}

		if len(drifts) < cfg.MinMatches {
			slog.DebugContext(ctx, "intro_detection.candidate_evaluated",
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
		coherenceRatio := float64(coherent) / float64(len(result.siblings))
		minRatio := effectiveMinRatio(cfg, len(result.siblings))
		if coherenceRatio < minRatio {
			slog.DebugContext(ctx, "intro_detection.candidate_evaluated",
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
				slog.Float64("coherenceRatio", coherenceRatio),
				slog.String("decision", "rejected_low_ratio"),
			)
			continue
		}

		slog.DebugContext(ctx, "intro_detection.candidate_evaluated",
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
			slog.Float64("coherenceRatio", coherenceRatio),
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

// ffmpegPkgSlidingBestPearsonBanded indirects via a package-level var so the
// service tests can stub it. The indirection exists for test stubs; the real
// implementation is the local anti-cycle copy below (slidingBestPearsonBandedImpl),
// not ffmpeg.SlidingBestPearsonBanded.
var ffmpegPkgSlidingBestPearsonBanded = slidingBestPearsonBandedImpl

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
	for i := range n {
		sumA += a[i]
		sumB += b[i]
	}
	meanA := sumA / float64(n)
	meanB := sumB / float64(n)
	var num, dA, dB float64
	for i := range n {
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
// prefetchAudioFingerprints batch-reads the audio fingerprint cache for many
// episodes at once and returns only the entries still VALID for the current
// file/algo/window (same invalidation contract as extractEpisodeAudioCached).
// Misses and stale entries are simply absent - the caller falls back to the
// per-episode compute path for those.
func (s *IntroDetectionService) prefetchAudioFingerprints(ctx context.Context, items []*domainlibrary.MediaItem, windowSec float64) map[string][]uint32 {
	out := make(map[string][]uint32, len(items))
	if s.audioFPCache == nil || len(items) == 0 {
		return out
	}
	ids := make([]string, 0, len(items))
	byID := make(map[string]*domainlibrary.MediaItem, len(items))
	for _, it := range items {
		if it.File == nil || it.File.Fingerprint == "" {
			continue
		}
		ids = append(ids, string(it.ID))
		byID[string(it.ID)] = it
	}
	if len(ids) == 0 {
		return out
	}
	entries, err := s.audioFPCache.GetBatch(ctx, ids)
	if err != nil {
		slog.WarnContext(ctx, "intro_detection: audio cache batch read failed", slog.Any("err", err))
		return out
	}
	for id, entry := range entries {
		it := byID[id]
		if it == nil || entry == nil {
			continue
		}
		if entry.FileFingerprint == it.File.Fingerprint &&
			entry.AlgoVersion == currentAudioAlgoVersion &&
			entry.WindowSec == int(windowSec) &&
			len(entry.Fingerprint) > 0 {
			out[id] = entry.Fingerprint
		}
	}
	return out
}

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
		slog.WarnContext(ctx, "intro_detection: audio cache upsert failed",
			slog.String("media_id", string(item.ID)),
			slog.Any("err", upsertErr),
		)
	}
	return fp, nil
}
