package ffmpeg_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/adapters/driven/ffmpeg"
	"github.com/gravity-zero/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found, skipping integration test")
	}
}

func generateTestVideo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "test.mp4")
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "testsrc=duration=2:size=640x360:rate=25",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2",
		"-c:v", "libx264", "-preset", "ultrafast",
		"-c:a", "aac",
		"-shortest", "-y", out,
	)
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Run(), "failed to generate test video")
	return out
}

func TestBuildFFmpegArgs_ModeA_VideoCopy(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:       "/tmp/test",
		FilePath:        "/f.mkv",
		Profiles:        transcoding.DefaultHLSProfiles,
		VideoDecision:   "copy",
		AudioDecision:   "transcode",
		AudioTrackIndex: 0,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-c:v copy")
	assert.Contains(t, joined, "-c:a aac")
	assert.NotContains(t, joined, "filter_complex")
	assert.NotContains(t, joined, "libx264")
}

func TestBuildFFmpegArgs_ModeA_AllCopy(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles,
		VideoDecision: "copy",
		AudioDecision: "copy",
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-c:v copy")
	assert.Contains(t, joined, "-c:a copy")
}

func TestBuildFFmpegArgs_ModeB_SingleTranscode(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles,
		VideoDecision: "transcode",
		AudioDecision: "transcode",
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "libx264")
	assert.Contains(t, joined, "-c:a aac")
	assert.NotContains(t, joined, "split=")
	assert.NotContains(t, joined, "var_stream_map")
}

func TestBuildFFmpegArgs_MultiVariant(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:       "/tmp/test",
		FilePath:        "/f.mkv",
		Profiles:        transcoding.DefaultHLSProfiles,
		VideoDecision:   "",
		AudioDecision:   "",
		AudioTrackIndex: 0,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "[0:v]split=")
	assert.Contains(t, joined, "var_stream_map")
	assert.Contains(t, joined, "master.m3u8")
}

func TestBuildFFmpegArgs_SeekBeforeInput(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles,
		VideoDecision: "copy",
		AudioDecision: "transcode",
		StartSec:      42.5,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	ssIdx, iIdx := -1, -1
	for i, a := range args {
		if a == "-ss" && ssIdx < 0 {
			ssIdx = i
		}
		if a == "-i" && iIdx < 0 {
			iIdx = i
		}
	}
	require.Greater(t, ssIdx, -1)
	require.Greater(t, iIdx, -1)
	assert.Less(t, ssIdx, iIdx, "-ss must come before -i")
}

func TestBuildFFmpegArgs_AudioNormalize(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:      "/tmp/test",
		FilePath:       "/f.mkv",
		Profiles:       transcoding.DefaultHLSProfiles,
		VideoDecision:  "copy",
		AudioDecision:  "transcode",
		AudioNormalize: true,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "loudnorm=I=-16:TP=-1.5:LRA=11:linear=true")
}

func TestBuildFFmpegArgs_BurnIn(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:          "/tmp/test",
		FilePath:           "/f.mkv",
		Profiles:           transcoding.DefaultHLSProfiles,
		SubtitleTrackIndex: 2,
		SubtitleCodec:      "hdmv_pgs_subtitle",
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "[0:v][0:s:2]overlay[vb]")
	assert.Contains(t, joined, "[vb]split=")
}

// Integration test — requires ffmpeg binary
func TestHLSTranscoder_Integration_ModeA(t *testing.T) {
	requireFFmpeg(t)
	videoPath := generateTestVideo(t)
	outputDir := t.TempDir()

	transcoder := ffmpeg.NewHLSTranscoder()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	handle, err := transcoder.Start(ctx, transcoding.HLSProcessOpts{
		OutputDir:       outputDir,
		FilePath:        videoPath,
		Profiles:        transcoding.DefaultHLSProfiles[:1], // 360p only
		VideoDecision:   "copy",
		AudioDecision:   "copy",
		AudioTrackIndex: 0,
	})
	require.NoError(t, err)

	err = handle.WaitReady(ctx)
	require.NoError(t, err, "WaitReady should succeed")

	// master.m3u8 should exist (generated for single-stream)
	masterPath := filepath.Join(outputDir, "master.m3u8")
	assert.FileExists(t, masterPath)

	// stream_0.m3u8 should exist
	streamPath := filepath.Join(outputDir, "stream_0.m3u8")
	assert.FileExists(t, streamPath)

	// Wait for ffmpeg to finish
	select {
	case <-handle.Done():
	case <-time.After(15 * time.Second):
		t.Fatal("ffmpeg did not finish in time")
	}

	require.NoError(t, handle.Err())

	// Verify segments were created
	entries, _ := os.ReadDir(outputDir)
	tsCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".ts") {
			tsCount++
		}
	}
	assert.Positive(t, tsCount, "should have produced .ts segments")
}

func TestHLSTranscoder_Stop(t *testing.T) {
	requireFFmpeg(t)
	videoPath := generateTestVideo(t)
	outputDir := t.TempDir()

	transcoder := ffmpeg.NewHLSTranscoder()
	ctx := context.Background()

	handle, err := transcoder.Start(ctx, transcoding.HLSProcessOpts{
		OutputDir:     outputDir,
		FilePath:      videoPath,
		Profiles:      transcoding.DefaultHLSProfiles[:1],
		VideoDecision: "copy",
		AudioDecision: "copy",
	})
	require.NoError(t, err)

	err = handle.WaitReady(context.Background())
	require.NoError(t, err)

	err = handle.Stop()
	require.NoError(t, err)

	select {
	case <-handle.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("handle.Done() not closed after Stop")
	}
}
