package transcoding

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAudioFolds(t *testing.T) {
	// Unknown source into a stereo target: the guard stays on.
	for _, ch := range []int{0, 1, 2} {
		assert.True(t, AudioFolds(0, ch), "unknown source, target=%d", ch)
	}
	assert.True(t, AudioFolds(6, 2))
	assert.True(t, AudioFolds(8, 0))
	assert.True(t, AudioFolds(2, 1))
	// Nothing sums: same layout, or a target that keeps the source's.
	assert.False(t, AudioFolds(2, 2))
	assert.False(t, AudioFolds(2, 0))
	assert.False(t, AudioFolds(1, 2))
	for _, ch := range []int{6, 8} {
		assert.False(t, AudioFolds(0, ch), "target=%d keeps its layout", ch)
		assert.False(t, AudioFolds(8, ch), "target=%d keeps its layout", ch)
	}
}

func TestAudioFoldFilter_OnlyWhenFolding(t *testing.T) {
	f := AudioFoldFilter(6, 2)
	require.NotEmpty(t, f)
	assert.Equal(t, f, AudioFoldFilter(0, 2), "unknown source gets the same guard")
	assert.Equal(t, f, AudioFoldFilter(0, 0), "0 resolves to stereo")
	assert.Equal(t, f, AudioDownmixPolicy(6, 2), "the hashed policy IS the chain ffmpeg runs")

	assert.Contains(t, AudioFoldFilter(6, 1), "out_chlayout=mono")
	for _, c := range [][2]int{{2, 2}, {2, 0}, {0, 6}, {6, 6}, {8, 8}} {
		assert.Empty(t, AudioFoldFilter(c[0], c[1]), "source=%d target=%d", c[0], c[1])
		assert.Empty(t, AudioDownmixPolicy(c[0], c[1]), "source=%d target=%d", c[0], c[1])
	}
}

// The chain folds in float THEN limits: the fold must come first, with the
// level cap that gives the 6 dB back, and the limiter must be latency
// compensated (an uncompensated look-ahead shifts a sample-aligned rendition).
func TestAudioFoldFilter_FoldsThenLimits(t *testing.T) {
	f := AudioFoldFilter(6, 2)
	parts := strings.Split(f, ",")
	require.Len(t, parts, 2)
	assert.True(t, strings.HasPrefix(parts[0], "aresample=out_chlayout=stereo:rematrix_maxval=2.0000"), parts[0])
	assert.True(t, strings.HasPrefix(parts[1], "alimiter="), parts[1])
	assert.Contains(t, parts[1], "latency=1")
	assert.Contains(t, parts[1], "level=0", "auto-level would undo the ceiling")
	assert.Contains(t, parts[1], "limit=0.7943")
}

// Both bounds are measured decisions (see the file header): at 1.0 the fold is
// 6-7.7 dB under every other player, above the 5.1 matrix sum the cap is pure
// gain. The ceiling must leave the AAC decode overshoot under full scale.
func TestAudioFoldFilter_ConstantsStayWithinMeasuredBounds(t *testing.T) {
	assert.Greater(t, DownmixRematrixMaxval, 1.0)
	assert.LessOrEqual(t, DownmixRematrixMaxval, 2.4142)
	assert.LessOrEqual(t, DownmixLimiterCeiling, 0.7943, "an AAC decode overshoots up to ~1.8 dB")
	assert.Greater(t, DownmixLimiterCeiling, 0.7)
}

func TestJoinAudioFilters(t *testing.T) {
	assert.Empty(t, JoinAudioFilters("", ""))
	assert.Equal(t, "a", JoinAudioFilters("", "a", ""))
	assert.Equal(t, "a,b", JoinAudioFilters("a", "", "b"))
}

func TestResolvedAudioChannels(t *testing.T) {
	assert.Equal(t, 2, ResolvedAudioChannels(0))
	assert.Equal(t, 1, ResolvedAudioChannels(1))
	assert.Equal(t, 6, ResolvedAudioChannels(6))
}
