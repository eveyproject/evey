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

// A GPU route draws on both pools: the cap is the tighter one, not their sum.
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
