package library

import (
	"context"
	"log/slog"
	"math/bits"
	"sort"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

// Audio-primary intro DISCOVERY (as opposed to confirmation).
//
// The pHash visual pass proposes candidate windows that the chromaprint pass
// then confirms. When the visual pass yields nothing (poor trickplay on SD
// sources, or a cold-open show whose title card isn't visually stable), there
// is no candidate to confirm. These pure helpers let chromaprint DISCOVER a
// recurring intro directly from episode fingerprints, at a free per-episode
// offset - the case proven necessary on variable-teaser shows where the intro
// floats across a multi-minute window.
//
// Everything here operates on raw Chromaprint fingerprints ([]uint32, ~8
// frames/sec) so it is deterministic and unit-testable without ffmpeg/fpcalc.

// chromaprintFramesPerSec is fpcalc's analysis rate: 11025 Hz resampling, one
// item per 1365-sample hop (a third of its 4096 frame), measured 8.08/s. The old
// 7.56 stretched every audio time by 6.8%, pushing late intro ends into content.
const chromaprintFramesPerSec = 11025.0 / 1365.0

func framesToMs(frames int) int64 { return int64(float64(frames) / chromaprintFramesPerSec * 1000.0) }

func msToFrames(ms int64) int { return int(float64(ms) / 1000.0 * chromaprintFramesPerSec) }

// discoverParams tunes the discovery. Defaults are derived from the seeded
// experiment (strong matches sat at per-window BER 0.04-0.18; unrelated audio
// is ~0.5).
type discoverParams struct {
	MaxFrameBits   int     // a frame "matches" when popcount(a^b) <= this (of 32). ~12 ≈ BER 0.375.
	MinRunFrames   int     // shortest accepted shared run (filters coincidental matches). ~80 ≈ 10 s.
	MinStartFrames int     // reject candidate runs that START before this offset in the target's timeline. Filters the recurring network/promo bumper that sits at 0:00 (e.g. an 18 s "FOX + this season on" card) from being mistaken for the intro. ~189 ≈ 25 s.
	SimThreshold   float64 // sliding-window similarity required to count a sibling as confirming. ~0.65.
	MinMatches     int     // minimum confirming siblings (incl. the reference).
}

func defaultDiscoverParams() discoverParams {
	return discoverParams{MaxFrameBits: 12, MinRunFrames: 80, MinStartFrames: msToFrames(25000), SimThreshold: 0.65, MinMatches: 3}
}

// longestSharedRun finds, over every relative offset, the longest contiguous
// run of frames where a and b align with per-frame Hamming distance <=
// maxFrameBits. Returns the run's half-open range [aStart, aEnd) in a's index
// space and whether a run of at least minRunFrames was found. This locates a
// segment shared between two episodes regardless of where it sits in each
// (variable cold-open / teaser length). Runs that START before minStartFrames
// in a's timeline are ignored, so a recurring 0:00 bumper never wins the
// candidate selection (pass 0 to disable the guard).
func longestSharedRun(a, b []uint32, maxFrameBits, minRunFrames, minStartFrames int) (aStart, aEnd int, found bool) {
	if len(a) == 0 || len(b) == 0 {
		return 0, 0, false
	}
	bestLen := 0
	// offset = index in b that aligns with a[0]. Range covers full overlap.
	for off := -(len(a) - 1); off <= len(b)-1; off++ {
		runStart, runLen := 0, 0
		for i := range a {
			j := i + off
			if j < 0 || j >= len(b) {
				runLen = 0
				continue
			}
			if bits.OnesCount32(a[i]^b[j]) <= maxFrameBits {
				if runLen == 0 {
					runStart = i
				}
				runLen++
				if runStart >= minStartFrames && runLen > bestLen {
					bestLen = runLen
					aStart = runStart
					aEnd = i + 1
				}
			} else {
				runLen = 0
			}
		}
	}
	return aStart, aEnd, bestLen >= minRunFrames
}

// discoverRecurringIntro finds an intro shared by `target` and `siblings`.
// It first locates the longest run shared between target and the best sibling,
// then confirms that run recurs (sliding match >= SimThreshold) in enough
// siblings. Returns the run's [startMs, endMs) in target's timeline and the
// number of confirming episodes (target counted). found is true when at least
// MinMatches episodes carry the run.
func discoverRecurringIntro(target []uint32, siblings [][]uint32, p discoverParams) (startMs, endMs int64, matches int, found bool) {
	if len(target) == 0 || len(siblings) == 0 {
		return 0, 0, 0, false
	}
	// 1) Candidate run = longest run shared between target and ANY sibling.
	bestStart, bestEnd, ok := 0, 0, false
	for _, sib := range siblings {
		s, e, f := longestSharedRun(target, sib, p.MaxFrameBits, p.MinRunFrames, p.MinStartFrames)
		if f && (e-s) > (bestEnd-bestStart) {
			bestStart, bestEnd, ok = s, e, true
		}
	}
	if !ok {
		return 0, 0, 0, false
	}
	// 2) Confirm the candidate recurs across siblings (free offset each).
	probe := target[bestStart:bestEnd]
	matches = 1 // target itself carries the run by construction
	for _, sib := range siblings {
		if len(sib) < len(probe) {
			continue
		}
		if _, sim := slidingChromaprint(probe, sib); sim >= p.SimThreshold {
			matches++
		}
	}
	if matches < p.MinMatches {
		return 0, 0, matches, false
	}
	return framesToMs(bestStart), framesToMs(bestEnd), matches, true
}

const (
	// discoverWindowSec bounds the audio window fingerprinted per episode. 720s
	// (12 min): the gold set holds real intros starting past 8 minutes (a title
	// sequence at 8:43-10:29 after a long cold open), which the previous 480s
	// window could not even see - the season cluster was structurally blind to
	// them. The cache keys on WindowSec, so bumping this recomputes cleanly.
	// Original note: intros
	// on cold-open shows can sit at 5+ min, so the window must be generous; 8 min
	// covers every observed case while keeping fpcalc cost bounded.
	discoverWindowSec = 720.0
	// discoverMaxSiblings caps how many other episodes we fingerprint to confirm
	// recurrence - enough for MinMatches, bounded so the season pass isn't O(n²)
	// in fpcalc.
	discoverMaxSiblings = 4
	// discoverSiblingScanCap bounds how many episode rows we collect before
	// picking the nearest neighbours, so a season with hundreds of episodes
	// can't make this pass unbounded.
	discoverSiblingScanCap = 400
)

// selectNearestSiblings returns up to max episodes closest to target by episode
// number (IndexNumber), so confirmation uses the target's neighbours (±2) rather
// than whichever episodes happen to come first. Episodes without an IndexNumber
// sort last. Falls back to the input order when the target has no IndexNumber.
func selectNearestSiblings(target *domainlibrary.MediaItem, candidates []*domainlibrary.MediaItem, max int) []*domainlibrary.MediaItem {
	if len(candidates) <= max {
		return candidates
	}
	if target.IndexNumber == nil {
		return candidates[:max]
	}
	tn := *target.IndexNumber
	sorted := append([]*domainlibrary.MediaItem(nil), candidates...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return siblingDistance(tn, sorted[i]) < siblingDistance(tn, sorted[j])
	})
	return sorted[:max]
}

func siblingDistance(targetIndex int, m *domainlibrary.MediaItem) int {
	if m.IndexNumber == nil {
		return 1 << 30
	}
	if d := *m.IndexNumber - targetIndex; d >= 0 {
		return d
	} else {
		return -d
	}
}

// discoverIntroByAudio is the audio-primary discovery entry point used as a
// pipeline fallback when the visual pHash pass yields no confirmable candidate.
// It fingerprints this episode + a few siblings over a wide window and locates
// the recurring intro at a free per-episode offset. Returns ok=false when no
// recurring run is confirmed (caller then falls through to silencedetect).
func (s *IntroDetectionService) discoverIntroByAudio(ctx context.Context, item *domainlibrary.MediaItem) (domainlibrary.MediaSegment, bool) {
	none := domainlibrary.MediaSegment{}
	if s.chromaprinter == nil || item.File == nil || item.File.Path == "" || item.ParentID == "" {
		return none, false
	}
	parentID := item.ParentID
	episodeType := string(domainlibrary.Episode)
	var candidates []*domainlibrary.MediaItem
	for offset := 0; len(candidates) < discoverSiblingScanCap; offset += 200 {
		page, err := s.mediaItems.List(ctx, domainlibrary.ListMediaItemsFilter{
			ParentID: &parentID, Type: &episodeType, Limit: 200, Offset: offset,
		})
		if err != nil || len(page) == 0 {
			break
		}
		for _, e := range page {
			if e.ID != item.ID && e.File != nil && e.File.Path != "" {
				candidates = append(candidates, e)
			}
		}
		if len(page) < 200 {
			break
		}
	}
	// Prefer the episodes ADJACENT to the target (±2 by episode number): an
	// intro can appear mid-season or evolve in blocks, so the first four of the
	// season are not necessarily representative of the target's intro.
	sibs := selectNearestSiblings(item, candidates, discoverMaxSiblings)
	if len(sibs) == 0 {
		return none, false
	}
	itemFP, err := s.extractEpisodeAudioCached(ctx, item, discoverWindowSec)
	if err != nil || len(itemFP) == 0 {
		return none, false
	}
	sibFPs := make([][]uint32, 0, len(sibs))
	for _, sib := range sibs {
		if ctx.Err() != nil {
			return none, false
		}
		fp, ferr := s.extractEpisodeAudioCached(ctx, sib, discoverWindowSec)
		if ferr == nil && len(fp) > 0 {
			sibFPs = append(sibFPs, fp)
		}
	}
	if len(sibFPs) == 0 {
		return none, false
	}
	startMs, endMs, matches, found := discoverRecurringIntro(itemFP, sibFPs, defaultDiscoverParams())
	if !found {
		return none, false
	}
	slog.InfoContext(ctx, "intro_detection: chromaprint discovery",
		slog.String("media_id", string(item.ID)),
		slog.Int64("start_ms", startMs),
		slog.Int64("end_ms", endMs),
		slog.Int("confirming_episodes", matches))
	return domainlibrary.MediaSegment{
		Type:       "intro",
		StartMs:    startMs,
		EndMs:      endMs,
		Source:     "chromaprint_discovery",
		Confidence: 0.85,
	}, true
}
