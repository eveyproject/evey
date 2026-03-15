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

// makeFakeFfmpegScript creates a fake ffmpeg that:
//   - In thumbnail mode (-ss flag present): creates the file passed after -y.
//   - In tile mode (-f concat): creates trickplay_001.jpg in the output dir.
func makeFakeFfmpegScript(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp("", "fake-ffmpeg-trickplay-*.sh")
	require.NoError(t, err)
	_, err = f.WriteString(`#!/bin/sh
# Detect thumbnail extraction vs tile step.
is_thumb=0
for arg; do
  [ "$arg" = "-ss" ] && is_thumb=1
done
if [ "$is_thumb" = "1" ]; then
  # Find file after -y flag and create it.
  prev=""
  for arg; do
    [ "$prev" = "-y" ] && touch "$arg" && break
    prev="$arg"
  done
else
  # Tile step: last arg is output pattern, create trickplay_001.jpg.
  for outpat; do true; done
  outdir=$(dirname "$outpat")
  touch "$outdir/trickplay_001.jpg"
fi
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
	result, err := g.Generate(context.Background(), "/fake/video.mkv", outputDir, 10, 160, 5, 5)

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

	_, err := g.Generate(context.Background(), "/fake/video.mkv", outputDir, 10, 160, 5, 5)

	require.Error(t, err)
}

func TestTrickplayGenerator_Generate_InvalidProbe(t *testing.T) {
	t.Parallel()
	g := NewTrickplayGenerator("/nonexistent/ffmpeg", "/nonexistent/ffprobe")
	outputDir := t.TempDir()

	_, err := g.Generate(context.Background(), "/fake/video.mkv", outputDir, 10, 160, 5, 5)

	require.Error(t, err)
}

func TestTrickplayGenerator_Generate_CreatesOutputDir(t *testing.T) {
	t.Parallel()
	probe := makeFakeProbeScript(t, 10.0)
	ffmpeg := makeFakeFfmpegScript(t)
	base := t.TempDir()
	outputDir := filepath.Join(base, "nested", "trickplay")

	g := NewTrickplayGenerator(ffmpeg, probe)
	_, err := g.Generate(context.Background(), "/fake/video.mkv", outputDir, 10, 160, 5, 5)

	require.NoError(t, err)
	info, statErr := os.Stat(outputDir)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}
