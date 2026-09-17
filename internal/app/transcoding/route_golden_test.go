package transcoding

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

// The route golden file pins EVERYTHING a resolution decides about the route
// and how the layers below read it: the effective codec and backend, the
// billed class (as the request derives it AND as the fingerprint derives it),
// the route cost key, the producer the request goes to, every GPU runtime
// option the builder receives, the admission outcome, and the cache hash. It is
// the byte-stability contract of the "one plan, one decision" migration
// (ADR-0030): the derivations move into one place, these rows do not change.
//
// It complements ladder_golden.txt with the shapes that file does not cover:
// a GPU pool that is full (spill), a title whose GPU route is blocked, a
// bitmap burn-in with and without a GPU compositor, an interlaced source on a
// QSV that deinterlaces on the GPU, a source the GPU cannot decode (decoupled),
// the HDR passthrough, an explicit codec pin, wasm audio, an audio offset, a
// two-backend host with route evidence. Regenerate on a DELIBERATE change with
// `go test ./internal/app/transcoding -run TestRouteGolden -update`.

type routeScenario struct {
	name string
	// build returns the resolver and the request; it may pre-load admission.
	build func() (*PlanResolver, PlaybackRequest)
}

// routeGoldenRequest is the base request every scenario mutates.
func routeGoldenRequest(src goldenSource, cl goldenClient) PlaybackRequest {
	return PlaybackRequest{
		MediaID: "m1", SourcePath: "/golden.mkv", Container: "matroska",
		Streams: src.streams, Client: cl.client, Prefs: cl.prefs,
		AudioTrack: 0, SubtitleBurnInTrack: -1,
		MaxBitrateKbps: cl.capKb, SourceBytes: src.bytes, SourceBitrateKbps: src.kbps, ScreenHeight: cl.screen,
	}
}

// routeGoldenResolver is goldenResolver with a REAL admission shaper over a
// limiter priced by the capability's route evidence (or the table), so the
// spill and the pin are the production ones, not stubs.
func routeGoldenResolver(cap *transcoding.EncodeCapability, hw bool) (*PlanResolver, *VideoSessionLimiter) {
	pr := goldenResolver(cap, hw, transcoding.ClassFilmNormal, 0)
	pr.HWLowPowerEnabled = func() bool { return false }
	pr.AV1Enabled = func() bool { return false }
	limiter := NewVideoSessionLimiter(0).WithCostModel(NewVideoCostModel(devBoxSamples()))
	if cap != nil {
		pr.RouteCosts = func() *RouteCostModel { return NewRouteCostModel(cap.RouteSamples, 8) }
		limiter.WithRouteModel(NewRouteCostModel(cap.RouteSamples, 8))
	}
	shaper := NewVideoAdmissionShaper(limiter)
	pr.AdmissionRoute = shaper.GrantRoute
	pr.AdmissionPinnedClass = shaper.PinnedClass
	pr.AdmissionBind = shaper.Bind
	pr.AdmissionFrozenLadder = shaper.FrozenLadder
	pr.AdmissionFreezeLadder = shaper.FreezeLadder
	return pr, limiter
}

// qsvDeintCapability: a QSV that proved its GPU deinterlacer, decodes H.264/
// HEVC but not VP9 (a VP9 source takes the decoupled route), composites
// subtitles on the GPU, and has route evidence for every shape.
func qsvDeintCapability() transcoding.EncodeCapability {
	rs := func(enc string, route transcoding.RouteCostClass, fps, cores float64) transcoding.RouteSample {
		return transcoding.RouteSample{Key: rk("qsv", enc, route), Height: 1080, FPS: fps, SourceFPS: 24, CPUCores: cores}
	}
	return transcoding.EncodeCapability{
		SoftwareMaxHeight: 1080, SoftwareHDRMaxHeight: 720, HWMaxHeight: 2160, ProbeVersion: transcoding.CapabilityProbeVersion,
		HWBackend: "qsv", HWDevice: "/dev/dri/renderD128", HWCodecs: []string{"h264", "hevc"},
		HWDecodeCodecs: []string{"h264", "hevc", transcoding.HWDecodeHEVC10}, HW10BitTranscode: true, HWOverlay: true,
		HWRoutes: []string{transcoding.HWRouteSDR, transcoding.HWRouteHEVC, transcoding.HWRouteSource10Bit, transcoding.HWRouteDecoupled, transcoding.HWRouteDeinterlace, transcoding.HWRouteBridge},
		Backends: []transcoding.HWBackendCap{{
			Backend: "qsv", Device: "/dev/dri/renderD128", Codecs: []string{"h264", "hevc"},
			DecodeCodecs: []string{"h264", "hevc", transcoding.HWDecodeHEVC10}, HW10Bit: true, HWOverlay: true, HWDeinterlace: true,
			Routes:   []string{transcoding.HWRouteSDR, transcoding.HWRouteHEVC, transcoding.HWRouteSource10Bit, transcoding.HWRouteDecoupled, transcoding.HWRouteDeinterlace, transcoding.HWRouteBridge},
			Features: []string{"low_power"}, MaxHeight: 2160, BridgeMaxHeight: 1080,
		}},
		RouteSamples: []transcoding.RouteSample{
			rs("h264_qsv", transcoding.RouteCostSDR, 149, 0.4), rs("hevc_qsv", transcoding.RouteCostSDR, 155, 0.4),
			rs("h264_qsv", transcoding.RouteCostDecoupled, 90, 2.1), rs("hevc_qsv", transcoding.RouteCostDecoupled, 88, 2.1),
			rs("h264_qsv", transcoding.RouteCostBridge, 60, 3.0), rs("hevc_qsv", transcoding.RouteCostBridge, 58, 3.0),
			{Key: rk("", "libx264", transcoding.RouteCostSDR), Height: 1080, FPS: 130, SourceFPS: 24, CPUCores: 5.9},
			{Key: rk("", "libx264", transcoding.RouteCostSoftwareHDR), Height: 720, FPS: 40, SourceFPS: 24, CPUCores: 6.5},
			{Key: rk("", "libx265", transcoding.RouteCostSDR), Height: 1080, FPS: 45, SourceFPS: 24, CPUCores: 7.2},
		},
	}
}

// hdrPassCapability: an NVENC that encodes HDR10 (Main10) and proved the
// passthrough route, so an HDR client keeps its HDR.
func hdrPassCapability() transcoding.EncodeCapability {
	c := nvencCapability
	c.ProbeVersion = transcoding.CapabilityProbeVersion
	c.HWHDR10Encode = true
	c.HWDecodeCodecs = []string{"h264", "hevc", transcoding.HWDecodeHEVC10}
	c.HWRoutes = []string{transcoding.HWRouteSDR, transcoding.HWRouteHEVC, transcoding.HWRouteAV1, transcoding.HWRouteSource10Bit, transcoding.HWRoutePassthrough, transcoding.HWRouteBridge}
	c.Backends = []transcoding.HWBackendCap{{
		Backend: "nvenc", Codecs: []string{"h264", "hevc", "av1"},
		DecodeCodecs: []string{"h264", "hevc", transcoding.HWDecodeHEVC10}, HW10Bit: true, HWHDR10Encode: true,
		Routes: c.HWRoutes, MaxHeight: 2160, BridgeMaxHeight: 2160,
	}}
	return c
}

func routeGoldenSources() map[string]goldenSource {
	out := map[string]goldenSource{}
	for _, s := range goldenSources() {
		out[s.name] = s
	}
	// Extra sources the ladder golden has no use for.
	out["1080p-vp9-8m"] = goldenSource{"1080p-vp9-8m", []*library.MediaStream{goldenVideo("vp9", 1920, 1080, func(s *library.MediaStream) {
		s.Profile = "Profile 0"
		s.Level = nil
		s.PixelFormat = new("yuv420p")
	}), goldenAudio("opus", 2, true)}, 8000, 7e9}
	out["1080i-h264-interlaced"] = goldenSource{"1080i-h264-interlaced", []*library.MediaStream{goldenVideo("h264", 1920, 1080, func(s *library.MediaStream) {
		s.IdetInterlaced = new(true)
	}), goldenAudio("ac3", 6, true)}, 12000, 10e9}
	out["1080p-h264-pgs"] = goldenSource{"1080p-h264-pgs", []*library.MediaStream{goldenVideo("h264", 1920, 1080, nil), goldenAudio("ac3", 6, true),
		&library.MediaStream{StreamType: library.StreamTypeSubtitle, Codec: "hdmv_pgs_subtitle", IsForced: true, IsDefault: true}}, 8000, 7e9}
	return out
}

func routeGoldenScenarios() []routeScenario {
	srcs := routeGoldenSources()
	cls := goldenClients() // web-h264, web-hevc, hdr-tv, tv-level41, laptop-768, pick-720, remote-20m, profile-5m, cellular-3m, pref-hevc
	client := func(name string) goldenClient {
		for _, c := range cls {
			if c.name == name {
				return c
			}
		}
		panic("no golden client " + name)
	}
	// Capped variants: the bitrate cap forces a video TRANSCODE of a source the
	// client could otherwise copy - the only way to see the HDR passthrough
	// (an HDR client on a Main10 GPU) and the deinterlace/overlay options on an
	// H.264 source.
	hdrTV20 := client("hdr-tv")
	hdrTV20.name, hdrTV20.capKb = "hdr-tv-20m", 20000
	web4 := client("web-h264")
	web4.name, web4.capKb = "web-h264-4m", 4000
	webHEVC4 := client("web-hevc")
	webHEVC4.name, webHEVC4.capKb = "web-hevc-4m", 4000
	cls = append(cls, hdrTV20, web4, webHEVC4)
	vaapi := vaapiCapable()
	vaapi.HWHDR10Encode = true
	vaapi.HWFeatures = append(vaapi.HWFeatures, transcoding.FeatureTonemapOpenCL)
	two := twoBackendCap()
	qsv := qsvDeintCapability()
	pass := hdrPassCapability()
	soft := transcoding.EncodeCapability{SoftwareMaxHeight: 1080, SoftwareHDRMaxHeight: 480}

	var out []routeScenario
	// The matrix: every host x a handful of sources x two clients, plain.
	hosts := []struct {
		name string
		cap  *transcoding.EncodeCapability
		hw   bool
	}{
		{"software-1080", &soft, false}, {"vaapi-full", &vaapi, true}, {"two-backend", &two, true}, {"qsv-deint", &qsv, true}, {"nvenc-pass", &pass, true},
	}
	sourceNames := []string{"1080p-h264-8m", "2160p-hevc-sdr-15m", "2160p-hevc-hdr10-70m", "1080p-hi10p", "1080p-vp9-8m", "1080i-h264-interlaced", "1080p-h264-pgs", "1080p-h264-telecine"}
	for _, h := range hosts {
		for _, sn := range sourceNames {
			for _, cn := range []string{"web-h264", "web-hevc", "hdr-tv", "hdr-tv-20m", "web-h264-4m", "web-hevc-4m"} {
				out = append(out, routeScenario{fmt.Sprintf("%s | %s | %s | plain", h.name, sn, cn), func() (*PlanResolver, PlaybackRequest) {
					pr, _ := routeGoldenResolver(h.cap, h.hw)
					return pr, routeGoldenRequest(srcs[sn], client(cn))
				}})
			}
		}
	}
	// Burn-in of the forced PGS track: with a GPU compositor (qsv) and without (vaapi).
	for _, h := range []struct {
		name string
		cap  *transcoding.EncodeCapability
	}{{"qsv-deint", &qsv}, {"vaapi-full", &vaapi}, {"nvenc-pass", &pass}} {
		out = append(out, routeScenario{fmt.Sprintf("%s | 1080p-h264-pgs | web-h264 | burn-in", h.name), func() (*PlanResolver, PlaybackRequest) {
			pr, _ := routeGoldenResolver(h.cap, true)
			req := routeGoldenRequest(srcs["1080p-h264-pgs"], client("web-h264"))
			req.SubtitleBurnInTrack = 0 // subtitle-relative: the PGS track
			return pr, req
		}})
	}
	// GPU pool full: the request spills to the CPU route (and is pinned there).
	for _, h := range []struct {
		name string
		cap  *transcoding.EncodeCapability
	}{{"vaapi-full", &vaapi}, {"two-backend", &two}, {"qsv-deint", &qsv}} {
		for _, sn := range []string{"1080p-h264-8m", "2160p-hevc-hdr10-70m", "1080p-vp9-8m"} {
			out = append(out, routeScenario{fmt.Sprintf("%s | %s | web-hevc-4m | gpu-full", h.name, sn), func() (*PlanResolver, PlaybackRequest) {
				pr, limiter := routeGoldenResolver(h.cap, true)
				limiter.SetBudget(PoolGPU, 0.0001) // nothing hardware fits
				return pr, routeGoldenRequest(srcs[sn], client("web-hevc-4m"))
			}})
		}
	}
	// The HDR passthrough spilled to the CPU: the fingerprint tone-maps (no
	// GPU keeps the range), a corner the plain scenarios never reach.
	for _, sc := range []string{"gpu-full", "title-blocked"} {
		out = append(out, routeScenario{"nvenc-pass | 2160p-hevc-hdr10-70m | hdr-tv-20m | " + sc, func() (*PlanResolver, PlaybackRequest) {
			pr, limiter := routeGoldenResolver(&pass, true)
			if sc == "gpu-full" {
				limiter.SetBudget(PoolGPU, 0.0001)
			} else {
				pr.HWRouteBlocked = func(mediaID string) bool { return mediaID == "m1" }
			}
			return pr, routeGoldenRequest(srcs["2160p-hevc-hdr10-70m"], client("hdr-tv-20m"))
		}})
	}
	// Both pools full: refused - the plan still resolves (natural route), admission says no.
	out = append(out, routeScenario{"vaapi-full | 1080p-h264-8m | web-hevc-4m | all-full", func() (*PlanResolver, PlaybackRequest) {
		pr, limiter := routeGoldenResolver(&vaapi, true)
		limiter.SetBudget(PoolGPU, 0.0001)
		limiter.SetBudget(PoolCPU, 0.0001)
		return pr, routeGoldenRequest(srcs["1080p-h264-8m"], client("web-hevc-4m"))
	}})
	// Title blocked: the GPU route failed for this title, served from the CPU.
	for _, sn := range []string{"1080p-h264-8m", "2160p-hevc-hdr10-70m"} {
		out = append(out, routeScenario{fmt.Sprintf("vaapi-full | %s | web-hevc-4m | title-blocked", sn), func() (*PlanResolver, PlaybackRequest) {
			pr, _ := routeGoldenResolver(&vaapi, true)
			pr.HWRouteBlocked = func(mediaID string) bool { return mediaID == "m1" }
			return pr, routeGoldenRequest(srcs[sn], client("web-hevc-4m"))
		}})
	}
	// Codec pin (vc=), wasm audio, audio offset, AV1 opt-in, HW accel off.
	out = append(out, routeScenario{"two-backend | 1080p-h264-8m | web-hevc-4m | pin-h264", func() (*PlanResolver, PlaybackRequest) {
		pr, _ := routeGoldenResolver(&two, true)
		req := routeGoldenRequest(srcs["1080p-h264-8m"], client("web-hevc-4m"))
		req.PinnedVideoCodec = "h264"
		return pr, req
	}})
	out = append(out, routeScenario{"vaapi-full | 1080p-h264-8m | web-h264 | wasm-audio", func() (*PlanResolver, PlaybackRequest) {
		pr, _ := routeGoldenResolver(&vaapi, true)
		req := routeGoldenRequest(srcs["1080p-h264-8m"], client("web-h264"))
		req.WasmAudio = true
		return pr, req
	}})
	out = append(out, routeScenario{"vaapi-full | 1080p-h264-8m | web-h264 | audio-offset", func() (*PlanResolver, PlaybackRequest) {
		pr, _ := routeGoldenResolver(&vaapi, true)
		req := routeGoldenRequest(srcs["1080p-h264-8m"], client("web-h264"))
		req.AudioOffsetMs = 250
		return pr, req
	}})
	out = append(out, routeScenario{"nvenc-pass | 1080p-h264-8m | hdr-tv-20m | av1-enabled", func() (*PlanResolver, PlaybackRequest) {
		pr, _ := routeGoldenResolver(&pass, true)
		pr.AV1Enabled = func() bool { return true }
		req := routeGoldenRequest(srcs["1080p-h264-8m"], client("hdr-tv-20m"))
		req.MaxBitrateKbps = 4000
		req.Prefs.PreferredVideoCodec = "av1"
		return pr, req
	}})
	out = append(out, routeScenario{"vaapi-full | 2160p-hevc-hdr10-70m | web-hevc | hw-off", func() (*PlanResolver, PlaybackRequest) {
		pr, _ := routeGoldenResolver(&vaapi, false)
		return pr, routeGoldenRequest(srcs["2160p-hevc-hdr10-70m"], client("web-hevc"))
	}})
	out = append(out, routeScenario{"qsv-deint | 1080p-h264-8m | web-hevc-4m | low-power", func() (*PlanResolver, PlaybackRequest) {
		pr, _ := routeGoldenResolver(&qsv, true)
		pr.HWLowPowerEnabled = func() bool { return true }
		return pr, routeGoldenRequest(srcs["1080p-h264-8m"], client("web-hevc-4m"))
	}})
	return out
}

// producerKind names the producer a plan goes to, without touching it.
func producerKind(pr *PlanResolver, req PlaybackRequest, plan PlaybackPlan) string {
	p := pr.ProducerFor(req, plan.Decision, plan.Packager == PackagerCopyFFmpeg)
	switch p {
	case nil:
		return "none"
	case pr.CopyProducer:
		return "copy"
	}
	if pr.Selector != nil {
		if p == pr.Selector.Software() {
			return "software"
		}
		return "hardware"
	}
	return "orchestrator"
}

func routeGoldenRow(sc routeScenario) string {
	pr, req := sc.build()
	plan, err := pr.Resolve(context.Background(), req)
	if err != nil {
		return "ERR " + err.Error()
	}
	fp := plan.Fingerprint
	d := plan.Decision
	var heights []string
	for _, p := range plan.Ladder {
		h := fmt.Sprint(p.Height)
		if p.Copy {
			h += "c"
		}
		heights = append(heights, h)
	}
	classFor := pr.LadderClassFor(d, req.Streams)
	classOf := LadderClassOf(fp)
	pinned := "none"
	if pr.AdmissionPinnedClass != nil {
		if cls, ok := pr.AdmissionPinnedClass(pr.PinKey(req, d)); ok {
			pinned = string(cls)
		}
	}
	return fmt.Sprintf("%s/%s/%s | codec=%s hw=%s dev=%s feat=%s | class=%s for=%s of=%s pin=%s | key=%s/%s/%s | usable=%t producer=%s | tm=%t hwtm=%t pass=%t swdec=%t ovl=%t deint=%t xfer=%s | atrack=%d acopy=%t sub=%d sidecar=%t | %s | %s",
		d.PlayMethod, d.VideoDecision, strings.Join(d.TranscodeReasons, ","),
		plan.VideoCodec, plan.HWAccel, fp.HWDevice, strings.Join(fp.HWFeatures, "+"),
		plan.Class, classFor, classOf, pinned,
		RouteCostKeyOf(fp).Backend, RouteCostKeyOf(fp).Encoder, RouteCostKeyOf(fp).Route,
		pr.HWRouteUsable(d, SourceDimsOf(req.Streams)), producerKind(pr, req, plan),
		fp.HDRTonemap, fp.HWTonemap, fp.HDRPassthrough, fp.HWSoftwareDecode, fp.HWOverlay, fp.HWDeinterlace, fp.HDRTransfer,
		fp.AudioTrack, fp.AudioCopy, fp.SubtitleBurnInTrack, fp.ForceSidecarSubs,
		strings.Join(heights, ","), plan.FingerprintHash)
}

func TestRouteGolden(t *testing.T) {
	var out []string
	for _, sc := range routeGoldenScenarios() {
		out = append(out, sc.name+" | "+routeGoldenRow(sc))
	}
	sort.Strings(out)
	got := strings.Join(out, "\n") + "\n"

	path := filepath.Join("testdata", "route_golden.txt")
	if *updateGolden {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
		t.Logf("golden rewritten: %d rows", len(out))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "missing golden file: run with -update once")
	if string(want) != got {
		wantLines := strings.Split(string(want), "\n")
		gotLines := strings.Split(got, "\n")
		var diff []string
		for i := range max(len(wantLines), len(gotLines)) {
			w, g := "", ""
			if i < len(wantLines) {
				w = wantLines[i]
			}
			if i < len(gotLines) {
				g = gotLines[i]
			}
			if w != g {
				diff = append(diff, "- "+w, "+ "+g)
			}
		}
		t.Fatalf("route golden drifted (%d lines differ) - a route derivation changed. If deliberate, re-run with -update and read the diff:\n%s", len(diff)/2, strings.Join(diff, "\n"))
	}
}

// Invariant 2 of ADR-0030: the fingerprint is a projection of the route. For
// every golden scenario the class and the route key read off the fingerprint
// equal the route's own, and the plan's summary fields are the route's.
func TestResolvedRoute_ProjectionsAgree(t *testing.T) {
	for _, sc := range routeGoldenScenarios() {
		pr, req := sc.build()
		plan, err := pr.Resolve(context.Background(), req)
		require.NoError(t, err, sc.name)
		fp := plan.Fingerprint
		require.Equal(t, plan.Route.Class(), LadderClassOf(fp), "%s: class", sc.name)
		require.Equal(t, plan.Route.CostKey(), RouteCostKeyOf(fp), "%s: cost key", sc.name)
		require.Equal(t, plan.Route.Class(), plan.Class, "%s: plan class", sc.name)
		require.Equal(t, plan.Route.Codec, plan.VideoCodec, "%s: plan codec", sc.name)
		require.Equal(t, plan.Route.Backend.Name, plan.HWAccel, "%s: plan accel", sc.name)
		require.True(t, plan.Route.Topology.Legal(), "%s: topology %s", sc.name, plan.Route.Topology)
		require.Equal(t, plan.Route.Copy, plan.Packager == PackagerCopyFFmpeg, "%s: copy", sc.name)
		// A spilled route is a software route with its reason; an unspilled
		// HW route has a backend.
		if plan.Route.Spilled() {
			require.False(t, plan.Route.HW(), "%s: a spilled route encodes on the CPU", sc.name)
		}
		if plan.Route.HW() {
			require.NotEmpty(t, plan.Route.Backend.Name, "%s: a GPU route names its backend", sc.name)
		}
	}
}
