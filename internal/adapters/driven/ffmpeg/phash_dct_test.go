package ffmpeg

import (
	"image"
	"image/color"
	"math/bits"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPHashDCT_HammingDistance_Identical: same input must give Hamming 0.
func TestPHashDCT_HammingDistance_Identical(t *testing.T) {
	t.Parallel()
	img := newSolidImage(64, 64, 128)
	h1 := pHashDCT(img, 0, 0, 64, 64)
	h2 := pHashDCT(img, 0, 0, 64, 64)
	assert.Zero(t, bits.OnesCount64(h1^h2), "identical inputs must hash to identical bits")
}

// TestPHashDCT_HammingDistance_RandomPair: two unrelated random images
// should be far apart — averaging at ~31.5 bits over many pairs (the
// theoretical expectation of a 63-bit signed comparison). We assert the
// observed mean over 50 pairs is in [25, 38] — wide enough to pass on
// every CI run, narrow enough to catch a hash that collapses to all-zeros
// or all-ones.
func TestPHashDCT_HammingDistance_RandomPair(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(0xCAFE))
	const N = 50
	var sum int
	for range N {
		a := makeRandomImage(rng, 64, 64)
		b := makeRandomImage(rng, 64, 64)
		sum += bits.OnesCount64(pHashDCT(a, 0, 0, 64, 64) ^ pHashDCT(b, 0, 0, 64, 64))
	}
	avg := sum / N
	assert.GreaterOrEqual(t, avg, 25, "random pairs avg Hamming must be ≥25 (got %d)", avg)
	assert.LessOrEqual(t, avg, 38, "random pairs avg Hamming must be ≤38 (got %d)", avg)
}

// TestPHashDCT_BitZeroAlwaysCleared: bit 0 corresponds to the dropped DC
// coefficient — must always be 0 regardless of input.
func TestPHashDCT_BitZeroAlwaysCleared(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(42))
	for range 10 {
		img := makeRandomImage(rng, 64, 64)
		h := pHashDCT(img, 0, 0, 64, 64)
		assert.Zero(t, h&1, "bit 0 (DC) must always be 0; got hash 0x%016X", h)
	}
}

// TestDct1D32_LinearRamp: DCT of a linear ramp should concentrate energy in
// the first AC coef (k=1). Cheap correctness sanity check on the DCT impl.
func TestDct1D32_LinearRamp(t *testing.T) {
	t.Parallel()
	var grid [32][32]float64
	for r := 0; r < 32; r++ {
		for c := 0; c < 32; c++ {
			grid[r][c] = float64(c) // ramp on x, constant on y
		}
	}
	dct := dct1D32Cols(dct1D32Rows(grid))
	// AC[0,1] (row 0, col 1) should dominate AC[0,2..7] for a linear x-ramp.
	require.Greater(t, abs(dct[0][1]), abs(dct[0][2]), "AC[0,1] must dominate AC[0,2] for x-linear ramp")
	require.Greater(t, abs(dct[0][1]), abs(dct[0][3]), "AC[0,1] must dominate AC[0,3]")
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// makeRandomImage returns a 64×64 grayscale image filled with uniform noise.
func makeRandomImage(rng *rand.Rand, w, h int) image.Image {
	img := image.NewGray(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetGray(x, y, color.Gray{Y: uint8(rng.Intn(256))})
		}
	}
	return img
}
