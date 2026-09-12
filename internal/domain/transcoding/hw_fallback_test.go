package transcoding

import (
	"slices"
	"testing"
)

func probedCap() EncodeCapability {
	return EncodeCapability{
		SoftwareMaxHeight: 1080, HWMaxHeight: 2160, HWBackend: "nvenc", HWDevice: "",
		HWCodecs: []string{"h264", "hevc"}, HWDecodeCodecs: []string{"h264", "hevc", HWDecodeHEVC10},
		HWFeatures: []string{"rc_lookahead", "temporal_aq"}, ProbeVersion: CapabilityProbeVersion,
		HWRoutes:     []string{HWRouteSDR, HWRouteBridge},
		BenchSamples: []BenchRungSample{{Ladder: LadderHW, Height: 1080, FPS: 100, RequiredFPS: 28.8, Passing: true}},
		SourceRate:   24,
	}
}

func TestFailureClass_RouteIncompatibleIsNotAHostVerdict(t *testing.T) {
	c := FailureRouteIncompatible
	if c.IsHWCapability() {
		t.Fatal("a media/route incompatibility is not a host capability failure")
	}
	if !c.Deterministic() || !c.BlocksTitleRoute() {
		t.Fatal("it is deterministic for the (media, route) pair, so the title falls back")
	}
	d := PlanDemotion(probedCap(), c, "h264")
	if d.Changed || d.Persist || d.RetryTier != TierSoftware {
		t.Fatalf("route incompatibility must retry in software and leave the host alone: %+v", d)
	}
	for _, blocks := range []FailureClass{FailureHWUnsupportedOption, FailureHWEncoderMissing, FailureHWInit, FailureHWCorruptOutput} {
		if !blocks.BlocksTitleRoute() {
			t.Errorf("%s must block the title's route before any host verdict", blocks)
		}
	}
	for _, free := range []FailureClass{FailureHWDeviceMemory, FailureTransient, FailureCancelled, FailureUnknown, FailureNone} {
		if free.BlocksTitleRoute() {
			t.Errorf("%s must not block the title", free)
		}
	}
	for _, silent := range []FailureClass{FailureUnknown, FailureNone, FailureRouteIncompatible} {
		if !silent.SilentDeathCandidate() {
			t.Errorf("%s with no segment written is a silent death", silent)
		}
	}
	for _, loud := range []FailureClass{FailureCancelled, FailureHWDeviceMemory, FailureTransient} {
		if loud.SilentDeathCandidate() {
			t.Errorf("%s must not be read as a silent death", loud)
		}
	}
}

func TestPlanFeatureDemotion_NarrowsToWhatWasReconfirmed(t *testing.T) {
	cap := probedCap()
	d := PlanFeatureDemotion(cap, []string{"rc_lookahead"})
	if !d.Changed || !d.Persist || !slices.Equal(d.Capability.HWFeatures, []string{"rc_lookahead"}) {
		t.Fatalf("unexpected demotion: %+v", d)
	}
	if d.Capability.HWBackend != "nvenc" || d.Capability.HWMaxHeight != 2160 {
		t.Fatal("a feature demotion keeps the backend and its ceiling")
	}
	same := PlanFeatureDemotion(cap, []string{"temporal_aq", "rc_lookahead"})
	if same.Changed {
		t.Fatal("every feature re-confirmed (any order) is no change")
	}
}

func TestCapability_RouteProofAndDecodeClasses(t *testing.T) {
	cap := probedCap()
	if !cap.HWRouteProven(HWRouteSDR) || cap.HWRouteProven(HWRouteHEVC) {
		t.Fatal("a probed row is authoritative: only listed routes are proven")
	}
	legacy := cap
	legacy.HWRoutes = nil
	if !legacy.HWRouteProven(HWRoutePassthrough) {
		t.Fatal("a pre-probe row (nil routes) is trusted until re-benched")
	}
	empty := cap
	empty.HWRoutes = []string{}
	if empty.HWRouteProven(HWRouteSDR) {
		t.Fatal("an empty probed list proves nothing (fail-closed)")
	}
	none := cap
	none.HWBackend = ""
	if none.HWRouteProven(HWRouteSDR) {
		t.Fatal("no backend, no route")
	}

	if !cap.HWCanDecodeSource("hevc", true) || !cap.HWCanDecodeSource("hevc", false) || !cap.HWCanDecodeSource("h264", false) {
		t.Fatal("the probed classes decode")
	}
	if cap.HWCanDecodeSource("h264", true) || cap.HWCanDecodeSource("vp9", true) {
		t.Fatal("10-bit H.264 / VP9 have no GPU decode class")
	}
	noMain10 := cap
	noMain10.HWDecodeCodecs = []string{"h264", "hevc"}
	if noMain10.HWCanDecodeSource("hevc", true) {
		t.Fatal("a probed row without hevc10 does not decode Main10")
	}
	old := noMain10
	old.ProbeVersion = 4
	if !old.HWCanDecodeSource("hevc", true) {
		t.Fatal("a pre-class row reads hevc as covering Main10, as it always did")
	}

	for _, ok := range []string{"", "yuv420p", "yuvj420p", "nv12", "yuv420p10le", "p010le", "YUV420P"} {
		if !HWChromaDecodable(ok) {
			t.Errorf("%q must be GPU-decodable chroma", ok)
		}
	}
	for _, bad := range []string{"yuv422p10le", "yuv444p", "yuv420p12le", "gbrp", "gray"} {
		if HWChromaDecodable(bad) {
			t.Errorf("%q must not be GPU-decodable", bad)
		}
	}
}

// The route proof must survive a boot: Derive rebuilds the capability from
// its Report, and a demotion that switches HW off must drop the proof too.
func TestCapability_RoutesRoundTripAndClearOnDisable(t *testing.T) {
	cap := probedCap()
	got := Derive(cap.Report(), DefaultDerivePolicy())
	if !slices.Equal(got.HWRoutes, cap.HWRoutes) {
		t.Fatalf("HWRoutes lost across Report/Derive: %v", got.HWRoutes)
	}
	if !slices.Equal(got.HWDecodeCodecs, cap.HWDecodeCodecs) {
		t.Fatal("decode classes lost across Report/Derive")
	}
	d := PlanDemotion(cap, FailureHWInit, "h264")
	if d.Capability.HWRoutes != nil || d.Capability.HWBackend != "" {
		t.Fatalf("disabling HW must clear the route proof: %+v", d.Capability)
	}
}
