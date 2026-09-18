package transcoding

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/evey-project/evey/internal/adapters/driven/ffmpeg"
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
		HWDecodeCodecs:    []string{"h264", "hevc", "hevc10"},
		HWFeatures:        []string{"low_power"},
		HW10BitTranscode:  true,
		HWTonemap:         true,
	}
}

func hwResolver(cap transcoding.EncodeCapability) *PlanResolver {
	return &PlanResolver{
		HWAccelEnabled:   func() bool { return true },
		HWTonemapEnabled: func() bool { return true },
		Capability:       func() transcoding.HostCapabilitySnapshot { return (cap).Snapshot() },
		Expressibility:   ffmpeg.Expressibility{}, // the builder's port, the one rule set the pick refuses on
	}
}

func TestResolveHW_RefusalGates(t *testing.T) {
	oldIGPUNoHEVCDecode := vaapiCapable()
	oldIGPUNoHEVCDecode.HWDecodeCodecs = []string{"h264"}

	noTenBitVPP := vaapiCapable()
	noTenBitVPP.HW10BitTranscode = false

	h264OnlyEncoder := vaapiCapable()
	h264OnlyEncoder.HWCodecs = []string{"h264"}

	nvencNoHEVCDecode := vaapiCapable()
	nvencNoHEVCDecode.HWBackend = "nvenc"
	nvencNoHEVCDecode.HWDevice = ""
	nvencNoHEVCDecode.HWDecodeCodecs = []string{"h264"}

	cases := []struct {
		name               string
		resolver           *PlanResolver
		outputCodec        string
		sourceCodec        string
		sourceTenBit       bool
		hwTonemap          bool
		wantUsable         bool
		wantSoftwareDecode bool
		why                string
	}{
		{
			name: "operator toggle off", resolver: &PlanResolver{
				HWAccelEnabled: func() bool { return false },
				Capability:     func() transcoding.HostCapabilitySnapshot { return (vaapiCapable()).Snapshot() },
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
			name:     "backend cannot decode the source codec → decoupled, not software",
			resolver: hwResolver(oldIGPUNoHEVCDecode),
			// -hwaccel on such a source fails ENOSYS, so the decode moves to the
			// CPU - but the GPU still encodes. Losing the encoder too is what put
			// a library of DVD rips on libx264 next to an idle GPU.
			outputCodec: "h264", sourceCodec: "hevc",
			wantUsable: true, wantSoftwareDecode: true,
			why: "an undecodable source keeps the GPU for scale+encode via hwupload",
		},
		{
			name:     "undecodable source on a backend with no upload chain",
			resolver: hwResolver(nvencNoHEVCDecode),
			// NVENC has no wired hwupload chain (hwUploadFilter returns ""): the
			// builder's port refuses the decoupled topology on it, so the pick
			// leaves it out and the request stays on the CPU.
			outputCodec: "h264", sourceCodec: "hevc",
			why: "only VAAPI/QSV have a probe-validated upload chain",
		},
		{
			name:     "undecodable source that needs a GPU tone-map",
			resolver: hwResolver(oldIGPUNoHEVCDecode),
			// The upload folds to 8-bit nv12 on the CPU, which is exactly the
			// depth tonemap_vaapi still needs - the two cannot combine.
			outputCodec: "h264", sourceCodec: "hevc", hwTonemap: true,
			why: "a decoupled session cannot also tone-map on the GPU",
		},
		{
			name:     "undecodable 10-bit source",
			resolver: hwResolver(oldIGPUNoHEVCDecode),
			// The software decode is the expensive half at 4K 10-bit; routing it
			// to the GPU pool would price a CPU workload as a GPU one.
			outputCodec: "h264", sourceCodec: "hevc", sourceTenBit: true,
			why: "a 10-bit software decode must not be billed to the GPU pool",
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
			accel, device, features, usable, softwareDecode := tc.resolver.ResolveHW(
				tc.outputCodec, tc.sourceCodec, tc.sourceTenBit, tc.hwTonemap)

			assert.Equal(t, tc.wantUsable, usable, tc.why)
			assert.Equal(t, tc.wantSoftwareDecode, softwareDecode, tc.why)
			if !tc.wantUsable {
				assert.Empty(t, accel, "a refused route must not name a backend")
				assert.Empty(t, device, "a refused route must not name a device")
				assert.Empty(t, features, "a refused route must not carry encoder features")
				assert.False(t, softwareDecode, "a refused route must not flag a decoupled decode")
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
		assert.True(t, hwResolver(vaapiCapable()).HWTonemapWanted(needsTonemap, "PQ"))
	})

	t.Run("qsv also carries a wired GPU tone-mapper", func(t *testing.T) {
		qsv := vaapiCapable()
		qsv.HWBackend = "qsv"
		assert.True(t, hwResolver(qsv).HWTonemapWanted(needsTonemap, "PQ"))
	})

	t.Run("nvenc without tonemap_cuda tone-maps in software", func(t *testing.T) {
		// A stock ffmpeg has no CUDA tone-mapper: the probe flag alone (a pre-
		// CUDA row) never routes NVENC to a GPU tone-map it cannot emit.
		assert.False(t, hwResolver(nvenc).HWTonemapWanted(needsTonemap, "PQ"))
	})

	t.Run("nvenc with tonemap_cuda takes PQ, HLG and Dolby Vision", func(t *testing.T) {
		cuda := nvenc
		cuda.HWFeatures = append(slices.Clone(nvenc.HWFeatures), transcoding.FeatureTonemapCUDA)
		assert.True(t, hwResolver(cuda).HWTonemapWanted(needsTonemap, "PQ"))
		assert.True(t, hwResolver(cuda).HWTonemapWanted(needsTonemap, "HLG"))
		assert.True(t, hwResolver(cuda).HWTonemapWanted(needsTonemap, ""))
	})

	t.Run("probe never proved GPU tone-mapping", func(t *testing.T) {
		assert.False(t, hwResolver(noProbedTonemap).HWTonemapWanted(needsTonemap, "PQ"))
	})

	t.Run("admin toggle off", func(t *testing.T) {
		pr := hwResolver(vaapiCapable())
		pr.HWTonemapEnabled = func() bool { return false }
		assert.False(t, pr.HWTonemapWanted(needsTonemap, "PQ"))
	})

	t.Run("HLG and Dolby Vision stay on tonemapx", func(t *testing.T) {
		// tonemap_vaapi reads PQ only; a DoVi RPU needs apply_dovi (software).
		assert.False(t, hwResolver(vaapiCapable()).HWTonemapWanted(needsTonemap, "HLG"))
		assert.False(t, hwResolver(vaapiCapable()).HWTonemapWanted(needsTonemap, ""))
	})

	t.Run("a certified Vulkan/libplacebo tone-mapper takes HLG and Dolby Vision too", func(t *testing.T) {
		vk := vaapiCapable()
		vk.HWFeatures = append(slices.Clone(vk.HWFeatures), transcoding.FeatureTonemapVulkan)
		assert.True(t, hwResolver(vk).HWTonemapWanted(needsTonemap, "PQ"))
		assert.True(t, hwResolver(vk).HWTonemapWanted(needsTonemap, "HLG"))
		assert.True(t, hwResolver(vk).HWTonemapWanted(needsTonemap, ""))
	})

	t.Run("a probed OpenCL tone-mapper takes HLG and Dolby Vision too", func(t *testing.T) {
		ocl := vaapiCapable()
		ocl.HWFeatures = append(ocl.HWFeatures, transcoding.FeatureTonemapOpenCL)
		assert.True(t, hwResolver(ocl).HWTonemapWanted(needsTonemap, "HLG"))
		assert.True(t, hwResolver(ocl).HWTonemapWanted(needsTonemap, ""))
	})

	t.Run("nothing to tone-map", func(t *testing.T) {
		// The decision does not ask for it: a capable host must not tone-map SDR.
		assert.False(t, hwResolver(vaapiCapable()).HWTonemapWanted(transcoding.PlaybackDecision{}, "PQ"))
	})
}

// The route decides which physical pool the session burns. When it disagrees
// with what the fingerprint tags, admission prices one pool while the spawn
// consumes the other.
func TestHWRouteUsable_TelecineStaysSoftware(t *testing.T) {
	pr := hwResolver(vaapiCapable())
	decision := transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"}
	assert.True(t, pr.HWRouteUsable(decision, SourceDims{Codec: "mpeg2video"}))
	assert.False(t, pr.HWRouteUsable(decision, SourceDims{Codec: "mpeg2video", Telecine: true}), "no VAAPI inverse telecine")
}

func TestResolveHW_LowPowerIsOptIn(t *testing.T) {
	c := vaapiCapable()
	c.HWFeatures = []string{"low_power_h264", "bframes_h264"}
	pr := hwResolver(c)
	_, _, feats, usable, _ := pr.ResolveHW("h264", "h264", false, false)
	assert.True(t, usable)
	assert.Equal(t, []string{"bframes_h264"}, feats, "low power stays out until the admin opts in")
	pr.HWLowPowerEnabled = func() bool { return true }
	_, _, feats, _, _ = pr.ResolveHW("h264", "h264", false, false)
	assert.Equal(t, []string{"low_power_h264", "bframes_h264"}, feats)
}

func TestHWRouteUsable(t *testing.T) {
	tonemapNeeded := transcoding.PlaybackDecision{
		VideoCodec:       "h264",
		TranscodeReasons: []string{"hdr_tonemapping_needed"},
	}
	src := SourceDims{Codec: "hevc", TenBit: true, HDRRange: "PQ"}

	t.Run("no GPU tone-mapper: the CPU tone-maps between GPU decode and encode", func(t *testing.T) {
		noGPUTonemap := vaapiCapable()
		noGPUTonemap.HWTonemap = false
		pr := hwResolver(noGPUTonemap)
		assert.True(t, pr.HWRouteUsable(tonemapNeeded, src), "the GPU decodes and encodes, tonemapx runs in between")

		// ...unless the GPU cannot decode the source: then nothing is left for it.
		cpuDecode := noGPUTonemap
		cpuDecode.HWDecodeCodecs = []string{"h264"}
		assert.False(t, hwResolver(cpuDecode).HWRouteUsable(tonemapNeeded, src), "a software decode keeps the whole job on the CPU")
	})

	t.Run("GPU tonemap keeps the encode on the GPU", func(t *testing.T) {
		assert.True(t, hwResolver(vaapiCapable()).HWRouteUsable(tonemapNeeded, src))
		hlg := src
		hlg.HDRRange = "HLG"
		assert.True(t, hwResolver(vaapiCapable()).HWRouteUsable(tonemapNeeded, hlg), "HLG rides the CPU bridge between GPU decode and encode, never the VPP")
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

// ============================================================================
// Route/spawn coherence
// ============================================================================

// LadderClassFor is what ADMISSION prices; LadderClassOf(fingerprint) is what
// the SPAWN charges. They must agree for every request shape, or admission
// reserves one resource pool while the session burns another - the pool ends up
// oversubscribed with no counter showing it.
//
// The handler's own copy of this equivalence is locked by
// TestLadderClassFor_AgreesWithFingerprint in the http adapter; this pins the
// resolver's, which is the one production reaches through Resolve.
func TestLadderClassFor_AgreesWithTheFingerprintTheSameRequestBuilds(t *testing.T) {
	av1HW := vaapiCapable()
	av1HW.HWCodecs = []string{"h264", "av1"}

	cases := []struct {
		name     string
		resolver *PlanResolver
		decision transcoding.PlaybackDecision
		want     LadderClass
	}{
		{
			name:     "no hardware at all",
			resolver: &PlanResolver{},
			decision: transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"},
			want:     LadderClassSoftware,
		},
		{
			name:     "hardware encode",
			resolver: hwResolver(vaapiCapable()),
			decision: transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"},
			want:     LadderClassHW,
		},
		{
			name: "AV1 on a host that HW-encodes it",
			resolver: func() *PlanResolver {
				pr := hwResolver(av1HW)
				pr.AV1Enabled = func() bool { return true }
				return pr
			}(),
			decision: transcoding.PlaybackDecision{VideoCodec: "av1", VideoDecision: "transcode"},
			want:     LadderClassHW,
		},
		{
			name: "AV1 with no hardware falls back to H.264 software",
			// The AV1 gate rewrites the codec, so this must NOT be priced as an
			// AV1 software stream (a pool the box would never actually use).
			resolver: &PlanResolver{AV1Enabled: func() bool { return true }},
			decision: transcoding.PlaybackDecision{VideoCodec: "av1", VideoDecision: "transcode"},
			want:     LadderClassSoftware,
		},
		{
			name:     "software HDR tone-mapping",
			resolver: &PlanResolver{},
			decision: transcoding.PlaybackDecision{
				VideoCodec:       "h264",
				VideoDecision:    "transcode",
				TranscodeReasons: []string{"hdr_tonemapping_needed"},
			},
			want: LadderClassSoftwareHDR,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := ladderRequest(2160)
			priced := tc.resolver.LadderClassFor(tc.decision, req.Streams)
			assert.Equal(t, tc.want, priced, "the class admission prices")

			profiles := tc.resolver.ShapeLadder(context.Background(), req, tc.decision)
			fp := tc.resolver.Fingerprint(context.Background(), req, tc.decision, profiles)
			assert.Equal(t, LadderClassOf(fp), priced,
				"admission must price the pool the spawn will charge")
		})
	}
}
