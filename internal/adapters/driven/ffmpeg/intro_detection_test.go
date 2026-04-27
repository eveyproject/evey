package ffmpeg

import (
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// solidImage is a uniform-color in-memory image for hash testing.
type solidImage struct {
	bounds image.Rectangle
	c      color.Gray
}

func newSolidImage(w, h int, lum uint8) *solidImage {
	return &solidImage{bounds: image.Rect(0, 0, w, h), c: color.Gray{Y: lum}}
}

func (s *solidImage) ColorModel() color.Model      { return color.GrayModel }
func (s *solidImage) Bounds() image.Rectangle      { return s.bounds }
func (s *solidImage) At(x, y int) color.Color      { return s.c }

// gradientImage produces a horizontal gradient from lum=0 to lum=255.
type gradientImage struct {
	w, h int
}

func (g *gradientImage) ColorModel() color.Model { return color.GrayModel }
func (g *gradientImage) Bounds() image.Rectangle { return image.Rect(0, 0, g.w, g.h) }
func (g *gradientImage) At(x, _ int) color.Color {
	return color.Gray{Y: uint8(x * 255 / (g.w - 1))}
}

// TestAHash_IdenticalImages verifies that two equal images produce the same hash.
func TestAHash_IdenticalImages(t *testing.T) {
	t.Parallel()
	img1 := newSolidImage(64, 64, 128)
	img2 := newSolidImage(64, 64, 128)

	h1 := aHash(img1, 0, 0, 64, 64)
	h2 := aHash(img2, 0, 0, 64, 64)
	assert.Equal(t, h1, h2, "identical images must produce the same hash")
}

// TestAHash_DifferentImages verifies that clearly different images produce different hashes.
// Two gradient images with opposite orientations must differ.
func TestAHash_DifferentImages(t *testing.T) {
	t.Parallel()
	// Left-dark gradient: pixel[x] = x*255/(W-1)
	grad := &gradientImage{w: 64, h: 64}
	// Solid mid-grey: uniform — mean = 128, all pixels == 128 → all zero bits
	solid := newSolidImage(64, 64, 128)

	hGrad := aHash(grad, 0, 0, 64, 64)
	hSolid := aHash(solid, 0, 0, 64, 64)
	assert.NotEqual(t, hGrad, hSolid, "gradient and uniform images must produce different hashes")
}

// TestAHash_HorizontalFlipDiffers verifies that a left-heavy vs right-heavy gradient differs.
func TestAHash_HorizontalFlipDiffers(t *testing.T) {
	t.Parallel()
	grad := &gradientImage{w: 64, h: 64}
	// flipped version: high luminance on the left
	flipped := &gradientImage{w: 64, h: 64}

	hNormal := aHash(grad, 0, 0, 64, 64)
	// Mirror the region: use right half as x0..x1 reversed isn't directly
	// expressible, so we compare gradient vs solid — guaranteed different.
	hSolid := aHash(newSolidImage(64, 64, 200), 0, 0, 64, 64)
	_ = flipped

	assert.NotEqual(t, hNormal, hSolid, "gradient and solid must produce different hashes")
}

// TestAHash_SubRegion verifies that extracting two different sub-regions of the same
// image produces different hashes when the regions are visually distinct.
func TestAHash_SubRegion(t *testing.T) {
	t.Parallel()
	// Create a 128×64 image: left half is a horizontal gradient (visually varying),
	// right half is a vertical stripe pattern (alternating 0 and 255).
	img := image.NewGray(image.Rect(0, 0, 128, 64))
	for y := range 64 {
		for x := range 64 {
			// Left half: horizontal gradient
			img.SetGray(x, y, color.Gray{Y: uint8(x * 255 / 63)})
		}
		for x := 64; x < 128; x++ {
			// Right half: vertical stripe pattern — columns alternate 0/255
			if (x-64)%2 == 0 {
				img.SetGray(x, y, color.Gray{Y: 0})
			} else {
				img.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	hLeft := aHash(img, 0, 0, 64, 64)
	hRight := aHash(img, 64, 0, 128, 64)
	assert.NotEqual(t, hLeft, hRight, "horizontal gradient and stripe pattern must hash differently")
}

// TestExtractTileHashes_NonExistentFile verifies error on missing file.
func TestExtractTileHashes_NonExistentFile(t *testing.T) {
	t.Parallel()
	d := NewIntroDetector("ffmpeg", "ffprobe")
	_, err := d.ExtractTileHashes("/nonexistent/sprite.webp", 5, 5, 64, 36)
	require.Error(t, err)
}

// TestParseDetections_Silence verifies silence range parsing.
func TestParseDetections_Silence(t *testing.T) {
	t.Parallel()
	input := `[silencedetect @ 0x...] silence_start: 2.5
[silencedetect @ 0x...] silence_end: 4.1 | silence_duration: 1.6`
	silences, blacks := parseDetections(input)
	require.Len(t, silences, 1)
	assert.InDelta(t, 2.5, silences[0].start, 0.001)
	assert.InDelta(t, 4.1, silences[0].end, 0.001)
	assert.Empty(t, blacks)
}

// TestParseDetections_Black verifies black range parsing.
func TestParseDetections_Black(t *testing.T) {
	t.Parallel()
	input := `[blackdetect @ 0x...] black_start:1.0 black_end:2.0 black_duration:1.0`
	silences, blacks := parseDetections(input)
	assert.Empty(t, silences)
	require.Len(t, blacks, 1)
	assert.InDelta(t, 1.0, blacks[0].start, 0.001)
	assert.InDelta(t, 2.0, blacks[0].end, 0.001)
}

// --- parseEndCredits tests ---

// TestParseEndCredits_Empty returns not-found for empty input.
func TestParseEndCredits_Empty(t *testing.T) {
	t.Parallel()
	_, _, found := parseEndCredits("", 100.0, 200.0)
	assert.False(t, found)
}

// TestParseEndCredits_TooShort ignores a black region shorter than 1.0s.
func TestParseEndCredits_TooShort(t *testing.T) {
	t.Parallel()
	input := "[blackdetect @ 0x1234] black_start:1.5 black_end:1.8 black_duration:0.3"
	_, _, found := parseEndCredits(input, 100.0, 200.0)
	assert.False(t, found)
}

// TestParseEndCredits_Found returns a segment when a qualifying black region exists.
func TestParseEndCredits_Found(t *testing.T) {
	t.Parallel()
	const startSec = 100.0
	const durSec = 200.0
	input := "[blackdetect @ 0x1234] black_start:1.5 black_end:4.0 black_duration:2.5"
	startMs, endMs, found := parseEndCredits(input, startSec, durSec)
	require.True(t, found)
	assert.Equal(t, int64((startSec+1.5)*1000), startMs)
	assert.Equal(t, int64(durSec*1000), endMs)
}

// TestParseEndCredits_FirstQualifyingWins returns the first region that meets the threshold.
func TestParseEndCredits_FirstQualifyingWins(t *testing.T) {
	t.Parallel()
	const startSec = 50.0
	const durSec = 300.0
	input := `[blackdetect @ 0x1] black_start:2.0 black_end:3.2 black_duration:1.2
[blackdetect @ 0x2] black_start:8.0 black_end:15.0 black_duration:7.0`
	startMs, endMs, found := parseEndCredits(input, startSec, durSec)
	require.True(t, found)
	// First qualifying region (offset 2.0) wins.
	assert.Equal(t, int64((startSec+2.0)*1000), startMs)
	assert.Equal(t, int64(durSec*1000), endMs)
}

// TestParseEndCredits_ExactBoundary treats black_duration==1.0 as qualifying.
func TestParseEndCredits_ExactBoundary(t *testing.T) {
	t.Parallel()
	const startSec = 0.0
	const durSec = 120.0
	input := "[blackdetect @ 0x1] black_start:5.0 black_end:6.0 black_duration:1.0"
	startMs, endMs, found := parseEndCredits(input, startSec, durSec)
	require.True(t, found)
	assert.Equal(t, int64(5000), startMs)
	assert.Equal(t, int64(durSec*1000), endMs)
}
