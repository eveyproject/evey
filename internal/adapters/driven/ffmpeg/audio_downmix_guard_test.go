package ffmpeg_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/ffmpeg"
	"github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/eveyproject/evey/internal/pkg/reduceddl"
)

// Every server-side path that folds a multichannel bed to stereo must carry
// the fold guard (float fold + limiter, audio_downmix.go): without it the
// integer encoder input hard-clips a loud mix (audio_downmix_integration_test.go).
func TestStereoFold_CarriesMatrixHeadroom(t *testing.T) {
	guard := transcoding.AudioFoldFilter(0, 2)
	require.NotEmpty(t, guard)

	cases := map[string][]string{
		"compat download": ffmpeg.BuildCompatArgs("/media/film.mkv", "/cache/out.mkv.tmp"),
		"reduced download": ffmpeg.BuildReducedArgs("/media/film.mkv", "/cache/j/slice-%05d.mkv", "/cache/j/slices-00000.csv",
			reduceddl.Spec{Width: 1280, SliceSec: 600}, transcoding.PicturePlan{}),
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
		assert.Contains(t, joined, guard, "%s: stereo fold ships without the guard", name)
	}
}

// A source the handler knows to be stereo sums nothing, so the guard - a
// filter node, and a limiter that would touch a hot master - stays out and the
// bytes are the pre-policy ones.
func TestStereoFold_SkipsTheGuardOnAKnownStereoSource(t *testing.T) {
	guard := transcoding.AudioFoldFilter(0, 2)
	cases := map[string][]string{
		"audio rendition": ffmpeg.BuildFFmpegAudioOnlyArgs(transcoding.AudioProcessOpts{
			FilePath: "/f.mkv", OutputDir: "/o", AudioCodec: "aac",
			AudioBitrate: transcoding.AudioRenditionBaseKbps, MaxAudioChannels: 2, SourceAudioChannels: 2,
		}),
		"muxed multi-variant": ffmpeg.BuildFFmpegArgs(transcoding.HLSProcessOpts{
			OutputDir: "/o", FilePath: "/f.mkv", Profiles: transcoding.DefaultHLSProfiles,
			VideoDecision: "transcode", AudioDecision: "transcode", MaxAudioChannels: 2, SourceAudioChannels: 2,
		}),
	}
	for name, args := range cases {
		joined := strings.Join(args, " ")
		require.Contains(t, joined, "-ac 2", name)
		assert.NotContains(t, joined, guard, "%s: a stereo source must not be guarded", name)
		assert.NotContains(t, joined, "alimiter", name)
	}
}

// loudnorm stacks on the guard in ONE filter option: ffmpeg keeps only the
// last -af, so a second option would silently drop the fold.
func TestStereoFold_StacksLoudnormInTheSameFilterOption(t *testing.T) {
	args := ffmpeg.BuildFFmpegAudioOnlyArgs(transcoding.AudioProcessOpts{
		FilePath: "/f.mkv", OutputDir: "/o", AudioCodec: "aac",
		AudioBitrate: transcoding.AudioRenditionBaseKbps, MaxAudioChannels: 2, AudioNormalize: true,
	})
	afs := 0
	for i, a := range args {
		if a == "-af" {
			afs++
			// The fold leads; the priming trim may close the chain (it drops input,
			// it does not touch the sound).
			assert.True(t, strings.HasPrefix(args[i+1], transcoding.AudioFoldFilter(0, 2)+",loudnorm="), args[i+1])
		}
	}
	assert.Equal(t, 1, afs, "exactly one audio filter option: %v", args)
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
			if (a == "-af" || strings.HasPrefix(a, "-filter:a")) && !isPrimingTrim(args[i+1]) {
				t.Errorf("%s: unexpected audio filter %q=%q", name, a, args[i+1])
			}
		}
	}
}

// isPrimingTrim reports whether an audio filter chain is the priming trim alone.
func isPrimingTrim(chain string) bool {
	return strings.HasPrefix(chain, "asetpts=PTS-STARTPTS,atrim=start=") && strings.Count(chain, ",") == 2
}
