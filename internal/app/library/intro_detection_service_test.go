package library

import (
	"testing"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFrequencyMap_AllPresent verifies that hashes present in all episodes are returned.
func TestFrequencyMap_AllPresent(t *testing.T) {
	t.Parallel()
	var hashA, hashB, hashC uint64 = 0xAAAA, 0xBBBB, 0xCCCC

	episodeHashes := [][]uint64{
		{hashA, hashB},
		{hashA, hashB},
		{hashA, hashC},
	}

	result := FrequencyMap(episodeHashes, 0.70)
	require.NotNil(t, result)
	assert.True(t, result[hashA], "hashA in 3/3 episodes must be in result")
	assert.False(t, result[hashB], "hashB in 2/3 (67%%) must NOT be in result at threshold 70%%")
	assert.False(t, result[hashC], "hashC in 1/3 must NOT be in result")
}

// TestFrequencyMap_ThresholdBoundary verifies exact threshold behaviour.
func TestFrequencyMap_ThresholdBoundary(t *testing.T) {
	t.Parallel()
	var hA, hB uint64 = 0x1111, 0x2222

	episodeHashes := [][]uint64{
		{hA, hB},
		{hA},
	}

	// At threshold 0.5: hA (2/2=100%) ✓, hB (1/2=50%) on the boundary.
	result := FrequencyMap(episodeHashes, 0.50)
	assert.True(t, result[hA])
	assert.True(t, result[hB], "exactly at threshold (50%%) must be included")

	// At threshold 0.70: hB (50%%) must be excluded.
	result70 := FrequencyMap(episodeHashes, 0.70)
	assert.True(t, result70[hA])
	assert.False(t, result70[hB])
}

// TestFrequencyMap_Empty handles an empty input gracefully.
func TestFrequencyMap_Empty(t *testing.T) {
	t.Parallel()
	result := FrequencyMap(nil, 0.70)
	assert.Nil(t, result)

	result2 := FrequencyMap([][]uint64{}, 0.70)
	assert.Nil(t, result2)
}

// TestFrequencyMap_SingleEpisode: with one episode, any hash reaches 100%%.
func TestFrequencyMap_SingleEpisode(t *testing.T) {
	t.Parallel()
	var h uint64 = 0xDEAD
	result := FrequencyMap([][]uint64{{h, h + 1}}, 0.70)
	assert.True(t, result[h])
	assert.True(t, result[h+1])
}

// TestFrequencyMap_DuplicatesCountedOnce verifies that duplicates within the same
// episode are not double-counted.
func TestFrequencyMap_DuplicatesCountedOnce(t *testing.T) {
	t.Parallel()
	// hash 0x1 appears 3x in ep1, 0x in ep2 → should count as 1/2 = 50%, not 3/2.
	var h uint64 = 0x0001
	episodeHashes := [][]uint64{
		{h, h, h},
		{0xFFFF},
	}
	result := FrequencyMap(episodeHashes, 0.70)
	assert.False(t, result[h], "duplicate hashes within same episode must only count once")
}

// TestLongestContiguousWindow_BasicMatch ensures a matching run is found.
func TestLongestContiguousWindow_BasicMatch(t *testing.T) {
	t.Parallel()
	introHashes := map[uint64]bool{0xAA: true, 0xBB: true, 0xCC: true}
	// Target: [0xDD, 0xAA, 0xBB, 0xCC, 0xEE]
	// Longest run of intro hashes: indices 1,2,3 → 3 frames.
	target := []uint64{0xDD, 0xAA, 0xBB, 0xCC, 0xEE}
	manifest := &domainlibrary.TrickplayManifest{Interval: 10}

	startMs, endMs, found := longestContiguousWindow(manifest, introHashes, target)
	require.True(t, found)
	assert.Equal(t, int64(1*10*1000), startMs)
	assert.Equal(t, int64(4*10*1000), endMs)
}

// TestLongestContiguousWindow_NoMatch returns not-found when no hash matches.
func TestLongestContiguousWindow_NoMatch(t *testing.T) {
	t.Parallel()
	introHashes := map[uint64]bool{0xAA: true}
	target := []uint64{0x11, 0x22, 0x33}
	manifest := &domainlibrary.TrickplayManifest{Interval: 10}

	_, _, found := longestContiguousWindow(manifest, introHashes, target)
	assert.False(t, found)
}

// TestLongestContiguousWindow_EmptyTarget returns not-found on empty hash slice.
func TestLongestContiguousWindow_EmptyTarget(t *testing.T) {
	t.Parallel()
	manifest := &domainlibrary.TrickplayManifest{Interval: 10}
	_, _, found := longestContiguousWindow(manifest, map[uint64]bool{0x1: true}, []uint64{})
	assert.False(t, found)
}

// TestLongestContiguousWindow_AllMatch verifies full-sequence match.
func TestLongestContiguousWindow_AllMatch(t *testing.T) {
	t.Parallel()
	var h1, h2 uint64 = 0x01, 0x02
	introHashes := map[uint64]bool{h1: true, h2: true}
	target := []uint64{h1, h2, h1}
	manifest := &domainlibrary.TrickplayManifest{Interval: 5}

	startMs, endMs, found := longestContiguousWindow(manifest, introHashes, target)
	require.True(t, found)
	assert.Equal(t, int64(0), startMs)
	assert.Equal(t, int64(3*5*1000), endMs)
}
