package transcoding

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/adapters/driven/ffmpeg"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

// playback_plan_resolver_fallback_test.go - the fallback invariants of the
// 2026-09-12 audit: the resolver only takes a GPU route the calibration proved
// through the production builder, refuses sources the GPU decoder does not
// take, classifies a CPU burn-in as software, and spills a title whose GPU
// route just failed exactly like an admission spill.

func probedNVENC() transcoding.EncodeCapability {
	return transcoding.EncodeCapability{
		SoftwareMaxHeight: 1080, SoftwareHDRMaxHeight: 720, HWMaxHeight: 2160, HWBridgeMaxHeight: 2160,
		HWBackend: "nvenc", HWCodecs: []string{"h264", "hevc"},
		HWDecodeCodecs:   []string{"h264", "hevc", transcoding.HWDecodeHEVC10, "hevc:yuv444p"},
		HW10BitTranscode: true, HWHDR10Encode: true, HWOverlay: false,
		ProbeVersion: transcoding.CapabilityProbeVersion,
		HWRoutes: []string{transcoding.HWRouteSDR, transcoding.HWRouteHEVC, transcoding.HWRouteSource10Bit,
			transcoding.HWRouteBridge, transcoding.HWRoutePassthrough},
	}
}

func fallbackResolver(cap transcoding.EncodeCapability) *PlanResolver {
	return &PlanResolver{
		HWAccelEnabled:   func() bool { return true },
		HWTonemapEnabled: func() bool { return true },
		Capability:       func() transcoding.EncodeCapability { return cap },
		Expressibility:   ffmpeg.Expressibility{}, // the builder's port, the one rule set the pick refuses on
	}
}

func hdrStreams(codec, pixFmt string, tenBit bool) []*library.MediaStream {
	w, h, depth := 3840, 2160, 8
	if tenBit {
		depth = 10
	}
	profile := "Main"
	if tenBit && codec == "hevc" {
		profile = "Main 10"
	}
	if tenBit && codec == "h264" {
		profile = "High 10"
	}
	trc := "smpte2084"
	return []*library.MediaStream{{
		StreamType: "video", Codec: codec, Width: &w, Height: &h, BitDepth: &depth,
		PixelFormat: &pixFmt, Profile: profile, ColorTransfer: &trc,
	}}
}

func sdrStreams(codec, pixFmt string, tenBit bool) []*library.MediaStream {
	s := hdrStreams(codec, pixFmt, tenBit)
	s[0].ColorTransfer = nil
	return s
}

var sdrTranscode = transcoding.PlaybackDecision{VideoDecision: "transcode", VideoCodec: "h264", TranscodeReasons: []string{"video_codec_unsupported"}}

// A route the calibration did not prove through the builder is never taken,
// whatever the filter-level booleans say.
func TestResolver_UnprovenRoutesStaySoftware(t *testing.T) {
	tonemap := transcoding.PlaybackDecision{VideoDecision: "transcode", VideoCodec: "h264", TranscodeReasons: []string{"hdr_tonemapping_needed"}}
	cases := []struct {
		name   string
		drop   string
		stream []*library.MediaStream
		dec    transcoding.PlaybackDecision
	}{
		{"sdr route unproven refuses everything", transcoding.HWRouteSDR, sdrStreams("h264", "yuv420p", false), sdrTranscode},
		{"bridge unproven keeps HDR tone-map on the CPU", transcoding.HWRouteBridge, hdrStreams("hevc", "yuv420p10le", true), tonemap},
		{"10-bit source route unproven", transcoding.HWRouteSource10Bit, sdrStreams("hevc", "yuv420p10le", true), sdrTranscode},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			full := probedNVENC()
			assert.True(t, fallbackResolver(full).HWRouteUsable(tc.dec, SourceDimsOf(tc.stream)), "sanity: the fully proven host takes the route")
			partial := probedNVENC()
			partial.HWRoutes = nil
			for _, r := range full.HWRoutes {
				if r != tc.drop {
					partial.HWRoutes = append(partial.HWRoutes, r)
				}
			}
			pr := fallbackResolver(partial)
			assert.False(t, pr.HWRouteUsable(tc.dec, SourceDimsOf(tc.stream)), "route %s unproven must stay software", tc.drop)
			assert.NotEqual(t, LadderClassHW, pr.LadderClassFor(tc.dec, tc.stream), "the admission class follows the route gate")
			_, accel, _, _, usable, _ := pr.EffectiveVideoCodec(tc.dec, tc.stream)
			assert.False(t, usable, "fingerprint agrees with the route gate")
			assert.Empty(t, accel)
		})
	}
	// An unprobed row (nil HWRoutes) keeps yesterday's trust until it is re-benched.
	legacy := probedNVENC()
	legacy.HWRoutes = nil
	assert.True(t, fallbackResolver(legacy).HWRouteUsable(sdrTranscode, SourceDimsOf(sdrStreams("h264", "yuv420p", false))))
}

// Sources the GPU decoder does not take (10-bit H.264, 4:2:2, HEVC Main10 on a
// host without the 10-bit decode class) never reach the zero-copy chain.
func TestResolver_UndecodableSourcesNeverReachTheGPU(t *testing.T) {
	pr := fallbackResolver(probedNVENC())
	cases := []struct {
		name   string
		stream []*library.MediaStream
	}{
		{"Hi10P", sdrStreams("h264", "yuv420p10le", true)},
		{"HEVC 4:2:2 10-bit", sdrStreams("hevc", "yuv422p10le", true)},
		{"H.264 4:4:4", sdrStreams("h264", "yuv444p", false)},
	}
	// A class this host's probe DID measure rides the GPU once its zero-copy
	// route ran end-to-end: no universal chroma rule, and no assumption either.
	assert.False(t, pr.HWRouteUsable(sdrTranscode, SourceDimsOf(sdrStreams("hevc", "yuv444p", false))), "decodable but route unproven")
	proven := probedNVENC()
	proven.HWRoutes = append(proven.HWRoutes, transcoding.HWRouteSourceFormat("yuv444p"))
	assert.True(t, fallbackResolver(proven).HWRouteUsable(sdrTranscode, SourceDimsOf(sdrStreams("hevc", "yuv444p", false))))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.False(t, pr.HWRouteUsable(sdrTranscode, SourceDimsOf(tc.stream)))
			assert.Equal(t, LadderClassSoftware, pr.LadderClassFor(sdrTranscode, tc.stream))
		})
	}
	// Main10 on a probed host WITHOUT the 10-bit decode class: no zero-copy.
	noMain10 := probedNVENC()
	noMain10.HWDecodeCodecs = []string{"h264", "hevc"}
	assert.False(t, fallbackResolver(noMain10).HWRouteUsable(sdrTranscode, SourceDimsOf(sdrStreams("hevc", "yuv420p10le", true))))
	// The same list on a pre-class row still means "hevc covers Main10".
	legacy := noMain10
	legacy.ProbeVersion = 4
	assert.True(t, fallbackResolver(legacy).HWRouteUsable(sdrTranscode, SourceDimsOf(sdrStreams("hevc", "yuv420p10le", true))))
	// And the plain 4:2:0 classes still ride the GPU.
	assert.True(t, pr.HWRouteUsable(sdrTranscode, SourceDimsOf(sdrStreams("hevc", "yuv420p10le", true))))
	assert.True(t, pr.HWRouteUsable(sdrTranscode, SourceDimsOf(sdrStreams("h264", "yuv420p", false))))
}

// A bitmap burn-in the GPU cannot composite takes every backend out of the
// field (the builder's port refuses it): route, class and fingerprint say
// software, and the builder is never handed a plan it would have to swap.
func TestResolver_BurnInWithoutOverlayIsSoftware(t *testing.T) {
	burn := transcoding.PlaybackDecision{VideoDecision: "transcode", VideoCodec: "h264", SubtitleDecision: "burn", TranscodeReasons: []string{"subtitle_burn_in"}}
	streams := sdrStreams("h264", "yuv420p", false)

	noOverlay := fallbackResolver(probedNVENC())
	assert.False(t, noOverlay.HWRouteUsable(burn, SourceDimsOf(streams)))
	assert.Equal(t, LadderClassSoftware, noOverlay.LadderClassFor(burn, streams))
	_, accel, _, _, usable, _ := noOverlay.EffectiveVideoCodec(burn, streams)
	assert.False(t, usable)
	assert.Empty(t, accel)

	withOverlay := probedNVENC()
	withOverlay.HWOverlay = true
	pr := fallbackResolver(withOverlay)
	assert.True(t, pr.HWRouteUsable(burn, SourceDimsOf(streams)), "a probed GPU compositor keeps the burn-in on the GPU")
	assert.Equal(t, LadderClassHW, pr.LadderClassFor(burn, streams))
}

// A title whose GPU route just failed is served like an admission spill: CPU
// route, CPU class, CPU producer, a fresh pin, the delivery caps kept.
func TestResolver_BlockedTitleSpillsToSoftware(t *testing.T) {
	blocked := map[string]bool{"m-hdr": true}
	cap := probedNVENC()
	sw := &fakeProducer{}
	hw := &fakeProducer{}
	pr := &PlanResolver{
		HWAccelEnabled:   func() bool { return true },
		HWTonemapEnabled: func() bool { return true },
		Capability:       func() transcoding.EncodeCapability { return cap },
		HWRouteBlocked:   func(id string) bool { return blocked[id] },
		Selector:         NewProducerSelector(sw, hw, func(transcoding.PlaybackDecision, SourceDims) bool { return true }),
	}
	tonemap := transcoding.PlaybackDecision{VideoDecision: "transcode", VideoCodec: "h264", TranscodeReasons: []string{"hdr_tonemapping_needed"}}
	streams := hdrStreams("hevc", "yuv420p10le", true)
	req := func(id string) PlaybackRequest {
		return PlaybackRequest{MediaID: id, Streams: streams, AudioTrack: 0, SubtitleBurnInTrack: -1, ScreenHeight: 1080}
	}

	// The same request on a healthy title rides the GPU.
	fpHW := pr.Fingerprint(context.Background(), req("m-ok"), tonemap, pr.ShapeLadder(context.Background(), req("m-ok"), tonemap))
	require.Equal(t, "nvenc", fpHW.HWAccel)
	assert.Equal(t, LadderClassHW, LadderClassOf(fpHW))
	assert.Same(t, hw, pr.ProducerFor(req("m-ok"), tonemap, false))

	// The blocked title: software fingerprint, software producer, new pin.
	ladder := pr.ShapeLadder(context.Background(), req("m-hdr"), tonemap)
	fpSW := pr.Fingerprint(context.Background(), req("m-hdr"), tonemap, ladder)
	assert.Empty(t, fpSW.HWAccel)
	assert.True(t, fpSW.HDRTonemap, "the CPU session tone-maps")
	assert.Equal(t, LadderClassSoftwareHDR, LadderClassOf(fpSW))
	assert.Same(t, sw, pr.ProducerFor(req("m-hdr"), tonemap, false))
	assert.True(t, strings.HasSuffix(pr.PinKey(req("m-hdr"), tonemap), "|rb=1"))
	assert.False(t, strings.HasSuffix(pr.PinKey(req("m-ok"), tonemap), "|rb=1"))
	for _, p := range ladder {
		assert.LessOrEqual(t, p.Height, cap.SoftwareHDRMaxHeight, "the CPU ladder is capped at the CPU HDR ceiling, not the GPU's")
		assert.LessOrEqual(t, p.Height, 1080, "delivery caps survive the spill")
	}
	assert.NotEqual(t, fpHW.Hash(), fpSW.Hash())

	// Unblocked again: back on the GPU, same hash as before.
	blocked["m-hdr"] = false
	fpBack := pr.Fingerprint(context.Background(), req("m-hdr"), tonemap, pr.ShapeLadder(context.Background(), req("m-hdr"), tonemap))
	assert.Equal(t, "nvenc", fpBack.HWAccel)
}
