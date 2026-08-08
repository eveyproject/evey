package ffmpeg_test

import (
	"context"
	"fmt"
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

func TestBuildFFmpegArgs_VariantIndex_SoftwareSingle(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles[:1],
		VideoDecision: "transcode",
		AudioDecision: "transcode",
		UseFMP4:       true,
		VariantIndex:  1,
	}
	joined := strings.Join(ffmpeg.BuildFFmpegArgs(opts), " ")
	assert.Contains(t, joined, "stream_1_init.mp4")
	assert.Contains(t, joined, "stream_1_%03d.m4s")
	assert.Contains(t, joined, "stream_1.m3u8")
	assert.NotContains(t, joined, "stream_0_init.mp4", "variant 1 must not write variant 0's rung")
}

func TestBuildFFmpegArgs_VariantIndex_HWSingle(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles[:1],
		VideoDecision: "transcode",
		AudioDecision: "transcode",
		UseFMP4:       true,
		VariantIndex:  2,
		HWAccel:       transcoding.NVENC,
		VideoCodec:    "h264",
	}
	joined := strings.Join(ffmpeg.BuildFFmpegArgs(opts), " ")
	assert.Contains(t, joined, "h264_nvenc", "single HW rung must use the GPU encoder")
	assert.Contains(t, joined, "stream_2_init.mp4")
	assert.Contains(t, joined, "stream_2.m3u8")
	assert.NotContains(t, joined, "stream_0.m3u8")
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

// An HEVC video-copy must tag the fMP4 sample entry hvc1: ffmpeg's default is
// hev1, which Safari/AVPlayer refuse even when the master CODECS says hvc1.
// H.264 copy keeps no tag override (avc1 is the mux default). The tag rides
// BOTH copy branches - cold start and copy-on-seek - since each writes the
// shared init segment.
func TestBuildFFmpegArgs_HevcCopyTagsHvc1(t *testing.T) {
	base := transcoding.HLSProcessOpts{
		OutputDir:        "/tmp/test",
		FilePath:         "/f.mkv",
		Profiles:         transcoding.DefaultHLSProfiles[:1],
		VideoDecision:    "copy",
		AudioDecision:    "copy",
		SourceVideoCodec: "hevc",
		UseFMP4:          true,
	}

	cold := strings.Join(ffmpeg.BuildFFmpegArgs(base), " ")
	assert.Contains(t, cold, "-c:v copy")
	assert.Contains(t, cold, "-tag:v hvc1", "cold copy session must tag hvc1")

	seek := base
	seek.StartSec = 120
	seek.CopyOnSeek = true
	seekJoined := strings.Join(ffmpeg.BuildFFmpegArgs(seek), " ")
	assert.Contains(t, seekJoined, "-c:v copy")
	assert.Contains(t, seekJoined, "-tag:v hvc1", "copy-on-seek session must tag hvc1 (it rewrites the shared init)")

	h264 := base
	h264.SourceVideoCodec = "h264"
	h264Joined := strings.Join(ffmpeg.BuildFFmpegArgs(h264), " ")
	assert.NotContains(t, h264Joined, "-tag:v", "h264 copy keeps the avc1 mux default")
}

// All builders MUST emit the event-mode HLS playlist flags. -hls_list_size 0
// + -hls_playlist_type event keep the full growing playlist so hls.js never
// snaps to the live edge (eliminates the drift-jump on long playback).
// Disk cleanup is guaranteed by HLSSession.stop() → os.RemoveAll(OutputDir).
//
// Note: -readrate is now PER-PRODUCER (HLSProcessOpts.ReadRate). The default
// (0) emits no -readrate - byte-identical to the multi-rafale HW/orchestrator
// path, which relies on its adaptive SIGSTOP/SIGCONT throttle and must NOT be
// input-paced (it cripples cold start). The single-ffmpeg software producer
// opts in (ReadRate>0); see TestBuildFFmpegArgs_ReadRatePacing.
func TestBuildFFmpegArgs_BoundedCacheArgs(t *testing.T) {
	cases := []struct {
		name string
		opts transcoding.HLSProcessOpts
	}{
		{
			name: "copy_video",
			opts: transcoding.HLSProcessOpts{
				OutputDir: "/tmp/t", FilePath: "/f.mkv",
				Profiles:      transcoding.DefaultHLSProfiles,
				VideoDecision: "copy", AudioDecision: "copy",
			},
		},
		{
			name: "single_transcode",
			opts: transcoding.HLSProcessOpts{
				OutputDir: "/tmp/t", FilePath: "/f.mkv",
				Profiles:      transcoding.DefaultHLSProfiles[:1],
				VideoDecision: "transcode", AudioDecision: "copy",
			},
		},
		{
			name: "multi_variant",
			opts: transcoding.HLSProcessOpts{
				OutputDir: "/tmp/t", FilePath: "/f.mkv",
				Profiles:      transcoding.DefaultHLSProfiles,
				VideoDecision: "transcode", AudioDecision: "transcode",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			joined := strings.Join(ffmpeg.BuildFFmpegArgs(tc.opts), " ")
			assert.NotContains(t, joined, "-readrate", "default ReadRate=0 must emit no -readrate - byte-identical to the HW/orchestrator path")
			assert.Contains(t, joined, "-hls_list_size 0", "playlist must be unbounded (event-type, growing) to avoid drift-out on long playback")
			assert.Contains(t, joined, "-hls_playlist_type event", "playlist must be marked event so hls.js doesn't snap to liveSyncPosition")
			assert.Contains(t, joined, "-hls_flags temp_file", "temp_file flag makes ffmpeg rename segments atomically - eliminates read-while-write races (audit indies: Stash pattern, free with this flag)")
			assert.NotContains(t, joined, "delete_segments", "delete_segments must not be set - session.stop() cleans up the OutputDir")
		})
	}
}

// TestBuildFFmpegArgs_ReadRatePacing locks the per-producer input pacing:
// ReadRate=0 (the HW/orchestrator default) emits no -readrate; ReadRate>0 (the
// single-ffmpeg software producer) emits `-readrate N` as a pre-`-i` flag so
// the lone encoder stays near wall-clock. This is the Phase-0 plumbing for the
// software-flow decoupling - inert until a producer sets ReadRate.
func TestBuildFFmpegArgs_ReadRatePacing(t *testing.T) {
	base := transcoding.HLSProcessOpts{
		OutputDir: "/tmp/t", FilePath: "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles[:1],
		VideoDecision: "transcode", AudioDecision: "copy",
	}

	off := strings.Join(ffmpeg.BuildFFmpegArgs(base), " ")
	assert.NotContains(t, off, "-readrate", "ReadRate=0 must not pace the input (HW/orchestrator default)")

	paced := base
	paced.ReadRate = 1.5
	on := strings.Join(ffmpeg.BuildFFmpegArgs(paced), " ")
	assert.Contains(t, on, "-readrate 1.5", "ReadRate=1.5 must emit `-readrate 1.5` (software producer pacing)")
	// Must be a PRE-input flag: -readrate appears before -i.
	assert.Less(t, strings.Index(on, "-readrate"), strings.Index(on, "-i "),
		"-readrate is an input option and must precede -i")
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
	assert.Contains(t, joined, "[0:v]bwdif=mode=send_frame:deint=interlaced,split=")
	assert.Contains(t, joined, "-force_key_frames")
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
	assert.Contains(t, joined, "[0:v]bwdif=mode=send_frame:deint=interlaced[vd]")
	assert.Contains(t, joined, "[vd][0:s:2]overlay[vb]")
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
	// Single transcode must deinterlace (fixes combing on interlaced sources)
	// and force segment-aligned keyframes (fixes slow first-segment startup).
	assert.Contains(t, joined, "bwdif=mode=send_frame:deint=interlaced,scale=")
	assert.Contains(t, joined, "-force_key_frames expr:gte(t,n_forced*3)")
}

func TestBuildFFmpegArgs_SingleTranscode_ToneMap_FoldsScaleIntoTonemap(t *testing.T) {
	// HDR->SDR on the single-variant software path: the downscale must be folded
	// into the tonemap's linearizing zscale so the heavy float pipeline runs at
	// the delivery resolution (~2× cheaper on a weak host = stays real-time on 4K
	// HDR), with NO separate scale appended after the tonemap output.
	opts := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles[:1], // 360p, width 640
		VideoDecision: "transcode",
		AudioDecision: "copy",
		ToneMap:       true,
	}
	joined := strings.Join(ffmpeg.BuildFFmpegArgs(opts), " ")
	w := opts.Profiles[0].Width
	// The downscale must fold into the heavy tonemap pass (run at delivery res).
	// The fold looks different per ffmpeg build: zscale=w=W:h=-2:t=linear (portable)
	// or scale=W:-2,tonemapx (jellyfin-ffmpeg's fused SIMD filter, in the container).
	if strings.Contains(joined, "tonemapx") {
		assert.Contains(t, joined, fmt.Sprintf("scale=%d:-2,tonemapx", w),
			"the downscale must fold into the tonemapx pass")
	} else {
		assert.Contains(t, joined, fmt.Sprintf("zscale=w=%d:h=-2:t=linear", w),
			"the downscale must fold into the linearizing zscale")
	}
	assert.Contains(t, joined, "tonemap=")
	assert.NotContains(t, joined, "format=yuv420p,scale=",
		"no separate scale after the tonemap - it already downscaled")
}

func TestBuildFFmpegArgs_MultiVariant_ToneMap_FoldsScaleAtTopRung(t *testing.T) {
	// Multi-variant HDR: the single shared tonemap feed must be downscaled to the
	// LARGEST rung width (folded into its zscale), so the heavy float pipeline
	// runs at the top delivery resolution instead of the 4K source; the per-rung
	// scalers then only downscale from there. Without this the tonemap pays for
	// the full 4K frame once, shared across rungs - the exact cost that pushed a
	// weak host under realtime.
	opts := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles, // 4 rungs, max width 1920 (1080p)
		VideoDecision: "transcode",
		AudioDecision: "copy",
		ToneMap:       true,
	}
	joined := strings.Join(ffmpeg.BuildFFmpegArgs(opts), " ")
	maxW := 0
	for _, p := range opts.Profiles {
		if p.Width > maxW {
			maxW = p.Width
		}
	}
	if strings.Contains(joined, "tonemapx") {
		assert.Contains(t, joined, fmt.Sprintf("scale=%d:-2,tonemapx", maxW),
			"the shared tonemap must fold the downscale to the largest rung width (tonemapx)")
	} else {
		assert.Contains(t, joined, fmt.Sprintf("zscale=w=%d:h=-2:t=linear", maxW),
			"the shared tonemap must fold the downscale to the largest rung width (zscale)")
	}
	// Still exactly one tonemap, before the split (shared by all rungs).
	assert.Equal(t, 1, strings.Count(joined, "tonemap="), "tonemap must run once, before the split")
	assert.Contains(t, joined, "split=")
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
	// Both ffprobe canonical codec_names and ffmpeg encoder aliases - dvb_subtitle
	// (canonical) must be matched, not only the dvbsub alias.
	imageCodecs := []string{
		"hdmv_pgs_subtitle", "pgssub",
		"dvd_subtitle", "dvdsub",
		"dvb_subtitle", "dvbsub",
		"xsub",
		"DVD_SUBTITLE", // case-insensitive
	}
	for _, c := range imageCodecs {
		assert.True(t, ffmpeg.IsImageSubtitle(c), "expected %s to be image subtitle", c)
	}
	assert.False(t, ffmpeg.IsImageSubtitle("srt"), "srt is not image subtitle")
	assert.False(t, ffmpeg.IsImageSubtitle("ass"), "ass is not image subtitle")
	assert.False(t, ffmpeg.IsImageSubtitle("subrip"), "subrip is not image subtitle")
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

// av1 software output maps to libsvtav1 (libaom-av1 is absent from the shipped
// jellyfin-ffmpeg build) and MUST carry a numeric -preset: libsvtav1 rejects the
// named x264/x265 presets ("veryfast") and fails to open the encoder otherwise.
// Asserted at the arg-building layer without spawning a real encode.
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
	assert.Contains(t, joined, "libsvtav1")
	assert.NotContains(t, joined, "libaom-av1")
	assert.Contains(t, joined, "-preset 10", "libsvtav1 needs a numeric preset")
	assert.NotContains(t, joined, "veryfast", "libsvtav1 rejects the named x264 preset")
}

// EVERY paced transcode gets a cold-start input burst: the opening seconds are
// read at full speed before the -readrate pace applies.
//
// This used to be AV1-only, on the theory that only SVT-AV1's encoder latency
// made the first segment late. That was the wrong variable. The binding
// constraint at startup is the INPUT PACE, which is codec-blind: at 1.5x, the
// third 3 s segment cannot exist before 9/1.5 = 6 s of reading plus ffmpeg's
// startup, whatever encoder is downstream. Measured in prod on 2026-07-25 (4K
// HEVC HDR -> h264 1080p): the player gave up at 7.75 s on a segment that was
// arithmetically unreachable, on a host that was NOT saturated. Pacing from the
// first frame also means the viewer's buffer only grows at 0.5x real time
// (1.5 produced - 1.0 consumed), so nothing absorbs an early hiccup.
func TestBuildFFmpegArgs_Transcode_ColdStartBurst(t *testing.T) {
	base := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles[:1],
		VideoDecision: "transcode",
		AudioDecision: "copy",
		ReadRate:      1.5,
	}

	for _, codec := range []string{"av1", "h264", "hevc"} {
		j := strings.Join(ffmpeg.BuildFFmpegArgs(withVideoCodec(base, codec)), " ")
		assert.Contains(t, j, "-readrate 1.5", codec)
		assert.Contains(t, j, "-readrate_initial_burst 60",
			"%s transcode needs the cold-start burst: the input pace, not the encoder, is what makes segment 0 late, "+
				"and the banked lead is what lets a session ride out a spike instead of starving", codec)
		// Input option: ffmpeg ignores it after -i.
		assert.Less(t, strings.Index(j, "-readrate_initial_burst"), strings.Index(j, "-i "),
			"%s: -readrate_initial_burst must precede -i", codec)
	}
}

// The burst rides ON the pace: an unpaced producer (the HW orchestrator leaves
// ReadRate 0 and self-throttles with SIGSTOP instead) must stay byte-identical.
func TestBuildFFmpegArgs_Transcode_NoPaceNoBurst(t *testing.T) {
	base := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles[:1],
		VideoDecision: "transcode",
		AudioDecision: "copy",
		ReadRate:      0,
	}
	for _, codec := range []string{"av1", "h264", "hevc"} {
		j := strings.Join(ffmpeg.BuildFFmpegArgs(withVideoCodec(base, codec)), " ")
		assert.NotContains(t, j, "-readrate", "%s: an unpaced session emits no pacing flags at all", codec)
		assert.NotContains(t, j, "readrate_initial_burst", codec)
	}
}

func withVideoCodec(o transcoding.HLSProcessOpts, codec string) transcoding.HLSProcessOpts {
	o.VideoCodec = codec
	return o
}

// Same guard on the multi-variant software path (the per-rung builder also
// hardcoded the named preset).
func TestBuildFFmpegArgs_AV1_MultiVariant_NumericPreset(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir:     "/tmp/test",
		FilePath:      "/f.mkv",
		Profiles:      transcoding.DefaultHLSProfiles[:3],
		VideoDecision: "transcode",
		AudioDecision: "copy",
		VideoCodec:    "av1",
	}
	joined := strings.Join(ffmpeg.BuildFFmpegArgs(opts), " ")
	assert.Contains(t, joined, "-c:v:0 libsvtav1")
	assert.NotContains(t, joined, "libaom-av1")
	assert.Contains(t, joined, "-preset 10", "libsvtav1 needs a numeric preset")
	assert.NotContains(t, joined, "veryfast", "libsvtav1 rejects the named x264 preset")
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
	assert.Contains(t, joined, "[0:v]bwdif=mode=send_frame:deint=interlaced,split=")
	assert.Contains(t, joined, "-force_key_frames")
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

// Used to assert that H.264 copy emitted TS segments, but fMP4 is now the
// project default for every codec (post-WWDC-2024 alignment). H.264 copy
// must produce .m4s like everyone else.
func TestBuildFFmpegArgs_H264Copy_UsesFMP4Segments(t *testing.T) {
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
	assert.Contains(t, joined, "%03d.m4s")
	assert.Contains(t, joined, "-hls_segment_type fmp4")
}

// Verifies that buildCopyVideoArgs forces a video re-encode when StartSec > 0.
// Pre-input -ss + re-encode: ffmpeg's accurate_seek default (since 2.1)
// makes the output frame-accurate, fast manifest write, and -muxpreload
// 0 -muxdelay 0 keeps HLS segment PTS flush with t=0 so hls.js's clock
// matches the requested second.
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
	assert.Contains(t, joined, "-ss 358.500", "must seek at requested timestamp")
	assert.Contains(t, joined, "-g 24", "1s GOP so first keyframe lands within 1s of seek")
	assert.Contains(t, joined, "-keyint_min 24", "minimum GOP must match -g so libx264 honours it")
	assert.Contains(t, joined, "-copyts", "preserve source PTS so the seek aligns")
	assert.Contains(t, joined, "-start_at_zero", "translate first segment PTS to 0 for hls.js startPosition: 0")
	assert.Contains(t, joined, "-avoid_negative_ts disabled", "let copyts pass through without ffmpeg clamping")
	// -ss MUST come BEFORE -i for fast seek + accurate_seek default.
	iIdx := -1
	ssIdx := -1
	for i, a := range args {
		if a == "-i" && iIdx == -1 {
			iIdx = i
		}
		if a == "-ss" && ssIdx == -1 {
			ssIdx = i
		}
	}
	require.GreaterOrEqual(t, iIdx, 0)
	require.GreaterOrEqual(t, ssIdx, 0)
	assert.Less(t, ssIdx, iIdx, "expected -ss BEFORE -i for fast accurate seek")
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

// --- Audio-only pipeline tests ---

// BuildFFmpegAudioOnlyArgs must always include -vn so no video track leaks
// into the audio-only rendition.
func TestBuildFFmpegAudioOnlyArgs_ContainsVN(t *testing.T) {
	opts := transcoding.AudioProcessOpts{
		FilePath:       "/media/movie.mkv",
		OutputDir:      "/tmp/audio",
		SourceTrackIdx: 0,
		AudioCodec:     "aac",
		AudioBitrate:   192,
	}
	args := ffmpeg.BuildFFmpegAudioOnlyArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-vn", "-vn must suppress the video stream")
}

// A passthrough (AudioCodec="copy") rendition muxes the source AC-3/E-AC-3
// verbatim for the wasm-audio path: -c:a copy, and NONE of the re-encode args
// (-b:a/-ac/-ar/-af) - ffmpeg refuses those alongside a stream copy.
func TestBuildFFmpegAudioOnlyArgs_CopyPassthrough(t *testing.T) {
	opts := transcoding.AudioProcessOpts{
		FilePath:         "/media/movie.mkv",
		OutputDir:        "/tmp/audio",
		SourceTrackIdx:   0,
		AudioCodec:       "copy",
		AudioBitrate:     192,  // must be ignored on a copy
		MaxAudioChannels: 6,    // must be ignored on a copy
		AudioNormalize:   true, // loudnorm cannot run on a copy
	}
	args := ffmpeg.BuildFFmpegAudioOnlyArgs(opts)
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-c:a copy", "passthrough must stream-copy the audio")
	assert.Contains(t, joined, "-vn", "-vn still suppresses video")
	assert.Contains(t, joined, "0:a:0", "still maps the selected audio track")
	assert.NotContains(t, joined, "-b:a", "no bitrate arg on a stream copy")
	assert.NotContains(t, joined, "-ac ", "no channel arg on a stream copy")
	assert.NotContains(t, joined, "-ar ", "no sample-rate arg on a stream copy")
	assert.NotContains(t, joined, "loudnorm", "loudnorm cannot run on a stream copy")
	// The fMP4 HLS muxer is still emitted (container work, not sample work).
	assert.Contains(t, joined, "-hls_segment_type fmp4")
}

// The -map selector must reference the correct 0-indexed audio stream.
func TestBuildFFmpegAudioOnlyArgs_MapTrackIndex(t *testing.T) {
	cases := []struct {
		trackIdx int
		want     string
	}{
		{0, "0:a:0"},
		{1, "0:a:1"},
		{3, "0:a:3"},
	}
	for _, tc := range cases {
		opts := transcoding.AudioProcessOpts{
			FilePath:       "/media/movie.mkv",
			OutputDir:      "/tmp/audio",
			SourceTrackIdx: tc.trackIdx,
		}
		args := ffmpeg.BuildFFmpegAudioOnlyArgs(opts)
		joined := strings.Join(args, " ")
		assert.Contains(t, joined, tc.want,
			"map flag must reference audio track index %d", tc.trackIdx)
	}
}

// The init segment filename must embed the track index (audio_T_init.mp4).
func TestBuildFFmpegAudioOnlyArgs_InitFilenameEmbedsTackIdx(t *testing.T) {
	cases := []int{0, 2, 5}
	for _, idx := range cases {
		opts := transcoding.AudioProcessOpts{
			FilePath:       "/media/movie.mkv",
			OutputDir:      "/tmp/audio",
			SourceTrackIdx: idx,
		}
		args := ffmpeg.BuildFFmpegAudioOnlyArgs(opts)
		joined := strings.Join(args, " ")
		want := fmt.Sprintf("audio_%d_init.mp4", idx)
		assert.Contains(t, joined, want,
			"init filename must be audio_%d_init.mp4 for track %d", idx, idx)
	}
}

// StartSec > 0 must emit -ss BEFORE -i; StartSec == 0 must not emit -ss.
// StartNumber must appear in the args.
func TestBuildFFmpegAudioOnlyArgs_StartSecAndStartNumber(t *testing.T) {
	t.Run("seek applied", func(t *testing.T) {
		opts := transcoding.AudioProcessOpts{
			FilePath:       "/media/movie.mkv",
			OutputDir:      "/tmp/audio",
			SourceTrackIdx: 0,
			StartSec:       90.0,
			StartNumber:    30, // floor(90/3)
		}
		args := ffmpeg.BuildFFmpegAudioOnlyArgs(opts)
		joined := strings.Join(args, " ")

		assert.Contains(t, joined, "-ss 90.000", "seek flag must be present")
		assert.Contains(t, joined, "-start_number 30", "start_number must match")

		// -ss must come before -i
		ssIdx, iIdx := -1, -1
		for i, a := range args {
			if a == "-ss" && ssIdx < 0 {
				ssIdx = i
			}
			if a == "-i" && iIdx < 0 {
				iIdx = i
			}
		}
		require.Greater(t, ssIdx, -1, "-ss must be present")
		require.Greater(t, iIdx, -1, "-i must be present")
		assert.Less(t, ssIdx, iIdx, "-ss must come before -i")
	})

	t.Run("no seek when StartSec zero", func(t *testing.T) {
		opts := transcoding.AudioProcessOpts{
			FilePath:       "/media/movie.mkv",
			OutputDir:      "/tmp/audio",
			SourceTrackIdx: 0,
			StartSec:       0,
			StartNumber:    0,
		}
		args := ffmpeg.BuildFFmpegAudioOnlyArgs(opts)
		for _, a := range args {
			assert.NotEqual(t, "-ss", a, "StartSec=0 must NOT emit -ss")
		}
		joined := strings.Join(args, " ")
		assert.Contains(t, joined, "-start_number 0")
	})
}

// AudioNormalize must inject the loudnorm filter; absence must not emit it.
func TestBuildFFmpegAudioOnlyArgs_AudioNormalize(t *testing.T) {
	base := transcoding.AudioProcessOpts{
		FilePath:       "/media/movie.mkv",
		OutputDir:      "/tmp/audio",
		SourceTrackIdx: 0,
	}

	withNorm := base
	withNorm.AudioNormalize = true
	joined := strings.Join(ffmpeg.BuildFFmpegAudioOnlyArgs(withNorm), " ")
	assert.Contains(t, joined, "loudnorm=I=-16:TP=-1.5:LRA=11:linear=true")

	withoutNorm := base
	withoutNorm.AudioNormalize = false
	joined = strings.Join(ffmpeg.BuildFFmpegAudioOnlyArgs(withoutNorm), " ")
	assert.NotContains(t, joined, "loudnorm")
}

// BitRate > 0 must emit -b:a; 0 must not.
func TestBuildFFmpegAudioOnlyArgs_Bitrate(t *testing.T) {
	opts := transcoding.AudioProcessOpts{
		FilePath:       "/media/movie.mkv",
		OutputDir:      "/tmp/audio",
		SourceTrackIdx: 0,
		AudioBitrate:   256,
	}
	joined := strings.Join(ffmpeg.BuildFFmpegAudioOnlyArgs(opts), " ")
	assert.Contains(t, joined, "-b:a 256k")

	opts.AudioBitrate = 0
	joined = strings.Join(ffmpeg.BuildFFmpegAudioOnlyArgs(opts), " ")
	assert.NotContains(t, joined, "-b:a")
}

// The output playlist path must be <OutputDir>/audio_T.m3u8.
func TestBuildFFmpegAudioOnlyArgs_PlaylistPath(t *testing.T) {
	opts := transcoding.AudioProcessOpts{
		FilePath:       "/media/movie.mkv",
		OutputDir:      "/tmp/session42",
		SourceTrackIdx: 2,
	}
	args := ffmpeg.BuildFFmpegAudioOnlyArgs(opts)
	last := args[len(args)-1]
	assert.Equal(t, "/tmp/session42/audio_2.m3u8", last,
		"last arg must be the playlist output path")
}

// The audio rendition must honour input pacing: ReadRate>0 emits `-readrate N`
// plus a full-speed `-readrate_initial_burst`, both BEFORE `-i` (input options),
// so the single long-lived AC-3/E-AC-3→AAC re-encode is bounded to N× real-time
// instead of bursting a full core per client. ReadRate 0 (default) must emit
// neither flag - byte-for-byte and CPU-profile identical to the pre-pacing path.
func TestBuildFFmpegAudioOnlyArgs_ReadRatePacing(t *testing.T) {
	base := transcoding.AudioProcessOpts{
		FilePath:       "/media/movie.mkv",
		OutputDir:      "/tmp/audio",
		SourceTrackIdx: 0,
		AudioCodec:     "aac",
		AudioBitrate:   128,
	}

	off := strings.Join(ffmpeg.BuildFFmpegAudioOnlyArgs(base), " ")
	assert.NotContains(t, off, "-readrate", "ReadRate 0 must not pace the input")

	paced := base
	paced.ReadRate = 8
	on := ffmpeg.BuildFFmpegAudioOnlyArgs(paced)
	joined := strings.Join(on, " ")
	assert.Contains(t, joined, "-readrate 8", "ReadRate 8 must emit `-readrate 8`")
	assert.Contains(t, joined, "-readrate_initial_burst 30",
		"paced audio must keep a full-speed initial burst so cold start/seek fill instantly")

	// Both are input options and MUST precede -i.
	assert.Less(t, strings.Index(joined, "-readrate"), strings.Index(joined, "-i "),
		"-readrate must precede -i")
	assert.Less(t, strings.Index(joined, "-readrate_initial_burst"), strings.Index(joined, "-i "),
		"-readrate_initial_burst must precede -i")
}

// Event-mode HLS flags must be present (consistent with video pipeline).
func TestBuildFFmpegAudioOnlyArgs_HLSEventModeFlags(t *testing.T) {
	opts := transcoding.AudioProcessOpts{
		FilePath:       "/media/movie.mkv",
		OutputDir:      "/tmp/audio",
		SourceTrackIdx: 0,
	}
	joined := strings.Join(ffmpeg.BuildFFmpegAudioOnlyArgs(opts), " ")
	assert.Contains(t, joined, "-hls_list_size 0")
	assert.Contains(t, joined, "-hls_playlist_type event")
	assert.Contains(t, joined, "-hls_flags temp_file")
	assert.Contains(t, joined, "-hls_segment_type fmp4")
	assert.NotContains(t, joined, "delete_segments")
}

// Integration test - requires ffmpeg binary
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

	// Verify segments were created. Default profiles now use fMP4 → .m4s
	// (per the HLS spec quick-wins flip), so we accept either container.
	entries, _ := os.ReadDir(outputDir)
	segCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".ts") || strings.HasSuffix(e.Name(), ".m4s") {
			segCount++
		}
	}
	assert.Positive(t, segCount, "should have produced segments (.ts or .m4s)")
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

// --- B3: deinterlace mode + anamorphic scale (unit, pure filter strings) ---

func TestDeinterlaceFilterFor(t *testing.T) {
	// Default (flag-gated) - a no-op on progressive content.
	assert.Equal(t, "bwdif=mode=send_frame:deint=interlaced",
		ffmpeg.DeinterlaceFilterForTest(transcoding.HLSProcessOpts{}))
	// idet flagged the content interlaced despite lying container flags.
	assert.Equal(t, "bwdif=mode=send_frame:deint=all",
		ffmpeg.DeinterlaceFilterForTest(transcoding.HLSProcessOpts{DeinterlaceAll: true}))
}

func TestScaleFilterFor(t *testing.T) {
	// Square pixels (or unset) keep the historical filter exactly.
	assert.Equal(t, "scale=640:-2,setsar=1",
		ffmpeg.ScaleFilterForTest(transcoding.HLSProcessOpts{}, 640))
	assert.Equal(t, "scale=640:-2,setsar=1",
		ffmpeg.ScaleFilterForTest(transcoding.HLSProcessOpts{SourceSAR: "1:1"}, 640))
	// Anamorphic PAL DVD (SAR 64:45): height computed from the DISPLAY aspect.
	// 720x576 DAR 16:9 → 640 wide must yield 360 high, via the embedded
	// trunc(W*ih*den/(iw*num)/2)*2 expression.
	assert.Equal(t, "scale=640:'trunc(640*ih*45/(iw*64)/2)*2',setsar=1",
		ffmpeg.ScaleFilterForTest(transcoding.HLSProcessOpts{SourceSAR: "64:45"}, 640))
	// A malformed SAR falls back to the square-pixel filter (never errors).
	assert.Equal(t, "scale=640:-2,setsar=1",
		ffmpeg.ScaleFilterForTest(transcoding.HLSProcessOpts{SourceSAR: "garbage"}, 640))
}

// End-to-end through the single-transcode builder: an anamorphic interlaced
// mpeg2 source emits BOTH the deint=all fallback and the SAR-corrected scale.
func TestBuildFFmpegArgs_LegacyInterlacedAnamorphic(t *testing.T) {
	opts := transcoding.HLSProcessOpts{
		OutputDir: "/tmp/t", FilePath: "/f.mpg",
		Profiles:      transcoding.DefaultHLSProfiles[:1],
		VideoDecision: "transcode", AudioDecision: "copy",
		DeinterlaceAll: true,
		SourceSAR:      "64:45",
	}
	joined := strings.Join(ffmpeg.BuildFFmpegArgs(opts), " ")
	assert.Contains(t, joined, "bwdif=mode=send_frame:deint=all")
	assert.Contains(t, joined, "trunc(", "anamorphic height expression present")
	assert.NotContains(t, joined, "deint=interlaced", "deint=all replaces the flag-gated mode")
}

// The audio-rendition path must scale the bitrate to the channel count the same
// way the muxed ladder does (audioOutputArgsSingle). Serving a 5.1 track at the
// stereo base would ship a 6-channel AAC at 128 kbps - the hybrid mkvgo rung is
// meant to preserve the source's audio, not quietly degrade it.
func TestBuildFFmpegAudioOnlyArgs_ScalesBitrateToChannels(t *testing.T) {
	args := ffmpeg.BuildFFmpegAudioOnlyArgs(transcoding.AudioProcessOpts{
		FilePath:         "/media/x.mkv",
		AudioCodec:       "aac",
		AudioBitrate:     128, // stereo base, as the ladder carries it
		MaxAudioChannels: 6,   // 5.1 source
	})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-ac 6") {
		t.Errorf("expected 6 channels preserved, got: %s", joined)
	}
	if !strings.Contains(joined, "-b:a 384k") { // 6 * 64 kbps, same rule as the muxed path
		t.Errorf("expected bitrate scaled to 384k for 5.1, got: %s", joined)
	}
}

// The audio rendition must land on the SAME timeline as the video.
//
// A video with B-frames cannot have a negative DTS in MP4, so its first frame is
// PRESENTED at has_b_frames/fps - measured: 42 ms (1 B-frame @ 23.976) and 83 ms
// (2 B-frames). Muxed, ffmpeg shifted both streams together. Split into its own
// rendition, the audio starts at 0 and therefore runs EARLY by exactly that
// amount - past the ~45 ms audibility threshold for early audio (ITU-R BT.1359),
// which is how the operator heard it.
//
// So the audio-only session must offset its output by the video's composition
// delay. -output_ts_offset shifts the OUTPUT timestamps (what the player reads),
// unlike -itsoffset which shifts the input.
func TestBuildFFmpegAudioOnlyArgs_OffsetsByVideoCompositionDelay(t *testing.T) {
	args := ffmpeg.BuildFFmpegAudioOnlyArgs(transcoding.AudioProcessOpts{
		FilePath:              "/media/x.mkv",
		AudioCodec:            "aac",
		AudioBitrate:          128,
		VideoCompositionDelay: 0.042, // 1 B-frame at 23.976 fps
	})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-output_ts_offset 0.042") {
		t.Errorf("audio must be shifted onto the video's timeline, got: %s", joined)
	}

	// No B-frames (or unknown): nothing to compensate, args stay as they were.
	plain := ffmpeg.BuildFFmpegAudioOnlyArgs(transcoding.AudioProcessOpts{
		FilePath: "/media/x.mkv", AudioCodec: "aac", AudioBitrate: 128,
	})
	if strings.Contains(strings.Join(plain, " "), "-output_ts_offset") {
		t.Error("a source with no composition delay must not be shifted")
	}
}
