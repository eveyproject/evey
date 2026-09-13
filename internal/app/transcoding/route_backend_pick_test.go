package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// A prod-host-shaped host: QSV and VAAPI both certified. QSV is the cheaper SDR/
// HEVC primary; only VAAPI proves the OpenCL tone-map route (QSV vpp ineligible).
func twoBackendCap() transcoding.EncodeCapability {
	return transcoding.EncodeCapability{
		SoftwareMaxHeight: 1440, SoftwareHDRMaxHeight: 1440, HWMaxHeight: 2160,
		ProbeVersion: transcoding.CapabilityProbeVersion,
		// Primary scalars = QSV (cheapest SDR).
		HWBackend: "qsv", HWDevice: "/dev/dri/renderD128", HWCodecs: []string{"h264", "hevc"},
		HWDecodeCodecs: []string{"h264", "hevc", transcoding.HWDecodeHEVC10}, HW10BitTranscode: true, HWHDR10Encode: true,
		HWRoutes: []string{transcoding.HWRouteSDR, transcoding.HWRouteHEVC, transcoding.HWRouteSource10Bit, transcoding.HWRoutePassthrough, transcoding.HWRouteDecoupled},
		Backends: []transcoding.HWBackendCap{
			{Backend: "qsv", Device: "/dev/dri/renderD128", Codecs: []string{"h264", "hevc"},
				DecodeCodecs: []string{"h264", "hevc", transcoding.HWDecodeHEVC10}, HW10Bit: true, HWHDR10Encode: true,
				Routes: []string{transcoding.HWRouteSDR, transcoding.HWRouteHEVC, transcoding.HWRouteSource10Bit, transcoding.HWRoutePassthrough, transcoding.HWRouteDecoupled}},
			{Backend: "vaapi", Device: "/dev/dri/renderD128", Codecs: []string{"h264", "hevc"},
				DecodeCodecs: []string{"h264", "hevc", transcoding.HWDecodeHEVC10}, HW10Bit: true, HWHDR10Encode: true, HWTonemap: true,
				Features: []string{transcoding.FeatureTonemapOpenCL},
				Routes:   []string{transcoding.HWRouteSDR, transcoding.HWRouteHEVC, transcoding.HWRouteSource10Bit, transcoding.HWRouteTonemap, transcoding.HWRouteBridge, transcoding.HWRoutePassthrough, transcoding.HWRouteDecoupled}},
		},
		RouteSamples: []transcoding.RouteSample{
			// QSV cheaper on SDR/HEVC (measured on the UHD 630).
			{Key: rk("qsv", "h264_qsv", transcoding.RouteCostSDR), Height: 1080, FPS: 149, SourceFPS: 24},
			{Key: rk("qsv", "hevc_qsv", transcoding.RouteCostSDR), Height: 1080, FPS: 155, SourceFPS: 24},
			{Key: rk("vaapi", "h264_vaapi", transcoding.RouteCostSDR), Height: 1080, FPS: 137, SourceFPS: 24},
			{Key: rk("vaapi", "hevc_vaapi", transcoding.RouteCostSDR), Height: 1080, FPS: 99, SourceFPS: 24},
			// Only VAAPI has the OpenCL tone-map route.
			{Key: rk("vaapi", "h264_vaapi", transcoding.RouteCostTonemap), Height: 1080, FPS: 130, SourceFPS: 24, CPUCores: 0.5},
		},
	}
}

func twoBackendResolver() *PlanResolver {
	cap := twoBackendCap()
	return &PlanResolver{
		HWAccelEnabled:   func() bool { return true },
		HWTonemapEnabled: func() bool { return true },
		Capability:       func() transcoding.EncodeCapability { return cap },
		RouteCosts:       func() *RouteCostModel { return NewRouteCostModel(cap.RouteSamples, 7) },
	}
}

func TestBackendPerRoute_HDRVAAPI_HEVCandSDRQSV(t *testing.T) {
	pr := twoBackendResolver()

	// SDR h264: both prove it, QSV cheaper -> QSV.
	sdr := transcoding.PlaybackDecision{VideoDecision: "transcode", VideoCodec: "h264", TranscodeReasons: []string{"video_codec_unsupported"}}
	_, accel, _, _, usable, _ := pr.EffectiveVideoCodec(sdr, sdrStreams("h264", "yuv420p", false))
	assert.True(t, usable)
	assert.Equal(t, "qsv", accel, "SDR takes the cheaper QSV")

	// HEVC output: QSV cheaper -> QSV.
	hevc := transcoding.PlaybackDecision{VideoDecision: "transcode", VideoCodec: "hevc", TranscodeReasons: []string{"video_codec_unsupported"}}
	_, accel, _, _, usable, _ = pr.EffectiveVideoCodec(hevc, sdrStreams("h264", "yuv420p", false))
	assert.True(t, usable)
	assert.Equal(t, "qsv", accel, "HEVC output takes the cheaper QSV")

	// HDR->SDR tone-map: only VAAPI proves the GPU tone-map route -> VAAPI/OpenCL,
	// even though QSV is the primary. This is the acceptance: HDR regains VAAPI.
	tm := transcoding.PlaybackDecision{VideoDecision: "transcode", VideoCodec: "h264", TranscodeReasons: []string{"hdr_tonemapping_needed"}}
	assert.True(t, pr.HWTonemapWanted(tm, "PQ"), "a proven VAAPI OpenCL tone-map is wanted")
	_, accel, _, _, usable, _ = pr.EffectiveVideoCodec(tm, hdrStreams("hevc", "yuv420p10le", true))
	assert.True(t, usable, "HDR tone-map runs on the GPU")
	assert.Equal(t, "vaapi", accel, "HDR->SDR tone-map lands on VAAPI/OpenCL, not the QSV primary")
	assert.True(t, pr.HWRouteUsable(tm, SourceDimsOf(hdrStreams("hevc", "yuv420p10le", true))))
}

// SDR near-tie must be stable across recalibrations: two backends whose SDR
// cost is within 5% never flip - the primary (then alphabetical) wins
// deterministically, whatever tiny fps difference a re-bench measures.
func TestBackendPerRoute_SDRNearTieStable(t *testing.T) {
	base := twoBackendCap()
	// Make VAAPI marginally FASTER than QSV on SDR (within 5%): 149 vs 152.
	for i := range base.RouteSamples {
		if base.RouteSamples[i].Key == rk("vaapi", "h264_vaapi", transcoding.RouteCostSDR) {
			base.RouteSamples[i].FPS = 152
		}
	}
	pr := &PlanResolver{
		HWAccelEnabled: func() bool { return true }, HWTonemapEnabled: func() bool { return true },
		Capability: func() transcoding.EncodeCapability { return base },
		RouteCosts: func() *RouteCostModel { return NewRouteCostModel(base.RouteSamples, 7) },
	}
	sdr := transcoding.PlaybackDecision{VideoDecision: "transcode", VideoCodec: "h264", TranscodeReasons: []string{"video_codec_unsupported"}}
	_, accel, _, _, _, _ := pr.EffectiveVideoCodec(sdr, sdrStreams("h264", "yuv420p", false))
	assert.Equal(t, "qsv", accel, "within 5% the primary (QSV) holds - no flip to a marginally-faster VAAPI")
}
