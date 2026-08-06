package ffmpeg

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHammingDistance verifies bit-difference counting across boundary cases.
func TestHammingDistance(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 0, hammingDistance(0, 0), "identical values → distance 0")
	assert.Equal(t, 0, hammingDistance(0xDEADBEEF, 0xDEADBEEF), "identical non-zero → distance 0")
	assert.Equal(t, 1, hammingDistance(0b0000, 0b0001), "one differing bit → distance 1")
	assert.Equal(t, 64, hammingDistance(0, ^uint64(0)), "all bits flipped → distance 64")
}

// TestMatchesPattern verifies that matchesPattern respects the Hamming distance threshold.
func TestMatchesPattern(t *testing.T) {
	t.Parallel()

	p := studioLogoPattern{Name: "Test", Hashes: []uint64{0xFF}}

	// Exact match: distance 0 ≤ maxDist 5.
	assert.True(t, matchesPattern(0xFF, p, 5), "exact match must return true")

	// Distance 5 (5 bits differ) on the boundary: 0xFF = 0b11111111, 0b11100000 = 0xE0 differs in 5 bits.
	assert.True(t, matchesPattern(0xE0, p, 5), "distance == maxDist must return true")

	// Distance 6 (0b11000000 = 0xC0 vs 0xFF differs in 6 bits) → false.
	assert.False(t, matchesPattern(0xC0, p, 5), "distance > maxDist must return false")

	// Empty hashes → no match.
	emptyP := studioLogoPattern{Name: "Empty"}
	assert.False(t, matchesPattern(0xFF, emptyP, 5), "empty hash list must return false")
}

// TestDetectStudioLogoIn_EmptyInputs verifies no match on degenerate inputs.
func TestDetectStudioLogoIn_EmptyInputs(t *testing.T) {
	t.Parallel()

	catalog := studioLogoCatalog{Patterns: []studioLogoPattern{
		{Name: "Test", Hashes: []uint64{0xAAAA}, MinMatches: 1},
	}}

	_, _, found := detectStudioLogoIn(nil, 15, catalog, 5)
	assert.False(t, found, "nil hashes → not found")

	_, _, found = detectStudioLogoIn([]uint64{}, 15, catalog, 5)
	assert.False(t, found, "empty hashes → not found")

	_, _, found = detectStudioLogoIn([]uint64{0xAAAA}, 0, catalog, 5)
	assert.False(t, found, "zero intervalSec → not found")

	emptyCatalog := studioLogoCatalog{}
	_, _, found = detectStudioLogoIn([]uint64{0xAAAA}, 15, emptyCatalog, 5)
	assert.False(t, found, "empty catalog → not found")
}

// TestDetectStudioLogoIn_FirstFrameMatch verifies that a match at frame 0
// produces startMs=0 and endMs=intervalMs.
func TestDetectStudioLogoIn_FirstFrameMatch(t *testing.T) {
	t.Parallel()

	catalog := studioLogoCatalog{Patterns: []studioLogoPattern{
		{Name: "Test", Hashes: []uint64{0xAAAA}, MinMatches: 1},
	}}
	hashes := []uint64{0xAAAA, 0x1111, 0x2222}

	startMs, endMs, found := detectStudioLogoIn(hashes, 15, catalog, 5)
	require.True(t, found)
	assert.Equal(t, int64(0), startMs)
	assert.Equal(t, int64(15_000), endMs, "end = (lastIdx+1) * intervalMs = 1 * 15000")
}

// TestDetectStudioLogoIn_MultipleConsecutiveMatches verifies that consecutive
// matching frames extend endMs.
func TestDetectStudioLogoIn_MultipleConsecutiveMatches(t *testing.T) {
	t.Parallel()

	catalog := studioLogoCatalog{Patterns: []studioLogoPattern{
		{Name: "Test", Hashes: []uint64{0xAAAA}, MinMatches: 1},
	}}
	// Frames 0,1,2 match; frame 3 does not.
	hashes := []uint64{0xAAAA, 0xAAAA, 0xAAAA, 0x1111}

	startMs, endMs, found := detectStudioLogoIn(hashes, 10, catalog, 5)
	require.True(t, found)
	assert.Equal(t, int64(0), startMs)
	assert.Equal(t, int64(30_000), endMs, "3 frames * 10s * 1000 = 30000ms")
}

// TestDetectStudioLogoIn_GapBreaksConsecutive verifies that only the first
// run of consecutive matches is reported.
func TestDetectStudioLogoIn_GapBreaksConsecutive(t *testing.T) {
	t.Parallel()

	catalog := studioLogoCatalog{Patterns: []studioLogoPattern{
		{Name: "Test", Hashes: []uint64{0xAAAA}, MinMatches: 1},
	}}
	// Frames: match, non-match, match, match.  Only the first run (frame 0) counts.
	hashes := []uint64{0xAAAA, 0x1111, 0xAAAA, 0xAAAA}

	startMs, endMs, found := detectStudioLogoIn(hashes, 10, catalog, 5)
	require.True(t, found)
	assert.Equal(t, int64(0), startMs)
	assert.Equal(t, int64(10_000), endMs, "only first run: frames 0..0 → end=(0+1)*10000")
}

// TestDetectStudioLogoIn_MinMatchesNotMet verifies that patterns with minMatches=2
// are not triggered when only 1 frame matches.
func TestDetectStudioLogoIn_MinMatchesNotMet(t *testing.T) {
	t.Parallel()

	catalog := studioLogoCatalog{Patterns: []studioLogoPattern{
		{Name: "Test", Hashes: []uint64{0xAAAA_AAAA_AAAA_AAAA}, MinMatches: 2},
	}}
	// Only frame 0 matches the pattern; frames 1 and 2 are distant (Hamming > 5).
	// 0xAAAA...AAAA XOR 0x5555...5555 = 0xFFFF...FFFF → 64 differing bits.
	hashes := []uint64{0xAAAA_AAAA_AAAA_AAAA, 0x5555_5555_5555_5555, 0x0000_0000_0000_0000}

	_, _, found := detectStudioLogoIn(hashes, 15, catalog, 5)
	assert.False(t, found, "1 match < minMatches=2 → not found")
}

// TestDetectStudioLogoIn_90sLimitRespected verifies that frames beyond 90s
// are not scanned even when the hash slice is longer.
func TestDetectStudioLogoIn_90sLimitRespected(t *testing.T) {
	t.Parallel()

	// With intervalSec=15, maxFrames = 90/15 = 6.  We provide 10 frames.
	// Only the first 6 are scanned. The matching hash appears at frame 7 (index 7).
	catalog := studioLogoCatalog{Patterns: []studioLogoPattern{
		{Name: "Test", Hashes: []uint64{0xAAAA}, MinMatches: 1},
	}}
	hashes := make([]uint64, 10)
	hashes[7] = 0xAAAA // beyond the 90s window

	_, _, found := detectStudioLogoIn(hashes, 15, catalog, 5)
	assert.False(t, found, "match beyond 90s window must not be detected")
}

// TestDetectStudioLogoIn_EmptyPatternHashes verifies that a pattern with no
// reference hashes never matches (mirrors placeholder JSON entries).
func TestDetectStudioLogoIn_EmptyPatternHashes(t *testing.T) {
	t.Parallel()

	catalog := studioLogoCatalog{Patterns: []studioLogoPattern{
		{Name: "Placeholder", Hashes: []uint64{}, MinMatches: 1},
	}}
	hashes := []uint64{0xAAAA, 0xBBBB, 0xCCCC}

	_, _, found := detectStudioLogoIn(hashes, 15, catalog, 5)
	assert.False(t, found, "pattern with empty hashes must never match")
}

// A frame near TWO patterns must credit both: with the old first-match break,
// the second pattern's count depended on catalog order and could miss its
// minMatches threshold despite genuinely matching enough frames.
func TestDetectStudioLogoIn_FrameCreditsEveryMatchingPattern(t *testing.T) {
	t.Parallel()

	// Both patterns contain the same reference hash; the second requires 2 hits.
	catalog := studioLogoCatalog{Patterns: []studioLogoPattern{
		{Name: "First", Hashes: []uint64{0xAAAA}, MinMatches: 3},
		{Name: "Second", Hashes: []uint64{0xAAAA}, MinMatches: 2},
	}}
	hashes := []uint64{0xAAAA, 0xAAAA, 0x1111}

	_, _, found := detectStudioLogoIn(hashes, 15, catalog, 0)
	assert.True(t, found, "the second pattern reaches its threshold when frames credit every match, not just the catalog's first")
}
