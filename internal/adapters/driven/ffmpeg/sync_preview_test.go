package ffmpeg

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appmaintenance "github.com/eveyproject/evey/internal/app/maintenance"
	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// fakeClipFFmpeg records every argv it was called with (one file per call,
// numbered) and writes a non-empty clip at its last argument. With failHW it
// exits non-zero whenever the argv carries a GPU decode flag, emulating a GPU
// session that fails to start.
func fakeClipFFmpeg(t *testing.T, failHW bool) (bin, argsDir string) {
	t.Helper()
	argsDir = t.TempDir()
	body := "#!/bin/sh\nn=$(ls " + argsDir + " | wc -l)\nprintf '%s\\n' \"$@\" > " + argsDir + "/call_$n\n"
	if failHW {
		body += "for a; do case \"$a\" in -hwaccel|-vaapi_device) echo 'Device creation failed' >&2; exit 1;; esac; done\n"
	}
	body += "for a; do out=$a; done\nprintf 'mp4' > \"$out\"\n"
	return writeFakeBinary(t, filepath.Join(t.TempDir(), "ffmpeg"), body), argsDir
}

func recordedCalls(t *testing.T, argsDir string) [][]string {
	t.Helper()
	entries, err := os.ReadDir(argsDir)
	require.NoError(t, err)
	var calls [][]string
	for _, e := range entries {
		raw, err := os.ReadFile(argsDir + "/" + e.Name())
		require.NoError(t, err)
		calls = append(calls, strings.Split(strings.TrimSpace(string(raw)), "\n"))
	}
	return calls
}

func hasArg(args []string, want string) bool { return slices.Contains(args, want) }

// A preview of a 10-bit source came out as H.264 High 10 - libx264 keeps the
// source depth unless told otherwise - and Firefox on Windows reported the
// clip as a corrupt file. The software route must always land on 8-bit 4:2:0.
func TestSyncPreview_SoftwareClipIsEightBit(t *testing.T) {
	t.Parallel()
	bin, argsDir := fakeClipFFmpeg(t, false)
	hdr := transcoding.PicturePlan{Codec: "h264", Topology: transcoding.TopologySoftwareHDR, ToneMap: true, HDRTransfer: "pq"}

	data, err := NewSyncPreview(bin).RenderSyncPreview(context.Background(),
		appmaintenance.SyncPreviewRequest{Path: "/m/film.mkv", AtSec: 600, LenSec: 20, Picture: hdr})
	require.NoError(t, err)
	assert.Equal(t, "mp4", string(data))

	calls := recordedCalls(t, argsDir)
	require.Len(t, calls, 1, "one ffmpeg run, no retry on a software route")
	args := calls[0]
	assert.True(t, hasArg(args, "-pix_fmt") && hasArg(args, "yuv420p"), "8-bit 4:2:0 pinned: %v", args)
	assert.True(t, hasArg(args, "libx264"))
	assert.False(t, hasArg(args, "-hwaccel"))
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "tonemap", "the HDR picture is folded to SDR like the player's")
	assert.Contains(t, joined, "-map_chapters -1")
}

// The zero picture (no resolver wired, streams unreadable) still renders: in
// software, uncorrected - what the tool did before it learned about colour.
func TestSyncPreview_ZeroPictureRendersSoftware(t *testing.T) {
	t.Parallel()
	bin, argsDir := fakeClipFFmpeg(t, false)
	_, err := NewSyncPreview(bin).RenderSyncPreview(context.Background(),
		appmaintenance.SyncPreviewRequest{Path: "/m/film.mkv", AtSec: 600, LenSec: 20})
	require.NoError(t, err)
	calls := recordedCalls(t, argsDir)
	require.Len(t, calls, 1)
	assert.True(t, hasArg(calls[0], "libx264"))
	assert.NotContains(t, strings.Join(calls[0], " "), "tonemap")
}

// A GPU route that fails to start is retried in software: the operator must
// see the clip, and a preview never demotes the host's calibration.
func TestSyncPreview_GPUFailureFallsBackToSoftware(t *testing.T) {
	t.Parallel()
	bin, argsDir := fakeClipFFmpeg(t, true)
	gpu := transcoding.PicturePlan{
		Codec: "h264", Accel: transcoding.VAAPI, Device: "/dev/dri/renderD128",
		Topology: transcoding.TopologyZeroCopy,
	}
	data, err := NewSyncPreview(bin).RenderSyncPreview(context.Background(),
		appmaintenance.SyncPreviewRequest{Path: "/m/film.mkv", AtSec: 600, LenSec: 20, Picture: gpu})
	require.NoError(t, err)
	assert.Equal(t, "mp4", string(data))

	calls := recordedCalls(t, argsDir)
	require.Len(t, calls, 2, "the GPU attempt, then the software retry")
	assert.True(t, hasArg(calls[0], "-hwaccel"), "first run takes the player's GPU route: %v", calls[0])
	assert.True(t, hasArg(calls[0], "h264_vaapi"))
	assert.False(t, hasArg(calls[1], "-hwaccel"), "the retry is all software: %v", calls[1])
	assert.True(t, hasArg(calls[1], "libx264"))
	assert.True(t, hasArg(calls[1], "yuv420p"))
}

// A GPU route that works is not retried, and the audio input never carries
// the decode flags (they bind to the video -i they precede).
func TestSyncPreview_GPURouteRunsOnce(t *testing.T) {
	t.Parallel()
	bin, argsDir := fakeClipFFmpeg(t, false)
	gpu := transcoding.PicturePlan{Codec: "h264", Accel: transcoding.VAAPI, Device: "/dev/dri/renderD128", Topology: transcoding.TopologyZeroCopy}
	_, err := NewSyncPreview(bin).RenderSyncPreview(context.Background(),
		appmaintenance.SyncPreviewRequest{Path: "/m/film.mkv", AtSec: 600, LenSec: 20, Track: 1, ShiftSec: 1.5, Picture: gpu})
	require.NoError(t, err)
	calls := recordedCalls(t, argsDir)
	require.Len(t, calls, 1)
	args := calls[0]
	var seeks []string
	firstInput := -1
	for i, a := range args {
		if a == "-ss" && i+1 < len(args) {
			seeks = append(seeks, args[i+1])
		}
		if a == "-i" && firstInput < 0 {
			firstInput = i
		}
	}
	assert.Equal(t, []string{"600.000", "598.500"}, seeks)
	hwIdx := -1
	for i, a := range args {
		if a == "-hwaccel" {
			hwIdx = i
		}
	}
	assert.Greater(t, hwIdx, -1)
	assert.Less(t, hwIdx, firstInput, "the hw flags precede the video input only")
	assert.Equal(t, 1, strings.Count(strings.Join(args, " "), "-hwaccel vaapi"))
	assert.Contains(t, strings.Join(args, " "), "-map 1:a:1")
}
