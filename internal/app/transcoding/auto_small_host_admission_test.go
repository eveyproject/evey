package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// auto_small_host_admission_test.go - the Auto mode's ADMISSION on synthetic
// small hosts: the route evidence a weak CPU / a 720p-only accelerator produce,
// and how the limiter prices it. The chain under test is calibration (route
// samples) -> RouteCostModel -> VideoSessionLimiter -> the count belt.

// A 4-core box whose x264 720p floor runs UNDER realtime (19.5 fps for 28.8
// required, 3.6 cores): the floor is kept, one session fits an idle CPU pool,
// nothing joins it, the count belt derives to 1. Without the clamp the route
// would price 1.26 of the pool and be refused forever on a host that has
// nothing else.
func TestAuto_SoftwareFloorUnderRealtime_ExactlyOneSessionFits(t *testing.T) {
	t.Parallel()
	floor := []transcoding.RouteSample{
		{Key: rk("", "libx264", transcoding.RouteCostSDR), Height: 720, FPS: 19.5, SourceFPS: 24, CPUCores: 3.6},
		{Key: rk("", "libx264", transcoding.RouteCostSoftwareHDR), Height: 480, FPS: 14.0, SourceFPS: 24, CPUCores: 3.8},
	}
	m := NewRouteCostModel(floor, 4)
	require.NotNil(t, m)
	c, ok := m.Cost(rk("", "libx264", transcoding.RouteCostSDR), 720, 24)
	require.True(t, ok)
	assert.InDelta(t, 1.0, c.CPU, 1e-9, "a route under realtime is clamped to one whole pool")
	assert.Zero(t, c.Accel)

	l := NewVideoSessionLimiter(0).WithRouteModel(m)
	assert.True(t, l.TryAcquireRoute("viewer-1", LadderClassSoftware, 720, transcoding.SourceClass{}, floor[0].Key, 24), "the floor may be attempted on an idle host")
	assert.False(t, l.TryAcquireRoute("viewer-2", LadderClassSoftware, 720, transcoding.SourceClass{}, floor[0].Key, 24), "conservative: nothing joins a pool one floor session already fills")
	assert.False(t, l.CanAdmitRoute("viewer-3", LadderClassSoftwareHDR, 480, transcoding.SourceClass{}, floor[1].Key, 24))
	l.Release("viewer-1")
	assert.True(t, l.CanAdmitRoute("viewer-3", LadderClassSoftwareHDR, 480, transcoding.SourceClass{}, floor[1].Key, 24), "the HDR floor (480p) is priced too, not refused for lack of evidence")
	assert.Equal(t, 1, l.AutoCountCap(), "the count belt derives to one stream")
	assert.Equal(t, 0, l.AutoCountCapFor(PoolGPU))

	// The clamp is the FLOOR's alone. A software route that clears realtime on
	// its sample but is asked at a rate it cannot sustain keeps its honest >1
	// price and is refused (4K 60p on a 24p-certified 1080 rung), exactly as an
	// accelerator route is (TestAcceptance_ProdHost_VAAPI).
	passing := []transcoding.RouteSample{{Key: rk("", "libx264", transcoding.RouteCostSDR), Height: 1080, FPS: 40, SourceFPS: 24, CPUCores: 3.5}}
	pm := NewRouteCostModel(passing, 4)
	c, ok = pm.Cost(passing[0].Key, 1080, 60)
	require.True(t, ok)
	assert.Greater(t, c.CPU, 1.0, "a 60p request on a 24p-certified rung is not a floor: no clamp")
	assert.False(t, NewVideoSessionLimiter(0).WithRouteModel(pm).CanAdmitRoute("v", LadderClassSoftware, 1080, transcoding.SourceClass{}, passing[0].Key, 60))

	// A rung ABOVE the policy floor measured under realtime is never made
	// admissible by the clamp: 1080p at 19.5 fps stays refused (only 720p SDR /
	// 480p HDR are the attemptable floors).
	tall := []transcoding.RouteSample{{Key: rk("", "libx264", transcoding.RouteCostSDR), Height: 1080, FPS: 19.5, SourceFPS: 24, CPUCores: 3.6}}
	tm := NewRouteCostModel(tall, 4)
	c, ok = tm.Cost(tall[0].Key, 1080, 24)
	require.True(t, ok)
	assert.Greater(t, c.CPU, 1.0, "a sub-realtime 1080p rung is not the floor: no clamp")
	assert.False(t, NewVideoSessionLimiter(0).WithRouteModel(tm).CanAdmitRoute("v", LadderClassSoftware, 1080, transcoding.SourceClass{}, tall[0].Key, 24))
	// An HDR route at 720p is above ITS floor (480p): no clamp either.
	hdr720 := []transcoding.RouteSample{{Key: rk("", "libx264", transcoding.RouteCostSoftwareHDR), Height: 720, FPS: 12, SourceFPS: 24, CPUCores: 3.8}}
	c, _ = NewRouteCostModel(hdr720, 4).Cost(hdr720[0].Key, 720, 24)
	assert.Greater(t, c.CPU, 1.0)

	// The floor is attemptable at the cadence it was measured for: a 60p source
	// on the 24p floor keeps its honest price and is refused.
	c, ok = m.Cost(floor[0].Key, 720, 60)
	require.True(t, ok)
	assert.Greater(t, c.CPU, 1.0, "a 60p source on the 24p floor is not clamped")
}

// A host with NO usable evidence at all keeps the table-free contract: admitted
// on the counter alone (the wiring arms that counter at 1 for such a host).
func TestAuto_NoEvidence_CounterOnly(t *testing.T) {
	t.Parallel()
	l := NewVideoSessionLimiter(1)
	assert.Nil(t, NewRouteCostModel(nil, 4))
	assert.True(t, l.TryAcquire("a", LadderClassSoftware, 720))
	assert.False(t, l.TryAcquire("b", LadderClassSoftware, 720))
	assert.Equal(t, 0, l.AutoCountCap(), "no evidence => nothing to derive; ResolveVideoSessionLimit floors it to 1")
	assert.Equal(t, 1, ResolveVideoSessionLimit(t.Context(), nil, l.AutoCountCap()))
}

// The point of measuring at the ceiling rung: a backend certified ONLY at 720p
// has its one route sample, so a session on it is admitted - even though the
// software routes carry 1080p evidence. Before, the route model existed (from
// the software samples), the hw key had no rung, and the named route was
// refused: "host at video capacity" on an idle box for every GPU play.
func TestAuto_HWOnly720_AdmittedNextToOtherBackendsEvidence(t *testing.T) {
	t.Parallel()
	samples := []transcoding.RouteSample{
		{Key: rk("", "libx264", transcoding.RouteCostSDR), Height: 1080, FPS: 40, SourceFPS: 24, CPUCores: 3.5},
		{Key: rk("", "libx265", transcoding.RouteCostSDR), Height: 1080, FPS: 30, SourceFPS: 24, CPUCores: 3.8},
		{Key: rk("vaapi", "h264_vaapi", transcoding.RouteCostSDR), Height: 720, FPS: 45, SourceFPS: 24, CPUCores: 0.4},
	}
	m := NewRouteCostModel(samples, 4)
	l := NewVideoSessionLimiter(0).WithRouteModel(m)
	hw := rk("vaapi", "h264_vaapi", transcoding.RouteCostSDR)

	assert.True(t, l.CanAdmitRoute("v1", LadderClassHW, 720, transcoding.SourceClass{}, hw, 24), "the 720p-only accelerator prices on its own sample")
	c, ok := m.Cost(hw, 720, 24)
	require.True(t, ok)
	assert.InDelta(t, 28.8/45, c.Accel, 1e-9)
	assert.InDelta(t, 0.4*28.8/45/4, c.CPU, 1e-9, "its CPU share lands on the CPU pool")
	// The ladder never offers 1080p on this backend; a caller asking anyway is
	// refused - a rung above the tallest proven one is never priced as it.
	assert.False(t, l.CanAdmitRoute("v1", LadderClassHW, 1080, transcoding.SourceClass{}, hw, 24))
	// Its evidence sizes the GPU count belt too. (The CPU belt is sized on the
	// cheapest CPU-charging route - the GPU session's 0.064 core share - and is
	// deliberately loose: the per-pool budget, not the belt, is the gate.)
	assert.Equal(t, 1, l.AutoCountCapFor(PoolGPU))
	assert.GreaterOrEqual(t, l.AutoCountCapFor(PoolCPU), 1)
	// Two GPU sessions: 0.64 + 0.64 > 1 on the accelerator pool -> the second spills.
	require.True(t, l.TryAcquireRoute("v1", LadderClassHW, 720, transcoding.SourceClass{}, hw, 24))
	assert.False(t, l.TryAcquireRoute("v2", LadderClassHW, 720, transcoding.SourceClass{}, hw, 24))
	assert.True(t, l.CanAdmitRoute("v2", LadderClassSoftware, 1080, transcoding.SourceClass{}, samples[0].Key, 24), "the CPU pool still takes a software stream (0.072 + 0.63)")
}

// A backend rejected by the ladder never reaches the evidence: an unnamed hw
// route on such a host prices on the table / counter, and the resolver never
// picks it (no Backends entry). Locks the "HW not viable => software" contract
// at the admission layer: a hw key with no sample AND a model built from
// software evidence refuses the hw route, so a stale fingerprint cannot spawn
// on a GPU the calibration rejected.
func TestAuto_HWRejected_StaleHWRouteRefusedSoftwareAdmitted(t *testing.T) {
	t.Parallel()
	samples := []transcoding.RouteSample{
		{Key: rk("", "libx264", transcoding.RouteCostSDR), Height: 720, FPS: 40, SourceFPS: 24, CPUCores: 3.5},
	}
	l := NewVideoSessionLimiter(0).WithRouteModel(NewRouteCostModel(samples, 4))
	assert.False(t, l.CanAdmitRoute("v", LadderClassHW, 720, transcoding.SourceClass{}, rk("vaapi", "h264_vaapi", transcoding.RouteCostSDR), 24))
	assert.True(t, l.CanAdmitRoute("v", LadderClassSoftware, 720, transcoding.SourceClass{}, samples[0].Key, 24))
}

// Two certified backends, chosen per workload: the rung the session wants
// FIRST (a 720p-only accelerator never beats a 2160p one for a 2160p request,
// however cheap it measures), then the measured cost (the same 720p-only
// backend wins a 720p request when it is cheaper).
func TestAuto_MultiBackend_PickedByMeasuredCost(t *testing.T) {
	t.Parallel()
	cap := twoBackendCap()
	cap.Backends[1].MaxHeight = 720 // VAAPI certified at 720p only
	cap.Backends[0].MaxHeight = 2160
	for i := range cap.RouteSamples {
		if cap.RouteSamples[i].Key.Backend == "vaapi" {
			cap.RouteSamples[i].Height = 720
		}
	}
	// Make VAAPI the faster SDR encoder by measure: the pick must follow the
	// measurement, not the primary's name.
	for i := range cap.RouteSamples {
		if cap.RouteSamples[i].Key == rk("vaapi", "h264_vaapi", transcoding.RouteCostSDR) {
			cap.RouteSamples[i].FPS = 300
		}
	}
	pr := &PlanResolver{
		HWAccelEnabled: func() bool { return true }, HWTonemapEnabled: func() bool { return true },
		Capability: func() transcoding.HostCapabilitySnapshot { return (cap).Snapshot() },
		RouteCosts: func() *RouteCostModel { return NewRouteCostModel(cap.RouteSamples, 7) },
	}
	// A 2160p source, no client cap: the session wants 2160 -> only QSV reaches
	// it; VAAPI's cheaper SDR price never enters the comparison.
	sdr := transcoding.PlaybackDecision{VideoDecision: "transcode", VideoCodec: "h264", TranscodeReasons: []string{"video_codec_unsupported"}}
	_, accel, _, _, usable, _ := pr.EffectiveVideoCodec(sdr, sdrStreams("h264", "yuv420p", false))
	assert.True(t, usable)
	assert.Equal(t, "qsv", accel, "a 2160p request goes to the backend certified for 2160, not the cheaper 720p-only one")
	assert.True(t, pr.HWRouteUsable(sdr, SourceDimsOf(sdrStreams("h264", "yuv420p", false))))

	// The same title on a client capped at 720p: both reach the rung, the
	// measured-cheaper VAAPI wins.
	sdr720 := sdr
	sdr720.MaxHeight = 720
	_, accel, _, _, usable, _ = pr.EffectiveVideoCodec(sdr720, sdrStreams("h264", "yuv420p", false))
	assert.True(t, usable)
	assert.Equal(t, "vaapi", accel, "a 720p request takes the cheaper backend that reaches 720p")

	// A 1080p request: VAAPI (720) cannot reach it, QSV takes it.
	sdr1080 := sdr
	sdr1080.MaxHeight = 1080
	_, accel, _, _, _, _ = pr.EffectiveVideoCodec(sdr1080, sdrStreams("h264", "yuv420p", false))
	assert.Equal(t, "qsv", accel)

	hevc := transcoding.PlaybackDecision{VideoDecision: "transcode", VideoCodec: "hevc", TranscodeReasons: []string{"video_codec_unsupported"}, MaxHeight: 720}
	_, accel, _, _, _, _ = pr.EffectiveVideoCodec(hevc, sdrStreams("h264", "yuv420p", false))
	assert.Equal(t, "qsv", accel, "HEVC at 720p stays on the backend measured cheaper for it")

	// No backend reaches the wanted rung (a 4K request on a 1080p QSV + 720p
	// VAAPI box): the tallest ceilings stay in the field, the request is capped
	// there - never demoted to software for want of a 2160p accelerator.
	cap.Backends[0].MaxHeight = 1080
	_, accel, _, _, usable, _ = pr.EffectiveVideoCodec(sdr, sdrStreams("h264", "yuv420p", false))
	assert.True(t, usable)
	assert.Equal(t, "qsv", accel)

	// The ladder of the VAAPI route is capped at VAAPI's own 720p ceiling; QSV's at 2160.
	src := transcoding.SourceFacts{Width: 3840, Height: 2160, Codec: "h264", PixelFormat: "yuv420p"}
	top := func(l []transcoding.HLSProfile) int { return l[len(l)-1].Height }
	assert.Equal(t, 720, top(transcoding.ShapeHardwareLadderFor(transcoding.LadderFilmHighMotion, src, cap, "vaapi")))
	assert.Equal(t, 1080, top(transcoding.ShapeHardwareLadderFor(transcoding.LadderFilmHighMotion, src, cap, "qsv")), "QSV was lowered to 1080 above")
	cap.Backends[0].MaxHeight = 2160
	assert.Equal(t, 2160, top(transcoding.ShapeHardwareLadderFor(transcoding.LadderFilmHighMotion, src, cap, "qsv")))
}
