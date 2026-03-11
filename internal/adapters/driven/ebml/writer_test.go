package ebml

import (
	"context"
	"os"
	"testing"

	"github.com/gravity-zero/evey/internal/domain/mkv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeFakeFFmpegScript(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp("", "fake-ffmpeg-ebml-*.sh")
	require.NoError(t, err)
	_, err = f.WriteString("#!/bin/sh\nexit 0\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.NoError(t, os.Chmod(f.Name(), 0755))
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}

func TestWriter_Mux_InvalidBinary(t *testing.T) {
	w := NewWriter("/nonexistent/ffmpeg")
	opts := mkv.MuxOptions{
		OutputPath: "/tmp/out.mkv",
		Tracks: []mkv.TrackInput{
			{SourcePath: "/tmp/video.mkv"},
		},
	}
	err := w.Mux(context.Background(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ffmpeg mux")
}

func TestWriter_Demux_InvalidBinary(t *testing.T) {
	w := NewWriter("/nonexistent/ffmpeg")
	opts := mkv.DemuxOptions{
		OutputDir: t.TempDir(),
		TrackIDs:  []uint64{0},
	}
	err := w.Demux(context.Background(), "/tmp/input.mkv", opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ffmpeg demux")
}

func TestWriter_Mux_WithFakeScript(t *testing.T) {
	script := makeFakeFFmpegScript(t)
	w := NewWriter(script)
	opts := mkv.MuxOptions{
		OutputPath: "/tmp/out.mkv",
		Tracks: []mkv.TrackInput{
			{SourcePath: "/tmp/video.mkv", Language: "eng", Name: "Main Video"},
			{SourcePath: "/tmp/audio.mkv", Language: "fra", Name: "French Audio"},
		},
	}
	err := w.Mux(context.Background(), opts)
	assert.NoError(t, err)
}

func TestWriter_Demux_WithFakeScript(t *testing.T) {
	script := makeFakeFFmpegScript(t)
	w := NewWriter(script)
	opts := mkv.DemuxOptions{
		OutputDir: t.TempDir(),
		TrackIDs:  []uint64{0, 1},
	}
	err := w.Demux(context.Background(), "/tmp/input.mkv", opts)
	assert.NoError(t, err)
}

func TestWriter_AddTrack_InvalidBinary(t *testing.T) {
	w := NewWriter("/nonexistent/ffmpeg")
	input := mkv.TrackInput{SourcePath: "/tmp/audio.mkv", Language: "eng"}
	err := w.AddTrack(context.Background(), "/tmp/container.mkv", input)
	require.Error(t, err)
}

func TestWriter_RemoveTrack_InvalidBinary(t *testing.T) {
	w := NewWriter("/nonexistent/ffmpeg")
	err := w.RemoveTrack(context.Background(), "/tmp/container.mkv", 1)
	require.Error(t, err)
}
