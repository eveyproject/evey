package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

// auto_multibackend_gaps_test.go - the three multi-backend gaps of the Auto
// mode: source compatibility filtered BEFORE the cost ranking (the next
// backend is tried, never software), the HDR bridge per backend, and the cost
// model failing closed above the tallest proven rung.

// A prod-host-shaped box: QSV 2160 (primary, cheapest SDR) that decodes H.264
// and HEVC only, VAAPI 1080 that also decodes AV1. Both prove SDR and the
// decoupled route.
func mixedDecodeCap() transcoding.EncodeCapability {
	cap := twoBackendCap()
	cap.Backends[0].MaxHeight = 2160
	cap.Backends[1].MaxHeight = 1080
	cap.Backends[1].DecodeCodecs = append(cap.Backends[1].DecodeCodecs, "av1", transcoding.DecodeClassKey("av1", "yuv420p10le"))
	return cap
}

func resolverFor(cap transcoding.EncodeCapability) *PlanResolver {
	return &PlanResolver{
		HWAccelEnabled: func() bool { return true }, HWTonemapEnabled: func() bool { return true },
		Capability: func() transcoding.EncodeCapability { return cap },
		RouteCosts: func() *RouteCostModel { return NewRouteCostModel(cap.RouteSamples, 7) },
	}
}

func av1Streams(pixFmt string, tenBit bool) []*library.MediaStream {
	return sdrStreams("av1", pixFmt, tenBit)
}

// Gap 1: the cheaper QSV cannot decode an AV1 source; the pick used to land
// on it (cost), then the decode gate dropped the whole request to software.
// Compatibility is now filtered before ranking: VAAPI, which decodes AV1, is
// chosen - and at ITS 1080p ceiling, not the CPU.
func TestAutoGap_SourceCompatibilityBeforeCost_NextBackendNotSoftware(t *testing.T) {
	t.Parallel()
	cap := mixedDecodeCap()
	pr := resolverFor(cap)
	sdr := transcoding.PlaybackDecision{VideoDecision: "transcode", VideoCodec: "h264", TranscodeReasons: []string{"video_codec_unsupported"}}

	// Sanity: an H.264 source still goes to the cheaper 2160p QSV.
	_, accel, _, _, usable, _ := pr.EffectiveVideoCodec(sdr, sdrStreams("h264", "yuv420p", false))
	require.True(t, usable)
	assert.Equal(t, "qsv", accel)

	// AV1 8-bit: QSV cannot decode it but proves the decoupled route (CPU
	// decode, capped at the 1440p CPU ceiling) - a route its evidence never
	// produced a point for; VAAPI decodes it zero-copy with a 1080p sample.
	// Quality is what is PROVEN: VAAPI's 1080p beats QSV's unproven decoupled
	// route for the 2160p request. Either way the request stays on the GPU.
	_, accel, _, _, usable, softwareDecode := pr.EffectiveVideoCodec(sdr, av1Streams("yuv420p", false))
	require.True(t, usable, "a backend that serves the source exists: never software")
	assert.Equal(t, "vaapi", accel, "a proven 1080p beats an unproven decoupled route")
	assert.False(t, softwareDecode)
	// Once QSV's decoupled route has a 1440p sample it is the better proven
	// quality for 2160p; at 1080p both reach and the cheaper VAAPI wins.
	cap.RouteSamples = append(cap.RouteSamples, transcoding.RouteSample{Key: rk("qsv", "h264_qsv", transcoding.RouteCostDecoupled), Height: 1440, FPS: 100, SourceFPS: 24, CPUCores: 5})
	pr = resolverFor(cap)
	_, accel, _, _, usable, softwareDecode = pr.EffectiveVideoCodec(sdr, av1Streams("yuv420p", false))
	require.True(t, usable)
	assert.Equal(t, "qsv", accel, "QSV's decoupled route is proven to 1440p: the best proven quality for 2160p")
	assert.True(t, softwareDecode, "QSV serves it through the decoupled route")
	at1080 := sdr
	at1080.MaxHeight = 1080
	_, accel, _, _, usable, softwareDecode = pr.EffectiveVideoCodec(at1080, av1Streams("yuv420p", false))
	require.True(t, usable)
	assert.Equal(t, "vaapi", accel, "both reach 1080: the cheaper zero-copy VAAPI (0.21) beats the decoupled QSV (0.29)")
	assert.False(t, softwareDecode)
	assert.True(t, pr.HWRouteUsable(at1080, SourceDimsOf(av1Streams("yuv420p", false))))

	// AV1 10-bit: the decoupled path is refused for a 10-bit source, so QSV is
	// OUT of the field entirely; VAAPI (decodes AV1 10-bit, 10-bit VPP proven)
	// takes it instead of the request dropping to the CPU.
	_, accel, _, _, usable, softwareDecode = pr.EffectiveVideoCodec(sdr, av1Streams("yuv420p10le", true))
	require.True(t, usable, "QSV cannot serve this source; VAAPI can: the next backend is tried")
	assert.Equal(t, "vaapi", accel)
	assert.False(t, softwareDecode)

	// No backend serves the source at all: software, as before.
	cap.Backends[1].DecodeCodecs = []string{"h264", "hevc"}
	_, _, _, _, usable, _ = pr.EffectiveVideoCodec(sdr, av1Streams("yuv420p10le", true))
	assert.False(t, usable)
}

// Gap 2: the CPU tone-map bridge is proven and capped PER backend.
func TestAutoGap_BridgePerBackend(t *testing.T) {
	t.Parallel()
	bridgeDecision := transcoding.PlaybackDecision{VideoDecision: "transcode", VideoCodec: "h264", TranscodeReasons: []string{"hdr_tonemapping_needed"}}
	hdr := hdrStreams("hevc", "yuv420p10le", true)
	// Two NVENC-like backends: no GPU tone-mapper on either (HWTonemap false),
	// so HDR->SDR is the bridge on both. Distinct bridge ceilings.
	base := func() transcoding.EncodeCapability {
		routes := []string{transcoding.HWRouteSDR, transcoding.HWRouteHEVC, transcoding.HWRouteSource10Bit, transcoding.HWRouteBridge}
		return transcoding.EncodeCapability{
			SoftwareMaxHeight: 1080, SoftwareHDRMaxHeight: 720, HWMaxHeight: 2160, HWBridgeMaxHeight: 2160, HWBackend: "nvenc",
			ProbeVersion: transcoding.CapabilityProbeVersion,
			HWCodecs:     []string{"h264", "hevc"}, HWDecodeCodecs: []string{"h264", "hevc", transcoding.HWDecodeHEVC10}, HW10BitTranscode: true,
			HWRoutes: routes,
			Backends: []transcoding.HWBackendCap{
				{Backend: "nvenc", Codecs: []string{"h264", "hevc"}, DecodeCodecs: []string{"h264", "hevc", transcoding.HWDecodeHEVC10}, HW10Bit: true, Routes: routes, MaxHeight: 2160, BridgeMaxHeight: 2160},
				{Backend: "vaapi", Codecs: []string{"h264", "hevc"}, DecodeCodecs: []string{"h264", "hevc", transcoding.HWDecodeHEVC10}, HW10Bit: true, Routes: routes, MaxHeight: 2160, BridgeMaxHeight: 1080},
			},
			RouteSamples: []transcoding.RouteSample{
				{Key: rk("nvenc", "h264_nvenc", transcoding.RouteCostBridge), Height: 1080, FPS: 70, SourceFPS: 24, CPUCores: 2.7},
				{Key: rk("nvenc", "h264_nvenc", transcoding.RouteCostBridge), Height: 2160, FPS: 40, SourceFPS: 24, CPUCores: 3.5},
				{Key: rk("vaapi", "h264_vaapi", transcoding.RouteCostBridge), Height: 1080, FPS: 120, SourceFPS: 24, CPUCores: 2.5},
			},
		}
	}

	// Primary bridge OK / secondary bridge KO (route never proven on vaapi):
	// the bridge goes to the primary even though vaapi measures cheaper.
	cap := base()
	cap.Backends[1].Routes = []string{transcoding.HWRouteSDR, transcoding.HWRouteHEVC, transcoding.HWRouteSource10Bit}
	cap.Backends[1].BridgeMaxHeight = 0
	pr := resolverFor(cap)
	_, accel, _, _, usable, _ := pr.EffectiveVideoCodec(bridgeDecision, hdr)
	require.True(t, usable)
	assert.Equal(t, "nvenc", accel, "only the primary proves the bridge")
	assert.True(t, pr.HWRouteUsable(bridgeDecision, SourceDimsOf(hdr)))

	// Primary bridge KO / secondary bridge OK: the bridge goes to the
	// secondary - not refused on the primary's missing proof, not software.
	cap = base()
	cap.HWRoutes = []string{transcoding.HWRouteSDR, transcoding.HWRouteHEVC, transcoding.HWRouteSource10Bit}
	cap.Backends[0].Routes = cap.HWRoutes
	cap.Backends[0].BridgeMaxHeight, cap.HWBridgeMaxHeight = 0, 0
	pr = resolverFor(cap)
	_, accel, _, _, usable, _ = pr.EffectiveVideoCodec(bridgeDecision, hdr)
	require.True(t, usable, "the secondary proves the bridge: the request stays on the GPU")
	assert.Equal(t, "vaapi", accel)
	assert.True(t, pr.HWRouteUsable(bridgeDecision, SourceDimsOf(hdr)))

	// Neither proves it: software (the historical refusal).
	cap.Backends[1].Routes = cap.HWRoutes
	_, _, _, _, usable, _ = pr.EffectiveVideoCodec(bridgeDecision, hdr)
	assert.False(t, usable)
	assert.False(t, pr.HWRouteUsable(bridgeDecision, SourceDimsOf(hdr)))

	// Distinct bridge ceilings: the ladder of a bridge resolved on vaapi tops
	// at ITS 1080p, on nvenc at 2160p; a 2160p request prefers the backend
	// whose bridge reaches 2160 over the cheaper 1080p one.
	cap = base()
	assert.Equal(t, 2160, cap.HWBridgeCeilingFor("nvenc"))
	assert.Equal(t, 1080, cap.HWBridgeCeilingFor("vaapi"))
	assert.Equal(t, 0, cap.HWBridgeCeilingFor("qsv"), "an unknown backend has no bridge ceiling")
	src := transcoding.SourceFacts{Width: 3840, Height: 2160, Codec: "hevc", TenBit: true, PixelFormat: "yuv420p10le", HDRRange: "PQ"}
	top := func(l []transcoding.HLSProfile) int { return l[len(l)-1].Height }
	build := func(accel transcoding.HWAccelType) []transcoding.HLSProfile {
		return transcoding.StaticLadderBuilder{}.Build(transcoding.LadderInput{
			Source: src, Complexity: transcoding.ComplexityVector{Class: transcoding.ClassFilmHighMotion},
			Route:    transcoding.EncodeRoute{Codec: "h264", Accel: accel, HWUsable: true, Tonemap: true},
			Delivery: transcoding.DeliveryConstraints{Capability: cap},
		})
	}
	assert.Equal(t, 2160, top(build(transcoding.NVENC)))
	assert.Equal(t, 1080, top(build(transcoding.VAAPI)))
	pr = resolverFor(cap)
	_, accel, _, _, _, _ = pr.EffectiveVideoCodec(bridgeDecision, hdr)
	assert.Equal(t, "nvenc", accel, "a 2160p bridge request goes to the backend whose bridge reaches 2160")
	capped := bridgeDecision
	capped.MaxHeight = 1080
	_, accel, _, _, _, _ = pr.EffectiveVideoCodec(capped, hdr)
	assert.Equal(t, "vaapi", accel, "a 1080p bridge request takes the cheaper bridge that reaches 1080")

	// A secondary whose bridge ceiling was never recorded caps at the software
	// HDR ceiling (0 => SoftwareHDRMaxHeight), never at the primary's 2160.
	cap.Backends[1].BridgeMaxHeight = 0
	assert.Equal(t, 720, top(build(transcoding.VAAPI)))

	// WithoutBackend purges only the removed backend's bridge proof: dropping
	// the secondary keeps the primary's bridge; dropping the primary promotes
	// the secondary WITH its own bridge ceiling.
	cap = base()
	cap.BenchSamples = []transcoding.BenchRungSample{{Ladder: transcoding.LadderHWBridge, Height: 2160, FPS: 40, RequiredFPS: 28.8, Passing: true}}
	noVAAPI := cap.WithoutBackend("vaapi")
	assert.Equal(t, 2160, noVAAPI.HWBridgeMaxHeight)
	assert.Len(t, noVAAPI.BenchSamples, 1, "the primary's bridge ladder survives a secondary's removal")
	noNVENC := cap.WithoutBackend("nvenc")
	assert.Equal(t, "vaapi", noNVENC.HWBackend)
	assert.Equal(t, 1080, noNVENC.HWBridgeMaxHeight, "the promoted backend's OWN bridge ceiling")
	assert.Empty(t, noNVENC.BenchSamples, "the dead primary's bridge ladder is gone")
	rederived := transcoding.Derive(noNVENC.Report(), transcoding.DefaultDerivePolicy())
	assert.Equal(t, 1080, rederived.HWBridgeMaxHeight, "and survives a re-derive without hw-bridge samples")
}

// Gap 3: the cost model fails closed above the tallest proven rung. A 2160
// point the calibration tried and that failed leaves no sample; a 2160 request
// must not be priced as the 1080 rung. Downwards stays conservative (a 1080
// sample bounds a 720 request), and the shaper steps the stream down to the
// tallest rung that has proof instead of refusing it.
func TestAutoGap_FailClosedAboveTallestProvenRung(t *testing.T) {
	t.Parallel()
	key := rk("vaapi", "h264_vaapi", transcoding.RouteCostSDR)
	m := NewRouteCostModel([]transcoding.RouteSample{
		{Key: key, Height: 1080, FPS: 90, SourceFPS: 24, CPUCores: 0.3},
	}, 7)
	_, ok := m.Cost(key, 2160, 24)
	assert.False(t, ok, "above the tallest proven rung: no price")
	_, ok = m.MaxStreams(key, 2160, 24)
	assert.False(t, ok)
	c, ok := m.Cost(key, 720, 24)
	require.True(t, ok, "downwards the 1080 sample bounds a 720 request")
	assert.InDelta(t, 28.8/90, c.Accel, 1e-9)
	c1080, _ := m.Cost(key, 1080, 24)
	assert.Equal(t, c1080, c)

	l := NewVideoSessionLimiter(0).WithRouteModel(m)
	assert.False(t, l.CanAdmitRoute("v", LadderClassHW, 2160, transcoding.SourceClass{}, key, 24))
	assert.False(t, l.TryAcquireRoute("v", LadderClassHW, 2160, transcoding.SourceClass{}, key, 24))
	assert.Equal(t, 0, l.Active())

	// The admission shaper walks the offered rungs tallest-first and grants the
	// tallest PROVEN one: 1080, never 2160, never a refusal.
	s := NewVideoAdmissionShaper(l)
	g := s.GrantRoute("stream", RouteAdmission{Natural: LadderClassHW, Heights: []int{480, 720, 1080, 2160}, Route: key, SourceFPS: 24})
	require.True(t, g.OK)
	assert.Equal(t, 1080, g.Ceiling)
	assert.True(t, g.Forced, "the ladder is trimmed below the unproven 2160")

	// The backend pick prices at the wanted rung bounded by the backend's
	// ceiling, so a backend whose evidence stops under its ceiling is not
	// preferred on cost for a taller request - but is still eligible.
	cap := twoBackendCap()
	cap.Backends[0].MaxHeight, cap.Backends[1].MaxHeight = 2160, 2160
	cap.RouteSamples = append(cap.RouteSamples, transcoding.RouteSample{Key: rk("vaapi", "h264_vaapi", transcoding.RouteCostSDR), Height: 2160, FPS: 60, SourceFPS: 24})
	pr := resolverFor(cap)
	sdr := transcoding.PlaybackDecision{VideoDecision: "transcode", VideoCodec: "h264", TranscodeReasons: []string{"video_codec_unsupported"}}
	_, accel, _, _, _, _ := pr.EffectiveVideoCodec(sdr, sdrStreams("h264", "yuv420p", false))
	assert.Equal(t, "vaapi", accel, "QSV has no 2160 proof (its 2160 point failed): VAAPI's proven 2160 wins the 2160 request")
	capped := sdr
	capped.MaxHeight = 1080
	_, accel, _, _, _, _ = pr.EffectiveVideoCodec(capped, sdrStreams("h264", "yuv420p", false))
	assert.Equal(t, "qsv", accel, "at 1080 both are proven and QSV is cheaper")
}

// Quality is ranked on what THIS route actually proved, bounded by the
// backend's generic ceiling: a QSV certified 2160p whose HEVC route only
// produced 1080p is a 1080p HEVC backend, and a 2160p HEVC request prefers the
// VAAPI whose HEVC route produced 1440p. Downwards nothing changes: at 1080p
// both are proven and the cheaper one wins.
func TestAutoGap_QualityRankedOnTheRoutesProvenCeiling(t *testing.T) {
	t.Parallel()
	cap := twoBackendCap()
	cap.Backends[0].MaxHeight = 2160 // QSV generic ceiling
	cap.Backends[1].MaxHeight = 1440 // VAAPI generic ceiling
	// twoBackendCap: hevc_qsv 1080 @155 fps, hevc_vaapi 1080 @99 fps. Add the
	// VAAPI 1440 HEVC point; QSV's HEVC route stops at 1080 (its 1440/2160
	// points failed).
	cap.RouteSamples = append(cap.RouteSamples,
		transcoding.RouteSample{Key: rk("vaapi", "hevc_vaapi", transcoding.RouteCostSDR), Height: 1440, FPS: 70, SourceFPS: 24, CPUCores: 0.2},
	)
	pr := resolverFor(cap)
	hevc := transcoding.PlaybackDecision{VideoDecision: "transcode", VideoCodec: "hevc", TranscodeReasons: []string{"video_codec_unsupported"}}
	src := sdrStreams("h264", "yuv420p", false) // 2160p source

	_, accel, _, _, usable, _ := pr.EffectiveVideoCodec(hevc, src)
	require.True(t, usable)
	assert.Equal(t, "vaapi", accel, "2160p HEVC: VAAPI's proven 1440 beats QSV's proven 1080, whatever QSV's generic 2160 ceiling")

	at1440 := hevc
	at1440.MaxHeight = 1440
	_, accel, _, _, _, _ = pr.EffectiveVideoCodec(at1440, src)
	assert.Equal(t, "vaapi", accel, "1440p HEVC: only VAAPI proved it")

	at1080 := hevc
	at1080.MaxHeight = 1080
	_, accel, _, _, _, _ = pr.EffectiveVideoCodec(at1080, src)
	assert.Equal(t, "qsv", accel, "1080p HEVC: both proven, the cheaper QSV wins - unchanged downwards")
	at720 := hevc
	at720.MaxHeight = 720
	_, accel, _, _, _, _ = pr.EffectiveVideoCodec(at720, src)
	assert.Equal(t, "qsv", accel, "720p: the 1080 samples bound it, cheaper QSV")

	// H.264 output: both routes proved 1080 only (twoBackendCap) - equal proven
	// quality, QSV cheaper - the generic 2160 vs 1440 ceilings do not decide.
	h264 := transcoding.PlaybackDecision{VideoDecision: "transcode", VideoCodec: "h264", TranscodeReasons: []string{"video_codec_unsupported"}}
	_, accel, _, _, _, _ = pr.EffectiveVideoCodec(h264, src)
	assert.Equal(t, "qsv", accel)

	// Legacy: no route model at all - the generic ceiling ranks (QSV 2160).
	pr.RouteCosts = nil
	_, accel, _, _, _, _ = pr.EffectiveVideoCodec(hevc, src)
	assert.Equal(t, "qsv", accel, "without route evidence the generic ceiling is all there is")
}

// A bridge that runs but clears no rung at the margin (BridgeMaxHeight 0) is
// capped at the software HDR ceiling everywhere - ladder, route evidence and
// the backend pick - never treated as "unknown" and given a 1080/2160 ceiling.
func TestAutoGap_BridgeWithNoPassingRung_FallsToSoftwareHDRCeiling(t *testing.T) {
	t.Parallel()
	routes := []string{transcoding.HWRouteSDR, transcoding.HWRouteHEVC, transcoding.HWRouteSource10Bit, transcoding.HWRouteBridge}
	decode := []string{"h264", "hevc", transcoding.HWDecodeHEVC10}
	cap := transcoding.EncodeCapability{
		SoftwareMaxHeight: 1080, SoftwareHDRMaxHeight: 480, HWMaxHeight: 2160, HWBridgeMaxHeight: 0, HWBackend: "nvenc",
		ProbeVersion: transcoding.CapabilityProbeVersion,
		HWCodecs:     []string{"h264", "hevc"}, HWDecodeCodecs: decode, HW10BitTranscode: true, HWRoutes: routes,
		Backends: []transcoding.HWBackendCap{
			{Backend: "nvenc", Codecs: []string{"h264", "hevc"}, DecodeCodecs: decode, HW10Bit: true, Routes: routes, MaxHeight: 2160, BridgeMaxHeight: 0},
			{Backend: "vaapi", Codecs: []string{"h264", "hevc"}, DecodeCodecs: decode, HW10Bit: true, Routes: routes, MaxHeight: 2160, BridgeMaxHeight: 1080},
		},
		RouteSamples: []transcoding.RouteSample{
			// The primary's bridge produced its one point at the 480p fallback
			// ceiling (what route evidence measures for it), under realtime.
			{Key: rk("nvenc", "h264_nvenc", transcoding.RouteCostBridge), Height: 480, FPS: 20, SourceFPS: 24, CPUCores: 3.0},
			{Key: rk("vaapi", "h264_vaapi", transcoding.RouteCostBridge), Height: 1080, FPS: 60, SourceFPS: 24, CPUCores: 2.5},
			{Key: rk("nvenc", "h264_nvenc", transcoding.RouteCostSDR), Height: 2160, FPS: 90, SourceFPS: 24, CPUCores: 0.5},
		},
	}
	assert.Equal(t, 480, cap.EffectiveBridgeCeilingFor("nvenc"), "no passing bridge rung: the software HDR ceiling")
	assert.Equal(t, 1080, cap.EffectiveBridgeCeilingFor("vaapi"))
	assert.Equal(t, 2160, cap.HWCeilingFor("nvenc"), "the plain encode ceiling is untouched")

	// Ladder: a bridge on nvenc tops at 480, never 2160.
	src := transcoding.SourceFacts{Width: 3840, Height: 2160, Codec: "hevc", TenBit: true, PixelFormat: "yuv420p10le", HDRRange: "PQ"}
	top := func(l []transcoding.HLSProfile) int { return l[len(l)-1].Height }
	build := func(accel transcoding.HWAccelType) []transcoding.HLSProfile {
		return transcoding.StaticLadderBuilder{}.Build(transcoding.LadderInput{
			Source: src, Complexity: transcoding.ComplexityVector{Class: transcoding.ClassFilmHighMotion},
			Route:    transcoding.EncodeRoute{Codec: "h264", Accel: accel, HWUsable: true, Tonemap: true},
			Delivery: transcoding.DeliveryConstraints{Capability: cap},
		})
	}
	assert.Equal(t, 480, top(build(transcoding.NVENC)))
	assert.Equal(t, 1080, top(build(transcoding.VAAPI)))

	// Pick: the 2160p bridge request goes to the secondary (proven 1080),
	// never to the primary on the strength of its generic 2160 ceiling.
	pr := resolverFor(cap)
	bridge := transcoding.PlaybackDecision{VideoDecision: "transcode", VideoCodec: "h264", TranscodeReasons: []string{"hdr_tonemapping_needed"}}
	hdr := hdrStreams("hevc", "yuv420p10le", true)
	_, accel, _, _, usable, _ := pr.EffectiveVideoCodec(bridge, hdr)
	require.True(t, usable)
	assert.Equal(t, "vaapi", accel)
	// Even a 480p request prefers the backend whose bridge really passed.
	at480 := bridge
	at480.MaxHeight = 480
	_, accel, _, _, _, _ = pr.EffectiveVideoCodec(at480, hdr)
	assert.Equal(t, "vaapi", accel, "both reach 480: vaapi's bridge is cheaper (0.48 vs the 480p nvenc point at 1.44)")

	// Alone, the primary still serves the bridge - at 480p, one session at a
	// time priced on its own sub-realtime point (an accelerator route is never
	// clamped: 1.44 > 1 => refused by admission, the shaper spills to the CPU).
	cap.Backends = cap.Backends[:1]
	pr = resolverFor(cap)
	_, accel, _, _, usable, _ = pr.EffectiveVideoCodec(bridge, hdr)
	require.True(t, usable)
	assert.Equal(t, "nvenc", accel)
	assert.Equal(t, 480, top(build(transcoding.NVENC)))
}
