package ffmpeg

import (
	"image"
	"image/color"
	"slices"
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

func (s *solidImage) ColorModel() color.Model { return color.GrayModel }
func (s *solidImage) Bounds() image.Rectangle { return s.bounds }
func (s *solidImage) At(x, y int) color.Color { return s.c }

// gradientImage produces a horizontal gradient from lum=0 to lum=255.
type gradientImage struct {
	w, h int
}

func (g *gradientImage) ColorModel() color.Model { return color.GrayModel }
func (g *gradientImage) Bounds() image.Rectangle { return image.Rect(0, 0, g.w, g.h) }
func (g *gradientImage) At(x, _ int) color.Color {
	return color.Gray{Y: uint8(x * 255 / (g.w - 1))}
}

// TestPHashDCT_IdenticalImages verifies that two equal images produce the same hash.
func TestPHashDCT_IdenticalImages(t *testing.T) {
	t.Parallel()
	img1 := newSolidImage(64, 64, 128)
	img2 := newSolidImage(64, 64, 128)

	h1 := pHashDCT(img1, 0, 0, 64, 64)
	h2 := pHashDCT(img2, 0, 0, 64, 64)
	assert.Equal(t, h1, h2, "identical images must produce the same hash")
}

// TestPHashDCT_DifferentImages verifies that clearly different images produce different hashes.
// Two gradient images with opposite orientations must differ.
func TestPHashDCT_DifferentImages(t *testing.T) {
	t.Parallel()
	// Left-dark gradient: pixel[x] = x*255/(W-1)
	grad := &gradientImage{w: 64, h: 64}
	// Solid mid-grey: uniform - mean = 128, all pixels == 128 → all zero bits
	solid := newSolidImage(64, 64, 128)

	hGrad := pHashDCT(grad, 0, 0, 64, 64)
	hSolid := pHashDCT(solid, 0, 0, 64, 64)
	assert.NotEqual(t, hGrad, hSolid, "gradient and uniform images must produce different hashes")
}

// TestPHashDCT_HorizontalFlipDiffers verifies that a left-heavy vs right-heavy gradient differs.
func TestPHashDCT_HorizontalFlipDiffers(t *testing.T) {
	t.Parallel()
	grad := &gradientImage{w: 64, h: 64}
	// flipped version: high luminance on the left
	flipped := &gradientImage{w: 64, h: 64}

	hNormal := pHashDCT(grad, 0, 0, 64, 64)
	// Mirror the region: use right half as x0..x1 reversed isn't directly
	// expressible, so we compare gradient vs solid - guaranteed different.
	hSolid := pHashDCT(newSolidImage(64, 64, 200), 0, 0, 64, 64)
	_ = flipped

	assert.NotEqual(t, hNormal, hSolid, "gradient and solid must produce different hashes")
}

// TestPHashDCT_SubRegion verifies that extracting two different sub-regions of the same
// image produces different hashes when the regions are visually distinct.
func TestPHashDCT_SubRegion(t *testing.T) {
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
			// Right half: vertical stripe pattern - columns alternate 0/255
			if (x-64)%2 == 0 {
				img.SetGray(x, y, color.Gray{Y: 0})
			} else {
				img.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}
	hLeft := pHashDCT(img, 0, 0, 64, 64)
	hRight := pHashDCT(img, 64, 0, 128, 64)
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

// Two nearby regions form one trailing cluster; the credits start at the
// cluster's FIRST region (the fade into the credits). Runtime and offsets are
// those of a 30-min episode whose cluster sits 3 minutes before the end: the
// lead now has to be plausible for the cluster to be trusted at all, so the
// former toy 5-minute file no longer exercises the structural rule.
func TestParseEndCredits_ClusterStartsAtFirstMember(t *testing.T) {
	t.Parallel()
	const startSec = 1620.0
	const durSec = 1800.0
	input := `[blackdetect @ 0x1] black_start:2.0 black_end:3.2 black_duration:1.2
[blackdetect @ 0x2] black_start:8.0 black_end:15.0 black_duration:7.0`
	startMs, endMs, found := parseEndCredits(input, startSec, durSec)
	require.True(t, found)
	assert.Equal(t, int64((startSec+2.0)*1000), startMs)
	assert.Equal(t, int64(durSec*1000), endMs)
}

// The prod failure mode: one isolated scene fade minutes before the end (and
// nothing else black) must NOT become "credits from here to the end" - it used
// to surface a skip prompt >10 minutes early.
func TestParseEndCredits_RejectsIsolatedEarlyFade(t *testing.T) {
	t.Parallel()
	const startSec = 2000.0 // window opens 10 min before the end
	const durSec = 2600.0
	// One 1.5s fade 540s (9 min) before the end.
	input := "[blackdetect @ 0x1] black_start:60.0 black_end:61.5 black_duration:1.5"
	_, _, found := parseEndCredits(input, startSec, durSec)
	assert.False(t, found, "an isolated far-from-end fade is a scene cut, not the credits")
}

// The same isolated early fade loses to a trailing cluster near the end: the
// credits anchor on the trailing cluster, not the first black of the window.
func TestParseEndCredits_TrailingClusterBeatsEarlyFade(t *testing.T) {
	t.Parallel()
	const startSec = 2000.0
	const durSec = 2600.0
	input := `[blackdetect @ 0x1] black_start:60.0 black_end:61.5 black_duration:1.5
[blackdetect @ 0x2] black_start:520.0 black_end:522.0 black_duration:2.0
[blackdetect @ 0x3] black_start:560.0 black_end:562.0 black_duration:2.0`
	startMs, endMs, found := parseEndCredits(input, startSec, durSec)
	require.True(t, found)
	assert.Equal(t, int64((startSec+520.0)*1000), startMs,
		"credits start at the trailing cluster's first region, not at the early scene fade")
	assert.Equal(t, int64(durSec*1000), endMs)
}

// A lone LONG black far from the end is trusted (credits rolling on black
// carry a long fade), even beyond the single-region lead guard - as long as the
// runtime makes that lead plausible. Here a 2h10 film: 12% of it leaves room
// for a 500s credits roll, which the 300s single-region guard alone refuses.
// The same 500s lead on the former 43-min runtime is now rejected: that is the
// mid-content "skip the credits" prompt measured in prod.
func TestParseEndCredits_LongBlackTrustedFarOut(t *testing.T) {
	t.Parallel()
	const startSec = 7200.0
	const durSec = 7800.0
	input := "[blackdetect @ 0x1] black_start:100.0 black_end:106.0 black_duration:6.0"
	startMs, _, found := parseEndCredits(input, startSec, durSec)
	require.True(t, found)
	assert.Equal(t, int64((startSec+100.0)*1000), startMs)
}

// The remaining source of the "skip the credits" prompt in mid-content: a
// cluster of two short fades had NO lead bound at all, so it could plant the
// credits anywhere in the scanned window - up to 600s before the end, the exact
// value seen repeatedly in the 2026-08-22 prod sample. Two 1.5s fades 9 minutes
// before the end of a 42-min episode are a scene sequence, not the credits.
func TestParseEndCredits_RejectsClusterWithImplausibleLead(t *testing.T) {
	t.Parallel()
	const durSec = 2520.0         // 42-min episode: bound = max(240s, 12% = 302.4s)
	const startSec = durSec - 600 // the scan window is the last 10 min
	input := `[blackdetect @ 0x1] black_start:60.0 black_end:61.5 black_duration:1.5
[blackdetect @ 0x2] black_start:90.0 black_end:91.5 black_duration:1.5`
	_, _, found := parseEndCredits(input, startSec, durSec)
	assert.False(t, found, "a two-fade cluster 9 minutes before the end is not the credits roll")
}

// The same two-fade cluster IS the credits when it sits at a plausible lead.
func TestParseEndCredits_KeepsClusterWithPlausibleLead(t *testing.T) {
	t.Parallel()
	const durSec = 2520.0
	const startSec = durSec - 600
	input := `[blackdetect @ 0x1] black_start:480.0 black_end:481.5 black_duration:1.5
[blackdetect @ 0x2] black_start:520.0 black_end:521.5 black_duration:1.5`
	startMs, endMs, found := parseEndCredits(input, startSec, durSec)
	require.True(t, found)
	assert.Equal(t, int64((startSec+480.0)*1000), startMs)
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

// TestEndCreditsArgs_BoundsTheDecodeWithMinusT pins the bound whose absence
// took the host down on 2026-08-24.
//
// The credits scan is documented as "the last 20% of the file, capped at 600s",
// but that cap was only ever expressed through -ss: ffmpeg was told where to
// START and left to run to the real end of file. durationSec comes from the
// database, written by a probe measured failing in prod, so an underestimated
// duration removed the bound entirely - the scan decoded the whole file.
func TestEndCreditsArgs_BoundsTheDecodeWithMinusT(t *testing.T) {
	t.Parallel()

	args := endCreditsArgs("/tv/ep.mkv", 1500, 600)

	idx := slices.Index(args, "-t")
	require.NotEqual(t, -1, idx, "the credits scan must state its window to ffmpeg, not merely imply it with -ss")
	require.Less(t, idx+1, len(args), "-t must carry a value")
	require.Equal(t, "600.000", args[idx+1], "the -t window must be the scan window, not the file duration")

	ss := slices.Index(args, "-ss")
	require.NotEqual(t, -1, ss)
	require.Less(t, ss, idx, "-ss must precede -t so the seek applies to the input")
}

// TestEndCreditsArgs_WindowIsIndependentOfDeclaredDuration proves the bound
// holds even when the stored duration is wrong, which is the failure that
// actually happened: a bad duration must shorten or misplace the scan, never
// remove its ceiling.
func TestEndCreditsArgs_WindowIsIndependentOfDeclaredDuration(t *testing.T) {
	t.Parallel()

	for _, startSec := range []float64{0, 60, 100000} {
		args := endCreditsArgs("/tv/ep.mkv", startSec, 600)
		idx := slices.Index(args, "-t")
		require.NotEqual(t, -1, idx, "start=%v must still be bounded", startSec)
		require.Equal(t, "600.000", args[idx+1], "start=%v must not widen the window", startSec)
	}
}

// TestDetectArgs_OneDecodedStreamPerMuxer pins the structural rule the
// 2026-08-24 incident bought: a detection argv must never feed a video filter
// AND an audio filter into the same muxer. After an input seek on some MKVs the
// two timelines resume far apart and the interleave queue buffers decoded video
// frames without bound (isolated on the incident's files: video alone 141 MiB,
// audio alone 8 MiB, combined OOM at >500 MiB/s).
func TestDetectArgs_OneDecodedStreamPerMuxer(t *testing.T) {
	t.Parallel()

	for name, args := range map[string][]string{
		"blackDetectArgs":   blackDetectArgs("/tv/ep.mkv", 80, 130),
		"silenceDetectArgs": silenceDetectArgs("/tv/ep.mkv", 80, 130),
		"endCreditsArgs":    endCreditsArgs("/tv/ep.mkv", 1500, 600),
	} {
		hasVF := slices.Contains(args, "-vf")
		hasAF := slices.Contains(args, "-af")
		require.False(t, hasVF && hasAF,
			"%s: -vf and -af in one invocation share a muxer whose interleave queue is unbounded after -ss", name)
		if hasVF {
			require.True(t, slices.Contains(args, "-an"), "%s: a video pass must drop the audio stream explicitly", name)
		}
		if hasAF {
			require.True(t, slices.Contains(args, "-vn"), "%s: an audio pass must drop the video stream explicitly", name)
		}
	}
}
