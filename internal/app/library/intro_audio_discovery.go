package library

import "math/bits"

// Audio-primary intro DISCOVERY (as opposed to confirmation).
//
// The pHash visual pass proposes candidate windows that the chromaprint pass
// then confirms. When the visual pass yields nothing (poor trickplay on SD
// sources, or a cold-open show whose title card isn't visually stable), there
// is no candidate to confirm. These pure helpers let chromaprint DISCOVER a
// recurring intro directly from episode fingerprints, at a free per-episode
// offset — the case proven necessary on variable-teaser shows where the intro
// floats across a multi-minute window.
//
// Everything here operates on raw Chromaprint fingerprints ([]uint32, ~8
// frames/sec) so it is deterministic and unit-testable without ffmpeg/fpcalc.

// chromaprintFramesPerSec is fpcalc's fixed analysis rate (~0.1238 s/frame).
const chromaprintFramesPerSec = 7.56

func framesToMs(frames int) int64 { return int64(float64(frames) / chromaprintFramesPerSec * 1000.0) }

// discoverParams tunes the discovery. Defaults are derived from the seeded
// experiment (strong matches sat at per-window BER 0.04-0.18; unrelated audio
// is ~0.5).
type discoverParams struct {
	MaxFrameBits int     // a frame "matches" when popcount(a^b) <= this (of 32). ~12 ≈ BER 0.375.
	MinRunFrames int     // shortest accepted shared run (filters coincidental matches). ~80 ≈ 10 s.
	SimThreshold float64 // sliding-window similarity required to count a sibling as confirming. ~0.65.
	MinMatches   int     // minimum confirming siblings (incl. the reference).
}

func defaultDiscoverParams() discoverParams {
	return discoverParams{MaxFrameBits: 12, MinRunFrames: 80, SimThreshold: 0.65, MinMatches: 3}
}

// longestSharedRun finds, over every relative offset, the longest contiguous
// run of frames where a and b align with per-frame Hamming distance <=
// maxFrameBits. Returns the run's half-open range [aStart, aEnd) in a's index
// space and whether a run of at least minRunFrames was found. This locates a
// segment shared between two episodes regardless of where it sits in each
// (variable cold-open / teaser length).
func longestSharedRun(a, b []uint32, maxFrameBits, minRunFrames int) (aStart, aEnd int, found bool) {
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
				if runLen > bestLen {
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
		s, e, f := longestSharedRun(target, sib, p.MaxFrameBits, p.MinRunFrames)
		if f && (e-s) > (bestEnd - bestStart) {
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
