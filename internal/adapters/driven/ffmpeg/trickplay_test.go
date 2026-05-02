package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
// parser has something to consume — without these, Generate fails on
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
	result, err := g.Generate(context.Background(), "/fake/video.mkv", outputDir, 10, 160, 5, 5, 1)

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

	_, err := g.Generate(context.Background(), "/fake/video.mkv", outputDir, 10, 160, 5, 5, 1)

	require.Error(t, err)
}

func TestTrickplayGenerator_Generate_InvalidProbe(t *testing.T) {
	t.Parallel()
	g := NewTrickplayGenerator("/nonexistent/ffmpeg", "/nonexistent/ffprobe")
	outputDir := t.TempDir()

	_, err := g.Generate(context.Background(), "/fake/video.mkv", outputDir, 10, 160, 5, 5, 1)

	require.Error(t, err)
}

func TestTrickplayGenerator_Generate_CreatesOutputDir(t *testing.T) {
	t.Parallel()
	probe := makeFakeProbeScript(t, 10.0)
	ffmpeg := makeFakeFfmpegScript(t)
	base := t.TempDir()
	outputDir := filepath.Join(base, "nested", "trickplay")

	g := NewTrickplayGenerator(ffmpeg, probe)
	_, err := g.Generate(context.Background(), "/fake/video.mkv", outputDir, 10, 160, 5, 5, 1)

	require.NoError(t, err)
	info, statErr := os.Stat(outputDir)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

func TestTrickplayWorkers_Adaptive(t *testing.T) {
	t.Parallel()
	n := TrickplayWorkers(true)
	assert.GreaterOrEqual(t, n, 1, "minimum 1 worker")
	assert.LessOrEqual(t, n, 4, "maximum 4 workers")
}

func TestTrickplayWorkers_NonAdaptive(t *testing.T) {
	t.Parallel()
	n := TrickplayWorkers(false)
	assert.GreaterOrEqual(t, n, 1, "minimum 1 worker")
	assert.LessOrEqual(t, n, 4, "maximum 4 workers")
}

func TestTrickplayWorkers_AdaptiveVsNonAdaptive(t *testing.T) {
	t.Parallel()
	adaptive := TrickplayWorkers(true)
	nonAdaptive := TrickplayWorkers(false)
	// non-adaptive uses 50% cores vs adaptive 25%, so non-adaptive >= adaptive
	assert.GreaterOrEqual(t, nonAdaptive, adaptive, "non-adaptive should use at least as many workers as adaptive")
}
