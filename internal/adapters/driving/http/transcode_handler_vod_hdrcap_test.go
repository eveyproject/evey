package httpapi

// Tests for the HDR-aware ABR ladder cap in profilesForDecision.
//
// The cap is the safety valve that keeps a weak host real-time on 4K HDR:
// when software tone-mapping is required (hwUsable=false) and the content
// carries the "hdr_tonemapping_needed" reason, the ladder is capped to
// EncodeCapability.SoftwareHDRMaxHeight, which is lower than the normal
// SoftwareMaxHeight.  These tests lock in that behaviour so a refactor
// can't silently remove or invert the gate.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// hdrCapTestCapability returns a software-only EncodeCapability for a weak
// host: software ceiling 1080p, HDR software ceiling 480p, no HW backend.
// This drives the soft-HDR cap path (hwUsable=false for all decisions).
func hdrCapTestCapability() domaintranscoding.EncodeCapability {
	return domaintranscoding.EncodeCapability{
		SoftwareMaxHeight:    1080,
		SoftwareHDRMaxHeight: 480,
		HWMaxHeight:          0, // no HW
	}
}

// hdrCapHWCapability returns a capability that enables the HW path:
// NVENC h264 + hevc-decode + 10-bit VPP + HW ceiling 2160p.
// Used for case (c): hwUsable=true → the SoftwareHDRMaxHeight cap is bypassed.
func hdrCapHWCapability() domaintranscoding.EncodeCapability {
	return domaintranscoding.EncodeCapability{
		SoftwareMaxHeight:    1080,
		SoftwareHDRMaxHeight: 480,
		HWMaxHeight:          2160,
		HWBackend:            "nvenc",
		HWCodecs:             []string{"h264"},
		HWDecodeCodecs:       []string{"hevc"},
		HW10BitTranscode:     true, // allows the 10-bit source through the HW gate
	}
}

// build4KHDRStreams returns a slice with one video stream: 3840×2160,
// HEVC Main 10 (10-bit), mimicking a real 4K HDR source. The 4K source
// dimension ensures CapLadderToSource doesn't pre-trim the ladder before
// the HDR cap has a chance to act.
func build4KHDRStreams() []*domainlibrary.MediaStream {
	w, h, bitDepth := 3840, 2160, 10
	return []*domainlibrary.MediaStream{
		{
			StreamType: domainlibrary.StreamTypeVideo,
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
func build4KSDRStreams() []*domainlibrary.MediaStream {
	w, h := 3840, 2160
	return []*domainlibrary.MediaStream{
		{
			StreamType: domainlibrary.StreamTypeVideo,
			Codec:      "h264",
			Width:      &w,
			Height:     &h,
		},
	}
}

// buildHDRDecision returns a PlaybackDecision that carries the
// hdr_tonemapping_needed reason, which gates the HDR cap.
func buildHDRDecision() domaintranscoding.PlaybackDecision {
	return domaintranscoding.PlaybackDecision{
		TranscodeReasons: []string{"hdr_tonemapping_needed"},
	}
}

// buildSDRDecision returns a PlaybackDecision with no tonemap reason -
// represents a plain SDR transcode request.
func buildSDRDecision() domaintranscoding.PlaybackDecision {
	return domaintranscoding.PlaybackDecision{
		TranscodeReasons: []string{"container_not_supported"},
	}
}

// maxHeight returns the tallest profile height in the ladder.
func maxHeight(profiles []domaintranscoding.HLSProfile) int {
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
		// Mutate h before calling profilesForDecision.
		setup    func(h *hlsVODServe)
		streams  []*domainlibrary.MediaStream
		decision domaintranscoding.PlaybackDecision
		// Assertions on the returned ladder.
		assertFn func(t *testing.T, profiles []domaintranscoding.HLSProfile)
	}{
		{
			// Case (a): software host + HDR source + hdr_tonemapping_needed reason.
			// The ladder MUST be capped to SoftwareHDRMaxHeight (480p).
			// No rung above 480p should be offered - the host cannot keep up
			// with 4K HEVC 10-bit decode + zscale tonemap in realtime.
			name: "software_HDR_capped_to_SoftwareHDRMaxHeight",
			setup: func(h *hlsVODServe) {
				cap := hdrCapTestCapability()
				h.capability = func() domaintranscoding.EncodeCapability { return cap }
				// hwAccelEnabled is nil → resolveHW returns usable=false immediately
			},
			streams:  build4KHDRStreams(),
			decision: buildHDRDecision(),
			assertFn: func(t *testing.T, profiles []domaintranscoding.HLSProfile) {
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
			setup: func(h *hlsVODServe) {
				cap := hdrCapTestCapability()
				h.capability = func() domaintranscoding.EncodeCapability { return cap }
			},
			streams:  build4KSDRStreams(),
			decision: buildSDRDecision(),
			assertFn: func(t *testing.T, profiles []domaintranscoding.HLSProfile) {
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
			setup: func(h *hlsVODServe) {
				cap := hdrCapHWCapability()
				h.capability = func() domaintranscoding.EncodeCapability { return cap }
				h.hwAccelEnabled = func() bool { return true }
				// hwTonemapEnabled is nil (nil → allowed) but hwTonemapWanted will
				// return false because HWBackend="nvenc" (not vaapi/qsv). That's
				// intentional: HW path is taken via the 10-bit gate (HW10BitTranscode),
				// not via tonemap. The key invariant is hwUsable=true, which alone
				// suppresses the SoftwareHDRMaxHeight cap.
			},
			streams:  build4KHDRStreams(),
			decision: buildHDRDecision(),
			assertFn: func(t *testing.T, profiles []domaintranscoding.HLSProfile) {
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
			// profilesForDecision (and its callees resolveHW / hwTonemapWanted)
			// need to be set. The rest remain zero/nil - profilesForDecision is
			// nil-safe for complexityRepo, capOverride, and capability.
			h := &hlsVODServe{}
			tc.setup(h)

			item := &domainlibrary.MediaItem{
				ID:        "m1",
				LibraryID: "lib1",
			}

			profiles := h.profilesForDecision(context.Background(), item, tc.streams, tc.decision)
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
	item := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}
	swCap := func() domaintranscoding.EncodeCapability { return hdrCapTestCapability() } // software, 1080p SDR / 480p HDR

	// Software HDR → the ladder capped at the HDR ceiling (SoftwareHDRMaxHeight=480):
	// every rung obeys the cap, NOT a single rung the box can't keep up with.
	t.Run("software_HDR_ladder_capped_to_hdr_ceiling", func(t *testing.T) {
		t.Parallel()
		h := &hlsVODServe{capability: swCap}
		profiles := h.profilesForDecision(context.Background(), item, build4KHDRStreams(), buildHDRDecision())
		require.NotEmpty(t, profiles)
		for _, p := range profiles {
			assert.LessOrEqual(t, p.Height, 480, "every HDR rung obeys SoftwareHDRMaxHeight (the honest weak-NAS cap)")
		}
		assert.Equal(t, 480, profiles[len(profiles)-1].Height, "the HDR top rung is the HDR ceiling")
	})

	// Software SDR → the full ABR ladder (more than one rung).
	t.Run("software_SDR_keeps_full_ladder", func(t *testing.T) {
		t.Parallel()
		h := &hlsVODServe{capability: swCap}
		profiles := h.profilesForDecision(context.Background(), item, build4KSDRStreams(), buildSDRDecision())
		assert.Greater(t, len(profiles), 1, "the software path serves the full ABR ladder")
	})

	// HW path → ladder kept (a GPU parallelises rungs).
	t.Run("hw_keeps_ladder", func(t *testing.T) {
		t.Parallel()
		h := &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return hdrCapHWCapability() },
			hwAccelEnabled: func() bool { return true },
		}
		profiles := h.profilesForDecision(context.Background(), item, build4KHDRStreams(), buildHDRDecision())
		assert.Greater(t, len(profiles), 1, "HW path keeps the ladder")
	})
}
