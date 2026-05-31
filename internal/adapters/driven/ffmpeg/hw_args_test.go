package ffmpeg

import (
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

func argsContain(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// argsContainSeq reports whether want appears as a contiguous subsequence.
func argsContainSeq(args []string, want ...string) bool {
	for i := 0; i+len(want) <= len(args); i++ {
		match := true
		for j, w := range want {
			if args[i+j] != w {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func sampleHWOpts(accel transcoding.HWAccelType, device string, nProfiles int) transcoding.HLSProcessOpts {
	profiles := []transcoding.HLSProfile{
		{Name: "360p", Width: 640, Height: 360, VideoBitrate: 800, AudioBitrate: 96},
		{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 2800, AudioBitrate: 128},
		{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 5000, AudioBitrate: 192},
	}
	if nProfiles < len(profiles) {
		profiles = profiles[:nProfiles]
	}
	return transcoding.HLSProcessOpts{
		OutputDir:          "/out",
		FilePath:           "/in.mkv",
		Profiles:           profiles,
		VideoDecision:      "transcode",
		AudioDecision:      "transcode",
		AudioTrackIndex:    0,
		SubtitleTrackIndex: -1,
		VideoCodec:         "h264",
		AudioCodec:         "aac",
		HWAccel:            accel,
		HWDevice:           device,
		UseFMP4:            true,
	}
}

func TestHWUsable(t *testing.T) {
	type tc struct {
		accel   transcoding.HWAccelType
		burnIn  bool
		toneMap string
		want    bool
	}
	cases := []tc{
		{transcoding.NVENC, false, "", true},
		{transcoding.VAAPI, false, "", true},
		{transcoding.QSV, false, "", true},
		{transcoding.None, false, "", false},
		{"", false, "", false},
		{transcoding.HWAccelType("bogus"), false, "", false},
		{transcoding.NVENC, true, "", false},                // burn-in → software
		{transcoding.NVENC, false, "zscale,tonemap", false}, // tonemap → software
		{transcoding.VAAPI, true, "zscale,tonemap", false},
	}
	for _, c := range cases {
		opts := transcoding.HLSProcessOpts{HWAccel: c.accel}
		if got := hwUsable(opts, c.burnIn, c.toneMap); got != c.want {
			t.Errorf("hwUsable(accel=%q,burnIn=%v,tone=%q) = %v, want %v", c.accel, c.burnIn, c.toneMap, got, c.want)
		}
	}
}

func TestBuildMultiVariantHWArgs_NVENC(t *testing.T) {
	args := BuildFFmpegArgs(sampleHWOpts(transcoding.NVENC, "", 3))
	if !argsContainSeq(args, "-hwaccel", "cuda") {
		t.Error("expected -hwaccel cuda")
	}
	if !argsContainSeq(args, "-hwaccel_output_format", "cuda") {
		t.Error("expected -hwaccel_output_format cuda")
	}
	if !argsContain(args, "h264_nvenc") {
		t.Error("expected h264_nvenc encoder")
	}
	if !argsContainSeq(args, "-preset", "p4") {
		t.Error("expected NVENC preset p4")
	}
	// filter_complex must carry split + scale_cuda per rung, no software scale.
	fc, ok := filterComplexOf(args)
	if !ok {
		t.Fatal("expected -filter_complex")
	}
	if !strings.Contains(fc, "split=3") || !strings.Contains(fc, "scale_cuda=640:-2") || !strings.Contains(fc, "scale_cuda=1920:-2") {
		t.Errorf("filter_complex missing HW split/scale: %q", fc)
	}
	// Deinterlace runs once on the source, before the split (not per rung).
	if !strings.Contains(fc, "[0:v]yadif_cuda=deint=interlaced,split=3") {
		t.Errorf("expected single pre-split yadif_cuda: %q", fc)
	}
	if strings.Contains(fc, "scale=") && !strings.Contains(fc, "scale_cuda") {
		t.Errorf("filter_complex leaked software scale: %q", fc)
	}
	// NVENC must NOT pin a host pixel format (frames are GPU surfaces).
	if argsContain(args, "yuv420p") {
		t.Error("HW path must not set -pix_fmt yuv420p")
	}
	// Container/naming tail must match the software multi-variant builder.
	if !argsContain(args, "fmp4") || !argsContainSeq(args, "-master_pl_name", "master.m3u8") {
		t.Error("expected shared fMP4 multi-variant muxer tail")
	}
}

func TestBuildMultiVariantHWArgs_VAAPI_Device(t *testing.T) {
	args := BuildFFmpegArgs(sampleHWOpts(transcoding.VAAPI, "/dev/dri/renderD128", 2))
	if !argsContainSeq(args, "-hwaccel", "vaapi") {
		t.Error("expected -hwaccel vaapi")
	}
	if !argsContainSeq(args, "-hwaccel_device", "/dev/dri/renderD128") {
		t.Error("expected -hwaccel_device with render node")
	}
	if !argsContain(args, "h264_vaapi") {
		t.Error("expected h264_vaapi encoder")
	}
	fc, ok := filterComplexOf(args)
	if !ok || !strings.Contains(fc, "scale_vaapi=640:-2") {
		t.Errorf("expected scale_vaapi in filter_complex, got %q", fc)
	}
	if !strings.Contains(fc, "[0:v]deinterlace_vaapi,split=2") {
		t.Errorf("expected single pre-split deinterlace_vaapi: %q", fc)
	}
}

func TestBuildSingleTranscodeHWArgs(t *testing.T) {
	args := BuildFFmpegArgs(sampleHWOpts(transcoding.NVENC, "", 1))
	if !argsContainSeq(args, "-hwaccel", "cuda") {
		t.Error("expected -hwaccel cuda on single-rung HW path")
	}
	// Deinterlace (no-op on progressive) precedes the GPU scale in one -vf chain.
	if !argsContainSeq(args, "-vf", "yadif_cuda=deint=interlaced,scale_cuda=640:-2") {
		t.Error("expected -vf yadif_cuda,scale_cuda on single-rung HW path")
	}
	if !argsContain(args, "h264_nvenc") {
		t.Error("expected h264_nvenc")
	}
}

func TestHWEncoderQualityArgs_FeatureGating(t *testing.T) {
	joined := func(a []string) string { return strings.Join(a, " ") }

	// Baseline (no probed features) must be byte-for-byte the pre-probe args.
	if got := joined(hwEncoderQualityArgs(transcoding.NVENC, nil)); got != "-preset p4 -forced-idr 1" {
		t.Errorf("NVENC baseline = %q, want '-preset p4 -forced-idr 1'", got)
	}
	if got := joined(hwEncoderQualityArgs(transcoding.QSV, nil)); got != "-preset veryfast" {
		t.Errorf("QSV baseline = %q, want '-preset veryfast'", got)
	}
	if got := hwEncoderQualityArgs(transcoding.VAAPI, nil); len(got) != 0 {
		t.Errorf("VAAPI baseline must be empty, got %v", got)
	}

	// NVENC tuning appears only for probed features, forced-idr stays.
	nv := joined(hwEncoderQualityArgs(transcoding.NVENC, []string{"temporal_aq", "rc_lookahead"}))
	for _, want := range []string{"-preset p4", "-forced-idr 1", "-temporal-aq 1", "-rc-lookahead 20"} {
		if !strings.Contains(nv, want) {
			t.Errorf("NVENC tuned args %q missing %q", nv, want)
		}
	}
	if strings.Contains(nv, "-b_ref_mode") || strings.Contains(nv, "-spatial-aq") || strings.Contains(nv, "-bf ") {
		t.Errorf("NVENC emitted an unprobed option: %q", nv)
	}

	// QSV forced_idr is gated (the option an old old iGPU QSV would reject).
	if got := joined(hwEncoderQualityArgs(transcoding.QSV, []string{"forced_idr"})); !strings.Contains(got, "-forced_idr 1") {
		t.Errorf("QSV with forced_idr probed = %q, want it to include -forced_idr 1", got)
	}
	if got := joined(hwEncoderQualityArgs(transcoding.QSV, nil)); strings.Contains(got, "forced_idr") {
		t.Errorf("QSV without the probe must NOT emit forced_idr: %q", got)
	}
}

func TestHWDeinterlaceFilter(t *testing.T) {
	cases := map[transcoding.HWAccelType]string{
		transcoding.NVENC: "yadif_cuda=deint=interlaced",
		transcoding.VAAPI: "deinterlace_vaapi",
		transcoding.QSV:   "", // vpp_qsv has no field-flag-respecting mode → none
		transcoding.None:  "",
		"":                "",
	}
	for accel, want := range cases {
		if got := hwDeinterlaceFilter(accel); got != want {
			t.Errorf("hwDeinterlaceFilter(%q) = %q, want %q", accel, got, want)
		}
	}
}

func TestBuildMultiVariantHWArgs_QSV_NoDeinterlace(t *testing.T) {
	// QSV must not inject a deinterlacer: split follows [0:v] directly so
	// progressive content is never wrongly deinterlaced by vpp_qsv.
	args := BuildFFmpegArgs(sampleHWOpts(transcoding.QSV, "", 2))
	fc, ok := filterComplexOf(args)
	if !ok {
		t.Fatal("expected -filter_complex")
	}
	if !strings.Contains(fc, "[0:v]split=2") {
		t.Errorf("QSV filter_complex must split directly off [0:v] with no deinterlace: %q", fc)
	}
	if strings.Contains(fc, "deint") || strings.Contains(fc, "yadif") {
		t.Errorf("QSV filter_complex leaked a deinterlacer: %q", fc)
	}
}

func TestBuildFFmpegArgs_HWGate_SoftwareWhenNone(t *testing.T) {
	args := BuildFFmpegArgs(sampleHWOpts(transcoding.None, "", 3))
	if argsContain(args, "-hwaccel") {
		t.Error("None accel must not emit -hwaccel")
	}
	if !argsContain(args, "libx264") {
		t.Error("None accel must use software libx264")
	}
	if argsContain(args, "h264_nvenc") {
		t.Error("None accel must not use h264_nvenc")
	}
}

func TestBuildFFmpegArgs_HWGate_BurnInFallsBackToSoftware(t *testing.T) {
	opts := sampleHWOpts(transcoding.NVENC, "", 3)
	opts.SubtitleTrackIndex = 0
	opts.SubtitleCodec = "pgssub" // bitmap subtitle → burn-in → software path
	args := BuildFFmpegArgs(opts)
	if argsContain(args, "-hwaccel") {
		t.Error("burn-in must fall back to software (no -hwaccel)")
	}
	if !argsContain(args, "libx264") {
		t.Error("burn-in fallback must use libx264")
	}
}

func TestBuildFFmpegArgs_NoAudio_MultiVariantSoftware(t *testing.T) {
	opts := sampleHWOpts(transcoding.None, "", 3)
	opts.AudioTrackIndex = -1 // source has no audio track
	joined := strings.Join(BuildFFmpegArgs(opts), " ")
	if strings.Contains(joined, "0:a:") {
		t.Errorf("no-audio source must not map any audio stream: %q", joined)
	}
	if strings.Contains(joined, "-c:a") {
		t.Errorf("no-audio source must not emit an audio codec: %q", joined)
	}
	// var_stream_map must be video-only (no a:N) but still present.
	if !strings.Contains(joined, "-var_stream_map v:0 v:1 v:2") {
		t.Errorf("expected video-only var_stream_map, got: %q", joined)
	}
	if !strings.Contains(joined, "libx264") {
		t.Error("video must still be encoded")
	}
}

func TestBuildFFmpegArgs_NoAudio_MultiVariantHW(t *testing.T) {
	opts := sampleHWOpts(transcoding.NVENC, "", 3)
	opts.AudioTrackIndex = -1
	joined := strings.Join(BuildFFmpegArgs(opts), " ")
	if strings.Contains(joined, "0:a:") || strings.Contains(joined, "-c:a") {
		t.Errorf("no-audio HW path must not map/encode audio: %q", joined)
	}
	if !strings.Contains(joined, "-var_stream_map v:0 v:1 v:2") {
		t.Errorf("expected video-only var_stream_map on HW path: %q", joined)
	}
	if !strings.Contains(joined, "h264_nvenc") || !strings.Contains(joined, "scale_cuda") {
		t.Error("HW video pipeline must still be present")
	}
}

func TestBuildFFmpegArgs_NoAudio_SingleTranscode(t *testing.T) {
	opts := sampleHWOpts(transcoding.None, "", 1)
	opts.AudioTrackIndex = -1
	joined := strings.Join(BuildFFmpegArgs(opts), " ")
	if strings.Contains(joined, "0:a:") || strings.Contains(joined, "-c:a") {
		t.Errorf("no-audio single transcode must not map/encode audio: %q", joined)
	}
	if !strings.Contains(joined, "libx264") {
		t.Error("video must still be encoded")
	}
}

func TestBuildFFmpegArgs_NoAudio_Copy(t *testing.T) {
	opts := sampleHWOpts(transcoding.None, "", 1)
	opts.VideoDecision = "copy"
	opts.AudioTrackIndex = -1
	joined := strings.Join(BuildFFmpegArgs(opts), " ")
	if strings.Contains(joined, "0:a:") || strings.Contains(joined, "-c:a") {
		t.Errorf("no-audio copy must not map/encode audio: %q", joined)
	}
	if !strings.Contains(joined, "-c:v copy") {
		t.Error("video copy must still be present")
	}
}

// filterComplexOf returns the value following -filter_complex.
func filterComplexOf(args []string) (string, bool) {
	for i, a := range args {
		if a == "-filter_complex" && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

func TestHWDomainCodecs(t *testing.T) {
	cases := []struct {
		in   []string
		want []string
	}{
		{nil, nil},
		{[]string{"h264_nvenc", "hevc_nvenc"}, []string{"h264", "hevc"}},
		{[]string{"h264_vaapi"}, []string{"h264"}},
		{[]string{"h264_qsv", "hevc_qsv", "av1_qsv"}, []string{"h264", "hevc", "av1"}},
		// de-dup if the same base appears twice (defensive).
		{[]string{"h264_nvenc", "h264_nvenc"}, []string{"h264"}},
	}
	for _, c := range cases {
		got := hwDomainCodecs(c.in)
		if len(got) != len(c.want) {
			t.Errorf("hwDomainCodecs(%v) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("hwDomainCodecs(%v) = %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
}

func TestHWDevicePath(t *testing.T) {
	if got := hwDevicePath("vaapi"); got != "/dev/dri/renderD128" {
		t.Errorf("vaapi device = %q", got)
	}
	if got := hwDevicePath("qsv"); got != "/dev/dri/renderD128" {
		t.Errorf("qsv device = %q", got)
	}
	if got := hwDevicePath("nvenc"); got != "" {
		t.Errorf("nvenc device must be empty (CUDA-selected), got %q", got)
	}
	if got := hwDevicePath(""); got != "" {
		t.Errorf("empty backend device = %q", got)
	}
}

func TestNormalizeVideoCodec(t *testing.T) {
	cases := map[string]string{
		"":           "h264",
		"h264":       "h264",
		"libx264":    "h264",
		"H264":       "h264",
		"hevc":       "hevc",
		"h265":       "hevc",
		"libx265":    "hevc",
		"av1":        "av1",
		"libaom-av1": "av1",
		"vp9":        "", // unrecognised → software fallback signal
		"nonsense":   "",
	}
	for in, want := range cases {
		if got := normalizeVideoCodec(in); got != want {
			t.Errorf("normalizeVideoCodec(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVideoEncoderFor(t *testing.T) {
	type tc struct {
		codec string
		accel transcoding.HWAccelType
		want  string
	}
	cases := []tc{
		// Software path: None / empty / invalid accel → unchanged software encoders.
		{"h264", transcoding.None, "libx264"},
		{"h264", "", "libx264"},
		{"hevc", transcoding.None, "libx265"},
		{"av1", transcoding.None, "libaom-av1"},
		{"", transcoding.None, "libx264"},
		{"h264", transcoding.HWAccelType("bogus"), "libx264"}, // invalid → software

		// NVENC.
		{"h264", transcoding.NVENC, "h264_nvenc"},
		{"hevc", transcoding.NVENC, "hevc_nvenc"},
		{"av1", transcoding.NVENC, "av1_nvenc"},
		{"", transcoding.NVENC, "h264_nvenc"}, // empty codec defaults to h264

		// VAAPI.
		{"h264", transcoding.VAAPI, "h264_vaapi"},
		{"hevc", transcoding.VAAPI, "hevc_vaapi"},
		{"h265", transcoding.VAAPI, "hevc_vaapi"},

		// QSV.
		{"h264", transcoding.QSV, "h264_qsv"},
		{"hevc", transcoding.QSV, "hevc_qsv"},

		// HW requested but codec unrecognised → software fallback (defensive).
		{"vp9", transcoding.NVENC, "libx264"},
	}
	for _, c := range cases {
		if got := videoEncoderFor(c.codec, c.accel); got != c.want {
			t.Errorf("videoEncoderFor(%q, %q) = %q, want %q", c.codec, c.accel, got, c.want)
		}
	}
}
