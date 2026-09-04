package ffmpeg_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/adapters/driven/ffmpeg"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/reduceddl"
)

// Every server-side path that folds a multichannel bed to stereo must bound the
// channel matrix. Without it swr sums FL + clev*FC + slev*BL into a float
// encoder and never normalises: measured through this very argv, +2.67 dBFS
// with 816 clipped samples on a 5.1 AC-3 bed mastered at -1 dBFS per channel
// (audio_downmix_integration_test.go).
func TestStereoFold_CarriesMatrixHeadroom(t *testing.T) {
	guard := transcoding.AudioDownmixArgs(2)
	require.NotEmpty(t, guard)
	needle := strings.Join(guard, " ")

	cases := map[string][]string{
		"compat download": ffmpeg.BuildCompatArgs("/media/film.mkv", "/cache/out.mkv.tmp"),
		"reduced download": ffmpeg.BuildReducedArgs("/media/film.mkv", "/cache/j/slice-%05d.mkv", "/cache/j/slices-00000.csv",
			reduceddl.Spec{Width: 1280, SliceSec: 600}),
		// The bitrate is the one the handler really passes on this path, taken
		// from the constant rather than pinned: it moved once already (128 -> 192
		// on 2026-09-04) and a fixture frozen on the old value would quietly stop
		// describing the argv this test is meant to guard.
		"audio rendition (explicit stereo)": ffmpeg.BuildFFmpegAudioOnlyArgs(transcoding.AudioProcessOpts{
			FilePath: "/f.mkv", OutputDir: "/o", AudioCodec: "aac",
			AudioBitrate: transcoding.AudioRenditionBaseKbps, MaxAudioChannels: 2,
		}),
		"audio rendition (default stereo)": ffmpeg.BuildFFmpegAudioOnlyArgs(transcoding.AudioProcessOpts{
			FilePath: "/f.mkv", OutputDir: "/o", AudioCodec: "aac",
			AudioBitrate: transcoding.AudioRenditionBaseKbps,
		}),
		"muxed copy-video": ffmpeg.BuildFFmpegArgs(transcoding.HLSProcessOpts{
			OutputDir: "/o", FilePath: "/f.mkv", Profiles: transcoding.DefaultHLSProfiles,
			VideoDecision: "copy", AudioDecision: "transcode",
		}),
		"muxed single transcode": ffmpeg.BuildFFmpegArgs(transcoding.HLSProcessOpts{
			OutputDir: "/o", FilePath: "/f.mkv", Profiles: transcoding.DefaultHLSProfiles[:1],
			VideoDecision: "transcode", AudioDecision: "transcode",
		}),
		"muxed multi-variant": ffmpeg.BuildFFmpegArgs(transcoding.HLSProcessOpts{
			OutputDir: "/o", FilePath: "/f.mkv", Profiles: transcoding.DefaultHLSProfiles,
			VideoDecision: "transcode", AudioDecision: "transcode",
		}),
		// The hardware builders reuse the same audio helpers, but a future
		// divergence there would be silent - pin them too.
		"hw single transcode": ffmpeg.BuildFFmpegArgs(transcoding.HLSProcessOpts{
			OutputDir: "/o", FilePath: "/f.mkv", Profiles: transcoding.DefaultHLSProfiles[:1],
			VideoDecision: "transcode", AudioDecision: "transcode", HWAccel: transcoding.VAAPI,
		}),
		"hw multi-variant": ffmpeg.BuildFFmpegArgs(transcoding.HLSProcessOpts{
			OutputDir: "/o", FilePath: "/f.mkv", Profiles: transcoding.DefaultHLSProfiles,
			VideoDecision: "transcode", AudioDecision: "transcode", HWAccel: transcoding.VAAPI,
		}),
	}

	for name, args := range cases {
		joined := strings.Join(args, " ")
		require.Contains(t, joined, "-ac 2", "%s: expected a stereo fold", name)
		assert.Contains(t, joined, needle, "%s: stereo fold ships without matrix headroom", name)
	}
}

// The nominal 5.1 path must be untouched: nothing sums, so no option and above
// all no audio filter (a filter graph node costs CPU and changes the sound for
// nothing on a weak NAS).
func TestMultichannelPassthrough_AddsNothing(t *testing.T) {
	guardOpt := "-rematrix_maxval"

	cases := map[string][]string{
		"audio rendition 5.1": ffmpeg.BuildFFmpegAudioOnlyArgs(transcoding.AudioProcessOpts{
			FilePath: "/f.mkv", OutputDir: "/o", AudioCodec: "aac", AudioBitrate: 128, MaxAudioChannels: 6,
		}),
		"muxed copy-video 5.1": ffmpeg.BuildFFmpegArgs(transcoding.HLSProcessOpts{
			OutputDir: "/o", FilePath: "/f.mkv", Profiles: transcoding.DefaultHLSProfiles,
			VideoDecision: "copy", AudioDecision: "transcode", MaxAudioChannels: 6,
		}),
		"muxed single transcode 7.1": ffmpeg.BuildFFmpegArgs(transcoding.HLSProcessOpts{
			OutputDir: "/o", FilePath: "/f.mkv", Profiles: transcoding.DefaultHLSProfiles[:1],
			VideoDecision: "transcode", AudioDecision: "transcode", MaxAudioChannels: 8,
		}),
		"muxed multi-variant 5.1": ffmpeg.BuildFFmpegArgs(transcoding.HLSProcessOpts{
			OutputDir: "/o", FilePath: "/f.mkv", Profiles: transcoding.DefaultHLSProfiles,
			VideoDecision: "transcode", AudioDecision: "transcode", MaxAudioChannels: 6,
		}),
		"hw multi-variant 5.1": ffmpeg.BuildFFmpegArgs(transcoding.HLSProcessOpts{
			OutputDir: "/o", FilePath: "/f.mkv", Profiles: transcoding.DefaultHLSProfiles,
			VideoDecision: "transcode", AudioDecision: "transcode", MaxAudioChannels: 6,
			HWAccel: transcoding.VAAPI,
		}),
	}

	for name, args := range cases {
		joined := strings.Join(args, " ")
		require.NotContains(t, joined, "-ac 2", "%s: expected a layout-preserving encode", name)
		assert.NotContains(t, joined, guardOpt, "%s: no fold, no headroom option", name)
		for i, a := range args {
			if a == "-af" || strings.HasPrefix(a, "-filter:a") {
				t.Errorf("%s: unexpected audio filter %q=%q", name, a, args[i+1])
			}
		}
	}
}

// A stream copy has no resampler to configure; ffmpeg rejects re-encode args
// next to -c:a copy, so the guard must stay out of the passthrough rendition.
func TestAudioPassthrough_KeepsNoResamplerOption(t *testing.T) {
	args := ffmpeg.BuildFFmpegAudioOnlyArgs(transcoding.AudioProcessOpts{
		FilePath: "/f.mkv", OutputDir: "/o", AudioCodec: "copy",
	})
	joined := strings.Join(args, " ")
	require.Contains(t, joined, "-c:a copy")
	assert.NotContains(t, joined, "-rematrix_maxval")
}
