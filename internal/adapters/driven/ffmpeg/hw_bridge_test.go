package ffmpeg

import (
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// A bridged session must not put the GPU deinterlacer in front of hwdownload:
// on progressive content yadif_cuda / deinterlace_vaapi=auto pass the decoder's
// own frame through and hwdownload refuses it (measured, RTX 4070 Ti,
// 2026-09-12). The deinterlace runs on the CPU inside the bridge instead.
func TestBridge_NoGPUDeinterlacerBeforeDownload(t *testing.T) {
	for _, accel := range []transcoding.HWAccelType{transcoding.NVENC, transcoding.VAAPI, transcoding.QSV} {
		opts := transcoding.HLSProcessOpts{
			FilePath: "/m.mkv", OutputDir: "/o", VideoDecision: "transcode", AudioTrackIndex: -1, SubtitleTrackIndex: -1,
			Profiles: []transcoding.HLSProfile{{Width: 1920, Height: 1080, VideoBitrate: 5000, AudioBitrate: 192}},
			HWAccel:  accel, ToneMap: true, HWTonemap: false, DeinterlaceAll: true,
		}
		if !hwBridges(opts) {
			t.Fatalf("%s: expected a bridged session", accel)
		}
		if got := hwSourceFilterPrefix(opts); got != "" {
			t.Errorf("%s: GPU prefix %q emitted in front of the bridge", accel, got)
		}
		vf := strings.Join(BuildFFmpegArgs(opts), " ")
		if !strings.Contains(vf, "-vf hwdownload,format=p010le,bwdif=mode=send_frame:deint=all,") {
			t.Errorf("%s: bridge must download first then deinterlace on the CPU (idet verdict honoured):\n%s", accel, vf)
		}
		if strings.Contains(vf, "yadif_cuda") || strings.Contains(vf, "deinterlace_vaapi") {
			t.Errorf("%s: GPU deinterlacer present in a bridged chain:\n%s", accel, vf)
		}
	}
	// The GPU tone-map route keeps its GPU deinterlacer: nothing is downloaded there.
	opts := transcoding.HLSProcessOpts{
		FilePath: "/m.mkv", OutputDir: "/o", VideoDecision: "transcode", AudioTrackIndex: -1, SubtitleTrackIndex: -1,
		Profiles: []transcoding.HLSProfile{{Width: 1920, Height: 1080, VideoBitrate: 5000, AudioBitrate: 192}},
		HWAccel:  transcoding.VAAPI, ToneMap: true, HWTonemap: true,
	}
	if hwBridges(opts) {
		t.Fatal("a GPU tone-map session is not a bridge")
	}
	if got := hwSourceFilterPrefix(opts); got != "deinterlace_vaapi=auto=1," {
		t.Errorf("GPU tone-map route lost its deinterlacer: %q", got)
	}
}
