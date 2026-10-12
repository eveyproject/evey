package ffmpeg

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// Every HLS session's stderr feeds ClassifyHWFailure; the jellyfin-ffmpeg
// banner lists --enable-vulkan/--enable-opencl/--enable-libplacebo, which used
// to turn a dead device into a route verdict. -hide_banner leads every argv.
func TestHLSSessionArgs_HideBanner(t *testing.T) {
	sessions := goldenSessions()
	sessions["sw-single-progress-log"] = goldenOpts(func(o *transcoding.HLSProcessOpts) { o.ProgressLogPath = "/cache/h/progress.log" })
	for name, opts := range sessions {
		args := BuildFFmpegArgs(opts)
		if len(args) == 0 || args[0] != "-hide_banner" {
			t.Errorf("%s: argv must open with -hide_banner, got %v", name, head(args))
		}
	}
	audio := map[string]transcoding.AudioProcessOpts{
		"audio-transcode": {FilePath: "/m.mkv", OutputDir: "/tmp/o", AudioCodec: "aac", AudioBitrate: 192, MaxAudioChannels: 2, SourceAudioChannels: 6},
		"audio-seek":      {FilePath: "/m.mkv", OutputDir: "/tmp/o", AudioCodec: "aac", AudioBitrate: 192, MaxAudioChannels: 2, StartSec: 600},
		"audio-silence":   {OutputDir: "/tmp/o", AudioCodec: "aac", MaxAudioChannels: 6, SilenceDurationSec: 3},
	}
	for name, opts := range audio {
		args := BuildFFmpegAudioOnlyArgs(opts)
		if len(args) == 0 || args[0] != "-hide_banner" {
			t.Errorf("%s: argv must open with -hide_banner, got %v", name, head(args))
		}
	}
}

func head(a []string) []string {
	if len(a) > 4 {
		return a[:4]
	}
	return a
}
