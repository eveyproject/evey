package transcoding

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAudioDownmixArgs_OnlyWhenFolding(t *testing.T) {
	// 0 resolves to stereo (the historical "-ac 2"), so it folds too.
	for _, ch := range []int{0, 1, 2} {
		assert.Equal(t, []string{"-rematrix_maxval", "1.0000"}, AudioDownmixArgs(ch),
			"channels=%d folds and must bound the matrix", ch)
	}
	// A layout-preserving target sums nothing: no option, no filter, no CPU.
	for _, ch := range []int{6, 8} {
		assert.Nil(t, AudioDownmixArgs(ch), "channels=%d does not fold", ch)
		assert.Empty(t, AudioDownmixPolicy(ch))
	}
}

// maxval IS the worst-case output of the fold, so anything above 1.0 leaves a
// source-dependent chance of clipping - measured, not assumed: 1.5233 still
// clipped a real AC-3 5.1 track. Guard the constant against a well-meaning
// "let's give a bit of level back".
func TestAudioDownmixArgs_MaxvalCannotExceedFullScale(t *testing.T) {
	args := AudioDownmixArgs(2)
	require.Len(t, args, 2)
	maxval, err := strconv.ParseFloat(args[1], 64)
	require.NoError(t, err)
	assert.LessOrEqual(t, maxval, 1.0,
		"a maxval above 1.0 lets the fold exceed full scale for coherent input")
	assert.Greater(t, maxval, 0.0)
}

// No filter-graph node is ever added: a resampler option costs nothing and, on
// a matrix that does not sum above the cap, does nothing.
func TestAudioDownmixArgs_IsNeverAFilter(t *testing.T) {
	for _, ch := range []int{0, 1, 2, 6, 8} {
		for _, a := range AudioDownmixArgs(ch) {
			assert.NotContains(t, a, "-af")
			assert.NotContains(t, a, "-filter")
		}
	}
}

func TestResolvedAudioChannels(t *testing.T) {
	assert.Equal(t, 2, ResolvedAudioChannels(0))
	assert.Equal(t, 1, ResolvedAudioChannels(1))
	assert.Equal(t, 6, ResolvedAudioChannels(6))
}
