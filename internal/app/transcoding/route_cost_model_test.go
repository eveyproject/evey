package transcoding

import (
	"math"
	"testing"

	"github.com/eveyproject/evey/internal/domain/transcoding"
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
	// CPU is normalised to the required (paced) rate, not charged at 98 fps:
	// 0.63 cores x 28.8/98 / 24 = 0.0077, not the raw 0.63/24 = 0.026.
	if !approx(c.CPU, 0.63*28.8/98/24) {
		t.Fatalf("cpu=%.4f want ~0.0077 (normalised to required rate)", c.CPU)
	}
	if c.CPU > 0.63/24 {
		t.Fatalf("cpu=%.4f must be BELOW the raw max-throughput charge 0.026", c.CPU)
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
	// 2.7 cores x 28.8/52 / 24 = 0.0623 (normalised), not 28.8/52 = 0.554.
	if !approx(c.CPU, 2.7*28.8/52/24) {
		t.Fatalf("cpu=%.4f want ~0.062 (cores at the paced rate over budget)", c.CPU)
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

// Keys is the admin panel's listing: deterministic order (backend, encoder,
// route) whatever the map iteration, and nil-safe.
func TestRouteCostModel_KeysSortedAndNilSafe(t *testing.T) {
	var none *RouteCostModel
	if none.Keys() != nil {
		t.Fatal("nil model lists nil")
	}
	m := NewRouteCostModel([]transcoding.RouteSample{
		{Key: rk("vaapi", "h264_vaapi", transcoding.RouteCostSDR), Height: 1080, FPS: 60, SourceFPS: 24, CPUCores: 1},
		{Key: rk("nvenc", "hevc_nvenc", transcoding.RouteCostSDR), Height: 1080, FPS: 60, SourceFPS: 24, CPUCores: 1},
		{Key: rk("nvenc", "h264_nvenc", transcoding.RouteCostTonemap), Height: 1080, FPS: 60, SourceFPS: 24, CPUCores: 1},
		{Key: rk("nvenc", "h264_nvenc", transcoding.RouteCostSDR), Height: 1080, FPS: 60, SourceFPS: 24, CPUCores: 1},
		{Key: rk("nvenc", "h264_nvenc", transcoding.RouteCostSDR), Height: 2160, FPS: 30, SourceFPS: 24, CPUCores: 1},
	}, 8)
	keys := m.Keys()
	if len(keys) != 4 {
		t.Fatalf("keys = %d, want 4 distinct routes (two rungs of one route count once)", len(keys))
	}
	for i := 1; i < len(keys); i++ {
		a, b := keys[i-1], keys[i]
		if a.Backend > b.Backend || (a.Backend == b.Backend && a.Encoder > b.Encoder) ||
			(a.Backend == b.Backend && a.Encoder == b.Encoder && string(a.Route) > string(b.Route)) {
			t.Fatalf("keys out of order at %d: %+v then %+v", i, a, b)
		}
	}
	if keys[0].Backend != "nvenc" || keys[len(keys)-1].Backend != "vaapi" {
		t.Fatalf("order = %+v", keys)
	}
}

// MaxStreams is floor(1/worst pool share), never below 1, only with evidence.
func TestRouteCostModel_MaxStreams(t *testing.T) {
	key := rk("nvenc", "h264_nvenc", transcoding.RouteCostSDR)
	// 24p source needs 28.8 fps; measured 96 fps -> accel 0.3 -> 3 streams.
	m := NewRouteCostModel([]transcoding.RouteSample{
		{Key: key, Height: 1080, FPS: 96, SourceFPS: 24, CPUCores: 0.5},
	}, 24)
	n, ok := m.MaxStreams(key, 1080, 24)
	if !ok || n != 3 {
		t.Fatalf("got %d,%v want 3 streams", n, ok)
	}
	// A 60p source needs 72 fps of a 96 fps route: 0.75 -> 1 stream.
	if n, _ := m.MaxStreams(key, 1080, 60); n != 1 {
		t.Fatalf("60p: got %d want 1", n)
	}
	// A route priced above a whole pool (10 fps for a 28.8 fps need) is one
	// admission refuses outright: the panel says 0, not a "1" nobody gets.
	tiny := NewRouteCostModel([]transcoding.RouteSample{
		{Key: key, Height: 1080, FPS: 10, SourceFPS: 24, CPUCores: 4},
	}, 4)
	if n, ok := tiny.MaxStreams(key, 1080, 24); !ok || n != 0 {
		t.Fatalf("over-budget route: got %d,%v want 0", n, ok)
	}
	// Priced exactly one pool: the one stream admission grants.
	exact := NewRouteCostModel([]transcoding.RouteSample{
		{Key: key, Height: 1080, FPS: 28.8, SourceFPS: 24, CPUCores: 0.5},
	}, 4)
	if n, ok := exact.MaxStreams(key, 1080, 24); !ok || n != 1 {
		t.Fatalf("route priced exactly one pool: got %d,%v want 1", n, ok)
	}
	if _, ok := m.MaxStreams(rk("vaapi", "h264_vaapi", transcoding.RouteCostSDR), 1080, 24); ok {
		t.Fatal("no evidence for the route: no verdict")
	}
}

// The CPU route's billed class follows the same precedence as a
// fingerprint's: AV1 first (its own ceiling), then a tone-map, else plain.
func TestResolvedRoute_SoftwareClass(t *testing.T) {
	sw := func(codec string, tonemap bool) ResolvedRoute {
		tm := transcoding.ToneMapNone
		if tonemap {
			tm = transcoding.ToneMapCPU
		}
		return ResolvedRoute{Codec: codec, Topology: transcoding.MustTopology(transcoding.StageCPU, transcoding.StageCPU, tm, transcoding.OutputSDR)}
	}
	cases := []struct {
		route ResolvedRoute
		want  LadderClass
	}{
		{sw("AV1", true), LadderClassSoftwareAV1},
		{sw("av1", false), LadderClassSoftwareAV1},
		{sw("h264", true), LadderClassSoftwareHDR},
		{sw("hevc", false), LadderClassSoftware},
		{sw("", false), LadderClassSoftware},
	}
	for _, tc := range cases {
		if got := tc.route.Class(); got != tc.want {
			t.Errorf("%+v: got %v want %v", tc.route, got, tc.want)
		}
	}
}
