package ffmpeg

import (
	"math"
	"slices"
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

func TestRouteBenchRoutes_FollowTheCapability(t *testing.T) {
	cap := transcoding.EncodeCapability{HWBackend: "nvenc", HWCodecs: []string{"h264", "hevc"}, HW10BitTranscode: true, HWHDR10Encode: true}
	var names []string
	for _, r := range routeBenchRoutes(cap, true) {
		names = append(names, r.name)
	}
	want := []string{"hw-sdr-h264", "hw-sdr-hevc", "hw-src10-h264", "hw-bridge-h264", "hw-passthrough-hevc", "cpu-sdr-h264", "cpu-hdr-h264", "cpu-sdr-hevc"}
	if !slices.Equal(names, want) {
		t.Fatalf("nvenc routes = %v, want %v", names, want)
	}
	names = nil
	for _, r := range routeBenchRoutes(cap, false) {
		names = append(names, r.name)
	}
	if !slices.Equal(names, []string{"cpu-sdr-h264", "cpu-hdr-h264", "cpu-sdr-hevc"}) {
		t.Fatalf("cpu-only routes = %v", names)
	}
	vaapi := transcoding.EncodeCapability{HWBackend: "vaapi", HWCodecs: []string{"h264"}, HWTonemap: true, HWFeatures: []string{transcoding.FeatureTonemapOpenCL}}
	names = nil
	for _, r := range routeBenchRoutes(vaapi, true) {
		names = append(names, r.name)
	}
	if !slices.Contains(names, "hw-tonemap-h264") || !slices.Contains(names, "hw-decoupled-h264") || slices.Contains(names, "hw-sdr-hevc") {
		t.Fatalf("vaapi routes = %v", names)
	}
}

// The bench's own price must be the admission model's: RequiredFPS/FPS of the
// smallest calibrated rung >= h in the class's ladder (software fallback for a
// class without evidence), with the decode term when both decodes are known.
func TestRouteBenchModel_PricesLikeTheAdmissionModel(t *testing.T) {
	cap := transcoding.EncodeCapability{
		BenchSamples: []transcoding.BenchRungSample{
			{Ladder: "software", Height: 1080, FPS: 96, RequiredFPS: 28.8, Passing: true},
			{Ladder: "software", Height: 720, FPS: 200, RequiredFPS: 28.8, Passing: true},
			{Ladder: "hw", Height: 2160, FPS: 120, RequiredFPS: 28.8, Passing: true},
			{Ladder: "hw", Height: 1440, FPS: 10, RequiredFPS: 28.8, Passing: false},
		},
		BenchSource:   transcoding.SourceClass{Codec: "hevc", Height: 2160},
		DecodeSamples: []transcoding.DecodeRungSample{{Codec: "hevc", Height: 2160, FPS: 360}, {Codec: "h264", Height: 2160, FPS: 800}},
	}
	m := newRouteBenchModel(cap)
	unknown := transcoding.SourceClass{}
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	if c, r, f := m.cost("software", 1080, unknown); !near(c, 28.8/96) || r != 1080 || f != 96 {
		t.Fatalf("software 1080 = %v %v %v", c, r, f)
	}
	if c, r, _ := m.cost("software", 480, unknown); !near(c, 28.8/200) || r != 720 {
		t.Fatalf("software 480 rounds up to the 720 rung: %v %v", c, r)
	}
	if c, r, _ := m.cost("hw", 1080, unknown); !near(c, 28.8/120) || r != 2160 {
		t.Fatalf("hw 1080: the failing 1440 rung is skipped, 2160 prices it: %v %v", c, r)
	}
	if c, _, _ := m.cost("software-hdr", 1080, unknown); !near(c, 28.8/96) {
		t.Fatalf("a class without evidence prices on the software table: %v", c)
	}
	// Decode term: cost = req x ((1/rungFPS - 1/benchDecode) + 1/srcDecode).
	src := transcoding.SourceClass{Codec: "h264", Height: 1080}
	srcDec := 800 * float64(2160*2160) / float64(1080*1080)
	want := 28.8 * ((1.0/96 - 1.0/360) + 1/srcDec)
	if c, _, _ := m.cost("software", 1080, src); math.Abs(c-want) > 1e-9 {
		t.Fatalf("decode-aware cost = %v, want %v", c, want)
	}
}
