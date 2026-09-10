package ffmpeg

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

func BenchmarkBuildFFmpegArgs(b *testing.B) {
	opts := sampleHWOpts(transcoding.VAAPI, "/dev/dri/renderD128", 3)
	opts.ToneMap, opts.HWTonemap = true, true
	for b.Loop() {
		BuildFFmpegArgs(opts)
	}
}
