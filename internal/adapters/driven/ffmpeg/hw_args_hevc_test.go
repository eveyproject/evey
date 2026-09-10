package ffmpeg

import (
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

func TestHWEncoderQualityArgs_LowPowerPerCodec(t *testing.T) {
	feats := []string{"low_power_hevc"}
	if got := strings.Join(hwEncoderQualityArgs(transcoding.VAAPI, "hevc", feats), " "); got != "-low_power 1" {
		t.Errorf("VAAPI hevc with the probed feature = %q, want '-low_power 1'", got)
	}
	if got := hwEncoderQualityArgs(transcoding.VAAPI, "h264", feats); len(got) != 0 {
		t.Errorf("the hevc probe must not switch h264 to low power: %v", got)
	}
	if got := strings.Join(hwEncoderQualityArgs(transcoding.QSV, "h264", []string{"low_power_h264"}), " "); got != "-preset veryfast -low_power 1" {
		t.Errorf("QSV h264 = %q", got)
	}
	if got := strings.Join(hwEncoderQualityArgs(transcoding.VAAPI, "h264", []string{"low_power_h264", "bframes_h264"}), " "); got != "-low_power 1" {
		t.Errorf("normal-mode B-frames must not leak into a low-power encode: %q", got)
	}
	if got := strings.Join(hwEncoderQualityArgs(transcoding.VAAPI, "h264", []string{"low_power_h264", "bframes_h264_lp"}), " "); got != "-low_power 1 -bf 3" {
		t.Errorf("VAAPI h264 low power + its own bframes = %q", got)
	}
	if got := strings.Join(hwEncoderQualityArgs(transcoding.VAAPI, "hevc", []string{"bframes_hevc"}), " "); got != "-bf 3" {
		t.Errorf("VAAPI hevc bframes = %q", got)
	}
}

func TestHWTonemapFilter_OpenCLWhenProbed(t *testing.T) {
	if got := hwTonemapFilter(transcoding.VAAPI, nil); !strings.HasPrefix(got, "tonemap_vaapi=") {
		t.Errorf("VPP fallback = %q", got)
	}
	got := hwTonemapFilter(transcoding.VAAPI, []string{transcoding.FeatureTonemapOpenCL})
	for _, want := range []string{"hwmap=derive_device=opencl", "tonemap_opencl=", "tonemap=bt2390", "peak=100", "hwmap=derive_device=vaapi:reverse=1"} {
		if !strings.Contains(got, want) {
			t.Errorf("OpenCL chain %q missing %q", got, want)
		}
	}
	opts := sampleHWOpts(transcoding.VAAPI, "/dev/dri/renderD128", 1)
	opts.ToneMap, opts.HWTonemap = true, true
	opts.HWFeatures = []string{transcoding.FeatureTonemapOpenCL}
	joined := strings.Join(BuildFFmpegArgs(opts), " ")
	if !strings.Contains(joined, "tonemap_opencl=") || strings.Contains(joined, "tonemap_vaapi") {
		t.Errorf("probed OpenCL must replace the VPP tone-mapper: %q", joined)
	}
}

func TestHWHDRPassthroughArgs(t *testing.T) {
	pq := strings.Join(hwHDRPassthroughArgs(transcoding.VAAPI, "pq", -1), " ")
	for _, want := range []string{"-profile:v main10", "-color_primaries:v bt2020", "-color_trc:v smpte2084", "-colorspace:v bt2020nc", "-sei:v hdr"} {
		if !strings.Contains(pq, want) {
			t.Errorf("PQ args %q missing %q", pq, want)
		}
	}
	hlg := strings.Join(hwHDRPassthroughArgs(transcoding.QSV, "hlg", 2), " ")
	for _, want := range []string{"-profile:v:2 main10", "-color_trc:v:2 arib-std-b67"} {
		if !strings.Contains(hlg, want) {
			t.Errorf("HLG args %q missing %q", hlg, want)
		}
	}
	if strings.Contains(hlg, "-sei") {
		t.Errorf("-sei is a VAAPI option, QSV must not get it: %q", hlg)
	}
}

func TestHevcTagArgs(t *testing.T) {
	if got := strings.Join(hevcTagArgs("hevc", -1), " "); got != "-tag:v hvc1" {
		t.Errorf("hevc single = %q", got)
	}
	if got := strings.Join(hevcTagArgs("h265", 1), " "); got != "-tag:v:1 hvc1" {
		t.Errorf("hevc rung 1 = %q", got)
	}
	if got := hevcTagArgs("h264", -1); got != nil {
		t.Errorf("h264 must not be tagged: %v", got)
	}
}

// The HDR passthrough: 10-bit surfaces to a Main10 HEVC encoder, no tone-map.
func TestBuildSingleTranscodeHWArgs_HDRPassthrough(t *testing.T) {
	opts := sampleHWOpts(transcoding.VAAPI, "/dev/dri/renderD128", 1)
	opts.VideoCodec = "hevc"
	opts.HDRPassthrough = true
	opts.HDRTransfer = "pq"
	opts.HWFeatures = []string{"low_power_hevc"}
	args := BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	for _, want := range []string{"scale_vaapi=640:-2:format=p010", "-c:v hevc_vaapi", "-low_power 1", "-profile:v main10", "-color_trc:v smpte2084", "-tag:v hvc1"} {
		if !strings.Contains(joined, want) {
			t.Errorf("passthrough args missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "tonemap") || strings.Contains(joined, "format=nv12") {
		t.Errorf("an HDR passthrough must not tone-map or fold to 8-bit:\n%s", joined)
	}
}

func TestBuildMultiVariantHWArgs_HDRPassthrough(t *testing.T) {
	opts := sampleHWOpts(transcoding.VAAPI, "/dev/dri/renderD128", 2)
	opts.VideoCodec = "hevc"
	opts.HDRPassthrough = true
	opts.HDRTransfer = "hlg"
	joined := strings.Join(BuildFFmpegArgs(opts), " ")
	for _, want := range []string{"scale_vaapi=640:-2:format=p010", "scale_vaapi=1280:-2:format=p010", "-profile:v:0 main10", "-profile:v:1 main10", "-color_trc:v:1 arib-std-b67", "-tag:v:0 hvc1", "-tag:v:1 hvc1"} {
		if !strings.Contains(joined, want) {
			t.Errorf("multi passthrough missing %q:\n%s", want, joined)
		}
	}
}

// SDR HEVC output keeps 8-bit surfaces and no colour override, but is tagged hvc1.
func TestBuildSingleTranscodeHWArgs_HevcSDR(t *testing.T) {
	opts := sampleHWOpts(transcoding.VAAPI, "/dev/dri/renderD128", 1)
	opts.VideoCodec = "hevc"
	joined := strings.Join(BuildFFmpegArgs(opts), " ")
	for _, want := range []string{"format=nv12", "-c:v hevc_vaapi", "-tag:v hvc1"} {
		if !strings.Contains(joined, want) {
			t.Errorf("SDR hevc missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "main10") || strings.Contains(joined, "bt2020") {
		t.Errorf("SDR hevc must not carry HDR signalling:\n%s", joined)
	}
}

// The probed Intel feature names must be the ones the builder gates on.
func TestIntelProbeNames_MatchBuilderGating(t *testing.T) {
	for _, codec := range []string{"h264", "hevc"} {
		feat := transcoding.LowPowerFeature(codec)
		got := strings.Join(hwEncoderQualityArgs(transcoding.VAAPI, codec, []string{feat}), " ")
		if !strings.Contains(got, "-low_power 1") {
			t.Errorf("feature %q is probed but never emitted for %s: %q", feat, codec, got)
		}
	}
}
