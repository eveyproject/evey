package transcoding

import (
	"fmt"
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// route_cost_acceptance_test.go - the admission now matches what the routes
// were MEASURED to cost on the two rigs (2026-09-12/13, route-bench), where the
// cumulative HW rung was wrong. Each case builds a limiter with the measured
// route evidence and asserts how many sessions actually fit each pool.

// admitN counts how many distinct streams of one route the limiter admits
// before it refuses, on a fresh limiter with unit budgets.
func admitN(t *testing.T, samples []transcoding.RouteSample, cpuBudget int, key transcoding.RouteCostKey, h int, fps float64) int {
	t.Helper()
	l := NewVideoSessionLimiter(0).WithRouteModel(NewRouteCostModel(samples, cpuBudget))
	class := LadderClassSoftware
	if key.Backend != "" {
		class = LadderClassHW
	}
	n := 0
	for i := range 64 {
		if !l.TryAcquireRoute(fmt.Sprintf("s%d", i), class, h, transcoding.SourceClass{}, key, fps) {
			break
		}
		n++
	}
	return n
}

func TestAcceptance_Ada_NVENC(t *testing.T) {
	// Measured on the RTX 4070 Ti (24 threads).
	s := []transcoding.RouteSample{
		{Key: rk("nvenc", "h264_nvenc", transcoding.RouteCostSDR), Height: 2160, FPS: 98, SourceFPS: 24, CPUCores: 0.63},
		{Key: rk("nvenc", "h264_nvenc", transcoding.RouteCostSDR), Height: 1080, FPS: 200, SourceFPS: 24, CPUCores: 0.70},
		{Key: rk("nvenc", "h264_nvenc", transcoding.RouteCostBridge), Height: 1080, FPS: 70, SourceFPS: 24, CPUCores: 2.76, SoftwareDecode: false},
	}
	k := rk("nvenc", "h264_nvenc", transcoding.RouteCostSDR)
	// 4K 24p: cumulative rung priced 0.483 (2 sessions); measured 0.294 -> 3.
	if n := admitN(t, s, 24, k, 2160, 24); n != 3 {
		t.Errorf("Ada 4K 24p GPU: admitted %d, measured cost 0.294 fits 3", n)
	}
	// 4K 60p: the encoder did not gain 2.5x, so a second no longer fits.
	if n := admitN(t, s, 24, k, 2160, 60); n != 1 {
		t.Errorf("Ada 4K 60p GPU: admitted %d, 0.615 each fits 1", n)
	}
	// The bridge charges the CPU pool it really burns (was 0 before).
	c, _ := NewRouteCostModel(s, 24).Cost(rk("nvenc", "h264_nvenc", transcoding.RouteCostBridge), 1080, 24)
	if c.CPU <= 0 {
		t.Error("Ada bridge: the 2.76 CPU cores must land on the CPU pool, not be free")
	}
}

func TestAcceptance_ProdHost_VAAPI(t *testing.T) {
	// Measured on the UHD 630 (7 threads). hevc_vaapi is far slower than h264.
	s := []transcoding.RouteSample{
		{Key: rk("vaapi", "h264_vaapi", transcoding.RouteCostSDR), Height: 2160, FPS: 73, SourceFPS: 24, CPUCores: 0.19},
		{Key: rk("vaapi", "hevc_vaapi", transcoding.RouteCostSDR), Height: 2160, FPS: 30, SourceFPS: 24, CPUCores: 0.11},
		{Key: rk("vaapi", "h264_vaapi", transcoding.RouteCostDecoupled), Height: 1080, FPS: 128, SourceFPS: 24, CPUCores: 5.6, SoftwareDecode: true},
	}
	h264 := rk("vaapi", "h264_vaapi", transcoding.RouteCostSDR)
	hevc := rk("vaapi", "hevc_vaapi", transcoding.RouteCostSDR)
	// h264 4K 24p: 0.395 -> 2 fit.
	if n := admitN(t, s, 7, h264, 2160, 24); n != 2 {
		t.Errorf("prod-host h264 4K 24p: admitted %d, want 2", n)
	}
	// hevc 4K 24p: 0.96 -> exactly 1 (priced on h264's 0.559, a 2nd would have slipped in).
	if n := admitN(t, s, 7, hevc, 2160, 24); n != 1 {
		t.Errorf("prod-host hevc_vaapi 4K 24p: admitted %d, 0.96 fits 1", n)
	}
	// hevc 4K 60p: 2.36 > 1 -> REFUSED even for one session (0.51x realtime).
	// The old model priced it 0.559 and would have admitted it.
	if n := admitN(t, s, 7, hevc, 2160, 60); n != 0 {
		t.Errorf("prod-host hevc_vaapi 4K 60p: admitted %d, it runs at 0.51x realtime and must be refused", n)
	}
	// Decoupled 1080: 5.6 of 7 cores -> the CPU pool binds it to 1, not the 4
	// the GPU-only price allowed.
	dec := rk("vaapi", "h264_vaapi", transcoding.RouteCostDecoupled)
	if n := admitN(t, s, 7, dec, 1080, 24); n != 1 {
		t.Errorf("prod-host decoupled 1080: admitted %d, 5.6/7 CPU fits 1", n)
	}
}

// QSV beats VAAPI on the UHD 630 (measured hevc 75 vs 30 fps): the fastest
// certified backend must be the one whose route evidence prices cheaper - the
// cost model reflects it, whatever the detector's order was.
func TestAcceptance_ProdHost_QSVCheaperThanVAAPI(t *testing.T) {
	s := []transcoding.RouteSample{
		{Key: rk("vaapi", "hevc_vaapi", transcoding.RouteCostSDR), Height: 2160, FPS: 30, SourceFPS: 24},
		{Key: rk("qsv", "hevc_qsv", transcoding.RouteCostSDR), Height: 2160, FPS: 75, SourceFPS: 24},
	}
	m := NewRouteCostModel(s, 7)
	va, _ := m.Cost(rk("vaapi", "hevc_vaapi", transcoding.RouteCostSDR), 2160, 24)
	qs, _ := m.Cost(rk("qsv", "hevc_qsv", transcoding.RouteCostSDR), 2160, 24)
	if !(qs.Accel < va.Accel) {
		t.Fatalf("QSV hevc (%.3f) must price cheaper than VAAPI hevc (%.3f)", qs.Accel, va.Accel)
	}
}
