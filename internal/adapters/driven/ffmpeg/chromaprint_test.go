package ffmpeg

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFpcalcRawOutput(t *testing.T) {
	t.Parallel()
	out := []byte(`DURATION=120
FINGERPRINT=1234567,890,42,-1,2147483647
`)
	fp, err := parseFpcalcRawOutput(out)
	require.NoError(t, err)
	require.Len(t, fp, 5)
	assert.Equal(t, uint32(1234567), fp[0])
	assert.Equal(t, uint32(890), fp[1])
	assert.Equal(t, uint32(42), fp[2])
	// -1 wraps to all-ones uint32.
	assert.Equal(t, uint32(0xFFFFFFFF), fp[3])
	assert.Equal(t, uint32(2147483647), fp[4])
}

func TestParseFpcalcRawOutput_MissingFingerprint(t *testing.T) {
	t.Parallel()
	_, err := parseFpcalcRawOutput([]byte("DURATION=120\n"))
	assert.ErrorContains(t, err, "FINGERPRINT line not found")
}

func TestEncodeDecodeBase64Fingerprint(t *testing.T) {
	t.Parallel()
	in := []uint32{1, 2, 3, 0xDEADBEEF, 0xFFFFFFFF, 0}
	enc := EncodeBase64Fingerprint(in)
	require.NotEmpty(t, enc)
	out, err := DecodeBase64Fingerprint(enc)
	require.NoError(t, err)
	assert.Equal(t, in, out)
}

func TestEncodeBase64Fingerprint_Empty(t *testing.T) {
	t.Parallel()
	assert.Empty(t, EncodeBase64Fingerprint(nil))
	assert.Empty(t, EncodeBase64Fingerprint([]uint32{}))
}

func TestSlidingBestMatch_Identical(t *testing.T) {
	t.Parallel()
	target := []uint32{0xAAAA, 0xBBBB, 0xCCCC, 0xDDDD}
	off, sim := SlidingBestMatch(target, target)
	assert.Equal(t, 0, off)
	assert.Equal(t, 1.0, sim)
}

func TestSlidingBestMatch_OffsetMatch(t *testing.T) {
	t.Parallel()
	intro := []uint32{0x1111, 0x2222, 0x3333, 0x4444}
	// reference: 3 frames of garbage, then the intro.
	ref := []uint32{0xFFFF, 0xEEEE, 0xDDDD}
	ref = append(ref, intro...)
	ref = append(ref, 0xCCCC, 0xBBBB)

	off, sim := SlidingBestMatch(intro, ref)
	assert.Equal(t, 3, off)
	assert.Equal(t, 1.0, sim)
}

func TestSlidingBestMatch_TargetTooLong(t *testing.T) {
	t.Parallel()
	off, sim := SlidingBestMatch([]uint32{1, 2, 3}, []uint32{1, 2})
	assert.Equal(t, 0, off)
	assert.Equal(t, 0.0, sim)
}

func TestSlidingBestMatch_PartialMatch(t *testing.T) {
	t.Parallel()
	// Half the bits flip → similarity ≈ 0.5
	target := []uint32{0x00000000}
	ref := []uint32{0x55555555} // 16 bits set out of 32
	_, sim := SlidingBestMatch(target, ref)
	assert.InDelta(t, 0.5, sim, 0.01)
}
