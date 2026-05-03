package ffmpeg_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/ffmpeg"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found, skipping integration test")
	}
}

func generateTestVideo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "test.mp4")
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "testsrc=duration=2:size=640x360:rate=25",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2",
		"-c:v", "libx264", "-preset", "ultrafast",
		"-c:a", "aac",
		"-shortest", "-y", out,
	)
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Run(), "failed to generate test video")
	return out
}

func TestBuildFFmpegArgs_ModeA_VideoCopy(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:       "/tmp/test",
		FilePath:        "/f.mkv",
		Profiles:        transcoding.DefaultHLSProfiles,
		VideoDecision:   "copy",
		AudioDecision:   "transcode",
		AudioTrackIndex: 0,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-c:v copy")
	assert.Contains(t, joined, "-c:a aac")
	assert.NotContains(t, joined, "filter_complex")
	assert.NotContains(t, joined, "libx264")
}

func TestBuildFFmpegArgs_ModeA_AllCopy(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles,
		VideoDecision: "copy",
		AudioDecision: "copy",
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-c:v copy")
	assert.Contains(t, joined, "-c:a copy")
}

func TestBuildFFmpegArgs_ModeB_MultiVariant(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles,
		VideoDecision: "transcode",
		AudioDecision: "transcode",
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "libx264")
	assert.Contains(t, joined, "split=")
	assert.Contains(t, joined, "var_stream_map")
}

func TestBuildFFmpegArgs_MultiVariant(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:       "/tmp/test",
		FilePath:        "/f.mkv",
		Profiles:        transcoding.DefaultHLSProfiles,
		VideoDecision:   "",
		AudioDecision:   "",
		AudioTrackIndex: 0,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "[0:v]split=")
	assert.Contains(t, joined, "var_stream_map")
	assert.Contains(t, joined, "master.m3u8")
	assert.Contains(t, joined, "-profile:v:0 high")
	assert.Contains(t, joined, "-level:v:0 4.1")
	assert.Contains(t, joined, "-pix_fmt yuv420p")
}

func TestBuildFFmpegArgs_AudioNormalize(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:      "/tmp/test",
		FilePath:       "/f.mkv",
		Profiles:       transcoding.DefaultHLSProfiles,
		VideoDecision:  "copy",
		AudioDecision:  "transcode",
		AudioNormalize: true,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "loudnorm=I=-16:TP=-1.5:LRA=11:linear=true")
}

func TestBuildFFmpegArgs_BurnIn(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:          "/tmp/test",
		FilePath:           "/f.mkv",
		Profiles:           transcoding.DefaultHLSProfiles,
		SubtitleTrackIndex: 2,
		SubtitleCodec:      "hdmv_pgs_subtitle",
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "[0:v][0:s:2]overlay[vb]")
	assert.Contains(t, joined, "[vb]split=")
}

func TestBuildFFmpegArgs_SingleTranscode_OneProfile(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles[:1],
		VideoDecision: "transcode",
		AudioDecision: "copy",
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-c:a copy")
	assert.Contains(t, joined, "libx264")
	assert.NotContains(t, joined, "var_stream_map")
}

func TestBuildFFmpegArgs_SingleTranscode_AudioNormalize(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:      "/tmp/test",
		FilePath:       "/f.mkv",
		Profiles:       transcoding.DefaultHLSProfiles,
		VideoDecision:  "transcode",
		AudioDecision:  "transcode",
		AudioNormalize: true,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "loudnorm")
}

func TestBuildFFmpegArgs_SingleTranscode_WithSeek(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles,
		VideoDecision: "transcode",
		AudioDecision: "transcode",
		StartSec:      10.5,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	ssIdx, iIdx := -1, -1
	for i, a := range args {
		if a == "-ss" && ssIdx < 0 {
			ssIdx = i
		}
		if a == "-i" && iIdx < 0 {
			iIdx = i
		}
	}
	require.Greater(t, ssIdx, -1)
	require.Greater(t, iIdx, -1)
	assert.Less(t, ssIdx, iIdx)
}

func TestBuildFFmpegArgs_MultiVariant_WithSeek(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:       "/tmp/test",
		FilePath:        "/f.mkv",
		Profiles:        transcoding.DefaultHLSProfiles,
		VideoDecision:   "",
		AudioDecision:   "",
		AudioTrackIndex: 0,
		StartSec:        30.0,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-ss 30.000")
}

func TestBuildFFmpegArgs_MultiVariant_AudioNormalize(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:       "/tmp/test",
		FilePath:        "/f.mkv",
		Profiles:        transcoding.DefaultHLSProfiles,
		VideoDecision:   "",
		AudioDecision:   "",
		AudioTrackIndex: 0,
		AudioNormalize:  true,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "loudnorm")
}

func TestBuildFFmpegArgs_VideoCopy_AudioCopy(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:       "/tmp/test",
		FilePath:        "/f.mkv",
		Profiles:        transcoding.DefaultHLSProfiles,
		VideoDecision:   "copy",
		AudioDecision:   "copy",
		AudioTrackIndex: 1,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-c:v copy")
	assert.Contains(t, joined, "-c:a copy")
	assert.Contains(t, joined, "0:a:1")
}

func TestBuildFFmpegArgs_VideoCopy_AudioNormalize(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:       "/tmp/test",
		FilePath:        "/f.mkv",
		Profiles:        transcoding.DefaultHLSProfiles,
		VideoDecision:   "copy",
		AudioDecision:   "transcode",
		AudioNormalize:  true,
		AudioTrackIndex: 0,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "loudnorm")
}

func TestBuildFFmpegArgs_SingleTranscode_BurnInSubtitle(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:          "/tmp/test",
		FilePath:           "/f.mkv",
		Profiles:           transcoding.DefaultHLSProfiles,
		VideoDecision:      "transcode",
		AudioDecision:      "transcode",
		SubtitleTrackIndex: 0,
		SubtitleCodec:      "pgssub",
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "overlay")
	assert.Contains(t, joined, "[0:s:0]")
}

func TestIsImageSubtitle(t *testing.T) {
	imageCodecs := []string{"pgssub", "hdmv_pgs_subtitle", "dvd_subtitle", "dvbsub", "xsub"}
	for _, c := range imageCodecs {
		assert.True(t, ffmpeg.IsImageSubtitle(c), "expected %s to be image subtitle", c)
	}
	assert.False(t, ffmpeg.IsImageSubtitle("srt"), "srt is not image subtitle")
	assert.False(t, ffmpeg.IsImageSubtitle("ass"), "ass is not image subtitle")
}

// --- Decision override tests ---

func TestBuildFFmpegArgs_VideoCodecOverride_HEVC_SingleTranscode(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles[:1],
		VideoDecision: "transcode",
		AudioDecision: "transcode",
		VideoCodec:    "hevc",
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "libx265")
	assert.NotContains(t, joined, "libx264")
}

func TestBuildFFmpegArgs_VideoCodecOverride_MultiVariant(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles,
		VideoDecision: "transcode",
		AudioDecision: "transcode",
		VideoCodec:    "h265",
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-c:v:0 libx265")
	assert.Contains(t, joined, "-c:v:3 libx265")
	assert.NotContains(t, joined, "libx264")
}

// av1 mapping accepted at the arg-building layer. The libaom-av1 encoder is
// extremely slow and may be absent from minimal ffmpeg images; we assert the
// mapping here without spawning a real encode.
func TestBuildFFmpegArgs_VideoCodecOverride_AV1_Mapping(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles[:1],
		VideoDecision: "transcode",
		AudioDecision: "copy",
		VideoCodec:    "av1",
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "libaom-av1")
}

func TestBuildFFmpegArgs_AudioCodecOverride_EAC3_CopyVideo(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles,
		VideoDecision: "copy",
		AudioDecision: "transcode",
		AudioCodec:    "eac3",
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-c:a eac3")
	assert.NotContains(t, joined, "-c:a aac")
}

func TestBuildFFmpegArgs_AudioCodecOverride_AC3_SingleTranscode(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles[:1],
		VideoDecision: "transcode",
		AudioDecision: "transcode",
		AudioCodec:    "ac3",
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-c:a ac3")
}

// opus → libopus mapping. libopus may be absent from very old ffmpeg builds;
// the mapping itself is verified without spawning ffmpeg.
func TestBuildFFmpegArgs_AudioCodecOverride_Opus_Mapping(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles,
		VideoDecision: "transcode",
		AudioDecision: "transcode",
		AudioCodec:    "opus",
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "libopus")
}

func TestBuildFFmpegArgs_MaxAudioChannels_CopyVideo(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:        "/tmp/test",
		FilePath:         "/f.mkv",
		Profiles:         transcoding.DefaultHLSProfiles,
		VideoDecision:    "copy",
		AudioDecision:    "transcode",
		MaxAudioChannels: 6,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-ac 6")
	assert.NotContains(t, joined, "-ac 2")
}

func TestBuildFFmpegArgs_MaxAudioChannels_SingleTranscode(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:        "/tmp/test",
		FilePath:         "/f.mkv",
		Profiles:         transcoding.DefaultHLSProfiles[:1],
		VideoDecision:    "transcode",
		AudioDecision:    "transcode",
		MaxAudioChannels: 8,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-ac 8")
}

func TestBuildFFmpegArgs_MaxAudioChannels_MultiVariant(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:        "/tmp/test",
		FilePath:         "/f.mkv",
		Profiles:         transcoding.DefaultHLSProfiles,
		VideoDecision:    "transcode",
		AudioDecision:    "transcode",
		MaxAudioChannels: 6,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-ac 6")
	assert.NotContains(t, joined, "-ac 2")
}

// Zero MaxAudioChannels must preserve the legacy -ac 2 default.
func TestBuildFFmpegArgs_MaxAudioChannels_ZeroKeepsDefault(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles,
		VideoDecision: "copy",
		AudioDecision: "transcode",
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-ac 2")
}

func TestBuildFFmpegArgs_ForceSidecarSubs_SkipsBurnIn_SingleTranscode(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:          "/tmp/test",
		FilePath:           "/f.mkv",
		Profiles:           transcoding.DefaultHLSProfiles[:1],
		VideoDecision:      "transcode",
		AudioDecision:      "transcode",
		SubtitleTrackIndex: 0,
		SubtitleCodec:      "pgssub",
		ForceSidecarSubs:   true,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.NotContains(t, joined, "overlay")
	assert.NotContains(t, joined, "[0:s:0]")
}

func TestBuildFFmpegArgs_ForceSidecarSubs_SkipsBurnIn_MultiVariant(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:          "/tmp/test",
		FilePath:           "/f.mkv",
		Profiles:           transcoding.DefaultHLSProfiles,
		VideoDecision:      "transcode",
		AudioDecision:      "transcode",
		SubtitleTrackIndex: 2,
		SubtitleCodec:      "hdmv_pgs_subtitle",
		ForceSidecarSubs:   true,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.NotContains(t, joined, "overlay")
	assert.Contains(t, joined, "[0:v]split=")
}

// Without ForceSidecarSubs the existing burn-in behavior remains (regression guard).
func TestBuildFFmpegArgs_ForceSidecarSubs_DefaultStillBurns(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:          "/tmp/test",
		FilePath:           "/f.mkv",
		Profiles:           transcoding.DefaultHLSProfiles,
		VideoDecision:      "transcode",
		AudioDecision:      "transcode",
		SubtitleTrackIndex: 2,
		SubtitleCodec:      "hdmv_pgs_subtitle",
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "overlay")
}

// All overrides combined: h265 + eac3 + 6 channels + sidecar subs.
func TestBuildFFmpegArgs_AllOverridesCombined(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:          "/tmp/test",
		FilePath:           "/f.mkv",
		Profiles:           transcoding.DefaultHLSProfiles[:1],
		VideoDecision:      "transcode",
		AudioDecision:      "transcode",
		SubtitleTrackIndex: 0,
		SubtitleCodec:      "pgssub",
		VideoCodec:         "hevc",
		AudioCodec:         "eac3",
		MaxAudioChannels:   6,
		ForceSidecarSubs:   true,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "libx265")
	assert.Contains(t, joined, "-c:a eac3")
	assert.Contains(t, joined, "-ac 6")
	assert.NotContains(t, joined, "overlay")
}

func TestBuildFFmpegArgs_HEVCCopy_UsesFMP4Segments(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:       "/tmp/test",
		FilePath:        "/f.mkv",
		Profiles:        transcoding.DefaultHLSProfiles,
		VideoDecision:   "copy",
		AudioDecision:   "copy",
		AudioTrackIndex: 0,
		Streams: []*library.MediaStream{
			{StreamType: "video", Codec: "hevc"},
			{StreamType: "audio", Codec: "eac3", IsDefault: true},
		},
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-hls_segment_type fmp4")
	assert.Contains(t, joined, ".m4s")
	assert.NotContains(t, joined, "%03d.ts")
}

func TestBuildFFmpegArgs_H264Copy_UsesTSSegments(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:       "/tmp/test",
		FilePath:        "/f.mkv",
		Profiles:        transcoding.DefaultHLSProfiles,
		VideoDecision:   "copy",
		AudioDecision:   "copy",
		AudioTrackIndex: 0,
		Streams: []*library.MediaStream{
			{StreamType: "video", Codec: "h264"},
			{StreamType: "audio", Codec: "aac", IsDefault: true},
		},
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "%03d.ts")
	assert.NotContains(t, joined, "-hls_segment_type fmp4")
}

// Verifies that buildCopyVideoArgs forces a video re-encode when StartSec > 0.
// Hybrid seek: pre-input fast seek to keyframe-before, post-input trim
// for accuracy, re-encode at ultrafast. Yields frame-accurate output
// without the manifestLoadTimeOut that pure post-input -ss caused on
// deep seeks (ffmpeg decoded everything from frame 0).
func TestTranscoding_BuildFFmpegArgs_CopyVideoSeekTriggersReencode(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:       "/tmp/test",
		FilePath:        "/source.mkv",
		Profiles:        transcoding.DefaultHLSProfiles,
		VideoDecision:   "copy",
		AudioDecision:   "copy",
		AudioTrackIndex: 0,
		StartSec:        358.5,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-c:v libx264", "StartSec>0 must force video re-encode (copy can't trim sub-keyframe)")
	assert.NotContains(t, joined, "-c:v copy", "copy mode must be replaced when seeking")
	assert.Contains(t, joined, "-preset ultrafast", "use cheap preset to keep CPU acceptable")
	// Hybrid seek: -ss BEFORE -i carries (StartSec - preRoll) for fast
	// keyframe-snap, -ss AFTER -i carries preRoll for accurate trim.
	// preRoll = 8s, so for StartSec=358.5 we expect -ss 350.500 then
	// -ss 8.000.
	assert.Contains(t, joined, "-ss 350.500", "expected pre-input fast seek at StartSec - preRoll")
	assert.Contains(t, joined, "-ss 8.000", "expected post-input trim at preRoll seconds")
	iIdx := -1
	preIdx := -1
	postIdx := -1
	for i, a := range args {
		if a == "-i" {
			iIdx = i
		}
		if a == "-ss" && preIdx == -1 && iIdx == -1 {
			preIdx = i
		}
		if a == "-ss" && iIdx >= 0 && postIdx == -1 {
			postIdx = i
		}
	}
	assert.GreaterOrEqual(t, preIdx, 0, "expected pre-input -ss")
	assert.Greater(t, postIdx, iIdx, "expected post-input -ss after -i")
}

// Pre-roll guard: when StartSec < preRoll (8s), the pre-input -ss must
// clamp to 0 (no negative seek) and the post-input trim drops to
// StartSec so the math still lands on the requested second.
func TestTranscoding_BuildFFmpegArgs_CopyVideoSmallSeek_ClampsPreRoll(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:       "/tmp/test",
		FilePath:        "/source.mkv",
		Profiles:        transcoding.DefaultHLSProfiles,
		VideoDecision:   "copy",
		AudioDecision:   "copy",
		AudioTrackIndex: 0,
		StartSec:        4.0,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-ss 0.000", "pre-roll must clamp to 0 for small seek")
	assert.Contains(t, joined, "-ss 4.000", "post-input trim must equal StartSec")
}

// Sanity: with StartSec == 0 we keep the cheap stream-copy path because
// no precision problem to solve.
func TestTranscoding_BuildFFmpegArgs_CopyVideoNoSeekKeepsCopy(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:       "/tmp/test",
		FilePath:        "/source.mkv",
		Profiles:        transcoding.DefaultHLSProfiles,
		VideoDecision:   "copy",
		AudioDecision:   "copy",
		AudioTrackIndex: 0,
		StartSec:        0,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-c:v copy", "StartSec=0 must stay on stream copy")
	assert.NotContains(t, joined, "-c:v libx264", "no re-encode needed at StartSec=0")
	assert.NotContains(t, joined, "-ss ", "no -ss when StartSec=0")
}

// Kills hls_transcoder.go:175:19 CONDITIONALS_BOUNDARY (`StartSec > 0` vs `>= 0`)
// in buildCopyVideoArgs. With StartSec==0, the `>` branch skips -ss; mutation `>=`
// would emit `-ss 0.000`.
func TestTranscoding_KillMutant_BuildFFmpegArgs_CopyVideoNoSeekWhenZero(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:       "/tmp/test",
		FilePath:        "/f.mkv",
		Profiles:        transcoding.DefaultHLSProfiles,
		VideoDecision:   "copy",
		AudioDecision:   "copy",
		AudioTrackIndex: 0,
		StartSec:        0,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	for _, a := range args {
		assert.NotEqual(t, "-ss", a, "StartSec=0 must NOT emit -ss in copy mode")
	}
}

// Kills hls_transcoder.go:214:19 CONDITIONALS_BOUNDARY (`StartSec > 0` vs `>= 0`)
// in buildSingleTranscodeArgs. Force single-transcode path via Profiles[:1] +
// VideoDecision="transcode".
func TestTranscoding_KillMutant_BuildFFmpegArgs_SingleTranscodeNoSeekWhenZero(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles[:1],
		VideoDecision: "transcode",
		AudioDecision: "copy",
		StartSec:      0,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	for _, a := range args {
		assert.NotEqual(t, "-ss", a, "StartSec=0 must NOT emit -ss in single-transcode mode")
	}
}

// Kills hls_transcoder.go:279:19 CONDITIONALS_BOUNDARY (`StartSec > 0` vs `>= 0`)
// in buildMultiVariantArgs (VideoDecision="" falls through to multi-variant).
func TestTranscoding_KillMutant_BuildFFmpegArgs_MultiVariantNoSeekWhenZero(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:       "/tmp/test",
		FilePath:        "/f.mkv",
		Profiles:        transcoding.DefaultHLSProfiles,
		VideoDecision:   "",
		AudioDecision:   "",
		AudioTrackIndex: 0,
		StartSec:        0,
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	for _, a := range args {
		assert.NotEqual(t, "-ss", a, "StartSec=0 must NOT emit -ss in multi-variant mode")
	}
}

// Kills hls_transcoder.go:210:36 CONDITIONALS_NEGATION/BOUNDARY on
// `SubtitleTrackIndex >= 0 && IsImageSubtitle(...)` inside buildSingleTranscodeArgs.
// With index=0 + image codec + single profile, mutant `> 0` disables burn-in and
// overlay filter disappears.
func TestTranscoding_KillMutant_BuildFFmpegArgs_SingleTranscodeBurnInImageSubtitle(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:          "/tmp/test",
		FilePath:           "/f.mkv",
		Profiles:           transcoding.DefaultHLSProfiles[:1],
		VideoDecision:      "transcode",
		AudioDecision:      "copy",
		SubtitleTrackIndex: 0,
		SubtitleCodec:      "pgssub",
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "filter_complex", "image subtitle burn-in must use filter_complex in single-transcode")
	assert.Contains(t, joined, "overlay", "burn-in overlay must be present")
	assert.Contains(t, joined, "[0:s:0]", "subtitle index 0 must be referenced")
}

// Kills hls_transcoder.go:221:23 CONDITIONALS_NEGATION on `toneMap != ""` inside
// buildSingleTranscodeArgs. With burnIn=false and no HDR streams (toneMap==""),
// original skips filter_complex; mutant flips to `toneMap == ""` → enters the
// block and emits filter_complex unnecessarily. We assert NO filter_complex.
func TestTranscoding_KillMutant_BuildFFmpegArgs_SingleTranscodeNoFilterComplexWhenNoBurnInNoHDR(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:          "/tmp/test",
		FilePath:           "/f.mkv",
		Profiles:           transcoding.DefaultHLSProfiles[:1],
		VideoDecision:      "transcode",
		AudioDecision:      "copy",
		SubtitleTrackIndex: -1, // no subtitles → burnIn=false
		// no Streams → toneMap=""
	}
	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	assert.NotContains(t, joined, "filter_complex",
		"single-transcode without burn-in and without HDR must use simple -vf scale (no filter_complex)")
	assert.Contains(t, joined, "-vf", "should use simple -vf scale path")
}

// Integration test — requires ffmpeg binary
func TestHLSTranscoder_Integration_ModeA(t *testing.T) {
	requireFFmpeg(t)
	videoPath := generateTestVideo(t)
	outputDir := t.TempDir()

	transcoder := ffmpeg.NewHLSTranscoder()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	handle, err := transcoder.Start(ctx, transcoding.HLSProcessOpts{
		OutputDir:       outputDir,
		FilePath:        videoPath,
		Profiles:        transcoding.DefaultHLSProfiles[:1], // 360p only
		VideoDecision:   "copy",
		AudioDecision:   "copy",
		AudioTrackIndex: 0,
	})
	require.NoError(t, err)

	err = handle.WaitReady(ctx)
	require.NoError(t, err, "WaitReady should succeed")

	// master.m3u8 should exist (generated for single-stream)
	masterPath := filepath.Join(outputDir, "master.m3u8")
	assert.FileExists(t, masterPath)

	// stream_0.m3u8 should exist
	streamPath := filepath.Join(outputDir, "stream_0.m3u8")
	assert.FileExists(t, streamPath)

	// Wait for ffmpeg to finish
	select {
	case <-handle.Done():
	case <-time.After(15 * time.Second):
		t.Fatal("ffmpeg did not finish in time")
	}

	require.NoError(t, handle.Err())

	// Verify segments were created
	entries, _ := os.ReadDir(outputDir)
	tsCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".ts") {
			tsCount++
		}
	}
	assert.Positive(t, tsCount, "should have produced .ts segments")
}

func TestHLSTranscoder_Stop(t *testing.T) {
	requireFFmpeg(t)
	videoPath := generateTestVideo(t)
	outputDir := t.TempDir()

	transcoder := ffmpeg.NewHLSTranscoder()
	ctx := context.Background()

	handle, err := transcoder.Start(ctx, transcoding.HLSProcessOpts{
		OutputDir:     outputDir,
		FilePath:      videoPath,
		Profiles:      transcoding.DefaultHLSProfiles[:1],
		VideoDecision: "copy",
		AudioDecision: "copy",
	})
	require.NoError(t, err)

	err = handle.WaitReady(context.Background())
	require.NoError(t, err)

	err = handle.Stop()
	require.NoError(t, err)

	select {
	case <-handle.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("handle.Done() not closed after Stop")
	}
}
