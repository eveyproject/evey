package ffmpeg

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// framed4x3 is the ladder a 640x480 source is served under a 360 cap and
// without it: the 360p box drawn 480x360, the 480p box at the source's own
// 640x480 (transcoding.RungFrame). The scaler must draw exactly those, on
// every path - a width pin made the 360p box 640x480 and the shrunk-to-nothing
// 480p box 854x640.
var framed4x3 = []transcoding.HLSProfile{
	{Name: "360p", Width: 640, Height: 360, FrameWidth: 480, FrameHeight: 360, VideoBitrate: 800, AudioBitrate: 128},
	{Name: "480p", Width: 854, Height: 480, FrameWidth: 640, FrameHeight: 480, VideoBitrate: 1400, AudioBitrate: 192},
}

func softwareFrameOpts(profiles []transcoding.HLSProfile) transcoding.HLSProcessOpts {
	return transcoding.HLSProcessOpts{
		OutputDir:          "/out",
		FilePath:           "/in.mkv",
		Profiles:           profiles,
		VideoDecision:      "transcode",
		AudioDecision:      "transcode",
		SubtitleTrackIndex: -1,
		VideoCodec:         "h264",
		AudioCodec:         "aac",
	}
}

func TestSoftwareScaler_PinsTheFramedPicture(t *testing.T) {
	single := strings.Join(buildSingleTranscodeArgs(softwareFrameOpts(framed4x3[:1])), " ")
	assert.Contains(t, single, "scale=480:360,setsar=1")
	assert.NotContains(t, single, "scale=640:-2")

	multi := strings.Join(buildMultiVariantArgs(softwareFrameOpts(framed4x3)), " ")
	assert.Contains(t, multi, "[v0]scale=480:360,setsar=1")
	assert.Contains(t, multi, "[v1]scale=640:480,setsar=1")
	assert.NotContains(t, multi, ":-2")

	// An anamorphic source's frame already carries the display aspect: the
	// pin replaces the storage-ratio expression, never stacks on it.
	sar := softwareFrameOpts(framed4x3[:1])
	sar.SourceSAR = "16:15"
	assert.Equal(t, "scale=480:360,setsar=1", softwareRungScaleFilter(sar, framed4x3[0]))
}

// The symmetric case: a rung the ladder did not frame (a hand-built profile,
// an unknown source) keeps the width pin it always had, bytes unchanged.
func TestSoftwareScaler_UnframedRungKeepsTheWidthPin(t *testing.T) {
	box := transcoding.HLSProfile{Name: "360p", Width: 640, Height: 360}
	assert.Equal(t, "scale=640:-2,setsar=1", softwareRungScaleFilter(softwareFrameOpts(nil), box))
	sar := softwareFrameOpts(nil)
	sar.SourceSAR = "64:45"
	assert.Equal(t, scaleFilterFor(sar, 640), softwareRungScaleFilter(sar, box))
}

func TestSoftwareToneMap_FoldsAtTheFrameWidthAndPinsTheHeight(t *testing.T) {
	opts := softwareFrameOpts(framed4x3[:1])
	opts.ToneMap = true
	single := strings.Join(buildSingleTranscodeArgs(opts), " ")
	if strings.Contains(single, "tonemapx") {
		assert.Contains(t, single, "scale=480:-2,tonemapx")
	} else {
		assert.Contains(t, single, "zscale=w=480:h=-2:t=linear")
	}
	assert.Contains(t, single, "format=yuv420p,scale=480:360,setsar=1", "the frame's height is pinned after the fold")

	opts.Profiles = framed4x3
	multi := strings.Join(buildMultiVariantArgs(opts), " ")
	if strings.Contains(multi, "tonemapx") {
		assert.Contains(t, multi, "scale=640:-2,tonemapx", "the shared fold runs at the widest FRAME, not the 854 box")
	} else {
		assert.Contains(t, multi, "zscale=w=640:h=-2:t=linear", "the shared fold runs at the widest FRAME, not the 854 box")
	}
	assert.Contains(t, multi, "[v0]scale=480:360,setsar=1")
}

func TestSoftwareBurnIn_PinsTheFramedPicture(t *testing.T) {
	opts := softwareFrameOpts(framed4x3[:1])
	opts.SubtitleTrackIndex = 2
	opts.SubtitleCodec = "hdmv_pgs_subtitle"
	single := strings.Join(buildSingleTranscodeArgs(opts), " ")
	assert.Contains(t, single, "overlay,scale=480:360,setsar=1")
}

// Every GPU scaler draws the same frame the software one does, both sides
// pinned (scale_qsv's trunc expression rounded a 1920x800 source to 1280x532
// where every other scaler drew 1280x534 - one picture, not one per backend).
func TestHWScaler_PinsTheFramedPicture(t *testing.T) {
	for _, tt := range []struct {
		accel     transcoding.HWAccelType
		rung0     string
		rung1     string
		passthru0 string
		unframed0 string
	}{
		{transcoding.NVENC, "scale_cuda=480:360:format=nv12", "scale_cuda=640:480:format=nv12", "scale_cuda=480:360:format=p010", "scale_cuda=640:-2:format=nv12"},
		{transcoding.VAAPI, "scale_vaapi=480:360:format=nv12", "scale_vaapi=640:480:format=nv12", "scale_vaapi=480:360:format=p010", "scale_vaapi=640:-2:format=nv12"},
		{transcoding.QSV, "scale_qsv=w=480:h=360:format=nv12", "scale_qsv=w=640:h=480:format=nv12", "scale_qsv=w=480:h=360:format=p010", "scale_qsv=w=640:h=trunc(640*ih/iw/2)*2:format=nv12"},
		{transcoding.RKMPP, "scale_rkrga=w=480:h=360:format=nv12", "scale_rkrga=w=640:h=480:format=nv12", "scale_rkrga=w=480:h=360:format=p010", "scale_rkrga=w=640:h=-2:format=nv12"},
	} {
		t.Run(string(tt.accel), func(t *testing.T) {
			opts := sampleHWOpts(tt.accel, "/dev/dri/renderD128", 2)
			opts.Profiles = framed4x3
			multi := strings.Join(buildMultiVariantHWArgs(opts), " ")
			assert.Contains(t, multi, tt.rung0)
			assert.Contains(t, multi, tt.rung1)

			opts.Profiles = framed4x3[:1]
			single := strings.Join(buildSingleTranscodeHWArgs(opts), " ")
			assert.Contains(t, single, tt.rung0)

			opts.HDRPassthrough = true
			w, h := rungPicture(framed4x3[0])
			assert.Equal(t, tt.passthru0, hwRungScaleFilter(opts, w, h))

			opts.HDRPassthrough = false
			w, h = rungPicture(transcoding.HLSProfile{Width: 640, Height: 360})
			assert.Equal(t, tt.unframed0, hwRungScaleFilter(opts, w, h), "an unframed rung keeps its width pin")
		})
	}
}
