package transcoding

import (
	"fmt"
	"testing"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// admittedUntilRefusal counts the streams admission really grants for a
// session shape on the given (fresh) limiter.
func admittedUntilRefusal(l *VideoSessionLimiter, class LadderClass, h int, src transcoding.SourceClass, route transcoding.RouteCostKey) int {
	n := 0
	for i := range 64 {
		if !l.TryAcquireRoute(fmt.Sprintf("s%d", i), class, h, src, route, 0) {
			break
		}
		n++
	}
	return n
}

func routeProdLimiter() *VideoSessionLimiter {
	route := []transcoding.RouteSample{
		{Key: rk("", "libx264", transcoding.RouteCostSDR), Height: 480, FPS: 50.8, SourceFPS: 24, CPUCores: 3.5},
	}
	return prodLimiter().WithRouteModel(NewRouteCostModel(route, 7))
}

// The panel's per-source stream count is the admission's, not the table's.
func TestRungsFor_PerSourceMatchesAdmission(t *testing.T) {
	t.Parallel()
	key := rk("", "libx264", transcoding.RouteCostSDR)
	rungs := routeProdLimiter().RungsFor(LadderClassSoftware)
	if len(rungs) != 1 || len(rungs[0].BySource) == 0 {
		t.Fatalf("expected one rung with per-source fits, got %+v", rungs)
	}
	for _, f := range rungs[0].BySource {
		got := admittedUntilRefusal(routeProdLimiter(), LadderClassSoftware, 480, f.Source, key)
		if got == 0 {
			t.Errorf("source %+v: degenerate fixture, admission grants none", f.Source)
		}
		if f.MaxStreams != got {
			t.Errorf("source %+v: panel shows %d streams, admission grants %d", f.Source, f.MaxStreams, got)
		}
	}
}

// A GPU route draws on both pools: alone, its cap is the tighter one, not their sum.
func TestAutoCountCap_RouteEvidenceIsMinOfPoolsNotSum(t *testing.T) {
	t.Parallel()
	// accel = 24*headroom/FPS = 0.25 (4 fit); cpu = 2*0.25/3 = 1/6 (6 fit).
	fps := 24 * transcoding.RouteCostHeadroom / 0.25
	key := rk("nvenc", "h264_nvenc", transcoding.RouteCostSDR)
	s := []transcoding.RouteSample{{Key: key, Height: 1080, FPS: fps, SourceFPS: 24, CPUCores: 2}}
	mk := func() *VideoSessionLimiter { return NewVideoSessionLimiter(0).WithRouteModel(NewRouteCostModel(s, 3)) }
	if got := admittedUntilRefusal(mk(), LadderClassHW, 1080, transcoding.SourceClass{}, key); got != 4 {
		t.Fatalf("fixture: admission grants %d, want 4", got)
	}
	if got := mk().AutoCountCap(); got != 4 {
		t.Errorf("AutoCountCap = %d, want 4 (GPU pool binds; CPU holds 6)", got)
	}
}

// A hardware session that barely touches the CPU leaves room for software
// ones: the belt must not refuse what the pool budgets admit together.
func TestAutoCountCap_RouteEvidenceCountsSoftwareInTheCPULeftOver(t *testing.T) {
	t.Parallel()
	hwKey := rk("nvenc", "h264_nvenc", transcoding.RouteCostSDR)
	swKey := rk("", "libx264", transcoding.RouteCostSDR)
	// hw: accel 28.8/75.28 = 0.383 (2 fit), cpu 0.57*28.8/75.28/8 = 0.027.
	// sw: cpu 5.41*36/64.99/8 = 0.375 (2 fit, and still 2 in the 0.945 left).
	s := []transcoding.RouteSample{
		{Key: hwKey, Height: 1080, FPS: 75.28, SourceFPS: 24, CPUCores: 0.57},
		{Key: swKey, Height: 1080, FPS: 64.99, SourceFPS: 24, CPUCores: 5.41},
	}
	l := NewVideoSessionLimiter(0).WithRouteModel(NewRouteCostModel(s, 8))
	if got := l.AutoCountCap(); got != 4 {
		t.Fatalf("AutoCountCap = %d, want 4 (2 hardware + 2 software)", got)
	}

	l.SetLimit(l.AutoCountCap())
	for i := range 2 {
		if !l.TryAcquireRoute(fmt.Sprintf("hw-%d", i), LadderClassHW, 1080, transcoding.SourceClass{}, hwKey, 24) {
			t.Fatalf("hardware viewer %d refused", i)
		}
	}
	for i := range 2 {
		if !l.TryAcquireRoute(fmt.Sprintf("sw-%d", i), LadderClassSoftware, 1080, transcoding.SourceClass{}, swKey, 24) {
			t.Fatalf("software viewer %d refused while the CPU budget has room", i)
		}
	}
	if l.TryAcquireRoute("hw-extra", LadderClassHW, 1080, transcoding.SourceClass{}, hwKey, 24) {
		t.Error("a third hardware viewer was admitted past the accelerator budget")
	}
	if l.TryAcquireRoute("sw-extra", LadderClassSoftware, 1080, transcoding.SourceClass{}, swKey, 24) {
		t.Error("a third software viewer was admitted past the CPU budget")
	}
}

// A hardware route that eats the CPU leaves software nothing: no stream is
// counted twice.
func TestAutoCountCap_RouteEvidenceHardwareThatFillsTheCPULeavesNoSoftware(t *testing.T) {
	t.Parallel()
	// hw: accel 28.8/57.6 = 0.5 (2 fit), cpu 4*28.8/57.6/4 = 0.5 (2 fit).
	// sw alone: cpu 2*36/36/4 = 0.5 (2 fit); nothing left once hardware runs.
	s := []transcoding.RouteSample{
		{Key: rk("nvenc", "h264_nvenc", transcoding.RouteCostSDR), Height: 1080, FPS: 57.6, SourceFPS: 24, CPUCores: 4},
		{Key: rk("", "libx264", transcoding.RouteCostSDR), Height: 1080, FPS: 36, SourceFPS: 24, CPUCores: 2},
	}
	l := NewVideoSessionLimiter(0).WithRouteModel(NewRouteCostModel(s, 4))
	if got := l.AutoCountCap(); got != 2 {
		t.Errorf("AutoCountCap = %d, want 2 (the CPU is spent either way)", got)
	}
}
