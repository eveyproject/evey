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
		assert.Equal(t, []string{"-rematrix_maxval", "2.0000"}, AudioDownmixArgs(ch),
			"channels=%d folds and must bound the matrix", ch)
	}
	// A layout-preserving target sums nothing: no option, no filter, no CPU.
	for _, ch := range []int{6, 8} {
		assert.Nil(t, AudioDownmixArgs(ch), "channels=%d does not fold", ch)
		assert.Empty(t, AudioDownmixPolicy(ch))
	}
}

// The cap is a measured trade, bounded on BOTH sides, and each bound is a
// separate report:
//
//   - at or below 1.0 the fold is fully normalised and the server hands back a
//     stream 6 to 7.7 dB under what a browser folding the same 5.1 itself
//     produces. That gap is the 2026-09-04 "voices are fine, the background is
//     not on the same plane" report, mobile-only.
//   - above the generic 5.1 matrix sum (1 + 2*0.7071 = 2.4142) the value stops
//     normalising anything at all and only adds raw gain, which is the clipping
//     the 2026-09-02 fix removed.
//
// Anything moving this constant outside those bounds is a new decision that
// needs its own measurements, not a tweak. See the file header for both.
func TestAudioDownmixArgs_MaxvalStaysWithinMeasuredBounds(t *testing.T) {
	args := AudioDownmixArgs(2)
	require.Len(t, args, 2)
	maxval, err := strconv.ParseFloat(args[1], 64)
	require.NoError(t, err)
	assert.Greater(t, maxval, 1.0,
		"a fully normalised fold leaves multichannel titles 6-7.7 dB below every other player in the chain")
	assert.LessOrEqual(t, maxval, 2.4142,
		"above the generic 5.1 matrix sum the cap normalises nothing and is pure gain")
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
