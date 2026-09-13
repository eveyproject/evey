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

	// Probed matrix row (v6): every answer comes from the listed classes.
	matrix := probedCap()
	matrix.HWDecodeCodecs = []string{"h264", "hevc", HWDecodeHEVC10, "hevc:yuv444p"}
	if !matrix.HWCanDecodeSource("hevc", true, "yuv420p10le") || !matrix.HWCanDecodeSource("hevc", false, "yuv420p") || !matrix.HWCanDecodeSource("h264", false, "yuv420p") {
		t.Fatal("the probed classes decode")
	}
	if !matrix.HWCanDecodeSource("hevc", false, "yuv444p") {
		t.Fatal("a 4:4:4 class the probe measured IS decodable - no universal chroma rule")
	}
	if matrix.HWCanDecodeSource("h264", true, "yuv420p10le") || matrix.HWCanDecodeSource("hevc", true, "yuv422p10le") || matrix.HWCanDecodeSource("hevc", false, "gbrp") {
		t.Fatal("a class the probe did not list, or the matrix does not cover, is not decodable (fail-closed)")
	}
	if !matrix.HWCanDecodeSource("hevc", true, "") || matrix.HWCanDecodeSource("h264", true, "") {
		t.Fatal("unknown pix_fmt falls back to the depth rule")
	}
	if got, want := DecodeClassKey("h265", "p010le"), HWDecodeHEVC10; got != want {
		t.Fatalf("DecodeClassKey = %q, want %q", got, want)
	}
	if got, want := DecodeClassKey("h264", "yuv422p10le"), "h264:yuv422p10le"; got != want {
		t.Fatalf("DecodeClassKey = %q, want %q", got, want)
	}
	// Rows probed before the matrix: depth rule + the conservative chroma table.
	noMain10 := probedCap()
	noMain10.ProbeVersion = 5
	noMain10.HWDecodeCodecs = []string{"h264", "hevc"}
	if noMain10.HWCanDecodeSource("hevc", true, "yuv420p10le") {
		t.Fatal("a v5 row without hevc10 does not decode Main10")
	}
	if noMain10.HWCanDecodeSource("hevc", false, "yuv444p") {
		t.Fatal("a pre-matrix row keeps the conservative chroma table")
	}
	old := noMain10
	old.ProbeVersion = 4
	if !old.HWCanDecodeSource("hevc", true, "yuv420p10le") {
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

func TestBackendCaps_SynthesisAndSelectors(t *testing.T) {
	// A pre-Backends row synthesises one backend from the scalars (single-backend
	// behaviour, unchanged).
	legacy := probedCap() // nvenc scalar, no Backends
	bcs := legacy.BackendCaps()
	if len(bcs) != 1 || bcs[0].Backend != "nvenc" {
		t.Fatalf("legacy row must synthesise one backend, got %v", bcs)
	}

	// A two-backend row: QSV (no GPU tonemap), VAAPI (OpenCL tonemap).
	cap := EncodeCapability{
		ProbeVersion: CapabilityProbeVersion, HWBackend: "qsv",
		Backends: []HWBackendCap{
			{Backend: "qsv", Codecs: []string{"h264", "hevc"}, DecodeCodecs: []string{"h264", "hevc", HWDecodeHEVC10},
				HW10Bit: true, HWHDR10Encode: true, HWOverlay: true,
				Routes: []string{HWRouteSDR, HWRouteHEVC, HWRoutePassthrough}},
			{Backend: "vaapi", Codecs: []string{"h264"}, DecodeCodecs: []string{"h264"}, HWTonemap: true,
				Features: []string{FeatureTonemapOpenCL}, Routes: []string{HWRouteSDR, HWRouteTonemap}},
		},
	}
	// Tone-map: only VAAPI proves it and carries the probe + OpenCL.
	if b, ok := cap.TonemapBackend("PQ"); !ok || b.Backend != "vaapi" {
		t.Fatalf("tone-map backend = %v/%v, want vaapi", b.Backend, ok)
	}
	if b, ok := cap.TonemapBackend("HLG"); !ok || b.Backend != "vaapi" {
		t.Fatalf("OpenCL handles HLG too: %v/%v", b.Backend, ok)
	}
	// QSV alone (no OpenCL, vpp ineligible => never proves the route): no GPU tonemap.
	qsvOnly := EncodeCapability{ProbeVersion: CapabilityProbeVersion, HWBackend: "qsv",
		Backends: []HWBackendCap{{Backend: "qsv", HWTonemap: true, Routes: []string{HWRouteSDR}}}}
	if _, ok := qsvOnly.TonemapBackend("PQ"); ok {
		t.Fatal("QSV without a proven tone-map route must not be a tone-map backend")
	}
	// Passthrough: QSV qualifies (hevc + hdr10 + 10bit + proves passthrough).
	if b, ok := cap.PassthroughBackend(); !ok || b.Backend != "qsv" {
		t.Fatalf("passthrough backend = %v/%v, want qsv", b.Backend, ok)
	}
	if !cap.AnyBackendSupports("hevc") {
		t.Fatal("QSV supports hevc")
	}
	if b, ok := cap.OverlayBackend(); !ok || b.Backend != "qsv" {
		t.Fatalf("overlay backend = %v/%v, want qsv", b.Backend, ok)
	}

	// Round-trip through Report/Derive keeps Backends; disableHW clears them.
	if got := Derive(cap.Report(), DefaultDerivePolicy()); len(got.Backends) != 2 {
		t.Fatalf("Backends lost across Report/Derive: %d", len(got.Backends))
	}
	d := PlanDemotion(cap, FailureHWInit, "h264")
	if d.Capability.Backends != nil {
		t.Fatal("disabling HW must clear Backends")
	}
}
