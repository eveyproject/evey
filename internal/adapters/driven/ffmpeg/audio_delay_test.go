package ffmpeg_test

// Source audio start-delay cancellation: some repacks carry an audio track
// whose first timestamp sits ~1s after the video's (a mux defect, not a
// creative delay). The HLS pipeline preserves source PTS (-copyts and
// friends), so the defect is reproduced verbatim and the viewer hears a
// constant A/V offset. The fix feeds the audio from a SECOND ffmpeg input
// shifted by -itsoffset -<delay>, leaving the video input (and its seek
// bookkeeping) untouched. Only TRANSCODED audio is eligible: copied audio
// packets cannot be trimmed at the seek edge, and a second input there was
// measured to corrupt the make_zero zero-point of copy-on-seek sessions.

import (
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driven/ffmpeg"
	"github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
)

func countArg(args []string, want string) int {
	n := 0
	for _, a := range args {
		if a == want {
			n++
		}
	}
	return n
}

func delayOpts(videoDecision string, profiles int) transcoding.HLSProcessOpts {
	return transcoding.HLSProcessOpts{
		OutputDir:          "/tmp/test",
		FilePath:           "/f.mkv",
		Profiles:           transcoding.DefaultHLSProfiles[:profiles],
		VideoDecision:      videoDecision,
		AudioDecision:      "transcode",
		AudioTrackIndex:    0,
		SubtitleTrackIndex: -1,
		UseFMP4:            true,
		AudioStartDelaySec: 0.9,
	}
}

func TestAudioDelayCancel_MultiVariant_SecondInput(t *testing.T) {
	opts := delayOpts("", 3)
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")

	assert.Equal(t, 2, countArg(args, "-i"), "delay cancel must add exactly one extra input")
	assert.Contains(t, joined, "-itsoffset -0.900 -i /f.mkv", "second input must be shifted back by the probed delay")
	assert.Contains(t, joined, "-map 1:a:0", "audio must be mapped from the shifted second input")
	assert.NotContains(t, joined, "-map 0:a:", "no audio may come from the unshifted first input")
	assert.Contains(t, joined, "[0:v]", "video filter graph must keep reading input 0")
}

func TestAudioDelayCancel_MultiVariant_SeekReplicatesSS(t *testing.T) {
	opts := delayOpts("", 3)
	opts.StartSec = 15
	joined := strings.Join(ffmpeg.BuildFFmpegArgs(opts), " ")

	// Both inputs must seek to the same target; the trim then lands the shifted
	// audio exactly at the scene the video session starts on (validated against
	// jellyfin-ffmpeg 7.1: -ss on a -copyts session compares POST-itsoffset
	// timestamps, so the replica needs no +delay adjustment).
	assert.Contains(t, joined, "-ss 15.000 -i /f.mkv -ss 15.000 -itsoffset -0.900 -i /f.mkv")
}

func TestAudioDelayCancel_SingleTranscode(t *testing.T) {
	cold := delayOpts("transcode", 1)
	coldArgs := ffmpeg.BuildFFmpegArgs(cold)
	coldJoined := strings.Join(coldArgs, " ")
	assert.Equal(t, 2, countArg(coldArgs, "-i"))
	assert.Contains(t, coldJoined, "-itsoffset -0.900 -i /f.mkv")
	assert.Contains(t, coldJoined, "-map 1:a:0")
	assert.NotContains(t, coldJoined, "-map 0:a:")

	seek := delayOpts("transcode", 1)
	seek.StartSec = 33
	seekJoined := strings.Join(ffmpeg.BuildFFmpegArgs(seek), " ")
	assert.Contains(t, seekJoined, "-ss 33.000 -itsoffset -0.900 -i /f.mkv", "seek must be replicated on the audio input")
}

func TestAudioDelayCancel_CopyVideo_TranscodedAudio(t *testing.T) {
	cold := delayOpts("copy", 1)
	coldArgs := ffmpeg.BuildFFmpegArgs(cold)
	coldJoined := strings.Join(coldArgs, " ")
	assert.Equal(t, 2, countArg(coldArgs, "-i"))
	assert.Contains(t, coldJoined, "-map 1:a:0")
	assert.Contains(t, coldJoined, "-map 0:v:0", "video still reads input 0")

	seek := delayOpts("copy", 1)
	seek.StartSec = 120
	seek.CopyOnSeek = true
	seek.SeekTargetSec = 120.05
	seekArgs := ffmpeg.BuildFFmpegArgs(seek)
	seekJoined := strings.Join(seekArgs, " ")
	assert.Contains(t, seekJoined, "-ss 120.050 -seek_timestamp 1 -itsoffset -0.900 -i /f.mkv", "audio input replicates the exact seek target, in source time")
	// Input 1 must keep ffmpeg's DEFAULT accurate seek: the decode trim is what
	// drops the audio content sitting before the seek target once -itsoffset
	// shifted it. -noaccurate_seek belongs to input 0 only.
	assert.Equal(t, 1, countArg(seekArgs, "-noaccurate_seek"), "accurate-seek trim must stay active on the audio input")
}

// Copied audio cannot be delay-cancelled: packets are not trimmable at the
// seek edge, and the early shifted packets were measured to hijack the
// make_zero zero-point (video landed +0.87s off plan → timeline tripwire).
// The session must stay byte-identical to the no-delay one.
func TestAudioDelayCancel_CopiedAudio_Unchanged(t *testing.T) {
	for _, tc := range []struct {
		name string
		mut  func(*transcoding.HLSProcessOpts)
	}{
		{"cold", func(o *transcoding.HLSProcessOpts) {}},
		{"copy-on-seek", func(o *transcoding.HLSProcessOpts) {
			o.StartSec = 120
			o.CopyOnSeek = true
			o.SeekTargetSec = 120.05
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			with := delayOpts("copy", 1)
			with.AudioDecision = "copy"
			tc.mut(&with)
			without := with
			without.AudioStartDelaySec = 0

			assert.Equal(t, ffmpeg.BuildFFmpegArgs(without), ffmpeg.BuildFFmpegArgs(with))
		})
	}
}

func TestAudioDelayCancel_BelowThreshold_Unchanged(t *testing.T) {
	with := delayOpts("", 3)
	with.AudioStartDelaySec = 0.08 // below the 0.1s floor: AAC priming territory
	without := with
	without.AudioStartDelaySec = 0

	assert.Equal(t, ffmpeg.BuildFFmpegArgs(without), ffmpeg.BuildFFmpegArgs(with))
}

func TestAudioDelayCancel_NoAudio_Unchanged(t *testing.T) {
	with := delayOpts("", 3)
	with.AudioTrackIndex = -1
	args := ffmpeg.BuildFFmpegArgs(with)
	assert.Equal(t, 1, countArg(args, "-i"))
}

func TestAudioDelayCancel_MirrorsInputPacing(t *testing.T) {
	opts := delayOpts("copy", 1)
	opts.ReadRate = 1.5
	joined := strings.Join(ffmpeg.BuildFFmpegArgs(opts), " ")
	// The audio input must be paced like the video input, or the muxer's
	// interleave queue re-creates the very skew the pacing exists to bound.
	assert.Contains(t, joined, "-readrate 1.5 -readrate_initial_burst 30 -itsoffset -0.900 -i /f.mkv")
}

func TestAudioDelayCancel_HWBuilders(t *testing.T) {
	single := delayOpts("transcode", 1)
	single.HWAccel = transcoding.NVENC
	single.VideoCodec = "h264"
	sArgs := ffmpeg.BuildFFmpegArgs(single)
	sJoined := strings.Join(sArgs, " ")
	assert.Contains(t, sJoined, "h264_nvenc", "precondition: HW path taken")
	assert.Equal(t, 2, countArg(sArgs, "-i"))
	assert.Contains(t, sJoined, "-itsoffset -0.900 -i /f.mkv")
	assert.Contains(t, sJoined, "-map 1:a:0")

	multi := delayOpts("", 3)
	multi.HWAccel = transcoding.NVENC
	multi.VideoCodec = "h264"
	mJoined := strings.Join(ffmpeg.BuildFFmpegArgs(multi), " ")
	assert.Contains(t, mJoined, "h264_nvenc", "precondition: HW path taken")
	assert.Contains(t, mJoined, "-itsoffset -0.900 -i /f.mkv")
	assert.Contains(t, mJoined, "-map 1:a:0")
	assert.NotContains(t, mJoined, "-map 0:a:")
	// The hwaccel decode flags belong to the VIDEO input; the audio input is
	// plain software decode.
	assert.Less(t, strings.Index(mJoined, "-hwaccel"), strings.Index(mJoined, "-i /f.mkv"))
}

// The second input rides on a non-zero AudioTrackIndex too (delayed track 2
// stays track 2 of the second input: the whole file is re-opened).
func TestAudioDelayCancel_NonZeroAudioTrack(t *testing.T) {
	opts := delayOpts("", 3)
	opts.AudioTrackIndex = 2
	joined := strings.Join(ffmpeg.BuildFFmpegArgs(opts), " ")
	assert.Contains(t, joined, "-map 1:a:2")
	assert.NotContains(t, joined, "-map 0:a:")
}

// User audio resync composes with the source-delay cancel into ONE net input
// shift: -itsoffset -(sourceDelay - userOffset). Positive user offsets (audio
// later) flip the sign; an offset that exactly cancels the source delay drops
// the second input entirely.
func TestAudioDelayCancel_UserOffsetComposition(t *testing.T) {
	userOnly := delayOpts("", 3)
	userOnly.AudioStartDelaySec = 0
	userOnly.AudioUserOffsetSec = 0.3
	joined := strings.Join(ffmpeg.BuildFFmpegArgs(userOnly), " ")
	assert.Contains(t, joined, "-itsoffset 0.300 -i /f.mkv",
		"a bare user resync (+300ms) delays the audio input by +0.3s")
	assert.Contains(t, joined, "-map 1:a:0")

	combined := delayOpts("", 3)
	combined.AudioStartDelaySec = 0.9
	combined.AudioUserOffsetSec = 0.3
	joined = strings.Join(ffmpeg.BuildFFmpegArgs(combined), " ")
	assert.Contains(t, joined, "-itsoffset -0.600 -i /f.mkv",
		"cancel 0.9s + user +0.3s = net -0.6s input shift")

	cancelled := delayOpts("", 3)
	cancelled.AudioStartDelaySec = 0.3
	cancelled.AudioUserOffsetSec = 0.3
	args := ffmpeg.BuildFFmpegArgs(cancelled)
	assert.Equal(t, 1, countArg(args, "-i"), "a net-zero shift needs no second input")

	earlier := delayOpts("", 3)
	earlier.AudioStartDelaySec = 0
	earlier.AudioUserOffsetSec = -0.1
	joined = strings.Join(ffmpeg.BuildFFmpegArgs(earlier), " ")
	assert.Contains(t, joined, "-itsoffset -0.100",
		"user asks audio EARLIER by 100ms: net = 0 - (-0.1) = +0.1 → -itsoffset -0.100")
}
