package ffmpeg

import (
	_ "embed"
	"encoding/json"
	"math/bits"
)

//go:embed studio_logos.json
var studioLogosData []byte

type studioLogoPattern struct {
	Name       string   `json:"name"`
	Hashes     []uint64 `json:"hashes"`
	MinMatches int      `json:"minMatches"`
}

type studioLogoCatalog struct {
	Version  int                 `json:"version"`
	Patterns []studioLogoPattern `json:"patterns"`
}

var loadedStudioLogos = mustLoadStudioLogos()

func mustLoadStudioLogos() studioLogoCatalog {
	var c studioLogoCatalog
	if err := json.Unmarshal(studioLogosData, &c); err != nil {
		return studioLogoCatalog{}
	}
	return c
}

// hammingDistance returns the number of differing bits between two 64-bit hashes.
func hammingDistance(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}

// matchesPattern returns true if any hash in the pattern is within maxDist Hamming bits of h.
func matchesPattern(h uint64, pattern studioLogoPattern, maxDist int) bool {
	for _, ph := range pattern.Hashes {
		if hammingDistance(h, ph) <= maxDist {
			return true
		}
	}
	return false
}

// detectStudioLogoIn is the pure detection logic, separated for unit testing.
// hashes are the trickplay frame hashes in chronological order.
// intervalSec is the seconds between trickplay frames (e.g. 15 for default).
// catalog is the set of known patterns to match against.
// maxHamming is the maximum Hamming distance to count as a match.
func detectStudioLogoIn(hashes []uint64, intervalSec int, catalog studioLogoCatalog, maxHamming int) (startMs, endMs int64, found bool) {
	if intervalSec <= 0 || len(hashes) == 0 || len(catalog.Patterns) == 0 {
		return 0, 0, false
	}

	// Limit scan to the first 90 seconds.
	maxFrames := 90 / intervalSec
	if maxFrames < 1 {
		maxFrames = 1
	}
	if maxFrames > len(hashes) {
		maxFrames = len(hashes)
	}
	target := hashes[:maxFrames]

	// For each frame, credit EVERY pattern within tolerance - breaking on the
	// first hit made a pattern's count depend on catalog ORDER: a frame close
	// to two logos only fed the earlier one, and the later pattern could miss
	// its minMatches threshold despite genuinely matching enough frames.
	matchedFrames := make([]bool, len(target))
	perPatternHits := make(map[string]int, len(catalog.Patterns))
	for i, h := range target {
		for _, p := range catalog.Patterns {
			if matchesPattern(h, p, maxHamming) {
				matchedFrames[i] = true
				perPatternHits[p.Name]++
			}
		}
	}

	// Require at least one pattern to have reached its minMatches threshold.
	atThreshold := false
	for _, p := range catalog.Patterns {
		min := p.MinMatches
		if min < 1 {
			min = 1
		}
		if perPatternHits[p.Name] >= min {
			atThreshold = true
			break
		}
	}
	if !atThreshold {
		return 0, 0, false
	}

	firstIdx := -1
	for i, m := range matchedFrames {
		if m {
			firstIdx = i
			break
		}
	}
	if firstIdx < 0 {
		return 0, 0, false
	}

	// Walk forward until the first non-matching gap, recording the last consecutive match.
	lastIdx := firstIdx
	for j := firstIdx + 1; j < len(matchedFrames); j++ {
		if !matchedFrames[j] {
			break
		}
		lastIdx = j
	}

	intervalMs := int64(intervalSec) * 1000
	return int64(firstIdx) * intervalMs, int64(lastIdx+1) * intervalMs, true
}
