package ffmpeg

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeFakeTrickplayScript(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp("", "fake-ffmpeg-trickplay-*.sh")
	require.NoError(t, err)
	// Last arg is the output pattern like /some/dir/trickplay_%03d.jpg
	// Extract the dir from it and create two dummy jpg files.
	_, err = f.WriteString("#!/bin/sh\n" +
		"for outpat; do true; done\n" +
		"outdir=$(dirname \"$outpat\")\n" +
		"touch \"$outdir/trickplay_001.jpg\"\n" +
		"touch \"$outdir/trickplay_002.jpg\"\n" +
		"exit 0\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.NoError(t, os.Chmod(f.Name(), 0755))
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}

func TestTrickplayGenerator_Generate_Success(t *testing.T) {
	script := makeFakeTrickplayScript(t)
	outputDir := t.TempDir()

	g := NewTrickplayGenerator(script)
	result, err := g.Generate(context.Background(), "/fake/video.mkv", outputDir, 10, 160, 5, 5)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, outputDir, result.ImageDir)
	assert.Equal(t, 2, result.ImageCount)
}

func TestTrickplayGenerator_Generate_InvalidBinary(t *testing.T) {
	g := NewTrickplayGenerator("/nonexistent/ffmpeg-trickplay")
	outputDir := t.TempDir()

	_, err := g.Generate(context.Background(), "/fake/video.mkv", outputDir, 10, 160, 5, 5)

	require.Error(t, err)
}

func TestTrickplayGenerator_Generate_CreatesOutputDir(t *testing.T) {
	script := makeFakeTrickplayScript(t)
	base := t.TempDir()
	outputDir := filepath.Join(base, "nested", "trickplay")

	g := NewTrickplayGenerator(script)
	_, err := g.Generate(context.Background(), "/fake/video.mkv", outputDir, 10, 160, 5, 5)

	require.NoError(t, err)
	info, statErr := os.Stat(outputDir)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}
