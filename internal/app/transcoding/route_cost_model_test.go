package transcoding

import (
	"math"
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

func rk(backend, enc string, route transcoding.RouteCostClass) transcoding.RouteCostKey {
	return transcoding.RouteCostKeyFor(transcoding.HWAccelType(backend), enc, route)
}

func approx(a, b float64) bool { return math.Abs(a-b) < 0.02 }

// The model prices the accelerator pool by required/measuredFPS and the CPU
// pool by the measured cores - both pools, unlike the old single "hw" cost.
func TestRouteCostModel_PricesBothPools(t *testing.T) {
	m := NewRouteCostModel([]transcoding.RouteSample{
		{Key: rk("nvenc", "h264_nvenc", transcoding.RouteCostSDR), Height: 2160, FPS: 98, SourceFPS: 24, CPUCores: 0.63},
		{Key: rk("nvenc", "h264_nvenc", transcoding.RouteCostSDR), Height: 1080, FPS: 200, SourceFPS: 24, CPUCores: 0.70},
	}, 24)
	c, ok := m.Cost(rk("nvenc", "h264_nvenc", transcoding.RouteCostSDR), 2160, 24)
	if !ok {
		t.Fatal("expected route evidence")
	}
	if !approx(c.Accel, 24*1.2/98) { // 0.294 - not the 0.483 the cumulative rung priced
		t.Fatalf("accel=%.3f want ~0.294", c.Accel)
	}
	if !approx(c.CPU, 0.63/24) {
		t.Fatalf("cpu=%.3f want ~0.026 (a GPU session still spends CPU)", c.CPU)
	}
}

// RequiredFPS scales with the SOURCE fps: a 60p source costs the accelerator
// 2.5x a 24p one when the encoder did not speed up (the UHD 630 case).
func TestRouteCostModel_RequiredFPSScalesWithSource(t *testing.T) {
	m := NewRouteCostModel([]transcoding.RouteSample{
		{Key: rk("vaapi", "h264_vaapi", transcoding.RouteCostSDR), Height: 2160, FPS: 73, SourceFPS: 24, CPUCores: 0.2},
	}, 7)
	c24, _ := m.Cost(rk("vaapi", "h264_vaapi", transcoding.RouteCostSDR), 2160, 24)
	c60, _ := m.Cost(rk("vaapi", "h264_vaapi", transcoding.RouteCostSDR), 2160, 60)
	if !approx(c60.Accel/c24.Accel, 2.5) {
		t.Fatalf("60p should cost 2.5x 24p: %.3f / %.3f", c60.Accel, c24.Accel)
	}
}

// A CPU route spends no accelerator; its whole cost is the CPU pool.
func TestRouteCostModel_CPURouteIsCPUOnly(t *testing.T) {
	m := NewRouteCostModel([]transcoding.RouteSample{
		{Key: rk("", "libx265", transcoding.RouteCostSDR), Height: 1080, FPS: 52, SourceFPS: 24, CPUCores: 2.7},
	}, 24)
	c, _ := m.Cost(rk("", "libx265", transcoding.RouteCostSDR), 1080, 24)
	if c.Accel != 0 {
		t.Fatalf("a CPU encoder spends no accelerator, got %.3f", c.Accel)
	}
	if !approx(c.CPU, 24*1.2/52) {
		t.Fatalf("cpu=%.3f want ~0.554", c.CPU)
	}
}

func TestRouteCostModel_NilAndMiss(t *testing.T) {
	if NewRouteCostModel(nil, 8) != nil {
		t.Fatal("no samples => nil model")
	}
	m := NewRouteCostModel([]transcoding.RouteSample{{Key: rk("nvenc", "h264_nvenc", transcoding.RouteCostSDR), Height: 1080, FPS: 200, SourceFPS: 24}}, 8)
	if _, ok := m.Cost(rk("nvenc", "hevc_nvenc", transcoding.RouteCostSDR), 1080, 24); ok {
		t.Fatal("a route with no evidence must miss so the caller falls back to the table")
	}
}

// RouteCostKeyOf(fingerprint) must equal EncodeRoute.CostKey() for the same
// resolved route - admission (which keys off the fingerprint at spawn) and the
// ladder shaping (which keys off EncodeRoute at manifest time) price the SAME
// route, or a session is admitted on one key and charged on another.
func TestRouteCostKeyOf_AgreesWithEncodeRouteCostKey(t *testing.T) {
	cases := []transcoding.EncodeRoute{
		{Codec: "h264", HWUsable: false},
		{Codec: "hevc", HWUsable: false},
		{Codec: "h264", HWUsable: false, Tonemap: true},
		{Codec: "h264", Accel: transcoding.NVENC, HWUsable: true},
		{Codec: "hevc", Accel: transcoding.NVENC, HWUsable: true},
		{Codec: "h264", Accel: transcoding.VAAPI, HWUsable: true, Tonemap: true, HWTonemap: true},
		{Codec: "h264", Accel: transcoding.NVENC, HWUsable: true, Tonemap: true},        // bridge
		{Codec: "hevc", Accel: transcoding.VAAPI, HWUsable: true, HDRPassthrough: true}, // passthrough
		{Codec: "h264", Accel: transcoding.VAAPI, HWUsable: true, SoftwareDecode: true}, // decoupled
	}
	for _, r := range cases {
		fp := DecisionFingerprint{
			VideoCodec:       r.Codec,
			HWAccel:          string(r.Accel),
			HDRTonemap:       r.Tonemap,
			HWTonemap:        r.HWTonemap,
			HDRPassthrough:   r.HDRPassthrough,
			HWSoftwareDecode: r.SoftwareDecode,
		}
		if got, want := RouteCostKeyOf(fp), r.CostKey(); got != want {
			t.Errorf("route %+v: RouteCostKeyOf=%+v EncodeRoute.CostKey=%+v", r, got, want)
		}
	}
}
