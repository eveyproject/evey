package ffmpeg

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

func TestBuildClipArgs_SoftwareSDR(t *testing.T) {
	spec := ClipSpec{
		Path: "/media/movie.mkv", AtSec: 10, LenSec: 5,
		AudioTrack: 0, Width: 854, Out: "/tmp/out.mp4",
	}
	args, err := BuildClipArgs(spec)
	require.NoError(t, err)

	assert.Equal(t, "10.000", argValue(args, "-ss"))
	assert.Equal(t, "5.000", argValue(args, "-t"))
	assert.Contains(t, args, "-map_chapters")
	assert.Equal(t, "-1", argValue(args, "-map_chapters"))
	assert.Contains(t, argValue(args, "-vf"), "scale=854:-2,setsar=1")
	assert.Equal(t, "libx264", argValue(args, "-c:v"))
	assert.Equal(t, "yuv420p", argValue(args, "-pix_fmt"))
	assert.Equal(t, "high", argValue(args, "-profile:v"))
	assert.Equal(t, "/tmp/out.mp4", args[len(args)-1])
	assert.NotContains(t, args, "-hwaccel")
}

func TestBuildClipArgs_SoftwareHDRFoldsScaleIntoTonemap(t *testing.T) {
	spec := ClipSpec{
		Path: "/media/hdr.mkv", AtSec: 0, LenSec: 5, Width: 854, Out: "/tmp/out.mp4",
		Picture: transcoding.PicturePlan{Codec: "h264", ToneMap: true, Topology: transcoding.TopologySoftwareHDR},
	}
	args, err := BuildClipArgs(spec)
	require.NoError(t, err)

	vf := argValue(args, "-vf")
	assert.Contains(t, vf, "tonemap")
	assert.True(t, strings.HasSuffix(vf, "setsar=1"), "vf must end on setsar=1, got %q", vf)
	// Exactly one setsar=1: the downscale is folded inside the tonemap's own
	// linearizing zscale, not appended as a separate trailing scale+setsar.
	assert.Equal(t, 1, strings.Count(vf, "setsar=1"))
}

func TestBuildClipArgs_AnamorphicSAR(t *testing.T) {
	spec := ClipSpec{
		Path: "/media/pal.mkv", AtSec: 0, LenSec: 5, Width: 640, Out: "/tmp/out.mp4",
		Picture: transcoding.PicturePlan{Codec: "h264", SourceSAR: "64:45"},
	}
	args, err := BuildClipArgs(spec)
	require.NoError(t, err)

	assert.Contains(t, argValue(args, "-vf"), "scale=640:'trunc(640*ih*45/(iw*64)/2)*2',setsar=1")
}

func TestBuildClipArgs_VAAPIZeroCopy(t *testing.T) {
	spec := ClipSpec{
		Path: "/media/movie.mkv", AtSec: 0, LenSec: 5, Width: 854, Out: "/tmp/out.mp4",
		Picture: transcoding.PicturePlan{
			Codec: "h264", Accel: transcoding.VAAPI, Device: "/dev/dri/renderD128",
			Topology: transcoding.TopologyZeroCopy,
		},
	}
	args, err := BuildClipArgs(spec)
	require.NoError(t, err)

	firstI := firstIndex(args, "-i")
	require.GreaterOrEqual(t, firstI, 0)
	prologue := args[:firstI]
	assert.Contains(t, prologue, "-hwaccel")
	assert.Equal(t, "vaapi", argValue(prologue, "-hwaccel"))
	assert.Equal(t, "vaapi", argValue(prologue, "-hwaccel_output_format"))
	assert.Equal(t, "/dev/dri/renderD128", argValue(prologue, "-hwaccel_device"))

	// The audio input (the second -i) must carry none of the hw flags.
	secondI := firstIndex(args[firstI+1:], "-i")
	require.GreaterOrEqual(t, secondI, 0)
	between := args[firstI+1 : firstI+1+secondI]
	assert.NotContains(t, between, "-hwaccel")

	vf := argValue(args, "-vf")
	assert.Contains(t, vf, "scale_vaapi=854:-2:format=nv12")
	assert.Equal(t, "h264_vaapi", argValue(args, "-c:v"))
	assert.Equal(t, "VBR", argValue(args, "-rc_mode"))
	assert.NotContains(t, args, "-pix_fmt")
}

func TestBuildClipArgs_VAAPIGPUToneMap(t *testing.T) {
	spec := ClipSpec{
		Path: "/media/hdr.mkv", AtSec: 0, LenSec: 5, Width: 854, Out: "/tmp/out.mp4",
		Picture: transcoding.PicturePlan{
			Codec: "h264", Accel: transcoding.VAAPI, Device: "/dev/dri/renderD128",
			Topology: transcoding.TopologyGPUToneMap, ToneMap: true,
			Features: []string{transcoding.FeatureTonemapOpenCL},
		},
	}
	args, err := BuildClipArgs(spec)
	require.NoError(t, err)

	assert.Contains(t, argValue(args, "-vf"), hwOpenCLTonemapChain)
}

func TestBuildClipArgs_VAAPIDecoupled(t *testing.T) {
	spec := ClipSpec{
		Path: "/media/movie.mkv", AtSec: 0, LenSec: 5, Width: 854, Out: "/tmp/out.mp4",
		Picture: transcoding.PicturePlan{
			Codec: "h264", Accel: transcoding.VAAPI, Device: "/dev/dri/renderD128",
			Topology: transcoding.TopologyDecoupled,
		},
	}
	args, err := BuildClipArgs(spec)
	require.NoError(t, err)

	assert.Equal(t, "/dev/dri/renderD128", argValue(args, "-vaapi_device"))
	assert.NotContains(t, args, "-hwaccel")
	assert.Contains(t, argValue(args, "-vf"), "format=nv12,hwupload")
}

func TestBuildClipArgs_AudioShiftAndTrack(t *testing.T) {
	spec := ClipSpec{
		Path: "/media/movie.mkv", AtSec: 600, LenSec: 5, AudioTrack: 2, AudioShiftSec: 1.5,
		Width: 854, Out: "/tmp/out.mp4",
	}
	args, err := BuildClipArgs(spec)
	require.NoError(t, err)

	// Two -ss occurrences: the video seek, then the audio seek shifted back.
	var seeks []string
	for i, a := range args {
		if a == "-ss" && i+1 < len(args) {
			seeks = append(seeks, args[i+1])
		}
	}
	require.Len(t, seeks, 2)
	assert.Equal(t, "600.000", seeks[0])
	assert.Equal(t, "598.500", seeks[1])
	assert.Contains(t, args, "1:a:2")
}

func TestBuildClipArgs_RefusesInexpressibleRoute(t *testing.T) {
	// NVENC has no wired upload chain (hwUploadFilter), so the decoupled
	// topology it is handed here is a route Expressibility must refuse.
	spec := ClipSpec{
		Path: "/media/movie.mkv", AtSec: 0, LenSec: 5, Width: 854, Out: "/tmp/out.mp4",
		Picture: transcoding.PicturePlan{
			Codec: "h264", Accel: transcoding.NVENC, Device: "0",
			Topology: transcoding.TopologyDecoupled,
		},
	}
	args, err := BuildClipArgs(spec)
	require.Error(t, err)
	assert.Nil(t, args)
}

// firstIndex returns the index of the first occurrence of v in s, or -1.
func firstIndex(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}
