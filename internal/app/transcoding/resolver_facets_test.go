package transcoding

// resolver_facets_test.go - the facet tests that lived in the HTTP adapter
// while it exposed sub-derivations (resolveHW, effectiveVideoCodec,
// hwRouteUsable, profilesForDecision): the backend pick, the codec gate, the
// route gate, the ladder shape. They read the facets off the plan
// (resolver_plan_test.go) and pin what they always pinned.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// ladderRequestFor is what the adapter's profilesForDecision marshalled: the
// item's id and the streams' bitrate (no file: the item carries none here).
func ladderRequestFor(item *library.MediaItem, streams []*library.MediaStream) PlaybackRequest {
	return PlaybackRequest{MediaID: string(item.ID), Streams: streams, SourceBitrateKbps: SourceVideoBitrateKbps(streams)}
}

// hdrCapTestCapability returns a software-only EncodeCapability for a weak
// host: software ceiling 1080p, HDR software ceiling 480p, no HW backend.
// This drives the soft-HDR cap path (hwUsable=false for all decisions).
func hdrCapTestCapability() transcoding.EncodeCapability {
	return transcoding.EncodeCapability{
		SoftwareMaxHeight:    1080,
		SoftwareHDRMaxHeight: 480,
		HWMaxHeight:          0, // no HW
	}
}

// hdrCapHWCapability returns a capability that enables the HW path:
// NVENC h264 + hevc-decode + 10-bit VPP + HW ceiling 2160p.
// Used for case (c): hwUsable=true → the SoftwareHDRMaxHeight cap is bypassed.
func hdrCapHWCapability() transcoding.EncodeCapability {
	return transcoding.EncodeCapability{
		SoftwareMaxHeight:    1080,
		SoftwareHDRMaxHeight: 480,
		HWMaxHeight:          2160,
		HWBackend:            "nvenc",
		HWCodecs:             []string{"h264"},
		HWDecodeCodecs:       []string{"hevc", "hevc10"},
		HW10BitTranscode:     true, // allows the 10-bit source through the HW gate
	}
}

// build4KHDRStreams returns a slice with one video stream: 3840×2160,
// HEVC Main 10 (10-bit), mimicking a real 4K HDR source. The 4K source
// dimension ensures CapLadderToSource doesn't pre-trim the ladder before
// the HDR cap has a chance to act.
func build4KHDRStreams() []*library.MediaStream {
	w, h, bitDepth := 3840, 2160, 10
	return []*library.MediaStream{
		{
			StreamType: library.StreamTypeVideo,
			Codec:      "hevc",
			Profile:    "Main 10",
			Width:      &w,
			Height:     &h,
			BitDepth:   &bitDepth,
		},
	}
}

// build4KSDRStreams returns a slice with one video stream: 3840×2160,
// H.264 8-bit - a 4K SDR source that does NOT trigger HDR tone-mapping.
func build4KSDRStreams() []*library.MediaStream {
	w, h := 3840, 2160
	return []*library.MediaStream{
		{
			StreamType: library.StreamTypeVideo,
			Codec:      "h264",
			Width:      &w,
			Height:     &h,
		},
	}
}

// buildHDRDecision returns a PlaybackDecision that carries the
// hdr_tonemapping_needed reason, which gates the HDR cap.
func buildHDRDecision() transcoding.PlaybackDecision {
	return transcoding.PlaybackDecision{
		TranscodeReasons: []string{"hdr_tonemapping_needed"},
	}
}

// buildSDRDecision returns a PlaybackDecision with no tonemap reason -
// represents a plain SDR transcode request.
func buildSDRDecision() transcoding.PlaybackDecision {
	return transcoding.PlaybackDecision{
		TranscodeReasons: []string{"container_not_supported"},
	}
}

// maxHeight returns the tallest profile height in the ladder.
func maxHeight(profiles []transcoding.HLSProfile) int {
	max := 0
	for _, p := range profiles {
		if p.Height > max {
			max = p.Height
		}
	}
	return max
}

// TestProfilesForDecision_HDRCap is the main table-test for the HDR ladder cap.
func TestProfilesForDecision_HDRCap(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		// Mutate pr before calling ShapeLadder.
		setup    func(pr *PlanResolver)
		streams  []*library.MediaStream
		decision transcoding.PlaybackDecision
		// Assertions on the returned ladder.
		assertFn func(t *testing.T, profiles []transcoding.HLSProfile)
	}{
		{
			// Case (a): software host + HDR source + hdr_tonemapping_needed reason.
			// The ladder MUST be capped to SoftwareHDRMaxHeight (480p).
			// No rung above 480p should be offered - the host cannot keep up
			// with 4K HEVC 10-bit decode + zscale tonemap in realtime.
			name: "software_HDR_capped_to_SoftwareHDRMaxHeight",
			setup: func(pr *PlanResolver) {
				cap := hdrCapTestCapability()
				pr.Capability = func() transcoding.HostCapabilitySnapshot { return (cap).Snapshot() }
				// hwAccelEnabled is nil → resolveHW returns usable=false immediately
			},
			streams:  build4KHDRStreams(),
			decision: buildHDRDecision(),
			assertFn: func(t *testing.T, profiles []transcoding.HLSProfile) {
				t.Helper()
				require.NotEmpty(t, profiles, "ladder must not be empty")
				assert.Equal(t, 480, maxHeight(profiles),
					"software HDR path must cap ladder to SoftwareHDRMaxHeight=480")
				for _, p := range profiles {
					assert.LessOrEqual(t, p.Height, 480,
						"no rung must exceed SoftwareHDRMaxHeight=480; got rung %s (%dp)", p.Name, p.Height)
				}
			},
		},
		{
			// Case (b): software host + SDR source (no hdr_tonemapping_needed).
			// The HDR cap must NOT apply - the ladder goes up to SoftwareMaxHeight
			// (1080p). Proves the cap is gated on the tonemap reason, not on the
			// fact that the host is software-only.
			name: "software_SDR_not_capped_to_HDR_ceiling",
			setup: func(pr *PlanResolver) {
				cap := hdrCapTestCapability()
				pr.Capability = func() transcoding.HostCapabilitySnapshot { return (cap).Snapshot() }
			},
			streams:  build4KSDRStreams(),
			decision: buildSDRDecision(),
			assertFn: func(t *testing.T, profiles []transcoding.HLSProfile) {
				t.Helper()
				require.NotEmpty(t, profiles, "ladder must not be empty")
				assert.Equal(t, 1080, maxHeight(profiles),
					"software SDR path must reach SoftwareMaxHeight=1080, not the HDR cap")
			},
		},
		{
			// Case (c): HW-capable host (hwUsable=true) + HDR source.
			// The GPU folds the 10-bit down cheaply, so the SoftwareHDRMaxHeight
			// cap (480p) must NOT apply. The effective ceiling is HWMaxHeight=2160
			// but since LadderFilmNormal (the default when no complexityRepo is
			// wired) only has rungs up to 1080p, the observable result is 1080p -
			// which is still proof that the 480p HDR cap was bypassed.
			//
			// Wiring: hwAccelEnabled=true, HWBackend=nvenc, HWCodecs=[h264],
			// HWDecodeCodecs=[hevc], HW10BitTranscode=true, HWMaxHeight=2160.
			// resolveHW returns usable=true because: flag on, cap.HWSupports("") →
			// treats "" as "h264" (in HWCodecs), source codec "hevc" is in
			// HWDecodeCodecs, and HW10BitTranscode=true clears the 10-bit gate.
			// With hwUsable=true the !hwUsable guard is false → HDR cap is skipped.
			name: "hw_HDR_without_GPU_tonemap_capped_by_SoftwareHDRMaxHeight",
			setup: func(pr *PlanResolver) {
				cap := hdrCapHWCapability()
				pr.Capability = func() transcoding.HostCapabilitySnapshot { return (cap).Snapshot() }
				pr.HWAccelEnabled = func() bool { return true }
				// hwTonemapEnabled is nil (nil → allowed) but hwTonemapWanted will
				// return false because HWBackend="nvenc" (not vaapi/qsv). That's
				// intentional: HW path is taken via the 10-bit gate (HW10BitTranscode),
				// not via tonemap. The key invariant is hwUsable=true, which alone
				// suppresses the SoftwareHDRMaxHeight cap.
			},
			streams:  build4KHDRStreams(),
			decision: buildHDRDecision(),
			assertFn: func(t *testing.T, profiles []transcoding.HLSProfile) {
				t.Helper()
				require.NotEmpty(t, profiles, "ladder must not be empty")
				// GPU decode + encode, CPU tone-map in between (no GPU tone-mapper
				// probed): the software HDR ceiling (480p here) still caps the ladder.
				assert.Equal(t, 480, maxHeight(profiles),
					"no GPU tone-mapper: the CPU tone-maps between GPU decode and encode, so the CPU (SoftwareHDRMaxHeight) stays the ceiling")
				for _, p := range profiles {
					assert.LessOrEqual(t, p.Height, 480, "rung %s exceeds the CPU tone-map ceiling", p.Name)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Build a minimal hlsVODServe. Only the fields consulted by
			// ShapeLadder (and its callees resolveHW / hwTonemapWanted)
			// need to be set. The rest remain zero/nil - ShapeLadder is
			// nil-safe for complexityRepo, capOverride, and capability.
			pr := &PlanResolver{}
			tc.setup(pr)

			item := &library.MediaItem{
				ID:        "m1",
				LibraryID: "lib1",
			}

			profiles := pr.planFor(context.Background(), ladderRequestFor(item, tc.streams), tc.decision).Ladder
			tc.assertFn(t, profiles)
		})
	}
}

// TestProfilesForDecision_SoftwareLadder locks the software ladder: the ABR
// ladder is the base rungs capped to the sustainable ceiling (keeping its 360p
// floor) so Auto can degrade DOWN and the viewer can pick a rung - never
// collapsed to one. The HW path keeps its ladder too.
func TestProfilesForDecision_SoftwareLadder(t *testing.T) {
	t.Parallel()
	item := &library.MediaItem{ID: "m1", LibraryID: "lib1"}
	swCap := func() transcoding.HostCapabilitySnapshot { return (hdrCapTestCapability()).Snapshot() } // software, 1080p SDR / 480p HDR

	// Software HDR → the ladder capped at the HDR ceiling (SoftwareHDRMaxHeight=480):
	// every rung obeys the cap, NOT a single rung the box can't keep up with.
	t.Run("software_HDR_ladder_capped_to_hdr_ceiling", func(t *testing.T) {
		t.Parallel()
		pr := &PlanResolver{Capability: swCap}
		profiles := pr.planFor(context.Background(), ladderRequestFor(item, build4KHDRStreams()), buildHDRDecision()).Ladder
		require.NotEmpty(t, profiles)
		for _, p := range profiles {
			assert.LessOrEqual(t, p.Height, 480, "every HDR rung obeys SoftwareHDRMaxHeight (the honest weak-NAS cap)")
		}
		assert.Equal(t, 480, profiles[len(profiles)-1].Height, "the HDR top rung is the HDR ceiling")
	})

	// Software SDR → the full ABR ladder (more than one rung).
	t.Run("software_SDR_keeps_full_ladder", func(t *testing.T) {
		t.Parallel()
		pr := &PlanResolver{Capability: swCap}
		profiles := pr.planFor(context.Background(), ladderRequestFor(item, build4KSDRStreams()), buildSDRDecision()).Ladder
		assert.Greater(t, len(profiles), 1, "the software path serves the full ABR ladder")
	})

	// HW path → ladder kept (a GPU parallelises rungs).
	t.Run("hw_keeps_ladder", func(t *testing.T) {
		t.Parallel()
		pr := &PlanResolver{
			Capability:     func() transcoding.HostCapabilitySnapshot { return (hdrCapHWCapability()).Snapshot() },
			HWAccelEnabled: func() bool { return true },
		}
		profiles := pr.planFor(context.Background(), ladderRequestFor(item, build4KHDRStreams()), buildHDRDecision()).Ladder
		assert.Greater(t, len(profiles), 1, "HW path keeps the ladder")
	})
}

func TestProfilesForDecision_AV1MaxHeightCap(t *testing.T) {
	box := transcoding.EncodeCapability{
		HWBackend: "nvenc", HWCodecs: []string{"av1"}, HWDecodeCodecs: []string{"h264"},
		HWMaxHeight: 2160, SoftwareMaxHeight: 2160,
	}
	w, hgt := 3840, 2160
	streams := []*library.MediaStream{{StreamType: "video", Codec: "h264", Width: &w, Height: &hgt}}
	dec := transcoding.PlaybackDecision{VideoCodec: "av1", VideoDecision: "transcode"}
	item := &library.MediaItem{ID: "mi-av1"}

	mk := func(maxH int) *PlanResolver {
		return &PlanResolver{
			Capability:     func() transcoding.HostCapabilitySnapshot { return (box).Snapshot() },
			HWAccelEnabled: func() bool { return true },
			AV1Enabled:     func() bool { return true },
			AV1MaxHeight:   func() int { return maxH },
		}
	}
	top := func(ps []transcoding.HLSProfile) int {
		m := 0
		for _, p := range ps {
			if p.Height > m {
				m = p.Height
			}
		}
		return m
	}

	uncapped := top(mk(0).planFor(context.Background(), ladderRequestFor(item, streams), dec).Ladder)
	if uncapped <= 720 {
		t.Fatalf("control: uncapped AV1 ladder should exceed 720p (got %dp) for the cap to be meaningful", uncapped)
	}
	capped := top(mk(720).planFor(context.Background(), ladderRequestFor(item, streams), dec).Ladder)
	if capped > 720 {
		t.Errorf("av1_max_height=720 must cap the AV1 ladder to 720p, got top rung %dp", capped)
	}
}

func TestResolveHW(t *testing.T) {
	nvenc := transcoding.EncodeCapability{
		HWBackend:  "nvenc",
		HWCodecs:   []string{"h264", "hevc", "av1"},
		HWDevice:   "",
		HWFeatures: []string{"temporal_aq", "rc_lookahead"},
	}
	vaapi := transcoding.EncodeCapability{
		HWBackend: "vaapi",
		HWCodecs:  []string{"h264", "hevc"},
		HWDevice:  "/dev/dri/renderD128",
	}

	t.Run("flag on, backend supports codec → HW", func(t *testing.T) {
		pr := &PlanResolver{
			Capability:     func() transcoding.HostCapabilitySnapshot { return (nvenc).Snapshot() },
			HWAccelEnabled: func() bool { return true },
		}
		accel, device, features, usable, _ := pr.resolveHW(pr.hwPolicy(), "h264", hwSource{Codec: "", TenBit: false}, false)
		if !usable || accel != "nvenc" || device != "" {
			t.Errorf("got (%q,%q,%v), want (nvenc,'',true)", accel, device, usable)
		}
		if len(features) != 2 || features[0] != "temporal_aq" {
			t.Errorf("probed features must be forwarded, got %v", features)
		}
	})

	t.Run("vaapi forwards device path", func(t *testing.T) {
		pr := &PlanResolver{
			Capability:     func() transcoding.HostCapabilitySnapshot { return (vaapi).Snapshot() },
			HWAccelEnabled: func() bool { return true },
		}
		accel, device, _, usable, _ := pr.resolveHW(pr.hwPolicy(), "hevc", hwSource{Codec: "", TenBit: false}, false)
		if !usable || accel != "vaapi" || device != "/dev/dri/renderD128" {
			t.Errorf("got (%q,%q,%v), want (vaapi,/dev/dri/renderD128,true)", accel, device, usable)
		}
	})

	t.Run("unsupported codec → software", func(t *testing.T) {
		pr := &PlanResolver{
			Capability:     func() transcoding.HostCapabilitySnapshot { return (vaapi).Snapshot() }, // no av1
			HWAccelEnabled: func() bool { return true },
		}
		if _, _, _, usable, _ := pr.resolveHW(pr.hwPolicy(), "av1", hwSource{Codec: "", TenBit: false}, false); usable {
			t.Error("av1 unsupported by vaapi caps → must be software")
		}
	})

	t.Run("source codec not HW-decodable → decoupled HW (old iGPU, HEVC)", func(t *testing.T) {
		// Encodes H.264 + HEVC but DECODES only H.264 - a pre-Skylake Intel iGPU.
		oldIGPU := transcoding.EncodeCapability{
			HWBackend:      "vaapi",
			HWCodecs:       []string{"h264", "hevc"},
			HWDecodeCodecs: []string{"h264"},
			HWDevice:       "/dev/dri/renderD128",
		}
		pr := &PlanResolver{
			Capability:     func() transcoding.HostCapabilitySnapshot { return (oldIGPU).Snapshot() },
			HWAccelEnabled: func() bool { return true },
		}
		if _, _, _, usable, _ := pr.resolveHW(pr.hwPolicy(), "h264", hwSource{Codec: "h264", TenBit: false}, false); !usable {
			t.Error("h264 source is HW-decodable → must take the HW path")
		}
		accel, _, _, usable, softwareDecode := pr.resolveHW(pr.hwPolicy(), "h264", hwSource{Codec: "hevc", TenBit: false}, false)
		if !usable || accel != "vaapi" || !softwareDecode {
			t.Errorf("hevc source not HW-decodable on this GPU → software decode + hwupload, GPU keeps the encode; got (%q,%v,%v)", accel, usable, softwareDecode)
		}
	})

	t.Run("flag off → software", func(t *testing.T) {
		pr := &PlanResolver{
			Capability:     func() transcoding.HostCapabilitySnapshot { return (nvenc).Snapshot() },
			HWAccelEnabled: func() bool { return false },
		}
		if _, _, _, usable, _ := pr.resolveHW(pr.hwPolicy(), "h264", hwSource{Codec: "", TenBit: false}, false); usable {
			t.Error("flag off → must be software")
		}
	})

	t.Run("nil flag func → software", func(t *testing.T) {
		pr := &PlanResolver{
			Capability: func() transcoding.HostCapabilitySnapshot { return (nvenc).Snapshot() },
		}
		if _, _, _, usable, _ := pr.resolveHW(pr.hwPolicy(), "h264", hwSource{Codec: "", TenBit: false}, false); usable {
			t.Error("nil hwAccelEnabled → must be software")
		}
	})

	t.Run("no calibrated backend → software", func(t *testing.T) {
		pr := &PlanResolver{
			Capability:     func() transcoding.HostCapabilitySnapshot { return (transcoding.SafeFallback()).Snapshot() },
			HWAccelEnabled: func() bool { return true },
		}
		if _, _, _, usable, _ := pr.resolveHW(pr.hwPolicy(), "h264", hwSource{Codec: "", TenBit: false}, false); usable {
			t.Error("no HW backend → must be software")
		}
	})

	t.Run("10-bit source + VPP can't do 10-bit → software (i965 VEBOX)", func(t *testing.T) {
		// Decodes hevc AND encodes h264, but the calibration found the VPP aborts
		// on a 10-bit (p010) surface - the Intel i965 case. A Main10 source
		// must software-transcode so it never crashes the GPU pipeline.
		box := transcoding.EncodeCapability{
			HWBackend:        "vaapi",
			HWCodecs:         []string{"h264"},
			HWDecodeCodecs:   []string{"h264", "hevc", "hevc10"},
			HW10BitTranscode: false,
			HWDevice:         "/dev/dri/renderD128",
		}
		pr := &PlanResolver{
			Capability:     func() transcoding.HostCapabilitySnapshot { return (box).Snapshot() },
			HWAccelEnabled: func() bool { return true },
		}
		if _, _, _, usable, _ := pr.resolveHW(pr.hwPolicy(), "h264", hwSource{Codec: "hevc", TenBit: false}, false); !usable {
			t.Error("8-bit hevc source → HW path (decode ok, no 10-bit VPP needed)")
		}
		if _, _, _, usable, _ := pr.resolveHW(pr.hwPolicy(), "h264", hwSource{Codec: "hevc", TenBit: true}, false); usable {
			t.Error("10-bit source + !HW10BitTranscode → must software-transcode (p010 aborts the VEBOX)")
		}
		// ...but when this request HW-tone-maps, tonemap_vaapi folds the 10-bit p010
		// itself, so the same 10-bit source CAN take the HW path without HW10BitTranscode.
		if _, _, _, usable, _ := pr.resolveHW(pr.hwPolicy(), "h264", hwSource{Codec: "hevc", TenBit: true}, true); !usable {
			t.Error("10-bit source + HW tonemap → HW path (tonemap_vaapi folds p010)")
		}
	})

	t.Run("10-bit source + VPP handles 10-bit → HW", func(t *testing.T) {
		box := transcoding.EncodeCapability{
			HWBackend:        "nvenc",
			HWCodecs:         []string{"h264", "hevc"},
			HWDecodeCodecs:   []string{"h264", "hevc", "hevc10"},
			HW10BitTranscode: true,
		}
		pr := &PlanResolver{
			Capability:     func() transcoding.HostCapabilitySnapshot { return (box).Snapshot() },
			HWAccelEnabled: func() bool { return true },
		}
		if _, _, _, usable, _ := pr.resolveHW(pr.hwPolicy(), "h264", hwSource{Codec: "hevc", TenBit: true}, false); !usable {
			t.Error("10-bit source + HW10BitTranscode → must take the HW path")
		}
	})
}

func TestEffectiveVideoCodec_AV1Gate(t *testing.T) {
	dec := func(codec string) transcoding.PlaybackDecision {
		return transcoding.PlaybackDecision{VideoCodec: codec}
	}

	t.Run("av1 + no HW + no AV1 sw ceiling falls back to h264", func(t *testing.T) {
		pr := &PlanResolver{
			Capability:     func() transcoding.HostCapabilitySnapshot { return (transcoding.SafeFallback()).Snapshot() },
			HWAccelEnabled: func() bool { return true },
		}
		got, _, _, _, _, _ := routeOf(pr.planFor(context.Background(), PlaybackRequest{Streams: nil}, dec("av1")))
		if got != "h264" {
			t.Errorf("av1 must fall back to h264 when not encodable here, got %q", got)
		}
	})

	t.Run("av1 + HW av1 + enabled stays av1", func(t *testing.T) {
		box := transcoding.EncodeCapability{HWBackend: "nvenc", HWCodecs: []string{"av1"}, HWMaxHeight: 2160}
		pr := &PlanResolver{
			Capability:     func() transcoding.HostCapabilitySnapshot { return (box).Snapshot() },
			HWAccelEnabled: func() bool { return true },
			AV1Enabled:     func() bool { return true },
		}
		got, _, _, _, usable, _ := routeOf(pr.planFor(context.Background(), PlaybackRequest{Streams: nil}, dec("av1")))
		if got != "av1" || !usable {
			t.Errorf("av1 with HW av1 + admin opt-in must stay av1 (usable), got %q usable=%v", got, usable)
		}
	})

	t.Run("av1 + HW av1 but admin disabled falls back to h264", func(t *testing.T) {
		// The opt-in gate: even when the GPU CAN encode AV1, an admin who hasn't
		// turned AV1 on (default) must keep streams on H.264.
		box := transcoding.EncodeCapability{HWBackend: "nvenc", HWCodecs: []string{"av1"}, HWMaxHeight: 2160}
		pr := &PlanResolver{
			Capability:     func() transcoding.HostCapabilitySnapshot { return (box).Snapshot() },
			HWAccelEnabled: func() bool { return true },
			AV1Enabled:     func() bool { return false },
		}
		got, _, _, _, _, _ := routeOf(pr.planFor(context.Background(), PlaybackRequest{Streams: nil}, dec("av1")))
		if got != "h264" {
			t.Errorf("av1 must fall back to h264 when the admin has not enabled it, got %q", got)
		}
	})

	t.Run("av1 + no HW falls back to h264 even with a calibrated AV1 sw ceiling", func(t *testing.T) {
		// AV1 is hardware-only for streaming: libsvtav1 can't sustain realtime
		// on the weak-NAS target (it starves → perpetual rebuffer), so a calibrated
		// software-AV1 ceiling must NOT keep a live session on software AV1.
		box := transcoding.EncodeCapability{SoftwareMaxHeight: 1080, SoftwareAV1MaxHeight: 720}
		pr := &PlanResolver{
			Capability:     func() transcoding.HostCapabilitySnapshot { return (box).Snapshot() },
			HWAccelEnabled: func() bool { return true },
		}
		got, _, _, _, _, _ := routeOf(pr.planFor(context.Background(), PlaybackRequest{Streams: nil}, dec("av1")))
		if got != "h264" {
			t.Errorf("av1 must fall back to h264 when the GPU can't encode it (HW-only policy), got %q", got)
		}
	})

	t.Run("h264 is unaffected", func(t *testing.T) {
		pr := &PlanResolver{
			Capability:     func() transcoding.HostCapabilitySnapshot { return (transcoding.SafeFallback()).Snapshot() },
			HWAccelEnabled: func() bool { return true },
		}
		got, _, _, _, _, _ := routeOf(pr.planFor(context.Background(), PlaybackRequest{Streams: nil}, dec("h264")))
		if got != "h264" {
			t.Errorf("h264 must be unchanged, got %q", got)
		}
	})
}

// The routing gate and the fingerprint tag must tell the same story: the route
// decides which physical resource (GPU vs CPU) the session burns, and the
// fingerprint is what admission prices. The AV1 fallback is where they used to
// diverge - an AV1-preferring client on an AV1-disabled host encodes H.264
// (fingerprint says HW when H.264 is HW-capable), so the ROUTE must be HW too.
func TestHWRouteUsable_MatchesEffectiveCodec_AV1Fallback(t *testing.T) {
	nvencH264 := transcoding.EncodeCapability{
		HWBackend:      "nvenc",
		HWCodecs:       []string{"h264", "hevc"}, // no AV1 encode
		HWDecodeCodecs: []string{"h264", "hevc", "hevc10"},
	}
	pr := &PlanResolver{
		Capability:     func() transcoding.HostCapabilitySnapshot { return (nvencH264).Snapshot() },
		HWAccelEnabled: func() bool { return true },
		AV1Enabled:     func() bool { return false },
	}
	decision := transcoding.PlaybackDecision{VideoCodec: "av1", VideoDecision: "transcode"}
	src := SourceDims{Codec: "h264"}

	_, accel, _, _, usable, _ := routeOf(pr.planFor(context.Background(), PlaybackRequest{Streams: []*library.MediaStream{{
		StreamType: library.StreamTypeVideo, Codec: "h264",
	}}}, decision))
	if !usable || accel != "nvenc" {
		t.Fatalf("fingerprint side: effective H.264 must be HW-usable, got (%q,%v)", accel, usable)
	}
	if !pr.routeUsable(decision, src) {
		t.Error("route side must agree: this session runs on the GPU")
	}
}

// A source that needs HDR tone-mapping on a backend that cannot tone-map on the
// GPU is encoded in SOFTWARE: the tonemap filter (tonemapx) runs on the CPU and
// the encoder follows it there. Routing such a request to the HARDWARE producer
// anyway made the ladder inherit the GPU ceiling - so the manifest advertised a
// 2160p rung for what is really a libx264 encode capped at software_hdr_max_height.
//
// Observed on a real 4K Dolby Vision title (dev box, NVENC, hw_tonemap=false):
// the master advertised 3840x2160, the player asked for it, ffmpeg started
// `-c:v libx264 scale=3840:-2 tonemapx`, and the segment 503'd after 5 s. The
// player then collapsed to the bottom 480p rung - which is exactly what the
// operator saw: slow to start, then stuck in 480p.
func TestHWRouteUsable_HDRWithoutHWTonemap_RoutesSoftware(t *testing.T) {
	// NVENC encodes fine, but has no modelled GPU tone-map (libplacebo path).
	// Mirrors the dev box's real calibration: NVENC encodes, decodes 10-bit HEVC,
	// but has NO GPU tone-map. This is what made the 4K rung look producible.
	nvencNoTonemap := transcoding.EncodeCapability{
		HWBackend:        "nvenc",
		HWCodecs:         []string{"h264", "hevc"},
		HWDecodeCodecs:   []string{"h264", "hevc", "hevc10"},
		HW10BitTranscode: true,
		HWTonemap:        false,
		HWMaxHeight:      2160,
	}
	pr := &PlanResolver{
		Capability:     func() transcoding.HostCapabilitySnapshot { return (nvencNoTonemap).Snapshot() },
		HWAccelEnabled: func() bool { return true },
		AV1Enabled:     func() bool { return false },
	}
	src := SourceDims{Codec: "hevc", TenBit: true}

	hdr := transcoding.PlaybackDecision{
		VideoCodec:       "h264",
		VideoDecision:    "transcode",
		TranscodeReasons: []string{"hdr_tonemapping_needed"},
	}
	if !pr.routeUsable(hdr, src) {
		t.Error("HDR source + no GPU tone-map: the CPU tone-maps between the GPU decode and encode (the ladder is capped by the software HDR ceiling in ShapeLadder)")
	}
	// The bridge needs the GPU to decode: a source it cannot decode stays on the CPU.
	if pr.routeUsable(hdr, SourceDims{Codec: "mpeg2video", TenBit: true}) {
		t.Error("HDR source the GPU cannot decode + no GPU tone-map must route SOFTWARE")
	}

	// Same host, an SDR source: nothing to tone-map, the GPU path stands.
	sdr := transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"}
	if !pr.routeUsable(sdr, SourceDims{Codec: "h264"}) {
		t.Error("an SDR source must still ride the GPU")
	}
}

func topHeight(ps []transcoding.HLSProfile) int {
	m := 0
	for _, p := range ps {
		if p.Height > m {
			m = p.Height
		}
	}
	return m
}

// The read-verbatim payoff (Étape 2): a stream's ABR ladder is shaped ONCE and
// frozen on its admission pin, so a mid-stream drift of a mutable shaping input
// (here the CMCD adaptive override, but equally a complexity re-class or a
// capability demotion) can NOT change the ladder a live player already has - the
// manifest's URLs and every segment stay addressed to the same cached bytes BY
// CONSTRUCTION, not by hoping every re-derivation agrees.
func TestProfilesForDecision_FrozenLadder_ImmuneToMidStreamDrift(t *testing.T) {
	w, hgt := 3840, 2160
	streams := []*library.MediaStream{{StreamType: "video", Codec: "h264", Width: &w, Height: &hgt}}
	dec := transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"}
	item := &library.MediaItem{ID: "mi-drift"}

	shaper := NewVideoAdmissionShaper(NewVideoSessionLimiter(0))

	var overrideH int // the mutable mid-stream input the test flips
	pr := &PlanResolver{
		Capability: func() transcoding.HostCapabilitySnapshot {
			return transcoding.EncodeCapability{SoftwareMaxHeight: 2160}.Snapshot()
		},
		AdmissionRoute:        shaper.GrantRoute,
		AdmissionPinnedClass:  shaper.PinnedClass,
		AdmissionFrozenLadder: shaper.FrozenLadder,
		AdmissionFreezeLadder: shaper.FreezeLadder,
		CapOverride: func(string, int) (transcoding.CapOverride, bool) {
			if overrideH > 0 {
				return transcoding.CapOverride{MaxHeight: overrideH}, true
			}
			return transcoding.CapOverride{}, false
		},
	}
	ctx := context.Background()

	// Manifest time: the override caps the ladder to 1080p. Shaped + FROZEN.
	overrideH = 1080
	manifestTop := topHeight(pr.planFor(ctx, ladderRequestFor(item, streams), dec).Ladder)
	require.Equal(t, 1080, manifestTop, "manifest ladder capped to the current override")

	// Mid-stream the override drops to 480p. A segment request for the SAME stream
	// must still shape the 1080p ladder - it reads the frozen plan, never the
	// drifted input.
	overrideH = 480
	segmentTop := topHeight(pr.planFor(ctx, ladderRequestFor(item, streams), dec).Ladder)
	assert.Equal(t, manifestTop, segmentTop, "frozen ladder is read verbatim, immune to the mid-stream drift")

	// Control: a DIFFERENT stream (its own fresh pin) DOES see the drifted 480p,
	// proving the override really bites and the 1080p above was the freeze at work,
	// not a no-op.
	controlTop := topHeight(pr.planFor(ctx, ladderRequestFor(&library.MediaItem{ID: "mi-other"}, streams), dec).Ladder)
	assert.Less(t, controlTop, manifestTop, "a fresh stream sees the current (drifted) override - the freeze is per-stream")
	assert.Equal(t, 480, controlTop)
}

// Without an admission shaper wired (hand-built handlers, the pre-Étape-2 path),
// ShapeLadder must keep re-deriving every call - the freeze is purely
// additive and never changes behaviour when it is not wired.
func TestProfilesForDecision_NoShaper_StillReDerivesEachCall(t *testing.T) {
	w, hgt := 3840, 2160
	streams := []*library.MediaStream{{StreamType: "video", Codec: "h264", Width: &w, Height: &hgt}}
	dec := transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"}
	item := &library.MediaItem{ID: "mi-noshaper"}

	var overrideH int
	pr := &PlanResolver{
		Capability: func() transcoding.HostCapabilitySnapshot {
			return transcoding.EncodeCapability{SoftwareMaxHeight: 2160}.Snapshot()
		},
		CapOverride: func(string, int) (transcoding.CapOverride, bool) {
			if overrideH > 0 {
				return transcoding.CapOverride{MaxHeight: overrideH}, true
			}
			return transcoding.CapOverride{}, false
		},
	}
	ctx := context.Background()

	overrideH = 1080
	first := topHeight(pr.planFor(ctx, ladderRequestFor(item, streams), dec).Ladder)
	overrideH = 480
	second := topHeight(pr.planFor(ctx, ladderRequestFor(item, streams), dec).Ladder)
	assert.Equal(t, 1080, first)
	assert.Equal(t, 480, second, "no freeze wired => each call re-derives against the current input")
}

// TestProfilesForDecision_HashInvariant locks the cache-hash invariant: the
// ladder ShapeLadder returns at manifest-time MUST be byte-identical to
// the one it returns at segment-time, so the DecisionFingerprint.Hash() agrees.
// Any drift = a permanent cache miss (every segment re-encoded). We assert the
// derived ProfileFingerprint slice hashes identically across two calls with the
// same inputs.
func TestProfilesForDecision_HashInvariant(t *testing.T) {
	t.Parallel()

	mkHandler := func() *PlanResolver {
		return &PlanResolver{Capability: func() transcoding.HostCapabilitySnapshot { return hdrCapTestCapability().Snapshot() }, HWAccelEnabled: func() bool { return false }} // software path, single-rendition
	}
	item := &library.MediaItem{ID: "m1", LibraryID: "lib1"}
	streams := build4KHDRStreams()
	decision := buildHDRDecision()

	// Two independent handlers (separate selector instances) with the SAME
	// inputs: simulate manifest-time and segment-time on different goroutines /
	// requests. The shaped ladders must hash identically.
	hManifest := mkHandler()
	hSegment := mkHandler()
	pManifest := hManifest.planFor(context.Background(), ladderRequestFor(item, streams), decision).Ladder
	pSegment := hSegment.planFor(context.Background(), ladderRequestFor(item, streams), decision).Ladder

	assert.Equal(t, fingerprintProfiles(pManifest), fingerprintProfiles(pSegment),
		"ShapeLadder must be deterministic across manifest- and segment-time (cache-hash invariant)")
	assert.Equal(t, hashOfProfiles(t, pManifest), hashOfProfiles(t, pSegment),
		"the DecisionFingerprint hash derived from the ladder must match across the two calls")
}

// fingerprintProfiles mirrors buildFingerprint's HLSProfile→ProfileFingerprint
// projection so the test compares exactly what enters the hash.
func fingerprintProfiles(ps []transcoding.HLSProfile) []ProfileFingerprint {
	out := make([]ProfileFingerprint, len(ps))
	for i, p := range ps {
		out[i] = ProfileFingerprint{
			Name:         p.Name,
			Width:        p.Width,
			Height:       p.Height,
			VideoBitrate: p.VideoBitrate,
			AudioBitrate: p.AudioBitrate,
		}
	}
	return out
}

// hashOfProfiles computes the DecisionFingerprint hash for a fixed media with
// the given ladder, isolating the ladder's contribution to the hash.
func hashOfProfiles(t *testing.T, ps []transcoding.HLSProfile) string {
	t.Helper()
	fp := DecisionFingerprint{
		MediaID:    "m1",
		SourcePath: "/m.mkv",
		VideoCodec: "h264",
		AudioCodec: "aac",
		Profiles:   fingerprintProfiles(ps),
	}
	return fp.Hash()
}
