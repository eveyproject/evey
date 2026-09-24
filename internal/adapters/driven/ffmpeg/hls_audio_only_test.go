package ffmpeg

import (
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// The encoder's priming sits before the first real sample. A video-timeline
// shift at least that long absorbs it; below that the muxer moves the whole
// track late by what did not fit (content-measured 42.6 ms late on a copy rung
// with libfdk_aac), so the filter graph drops that much input, to the sample.
func TestBuildFFmpegAudioOnlyArgs_DropsThePrimingTheShiftCannotAbsorb(t *testing.T) {
	trimOf := func(o transcoding.AudioProcessOpts) string {
		a := BuildFFmpegAudioOnlyArgs(o)
		for i := 0; i+1 < len(a); i++ {
			if a[i] == "-af" {
				if j := strings.Index(a[i+1], "atrim=start="); j >= 0 {
					return strings.SplitN(a[i+1][j+len("atrim=start="):], ",", 2)[0]
				}
			}
		}
		return ""
	}
	base := transcoding.AudioProcessOpts{FilePath: "/m.mkv", OutputDir: "/tmp/o", AudioCodec: "aac", AudioBitrate: 192, MaxAudioChannels: 2, SourceAudioChannels: 6}
	cases := []struct {
		name    string
		encoder string
		start   float64
		delay   float64
		want    string
	}{
		{"libfdk, no shift: the whole priming", PreferredAACEncoder, 0, 0, "0.042667"},
		{"native, no shift", NativeAACEncoder, 0, 0, "0.021333"},
		{"shift past the priming: nothing to drop", PreferredAACEncoder, 0, 0.125, ""},
		{"shift shorter than the priming: the rest", PreferredAACEncoder, 0, 0.0417, "0.000967"},
		{"a seeked rafale drops it after its seek", PreferredAACEncoder, 6, 0, "0.042667"},
	}
	for _, c := range cases {
		o := base
		o.AudioEncoder, o.StartSec, o.VideoCompositionDelay = c.encoder, c.start, c.delay
		if got := trimOf(o); got != c.want {
			t.Errorf("%s: atrim start %q, want %q", c.name, got, c.want)
		}
	}
	cp := base
	cp.AudioCodec = "copy"
	if got := trimOf(cp); got != "" {
		t.Errorf("a passthrough has no priming: atrim %q", got)
	}
}
