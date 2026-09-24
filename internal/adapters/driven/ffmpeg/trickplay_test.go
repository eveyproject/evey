package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeFakeProbeScript(t *testing.T, duration float64) string {
	t.Helper()
	f, err := os.CreateTemp("", "fake-ffprobe-*.sh")
	require.NoError(t, err)
	_, err = fmt.Fprintf(f, "#!/bin/sh\nprintf '{\"format\":{\"duration\":\"%g\"}}'\n", duration)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.NoError(t, os.Chmod(f.Name(), 0755))
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}

// makeFakeFfmpegScript creates a fake ffmpeg that emulates the
// single-pass trickplay build: find the -y output pattern (containing
// "%03d") and touch trickplay_001.webp in its parent directory so the
// sprite-counting loop in Generate finds at least one file.
//
// Also emits stderr lines mimicking the showinfo filter so the timestamp
// parser has something to consume - without these, Generate fails on
// "no timestamps captured from showinfo".
func makeFakeFfmpegScript(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp("", "fake-ffmpeg-trickplay-*.sh")
	require.NoError(t, err)
	_, err = f.WriteString(`#!/bin/sh
prev=""
for arg; do
  if [ "$prev" = "-y" ]; then
    outdir=$(dirname "$arg")
    mkdir -p "$outdir"
    touch "$outdir/trickplay_001.webp"
    >&2 echo "[Parsed_showinfo_2 @ 0xdead] n: 0 pts: 0 pts_time:0.000 ..."
    >&2 echo "[Parsed_showinfo_2 @ 0xdead] n: 1 pts: 100000 pts_time:10.000 ..."
    exit 0
  fi
  prev="$arg"
done
exit 0
`)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.NoError(t, os.Chmod(f.Name(), 0755))
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}

func TestTrickplayGenerator_Generate_Success(t *testing.T) {
	t.Parallel()
	probe := makeFakeProbeScript(t, 25.0) // 25s → timestamps 0,10,20 → 3 thumbs
	ffmpeg := makeFakeFfmpegScript(t)
	outputDir := t.TempDir()

	g := NewTrickplayGenerator(ffmpeg, probe)
	result, err := g.Generate(context.Background(), "/fake/video.mkv", outputDir, 10, 160, 5, 5, 1, 0)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, outputDir, result.ImageDir)
	assert.Equal(t, 1, result.ImageCount)
}

func TestTrickplayGenerator_Generate_InvalidBinary(t *testing.T) {
	t.Parallel()
	probe := makeFakeProbeScript(t, 10.0)
	g := NewTrickplayGenerator("/nonexistent/ffmpeg-trickplay", probe)
	outputDir := t.TempDir()

	_, err := g.Generate(context.Background(), "/fake/video.mkv", outputDir, 10, 160, 5, 5, 1, 0)

	require.Error(t, err)
}

func TestTrickplayGenerator_Generate_InvalidProbe(t *testing.T) {
	t.Parallel()
	g := NewTrickplayGenerator("/nonexistent/ffmpeg", "/nonexistent/ffprobe")
	outputDir := t.TempDir()

	_, err := g.Generate(context.Background(), "/fake/video.mkv", outputDir, 10, 160, 5, 5, 1, 0)

	require.Error(t, err)
}

func TestTrickplayGenerator_Generate_CreatesOutputDir(t *testing.T) {
	t.Parallel()
	probe := makeFakeProbeScript(t, 10.0)
	ffmpeg := makeFakeFfmpegScript(t)
	base := t.TempDir()
	outputDir := filepath.Join(base, "nested", "trickplay")

	g := NewTrickplayGenerator(ffmpeg, probe)
	_, err := g.Generate(context.Background(), "/fake/video.mkv", outputDir, 10, 160, 5, 5, 1, 0)

	require.NoError(t, err)
	info, statErr := os.Stat(outputDir)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

// Trickplay thumbnails of an HDR / Dolby Vision source must be tone-mapped.
// The filter graph was select,scale,showinfo,tile - no colour handling at all -
// so a PQ (or DoVi P5, whose base layer is PQ-ish) source was sampled as if it
// were SDR: washed-out, dull, green/purple-tinted tiles. The player already
// tone-maps these files correctly; the scrubbing strip did not, which is why the
// thumbnails don't match the picture. Reported on a real DoVi P5 title.
func TestTrickplayFilter_TonemapsHDRSources(t *testing.T) {
	base := "select='isnan(prev_selected_t)+gte(t-prev_selected_t\\,10)',scale=320:-1,showinfo,tile=5x5"

	// SDR: untouched - no needless colour pass on the vast majority of files.
	if got := trickplayFilter(base, trickplayColour{}); got != base {
		t.Errorf("SDR filter must be unchanged:\n got %q\nwant %q", got, base)
	}

	for _, tc := range []struct {
		name string
		col  trickplayColour
	}{
		{"HDR10 (PQ)", trickplayColour{Transfer: "smpte2084"}},
		{"HLG", trickplayColour{Transfer: "arib-std-b67"}},
		// DoVi P5 tags NOTHING in the container (color_transfer=unknown): the
		// only signal is the DOVI configuration record. Missed → washed out.
		{"Dolby Vision P5, untagged container", trickplayColour{DoVi: true}},
	} {
		got := trickplayFilter(base, tc.col)
		if !strings.Contains(got, "tonemap") {
			t.Errorf("%s: filter must tone-map, got %q", tc.name, got)
		}
		if !strings.Contains(got, "setparams") {
			t.Errorf("%s: filter must tag the colour metadata first (rips under-tag), got %q", tc.name, got)
		}
		if !strings.Contains(got, "tile=5x5") {
			t.Errorf("%s: the tiling must survive, got %q", tc.name, got)
		}
	}
}
