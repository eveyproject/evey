package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// Every gate in ResolveHW / HWRouteUsable was added after a real box broke on
// it: an ENOSYS decode stall, a SIGABRT on scale_vaapi, a 2160p rung advertised
// for what was really a libx264 encode. A gate that silently stops holding
// reopens the incident, so each one gets its own case here.

// vaapiCapable is a fully capable VAAPI host: encodes and decodes h264/hevc,
// 10-bit VPP proven, GPU tone-mapper proven.
func vaapiCapable() transcoding.EncodeCapability {
	return transcoding.EncodeCapability{
		SoftwareMaxHeight: 1080,
		HWMaxHeight:       2160,
		HWBackend:         "vaapi",
		HWDevice:          "/dev/dri/renderD128",
		HWCodecs:          []string{"h264", "hevc"},
		HWDecodeCodecs:    []string{"h264", "hevc"},
		HWFeatures:        []string{"low_power"},
		HW10BitTranscode:  true,
		HWTonemap:         true,
	}
}

func hwResolver(cap transcoding.EncodeCapability) *PlanResolver {
	return &PlanResolver{
		HWAccelEnabled:   func() bool { return true },
		HWTonemapEnabled: func() bool { return true },
		Capability:       func() transcoding.EncodeCapability { return cap },
	}
}

func TestResolveHW_RefusalGates(t *testing.T) {
	oldIGPUNoHEVCDecode := vaapiCapable()
	oldIGPUNoHEVCDecode.HWDecodeCodecs = []string{"h264"}

	noTenBitVPP := vaapiCapable()
	noTenBitVPP.HW10BitTranscode = false

	h264OnlyEncoder := vaapiCapable()
	h264OnlyEncoder.HWCodecs = []string{"h264"}

	cases := []struct {
		name         string
		resolver     *PlanResolver
		outputCodec  string
		sourceCodec  string
		sourceTenBit bool
		hwTonemap    bool
		wantUsable   bool
		why          string
	}{
		{
			name: "operator toggle off", resolver: &PlanResolver{
				HWAccelEnabled: func() bool { return false },
				Capability:     func() transcoding.EncodeCapability { return vaapiCapable() },
			},
			outputCodec: "h264", sourceCodec: "h264",
			why: "the admin switch must win over any calibration",
		},
		{
			name:        "no capability wired",
			resolver:    &PlanResolver{HWAccelEnabled: func() bool { return true }},
			outputCodec: "h264", sourceCodec: "h264",
			why: "an uncalibrated host must never be assumed HW-capable",
		},
		{
			name:     "backend cannot encode the output codec",
			resolver: hwResolver(h264OnlyEncoder),
			// An old QSV box encodes H.264 only: HEVC output must
			// software-encode instead of failing the spawn.
			outputCodec: "hevc", sourceCodec: "h264",
			why: "an unencodable output codec must not route to the GPU",
		},
		{
			name:     "backend cannot decode the source codec",
			resolver: hwResolver(oldIGPUNoHEVCDecode),
			// Pre-Skylake Intel: -hwaccel on an HEVC source fails ENOSYS and the
			// player stalls on rebuffer rather than erroring.
			outputCodec: "h264", sourceCodec: "hevc",
			why: "an undecodable source must fall back to full software, not stall",
		},
		{
			name:     "10-bit source without proven VPP",
			resolver: hwResolver(noTenBitVPP),
			// i965 VEBOX SIGABRT the moment a p010 surface reaches
			// scale_vaapi.
			outputCodec: "h264", sourceCodec: "hevc", sourceTenBit: true,
			why: "an unproven 10-bit VPP must not be handed a p010 surface",
		},
		{
			name:     "10-bit source without proven VPP but tone-mapping on GPU",
			resolver: hwResolver(noTenBitVPP),
			// The GPU tone-mapper folds p010 to nv12 itself, so the
			// scale_vaapi-on-p010 hazard never arises: the gate must NOT fire.
			outputCodec: "h264", sourceCodec: "hevc", sourceTenBit: true, hwTonemap: true,
			wantUsable: true,
			why:        "HW tone-mapping is the documented exception to the 10-bit gate",
		},
		{
			name:     "fully capable host",
			resolver: hwResolver(vaapiCapable()),
			// An unknown source codec ("") skips the decode gate by design: the
			// caller could not identify the source, and the encode gate already
			// held.
			outputCodec: "h264", sourceCodec: "", wantUsable: true,
			why: "nothing refuses this request",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			accel, device, features, usable := tc.resolver.ResolveHW(
				tc.outputCodec, tc.sourceCodec, tc.sourceTenBit, tc.hwTonemap)

			assert.Equal(t, tc.wantUsable, usable, tc.why)
			if !tc.wantUsable {
				assert.Empty(t, accel, "a refused route must not name a backend")
				assert.Empty(t, device, "a refused route must not name a device")
				assert.Empty(t, features, "a refused route must not carry encoder features")
				return
			}
			assert.Equal(t, "vaapi", accel)
			assert.Equal(t, "/dev/dri/renderD128", device)
			assert.Equal(t, []string{"low_power"}, features)
		})
	}
}

func TestHWTonemapWanted(t *testing.T) {
	needsTonemap := transcoding.PlaybackDecision{TranscodeReasons: []string{"hdr_tonemapping_needed"}}

	nvenc := vaapiCapable()
	nvenc.HWBackend = "nvenc"

	noProbedTonemap := vaapiCapable()
	noProbedTonemap.HWTonemap = false

	t.Run("wanted on a proven vaapi host", func(t *testing.T) {
		assert.True(t, hwResolver(vaapiCapable()).HWTonemapWanted(needsTonemap))
	})

	t.Run("qsv also carries a wired GPU tone-mapper", func(t *testing.T) {
		qsv := vaapiCapable()
		qsv.HWBackend = "qsv"
		assert.True(t, hwResolver(qsv).HWTonemapWanted(needsTonemap))
	})

	t.Run("nvenc tone-maps in software", func(t *testing.T) {
		// NVENC's libplacebo path is not modelled, so the backend has no wired
		// GPU tone-map filter even though the probe passed.
		assert.False(t, hwResolver(nvenc).HWTonemapWanted(needsTonemap))
	})

	t.Run("probe never proved GPU tone-mapping", func(t *testing.T) {
		assert.False(t, hwResolver(noProbedTonemap).HWTonemapWanted(needsTonemap))
	})

	t.Run("admin toggle off", func(t *testing.T) {
		pr := hwResolver(vaapiCapable())
		pr.HWTonemapEnabled = func() bool { return false }
		assert.False(t, pr.HWTonemapWanted(needsTonemap))
	})

	t.Run("nothing to tone-map", func(t *testing.T) {
		// The decision does not ask for it: a capable host must not tone-map SDR.
		assert.False(t, hwResolver(vaapiCapable()).HWTonemapWanted(transcoding.PlaybackDecision{}))
	})
}

// The route decides which physical pool the session burns. When it disagrees
// with what the fingerprint tags, admission prices one pool while the spawn
// consumes the other.
func TestHWRouteUsable(t *testing.T) {
	tonemapNeeded := transcoding.PlaybackDecision{
		VideoCodec:       "h264",
		TranscodeReasons: []string{"hdr_tonemapping_needed"},
	}
	src := SourceDims{Codec: "hevc", TenBit: true}

	t.Run("software tonemap keeps the encode on the CPU", func(t *testing.T) {
		// tonemapx runs on the CPU and the encoder follows it there. Routing to
		// the hardware producer anyway made the LADDER inherit the GPU ceiling:
		// the master advertised 2160p for a libx264 encode, the segment 503'd and
		// playback collapsed to the bottom rung.
		noGPUTonemap := vaapiCapable()
		noGPUTonemap.HWTonemap = false
		pr := hwResolver(noGPUTonemap)

		assert.False(t, pr.HWRouteUsable(tonemapNeeded, src),
			"the route must follow the silicon that actually runs the encode")
	})

	t.Run("GPU tonemap keeps the encode on the GPU", func(t *testing.T) {
		assert.True(t, hwResolver(vaapiCapable()).HWRouteUsable(tonemapNeeded, src))
	})

	t.Run("AV1-disabled host routes on the H.264 fallback", func(t *testing.T) {
		// An AV1-preferring client on an AV1-disabled host is ENCODED as H.264,
		// so the route must be resolved for H.264 - answering "software" here
		// would misprice admission against the CPU pool.
		pr := hwResolver(vaapiCapable()) // HWCodecs has no av1
		pr.AV1Enabled = func() bool { return false }

		assert.True(t, pr.HWRouteUsable(
			transcoding.PlaybackDecision{VideoCodec: "av1"},
			SourceDims{Codec: "h264"}),
			"the fallback codec is H.264 and this host HW-encodes H.264")
	})

	t.Run("AV1-disabled host with no usable HW at all stays software", func(t *testing.T) {
		pr := &PlanResolver{HWAccelEnabled: func() bool { return false }}
		assert.False(t, pr.HWRouteUsable(
			transcoding.PlaybackDecision{VideoCodec: "av1"},
			SourceDims{Codec: "h264"}))
	})
}
