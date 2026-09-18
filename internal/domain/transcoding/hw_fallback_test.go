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
	// The legacy/web codecs key on their ffprobe name, 10-bit VP9/AV1 on the
	// generic "<codec>:yuv420p10le" (only HEVC Main10 has a short name); a
	// class the host did not list (VC-1 without its fixture, a codec the
	// matrix never covered) is not decodable.
	for in, want := range map[[2]string]string{
		{"mpeg2video", "yuv420p"}: "mpeg2video", {"vp8", "yuv420p"}: "vp8", {"vp9", "yuv420p"}: "vp9",
		{"vp9", "yuv420p10le"}: "vp9:yuv420p10le", {"av1", "yuv420p10le"}: "av1:yuv420p10le", {"vc1", "yuv420p"}: "vc1",
	} {
		if got := DecodeClassKey(in[0], in[1]); got != want {
			t.Errorf("DecodeClassKey(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
	web := probedCap()
	web.HWDecodeCodecs = []string{"h264", "hevc", "mpeg2video", "vp9", "av1:yuv420p10le"}
	if !web.HWCanDecodeSource("mpeg2video", false, "yuv420p") || !web.HWCanDecodeSource("vp9", false, "yuv420p") || !web.HWCanDecodeSource("av1", true, "yuv420p10le") {
		t.Fatal("a listed legacy/web class is decodable")
	}
	withVC1 := probedCap()
	withVC1.HWDecodeCodecs = []string{"h264", "vc1"}
	if !withVC1.HWCanDecodeSource("vc1", false, "yuv420p") || !withVC1.HWCanDecodeSource("wmv3", false, "") {
		t.Fatal("a host whose fixture probe listed VC-1 decodes it like any other class")
	}
	if web.HWCanDecodeSource("vc1", false, "yuv420p") || web.HWCanDecodeSource("vp9", true, "yuv420p10le") || web.HWCanDecodeSource("av1", false, "yuv420p") {
		t.Fatal("an unlisted class (VC-1 here, 10-bit VP9, 8-bit AV1) is not decodable - fail-closed")
	}
	// Rows probed before the matrix read in the same vocabulary: a plain
	// codec is its 4:2:0 8-bit class, an unlisted class is not decodable.
	noMain10 := probedCap()
	noMain10.ProbeVersion = 5
	noMain10.HWDecodeCodecs = []string{"h264", "hevc"}
	if noMain10.HWCanDecodeSource("hevc", true, "yuv420p10le") {
		t.Fatal("a v5 row without hevc10 does not decode Main10")
	}
	if noMain10.HWCanDecodeSource("hevc", false, "yuv444p") {
		t.Fatal("a pre-matrix row lists no 4:4:4 class: not decodable")
	}
	// A pre-class row (v4) listed "hevc" for Main10 too: Normalized, applied
	// once at load, says so in the class vocabulary; the gate knows no version.
	old := noMain10
	old.ProbeVersion = 4
	if old.HWCanDecodeSource("hevc", true, "yuv420p10le") {
		t.Fatal("the gate itself reads the list as written: no version branch")
	}
	norm := old.Snapshot().EncodeCapability
	if !norm.HWCanDecodeSource("hevc", true, "yuv420p10le") || !norm.HWCanDecodeSource("hevc", true, "") {
		t.Fatal("a normalized pre-class row reads hevc as covering Main10, as it always did")
	}
	if !slices.Equal(old.HWDecodeCodecs, []string{"h264", "hevc"}) {
		t.Fatal("Normalized copies, it never edits the persisted row")
	}
	if got := noMain10.Snapshot().EncodeCapability; !slices.Equal(got.HWDecodeCodecs, noMain10.HWDecodeCodecs) {
		t.Fatal("a row with the classes is normalized to itself")
	}
	// Bit depth alone can only name HEVC's 10-bit class: a 10-bit source of
	// another codec and unknown format is not decodable, fail-closed.
	if probedCap().HWCanDecodeSource("vp9", true, "") {
		t.Fatal("a 10-bit non-HEVC source of unknown format names no class")
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

// TonemapBackend with the fixed-function VPP only (no OpenCL feature): it
// tone-maps PQ but refuses non-PQ ranges - the inverse of the OpenCL case, and
// the reason native and OpenCL must be probed independently (a host with only
// one of them still gets a correct, range-appropriate verdict).
func TestTonemapBackend_NativeVPPHandlesPQOnly(t *testing.T) {
	cap := EncodeCapability{ProbeVersion: CapabilityProbeVersion, HWBackend: "vaapi",
		Backends: []HWBackendCap{{Backend: "vaapi", Codecs: []string{"h264"}, HWTonemap: true,
			Routes: []string{HWRouteSDR, HWRouteTonemap}}}} // native VPP, NO FeatureTonemapOpenCL
	if b, ok := cap.TonemapBackend("PQ"); !ok || b.Backend != "vaapi" {
		t.Fatalf("fixed VPP must tone-map PQ: %v/%v", b.Backend, ok)
	}
	if _, ok := cap.TonemapBackend("HLG"); ok {
		t.Fatal("fixed VPP must NOT claim non-PQ ranges (needs OpenCL)")
	}
}

// RetryableInit is the calibration-only retry set (init/readiness/transient),
// deliberately distinct from Deterministic(): hw_init is Deterministic for a
// persisted runtime verdict yet retryable while calibrating (a cold GPU clears
// it on replay). Route/format/unsupported/encoder-missing/corrupt/cancelled
// are never retried.
func TestFailureClass_RetryableInit(t *testing.T) {
	for _, c := range []FailureClass{FailureHWInit, FailureHWDeviceMemory, FailureTransient} {
		if !c.RetryableInit() {
			t.Errorf("%s must be retryable at calibration", c)
		}
	}
	for _, c := range []FailureClass{FailureRouteIncompatible, FailureHWUnsupportedOption,
		FailureHWEncoderMissing, FailureHWCorruptOutput, FailureCancelled, FailureNone} {
		if c.RetryableInit() {
			t.Errorf("%s must NOT be retried", c)
		}
	}
}

func TestHWMode_RKMPP(t *testing.T) {
	m, ok := ParseHWMode(" RKMPP ")
	if !ok || m != HWModeRKMPP || m.Preference().Backend != "rkmpp" {
		t.Fatalf("rkmpp mode = %q ok=%v pref=%+v", m, ok, m.Preference())
	}
	if !RKMPP.IsValid() {
		t.Fatal("rkmpp is a valid accel type")
	}
	rk := HWBackendCap{Backend: "rkmpp", HWDeinterlace: true, Routes: []string{HWRouteDeinterlace}}
	if rk.DeinterlacesOnGPU() {
		t.Fatal("RGA has no deinterlacer: never session-level on RKMPP, whatever a row claims")
	}
}
