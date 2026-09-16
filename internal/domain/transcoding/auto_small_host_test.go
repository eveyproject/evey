package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// auto_small_host_test.go - the Auto mode's ceilings on synthetic hosts, from
// a very weak CPU-only box to a multi-backend GPU box. Derive is pure, so each
// host is a table of bench samples; the runtime the samples model is
// single-variant (one rung per probe, ParallelEncodes 1).

func rung(ladder string, h int, fps float64) BenchRungSample {
	return BenchRungSample{Ladder: ladder, Height: h, FPS: fps, RequiredFPS: 28.8, Passing: fps >= 28.8, ParallelEncodes: 1}
}

// A CPU that clears no software rung at the 1.2x margin keeps the 720p floor:
// the host may still ATTEMPT 720p (admission prices it conservatively). The HDR
// ladder floors at 480p, AV1 stays hardware-only, no hw ceiling.
func TestAuto_VeryWeakCPU_KeepsSoftwareFloor720(t *testing.T) {
	t.Parallel()
	rep := BenchReport{
		SourceRate: 24,
		Samples: []BenchRungSample{
			rung(LadderSoftware, 720, 19.5), // under 28.8: the ladder stops here
			rung(LadderSoftwareHDR, 720, 9.1),
			rung(LadderSoftwareAV1, 720, 3.2),
		},
	}
	got := Derive(rep, DefaultDerivePolicy())
	assert.Equal(t, 720, got.SoftwareMaxHeight, "the SDR floor is kept, never removed")
	assert.Equal(t, 480, got.SoftwareHDRMaxHeight)
	assert.Equal(t, 0, got.SoftwareAV1MaxHeight, "AV1 software not viable => hardware-only")
	assert.Equal(t, 0, got.HWMaxHeight)
	assert.Empty(t, got.HWBackend)

	// The ladder the viewer sees: 360/480/720, nothing above.
	ladder := ShapeSoftwareLadder(LadderFilmNormal, SourceFacts{Width: 3840, Height: 2160}, "h264", false, got)
	var heights []int
	for _, p := range ladder {
		heights = append(heights, p.Height)
	}
	assert.Equal(t, []int{360, 480, 720}, heights)
}

// A CPU that sustains 1080p (not 1440p) is capped at 1080p, tallest passing rung.
func TestAuto_CPUCapableOf1080_CeilingDerived(t *testing.T) {
	t.Parallel()
	rep := BenchReport{
		SourceRate: 24,
		Samples: []BenchRungSample{
			rung(LadderSoftware, 720, 61.0),
			rung(LadderSoftware, 1080, 31.2),
			rung(LadderSoftware, 1440, 17.9),
		},
	}
	got := Derive(rep, DefaultDerivePolicy())
	assert.Equal(t, 1080, got.SoftwareMaxHeight)
}

// Hardware detected but under realtime at 720p: rejected (no hw ceiling), the
// host is software-only for encoding. The bench records the failed rung so the
// admin can see why.
func TestAuto_HWUnderRealtime_RejectedFallsBackToSoftware(t *testing.T) {
	t.Parallel()
	rep := BenchReport{
		SourceRate: 24,
		HWBackend:  "vaapi", // certified would set this; a failed ladder never does
		Samples: []BenchRungSample{
			rung(LadderSoftware, 720, 40),
			rung(LadderHW, 720, 22.0),
		},
	}
	got := Derive(rep, DefaultDerivePolicy())
	assert.Equal(t, 0, got.HWMaxHeight, "no passing hw rung => no hw ceiling, never a forced floor")
	assert.Equal(t, 720, got.MaxHeightFor(true), "a hw-routed request falls back to the software ceiling")
}

// Hardware viable only at 720p: certified at 720p, its own ceiling, no
// fictitious 1080p; the ladder of a route on it tops out at 720p.
func TestAuto_HWOnly720Viable_Certified720NoFictitious1080(t *testing.T) {
	t.Parallel()
	rep := BenchReport{
		SourceRate: 24,
		HWBackend:  "vaapi",
		HWCodecs:   []string{"h264"},
		Backends:   []HWBackendCap{{Backend: "vaapi", Codecs: []string{"h264"}, DecodeCodecs: []string{"h264", "hevc"}, MaxHeight: 720}},
		Samples: []BenchRungSample{
			rung(LadderSoftware, 720, 40),
			rung(LadderHW, 720, 45.0),
			rung(LadderHW, 1080, 21.0),
		},
	}
	got := Derive(rep, DefaultDerivePolicy())
	assert.Equal(t, 720, got.HWMaxHeight)
	assert.Equal(t, 720, got.HWCeilingFor("vaapi"))

	ladder := ShapeHardwareLadderFor(LadderFilmNormal, SourceFacts{Width: 1920, Height: 1080, Codec: "h264"}, got, "vaapi")
	assert.Equal(t, 720, ladder[len(ladder)-1].Height, "no 1080p rung on a 720p-certified accelerator")
}

// Two certified backends with different ceilings: a route resolved on the
// secondary is capped at ITS ceiling, the primary keeps its own.
func TestAuto_MultiBackend_LadderCappedAtThePickedBackendsCeiling(t *testing.T) {
	t.Parallel()
	cap := EncodeCapability{
		SoftwareMaxHeight: 1080, HWMaxHeight: 2160, HWBackend: "qsv",
		HWCodecs: []string{"h264", "hevc"}, HWDecodeCodecs: []string{"h264", "hevc"},
		Backends: []HWBackendCap{
			{Backend: "qsv", Codecs: []string{"h264", "hevc"}, DecodeCodecs: []string{"h264", "hevc"}, MaxHeight: 2160},
			{Backend: "vaapi", Codecs: []string{"h264"}, DecodeCodecs: []string{"h264", "hevc"}, MaxHeight: 1080},
		},
	}
	assert.Equal(t, 2160, cap.HWCeilingFor("qsv"))
	assert.Equal(t, 1080, cap.HWCeilingFor("vaapi"))
	assert.Equal(t, 2160, cap.HWCeilingFor("nvenc"), "an unknown backend reads the primary scalar")

	src := SourceFacts{Width: 3840, Height: 2160, Codec: "hevc"}
	top := func(l []HLSProfile) int { return l[len(l)-1].Height }
	assert.Equal(t, 2160, top(ShapeHardwareLadderFor(LadderFilmHighMotion, src, cap, "qsv")))
	assert.Equal(t, 1080, top(ShapeHardwareLadderFor(LadderFilmHighMotion, src, cap, "vaapi")))
	assert.Equal(t, 2160, top(ShapeHardwareLadder(LadderFilmHighMotion, src, cap)), "the unnamed form is the primary")

	// Through the ladder engine: the route names the accelerator.
	in := LadderInput{
		Source: src, Complexity: ComplexityVector{Class: ClassFilmHighMotion},
		Route: EncodeRoute{Codec: "h264", Accel: VAAPI, HWUsable: true}, Delivery: DeliveryConstraints{Capability: cap},
	}
	assert.Equal(t, 1080, top(StaticLadderBuilder{}.Build(in)))
	in.Route.Accel = QSV
	assert.Equal(t, 2160, top(StaticLadderBuilder{}.Build(in)))
}

// A secondary backend that cannot decode the source takes the decoupled path
// under the SOFTWARE ceiling, whatever its own encode ceiling says.
func TestAuto_MultiBackend_DecoupledOnTheSecondaryKeepsTheCPUCeiling(t *testing.T) {
	t.Parallel()
	cap := EncodeCapability{
		SoftwareMaxHeight: 720, HWMaxHeight: 2160, HWBackend: "nvenc", ProbeVersion: CapabilityProbeVersion,
		Backends: []HWBackendCap{
			{Backend: "nvenc", DecodeCodecs: []string{"h264", "hevc", "av1"}, MaxHeight: 2160},
			{Backend: "vaapi", DecodeCodecs: []string{"h264"}, MaxHeight: 1080},
		},
	}
	src := SourceFacts{Width: 3840, Height: 2160, Codec: "av1", PixelFormat: "yuv420p"}
	l := ShapeHardwareLadderFor(LadderFilmHighMotion, src, cap, "vaapi")
	assert.Equal(t, 720, l[len(l)-1].Height)
}

// A dead backend is dropped with its route evidence; a dead primary hands
// over to the next certified backend with THAT backend's ceiling; the last
// one gone leaves a software-only host.
func TestWithoutBackend_DropsEvidenceAndPromotesTheNextPrimary(t *testing.T) {
	t.Parallel()
	cap := EncodeCapability{
		SoftwareMaxHeight: 1080, HWMaxHeight: 2160, HWBackend: "qsv", HWDevice: "/dev/dri/renderD128",
		HWCodecs: []string{"h264", "hevc"}, HWFeatures: []string{"low_power_h264"}, HWRoutes: []string{HWRouteSDR, HWRouteHEVC},
		HWBridgeMaxHeight: 2160,
		Backends: []HWBackendCap{
			{Backend: "qsv", Device: "/dev/dri/renderD128", Codecs: []string{"h264", "hevc"}, Routes: []string{HWRouteSDR, HWRouteHEVC}, MaxHeight: 2160},
			{Backend: "vaapi", Device: "/dev/dri/renderD128", Codecs: []string{"h264"}, Routes: []string{HWRouteSDR, HWRouteTonemap}, HWTonemap: true, MaxHeight: 1080},
		},
		RouteSamples: []RouteSample{
			{Key: RouteCostKey{Backend: "qsv", Encoder: "h264_qsv", Route: RouteCostSDR}, Height: 1080, FPS: 200, SourceFPS: 24},
			{Key: RouteCostKey{Backend: "vaapi", Encoder: "h264_vaapi", Route: RouteCostSDR}, Height: 1080, FPS: 90, SourceFPS: 24},
			{Key: RouteCostKey{Backend: "", Encoder: "libx264", Route: RouteCostSDR}, Height: 1080, FPS: 40, SourceFPS: 24},
		},
		BenchSamples: []BenchRungSample{rung(LadderSoftware, 1080, 40), rung(LadderHW, 2160, 60), rung(LadderHWBridge, 1080, 50)},
	}

	// Dropping the secondary: primary untouched, its evidence gone.
	noVAAPI := cap.WithoutBackend("vaapi")
	assert.Equal(t, "qsv", noVAAPI.HWBackend)
	assert.Equal(t, 2160, noVAAPI.HWMaxHeight)
	require.Len(t, noVAAPI.Backends, 1)
	require.Len(t, noVAAPI.RouteSamples, 2)
	for _, s := range noVAAPI.RouteSamples {
		assert.NotEqual(t, "vaapi", s.Key.Backend)
	}

	// Dropping the primary: vaapi promoted, with ITS ceiling, routes, tonemap;
	// the dead primary's hw ladder samples no longer back a re-derive.
	noQSV := cap.WithoutBackend("qsv")
	assert.Equal(t, "vaapi", noQSV.HWBackend)
	assert.Equal(t, 1080, noQSV.HWMaxHeight)
	assert.Equal(t, []string{"h264"}, noQSV.HWCodecs)
	assert.Equal(t, []string{HWRouteSDR, HWRouteTonemap}, noQSV.HWRoutes)
	assert.True(t, noQSV.HWTonemap)
	assert.Equal(t, 0, noQSV.HWBridgeMaxHeight)
	for _, s := range noQSV.BenchSamples {
		assert.Equal(t, LadderSoftware, s.Ladder, "only software proof survives a primary hand-over")
	}
	rederived := Derive(noQSV.Report(), DefaultDerivePolicy())
	assert.Equal(t, 1080, rederived.HWMaxHeight, "the promoted backend's own ceiling survives a re-derive")
	assert.Equal(t, "vaapi", rederived.HWBackend)

	// Both gone: software-only, and the CPU's own route evidence survives - the
	// x264 route admission falls back to is still priced.
	none := noQSV.WithoutBackend("vaapi")
	assert.Empty(t, none.HWBackend)
	assert.Equal(t, 0, none.HWMaxHeight)
	assert.Empty(t, none.Backends)
	require.Len(t, none.RouteSamples, 1)
	assert.Equal(t, RouteCostKey{Backend: "", Encoder: "libx264", Route: RouteCostSDR}, none.RouteSamples[0].Key)
	assert.Equal(t, 1080, none.SoftwareMaxHeight, "the software ceiling is untouched")

	// A runtime HW init demotion (disableHW) keeps the software evidence too.
	d := PlanDemotion(cap, FailureHWInit, "h264")
	assert.Empty(t, d.Capability.HWBackend)
	assert.Empty(t, d.Capability.Backends)
	require.Len(t, d.Capability.RouteSamples, 1)
	assert.Empty(t, d.Capability.RouteSamples[0].Key.Backend)

	// A pre-Backends row (scalars only) behaves as a single-backend host.
	legacy := EncodeCapability{SoftwareMaxHeight: 1080, HWMaxHeight: 2160, HWBackend: "nvenc"}
	assert.Empty(t, legacy.WithoutBackend("nvenc").HWBackend)
	assert.Equal(t, 2160, legacy.BackendCaps()[0].MaxHeight, "the synthesised entry carries the primary ceiling")
}
