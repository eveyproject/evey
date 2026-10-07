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
	l := NewVideoSessionLimiter(0).WithRouteModel(NewRouteCostModel(samples, float64(cpuBudget)))
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
	// The bridge charges the CPU pool it burns AT THE PACED RATE (was 0 before,
	// then over-charged at the racing rate): 2.76 x 28.8/70 / 24 = 0.047, not
	// the raw 2.76/24 = 0.115.
	c, _ := NewRouteCostModel(s, 24).Cost(rk("nvenc", "h264_nvenc", transcoding.RouteCostBridge), 1080, 24)
	if !approx(c.CPU, 2.76*28.8/70/24) {
		t.Errorf("Ada bridge cpu=%.4f want ~0.047 (paced, not the racing 0.115)", c.CPU)
	}
	if c.CPU >= 2.76/24 {
		t.Errorf("Ada bridge cpu=%.4f must be below the raw max-throughput charge", c.CPU)
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
		t.Errorf("prod host h264 4K 24p: admitted %d, want 2", n)
	}
	// hevc 4K 24p: 0.96 -> exactly 1 (priced on h264's 0.559, a 2nd would have slipped in).
	if n := admitN(t, s, 7, hevc, 2160, 24); n != 1 {
		t.Errorf("prod host hevc_vaapi 4K 24p: admitted %d, 0.96 fits 1", n)
	}
	// hevc 4K 60p: 2.36 > 1 -> REFUSED even for one session (0.51x realtime).
	// The old model priced it 0.559 and would have admitted it.
	if n := admitN(t, s, 7, hevc, 2160, 60); n != 0 {
		t.Errorf("prod host hevc_vaapi 4K 60p: admitted %d, it runs at 0.51x realtime and must be refused", n)
	}
	// Decoupled 1080: 5.6 cores were burnt RACING at 128 fps; paced to realtime
	// it is 5.6 x 28.8/128 = 1.26 cores = 0.18 of 7, and accel 28.8/128 = 0.225.
	// So the GPU pool binds it (~4), the CPU pool holds ~5 - NOT the pathological
	// "1 session" the max-throughput CPU charge wrongly produced.
	dec := rk("vaapi", "h264_vaapi", transcoding.RouteCostDecoupled)
	m := NewRouteCostModel(s, 7)
	dc, _ := m.Cost(dec, 1080, 24)
	if !approx(dc.CPU, 5.6*28.8/128/7) {
		t.Errorf("prod host decoupled cpu=%.3f want ~0.18 (paced)", dc.CPU)
	}
	if n := admitN(t, s, 7, dec, 1080, 24); n != 4 {
		t.Errorf("prod host decoupled 1080: admitted %d, GPU-bound at 0.225 => 4 (CPU 0.18 holds 5)", n)
	}
}

// x264 software: priced by the cores it burns at the paced rate over the CPU
// budget, so a weak 7-thread box fits a handful of realtime 1080 streams - not
// the 30+ the racing core count would wrongly imply, nor 1.
func TestAcceptance_ProdHost_x264(t *testing.T) {
	s := []transcoding.RouteSample{
		{Key: rk("", "libx264", transcoding.RouteCostSDR), Height: 1080, FPS: 130, SourceFPS: 24, CPUCores: 5.95},
	}
	k := rk("", "libx264", transcoding.RouteCostSDR)
	c, _ := NewRouteCostModel(s, 7).Cost(k, 1080, 24)
	if c.Accel != 0 {
		t.Fatalf("x264 spends no accelerator, got %.3f", c.Accel)
	}
	// A software session is input-paced at SoftwareReadRate (1.5x), not at the
	// 1.2x headroom it must clear: its cores are charged at the rate it really
	// runs. Priced at 1.2x the same session was 0.188 and five "fit" on seven
	// cores while the five producers together asked for 8.8 of them.
	if !approx(c.CPU, 5.95*24*transcoding.SoftwareReadRate/130/7) { // ~0.235
		t.Fatalf("x264 cpu=%.3f want ~0.235", c.CPU)
	}
	if n := admitN(t, s, 7, k, 1080, 24); n != 4 {
		t.Errorf("prod host x264 1080: admitted %d, ~0.235 each fits 4 on 7 cores", n)
	}
}

// The steady-state price of a software route is its paced consumption; the
// exact cgroup quota is the capacity it is a fraction of (a 0.75-core route
// is half of `cpus: 1.5`, not 37.5 % of the two workers the quota rounds to).
func TestRouteCostModel_CPUIsAFractionOfTheExactCapacity(t *testing.T) {
	s := []transcoding.RouteSample{
		{Key: rk("vaapi", "h264_vaapi", transcoding.RouteCostSDR), Height: 1080, FPS: 28.8, SourceFPS: 24, CPUCores: 0.75},
	}
	k := rk("vaapi", "h264_vaapi", transcoding.RouteCostSDR)
	c, _ := NewRouteCostModel(s, 1.5).Cost(k, 1080, 24)
	if !approx(c.CPU, 0.5) {
		t.Fatalf("0.75 core on a 1.5-CPU quota = 0.5 of the pool, got %.3f", c.CPU)
	}
	if !approx(c.Accel, 1.0) {
		t.Fatalf("a route measured exactly at the required rate fills the accelerator pool, got %.3f", c.Accel)
	}
}

// A host WITH route evidence must NOT price an uncovered non-equivalent route
// (x265, hevc_vaapi, bridge...) on the generic h264 table: it is refused so the
// resolver spills, never admitted on a fiction.
func TestAcceptance_UncoveredRouteRefusedNotTablePriced(t *testing.T) {
	s := []transcoding.RouteSample{
		{Key: rk("vaapi", "h264_vaapi", transcoding.RouteCostSDR), Height: 2160, FPS: 73, SourceFPS: 24, CPUCores: 0.2},
	}
	l := NewVideoSessionLimiter(0).
		WithCostModel(NewVideoCostModel([]transcoding.BenchRungSample{
			{Ladder: "hw", Height: 2160, FPS: 300, RequiredFPS: 28.8, Passing: true},
			{Ladder: "software", Height: 1080, FPS: 300, RequiredFPS: 28.8, Passing: true},
		})).
		WithRouteModel(NewRouteCostModel(s, 7))
	// hevc_vaapi has NO route sample; the generous table would admit it, the
	// route model must refuse it.
	hevc := rk("vaapi", "hevc_vaapi", transcoding.RouteCostSDR)
	if l.CanAdmitRoute("x", LadderClassHW, 2160, transcoding.SourceClass{}, hevc, 24) {
		t.Error("an uncovered hevc_vaapi route must be refused, not priced on the h264 table")
	}
	// The covered h264_vaapi route is admitted from its own evidence.
	if !l.CanAdmitRoute("y", LadderClassHW, 2160, transcoding.SourceClass{}, rk("vaapi", "h264_vaapi", transcoding.RouteCostSDR), 24) {
		t.Error("the covered route must be admitted from its evidence")
	}
	// An UNNAMED route (CanAdmit / TryAcquire, zero key) is the table, as the
	// contract says - not "uncovered". Prod 2026-09-13 21:53: the master shed
	// probe asked with a zero route and every title was refused on an idle host.
	if !l.CanAdmit("z", LadderClassSoftware, 1080) {
		t.Error("a zero route must fall back to the table, not be refused as uncovered")
	}
	if !l.TryAcquire("w", LadderClassHW, 2160) {
		t.Error("TryAcquire with a zero route must price on the table")
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

// The legacy count belt (AutoCountCap) must not become the real ceiling once
// route evidence exists: it has to be at least as generous as what the
// route-aware per-pool admission actually admits. On Ada, the cumulative table
// priced a 4K GPU session ~0.483 (belt 2), while the route evidence prices it
// 0.294 (admission fits 3) - the belt must reflect the route evidence, not the
// table, so it never caps below 3.
func TestAcceptance_CountBeltNotTighterThanRouteAdmission(t *testing.T) {
	route := []transcoding.RouteSample{
		{Key: rk("nvenc", "h264_nvenc", transcoding.RouteCostSDR), Height: 2160, FPS: 98, SourceFPS: 24, CPUCores: 0.63},
	}
	// A cumulative-table cost model that prices the hw 2160 rung dear (0.483) -
	// the very over-charge route evidence replaces.
	table := NewVideoCostModel([]transcoding.BenchRungSample{
		{Ladder: "hw", Height: 2160, FPS: 59.6, RequiredFPS: 28.8, Passing: true},
		{Ladder: "software", Height: 1080, FPS: 188, RequiredFPS: 28.8, Passing: true},
	})
	l := NewVideoSessionLimiter(0).WithCostModel(table).WithRouteModel(NewRouteCostModel(route, 24))

	// Route-aware admission fits 3 sessions of the 4K GPU route.
	admitted := admitN(t, route, 24, rk("nvenc", "h264_nvenc", transcoding.RouteCostSDR), 2160, 24)
	if admitted != 3 {
		t.Fatalf("route admission fits %d 4K GPU sessions, expected 3", admitted)
	}
	// The GPU count belt must be >= that, not the table's floor(1/0.483)=2.
	if gpu := l.AutoCountCapFor(PoolGPU); gpu < admitted {
		t.Errorf("GPU count belt %d is TIGHTER than route admission (%d): the legacy table leaked back in as the ceiling", gpu, admitted)
	}
}
