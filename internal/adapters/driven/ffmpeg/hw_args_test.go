package ffmpeg

import (
	"slices"
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

func argsContain(args []string, want string) bool {
	return slices.Contains(args, want)
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
	if !strings.Contains(fc, "[0:v]deinterlace_vaapi=auto=1,split=2") {
		t.Errorf("expected single pre-split deinterlace_vaapi: %q", fc)
	}
}

func TestBuildSingleTranscodeHWArgs(t *testing.T) {
	args := BuildFFmpegArgs(sampleHWOpts(transcoding.NVENC, "", 1))
	if !argsContainSeq(args, "-hwaccel", "cuda") {
		t.Error("expected -hwaccel cuda on single-rung HW path")
	}
	// Deinterlace (no-op on progressive) precedes the GPU scale in one -vf chain.
	if !argsContainSeq(args, "-vf", "yadif_cuda=deint=interlaced,scale_cuda=640:-2:format=nv12") {
		t.Error("expected -vf yadif_cuda,scale_cuda(:format=nv12) on single-rung HW path")
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

func TestNVENCProbeArgs_DeviceFree(t *testing.T) {
	j := strings.Join(nvencProbeArgs([]string{"-temporal-aq", "1"}), " ")
	for _, want := range []string{"h264_nvenc", "-temporal-aq 1", "lavfi", "-f null"} {
		if !strings.Contains(j, want) {
			t.Errorf("probe args %q missing %q", j, want)
		}
	}
	if strings.Contains(j, "-hwaccel") {
		t.Errorf("the NVENC option probe must be device-free (no -hwaccel): %q", j)
	}
}

// Every probed NVENC feature must be one the builder actually gates on - guards
// against the probe list and hwEncoderQualityArgs drifting apart (a probed
// option that the builder ignores, or vice versa).
func TestNVENCProbeNames_MatchBuilderGating(t *testing.T) {
	baseline := strings.Join(hwEncoderQualityArgs(transcoding.NVENC, nil), " ")
	for _, p := range nvencFeatureProbes {
		got := strings.Join(hwEncoderQualityArgs(transcoding.NVENC, []string{p.name}), " ")
		if got == baseline {
			t.Errorf("feature %q is probed but never emitted by the builder (drift)", p.name)
		}
		if !strings.Contains(got, p.args[0]) {
			t.Errorf("feature %q: builder emits %q, expected the probed flag %q", p.name, got, p.args[0])
		}
	}
}

func TestHWDeinterlaceFilter(t *testing.T) {
	cases := map[transcoding.HWAccelType]string{
		transcoding.NVENC: "yadif_cuda=deint=interlaced",
		transcoding.VAAPI: "deinterlace_vaapi=auto=1",
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

func TestBuildFFmpegArgs_ToneMap_EmitsTonemapOnceBeforeSplit(t *testing.T) {
	// HDR source + non-HDR client (ToneMap=true) must emit an HDR->SDR tonemap.
	// Multi-variant: applied ONCE before the split (the tonemap is expensive),
	// not per-rung - otherwise a weak box pays N× the cost.
	opts := sampleHWOpts(transcoding.None, "", 3)
	opts.ToneMap = true
	joined := strings.Join(BuildFFmpegArgs(opts), " ")
	if !strings.Contains(joined, "tonemap=") {
		t.Fatalf("ToneMap=true must emit a tonemap filter: %q", joined)
	}
	// The software tonemap is either the portable zscale chain or jellyfin-ffmpeg's
	// fused tonemapx (one SIMD pass, no separate zscale) - which one depends on the
	// ffmpeg build (tonemapx is present in the container, absent on a plain host).
	// Either is a valid software HDR->SDR tonemap; assert one is present.
	if !strings.Contains(joined, "zscale") && !strings.Contains(joined, "tonemapx") {
		t.Errorf("expected a software tonemap (zscale or tonemapx): %q", joined)
	}
	if n := strings.Count(joined, "tonemap="); n != 1 {
		t.Errorf("tonemap must run exactly once before the split, got %d: %q", n, joined)
	}
	ti, si := strings.Index(joined, "tonemap="), strings.Index(joined, "split=")
	if ti < 0 || si < 0 || ti > si {
		t.Errorf("tonemap must precede split (shared by all rungs): tonemapIdx=%d splitIdx=%d", ti, si)
	}
}

func TestBuildFFmpegArgs_ToneMap_ForcesSoftware(t *testing.T) {
	// Even with a HW backend set, a tonemap request uses software: the
	// orchestrated VAAPI/NVENC path does no tonemap, and old iGPU VEBOX can't fold
	// 10-bit. hwUsable already gates on toneMap=="".
	opts := sampleHWOpts(transcoding.NVENC, "", 3)
	opts.ToneMap = true
	joined := strings.Join(BuildFFmpegArgs(opts), " ")
	if strings.Contains(joined, "-hwaccel") || strings.Contains(joined, "h264_nvenc") {
		t.Errorf("tonemap must force the software path (no HW): %q", joined)
	}
	if !strings.Contains(joined, "libx264") {
		t.Error("tonemap path must encode with libx264")
	}
}

func TestBuildFFmpegArgs_ToneMap_HDRClientSkips(t *testing.T) {
	// A client that renders HDR natively (HDRSupported) is never tonemapped.
	opts := sampleHWOpts(transcoding.None, "", 3)
	opts.ToneMap = true
	opts.HDRSupported = true
	joined := strings.Join(BuildFFmpegArgs(opts), " ")
	if strings.Contains(joined, "tonemap=") {
		t.Errorf("HDRSupported client must not tonemap: %q", joined)
	}
}

func TestBuildFFmpegArgs_ToneMap_SingleVariant(t *testing.T) {
	opts := sampleHWOpts(transcoding.None, "", 1)
	opts.ToneMap = true
	joined := strings.Join(BuildFFmpegArgs(opts), " ")
	if !strings.Contains(joined, "tonemap=") {
		t.Errorf("single-variant ToneMap must emit tonemap: %q", joined)
	}
}

func TestBuildFFmpegArgs_SoftwareUsesCappedCRF(t *testing.T) {
	// The software ladder must use capped CRF (-crf + -maxrate + -bufsize), not a
	// plain -b:v average: easy scenes spend fewer bits while peaks stay under the
	// rung bitrate the manifest advertises.
	single := strings.Join(BuildFFmpegArgs(sampleHWOpts(transcoding.None, "", 1)), " ")
	for _, want := range []string{"-crf", "-maxrate", "-bufsize"} {
		if !strings.Contains(single, want) {
			t.Errorf("single software path must contain %q: %q", want, single)
		}
	}
	if strings.Contains(single, "-b:v") {
		t.Errorf("single software video must not use a plain -b:v average: %q", single)
	}
	multi := strings.Join(BuildFFmpegArgs(sampleHWOpts(transcoding.None, "", 3)), " ")
	// maxrate ceiling per rung = the rung's bitrate x maxrateHeadroomPct, bufsize
	// 2x the ceiling (1080p rung = 5000k -> 8000k / 16000k). The literals are
	// spelled out rather than computed from the constant so that retuning the
	// headroom has to restate the resulting encoder contract here, on purpose.
	for _, want := range []string{"-crf:v:0", "-maxrate:v:2 8000k", "-bufsize:v:2 16000k"} {
		if !strings.Contains(multi, want) {
			t.Errorf("multi-variant software path must contain %q: %q", want, multi)
		}
	}
	if strings.Contains(multi, "-b:v:") {
		t.Errorf("multi-variant software video must not use a plain -b:v average: %q", multi)
	}
}

func TestBuildFFmpegArgs_HWTonemap_VAAPI_StaysOnGPU(t *testing.T) {
	// HWTonemap + a tonemap need on VAAPI: the GPU folds HDR->SDR (tonemap_vaapi)
	// once before the split and the pipeline stays hardware (h264_vaapi), NOT the
	// software zscale/libx264 path.
	opts := sampleHWOpts(transcoding.VAAPI, "/dev/dri/renderD128", 3)
	opts.ToneMap = true
	opts.HWTonemap = true
	joined := strings.Join(BuildFFmpegArgs(opts), " ")
	if n := strings.Count(joined, "tonemap_vaapi"); n != 1 {
		t.Errorf("tonemap_vaapi must run once before the split, got %d: %q", n, joined)
	}
	if !strings.Contains(joined, "h264_vaapi") {
		t.Errorf("HW tonemap must keep the HW encoder: %q", joined)
	}
	if strings.Contains(joined, "zscale") || strings.Contains(joined, "libx264") {
		t.Errorf("HW tonemap must NOT fall to the software path: %q", joined)
	}
}

func TestBuildFFmpegArgs_HWTonemap_OffStaysSoftware(t *testing.T) {
	// The invariant: tonemap needed but HWTonemap=false (the probe failed - every
	// host on the current fleet) must run the software zscale + libx264 path,
	// byte-identical to a host with no GPU tone-mapping. No HW filters leak in.
	opts := sampleHWOpts(transcoding.VAAPI, "/dev/dri/renderD128", 3)
	opts.ToneMap = true
	opts.HWTonemap = false
	joined := strings.Join(BuildFFmpegArgs(opts), " ")
	if strings.Contains(joined, "tonemap_vaapi") || strings.Contains(joined, "h264_vaapi") {
		t.Errorf("HWTonemap=false must NOT use the HW path: %q", joined)
	}
	if !strings.Contains(joined, "tonemap=") || !strings.Contains(joined, "libx264") {
		t.Errorf("HWTonemap=false must use software zscale tonemap + libx264: %q", joined)
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
		{"av1", transcoding.None, "libsvtav1"},
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
