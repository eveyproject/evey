package httpapi

// Tests for the software/hardware producer selector - the seam that keeps the
// software flow (single long-lived paced ffmpeg) off the multi-session
// orchestrator built for the hardware path. The matrix below locks the routing
// per (hw_mode × capability × content) so a refactor can't silently send a
// weak-NAS software request through the HW orchestrator (the 360p-buffering
// thrash) or vice versa.

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// sentinelProducer is a StreamProducer that does nothing but be identifiable by
// the tag it carries. The switch test asserts which sentinel the selector
// returns, so we never need a real ffmpeg or cache.
type sentinelProducer struct{ tag string }

func (s *sentinelProducer) Profiles(base []domaintranscoding.HLSProfile, _ apptranscoding.SourceDims, _ domaintranscoding.PlaybackDecision, _ domaintranscoding.EncodeCapability) []domaintranscoding.HLSProfile {
	return base
}
func (s *sentinelProducer) EnsureSegmentRange(_ context.Context, _ apptranscoding.DecisionFingerprint, _, _ int) error {
	return nil
}
func (s *sentinelProducer) PreWarm(_ apptranscoding.DecisionFingerprint, _, _ int) {}
func (s *sentinelProducer) WaitBudget() time.Duration                              { return 0 }
func (s *sentinelProducer) NotifyClientHint(_ string, _ int, _ int, _ bool, _ int) {}
func (s *sentinelProducer) ActiveSessionCount() int                                { return 0 }
func (s *sentinelProducer) RemoveSession(_ string)                                 {}
func (s *sentinelProducer) Close()                                                 {}

// sentinelManifestProducer is a sentinelProducer that ALSO implements
// ManifestProvider (its Manifest errors) - exactly like the software TRANSCODE
// producer after it gained a Manifest method for the gated copy mode. Used to
// prove serveVariant does NOT call Manifest on the transcode path.
type sentinelManifestProducer struct{ sentinelProducer }

func (s *sentinelManifestProducer) Manifest(_ context.Context, _ apptranscoding.DecisionFingerprint) ([]byte, error) {
	return nil, errors.New("manifest unavailable (transcode instance, copyPlanner nil)")
}

// TestHLSVOD_Variant_TranscodeManifestProducer_FallsThrough is the regression
// guard for the bug where SoftwareProducer gaining a Manifest method made the
// TRANSCODE producer satisfy ManifestProvider, so serveVariant's structural
// type-assertion called Manifest on the transcode path and 500'd every variant
// playlist. With copyProducer nil (the production default), isCopyRequest is
// false, so the assertion must be skipped and the precalculated manifest served.
func TestHLSVOD_Variant_TranscodeManifestProducer_FallsThrough(t *testing.T) {
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: testMediaItemWith1Minute()},
		allowAll(),
		testFakeParentalChecker{},
	)
	// Route the request through a producer that implements ManifestProvider (its
	// Manifest errors). copyProducer stays nil → isCopyRequest is always false.
	h.selector = apptranscoding.NewProducerSelector(
		&sentinelManifestProducer{}, &sentinelProducer{tag: "hw"},
		func(_ domaintranscoding.PlaybackDecision, _ apptranscoding.SourceDims) bool { return false },
	)
	w := doVODRequest(h, "m1", "stream_0.m3u8")
	require.Equal(t, http.StatusOK, w.Code, "transcode variant must serve the precalculated manifest, not 500")
	assert.Contains(t, w.Body.String(), "#EXTM3U")
}

// newSwitchHandler builds an hlsVODServe whose selector routes between two
// sentinel producers, using the handler's real resolveHW as the predicate (the
// same wiring newHLSVODServe does in production). hwEnabled mirrors the
// hw_accel_enabled flag; capFn supplies the calibrated capability.
func newSwitchHandler(hwEnabled bool, capFn func() domaintranscoding.EncodeCapability) (*hlsVODServe, *sentinelProducer, *sentinelProducer) {
	sw := &sentinelProducer{tag: "software"}
	hw := &sentinelProducer{tag: "hardware"}
	h := &hlsVODServe{
		capability:     capFn,
		hwAccelEnabled: func() bool { return hwEnabled },
		// HW tonemap allowed when probed; mirrors production default.
		hwTonemapEnabled: func() bool { return true },
	}
	h.selector = apptranscoding.NewProducerSelector(sw, hw,
		func(decision domaintranscoding.PlaybackDecision, src apptranscoding.SourceDims) bool {
			_, _, _, usable := h.resolveHW(decision.VideoCodec, src.Codec, src.TenBit, h.hwTonemapWanted(decision))
			return usable
		},
	)
	return h, sw, hw
}

// vaapiPreSkylakeCap returns a VAAPI capability that encodes H.264 but CANNOT
// decode HEVC - a pre-Skylake Intel iGPU (the prod NAS).
func vaapiPreSkylakeCap() domaintranscoding.EncodeCapability {
	return domaintranscoding.EncodeCapability{
		SoftwareMaxHeight: 1080,
		HWMaxHeight:       1080,
		HWBackend:         "vaapi",
		HWCodecs:          []string{"h264"},
		HWDecodeCodecs:    []string{"h264"}, // NO hevc decode
		HWDevice:          "/dev/dri/renderD128",
	}
}

func TestProducerSwitch_Matrix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		hwEnabled bool
		capFn     func() domaintranscoding.EncodeCapability
		streams   []*domainlibrary.MediaStream
		decision  domaintranscoding.PlaybackDecision
		wantTag   string
	}{
		{
			// (1) hw off + 4K HEVC10 HDR → software. The flag is off, so the
			// orchestrator is never reached regardless of content.
			name:      "hw_off_4K_HEVC10_HDR_to_software",
			hwEnabled: false,
			capFn:     hdrCapHWCapability, // capable HW present, but flag off
			streams:   build4KHDRStreams(),
			decision:  buildHDRDecision(),
			wantTag:   "software",
		},
		{
			// (2) hw off + 1080p H264 SDR → software.
			name:      "hw_off_1080p_H264_SDR_to_software",
			hwEnabled: false,
			capFn:     hdrCapHWCapability,
			streams:   build1080pH264Streams(),
			decision:  buildSDRDecision(),
			wantTag:   "software",
		},
		{
			// (3) hw auto + no backend → software (capability has no HWBackend).
			name:      "hw_auto_no_backend_to_software",
			hwEnabled: true,
			capFn:     hdrCapTestCapability, // software-only capability
			streams:   build1080pH264Streams(),
			decision:  buildSDRDecision(),
			wantTag:   "software",
		},
		{
			// (4) hw auto + NVENC h264 + hevc-decode + 10-bit OK → hardware.
			name:      "hw_auto_NVENC_full_to_hardware",
			hwEnabled: true,
			capFn:     hdrCapHWCapability, // nvenc h264, decodes hevc, HW10BitTranscode
			streams:   build4KHDRStreams(),
			decision:  buildHDRDecision(),
			wantTag:   "hardware",
		},
		{
			// (5) hw auto + VAAPI pre-Skylake + HEVC source → software (decode gate).
			name:      "hw_auto_VAAPI_preSkylake_HEVC_to_software",
			hwEnabled: true,
			capFn:     vaapiPreSkylakeCap,
			streams:   build4KHDRStreams(), // HEVC source the iGPU can't decode
			decision:  buildSDRDecision(),
			wantTag:   "software",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, sw, hw := newSwitchHandler(tc.hwEnabled, tc.capFn)
			got := h.selector.For(tc.decision, sourceDimsOf(tc.streams))
			want := sw
			if tc.wantTag == "hardware" {
				want = hw
			}
			assert.Same(t, want, got, "selector routed to the wrong producer (want %s)", tc.wantTag)
		})
	}
}

// TestProducerSwitch_SoftwareLadderCappedToHDR asserts case (1)'s downstream
// effect: the routed SoftwareProducer shapes a ladder capped to the honest HDR
// ceiling (keeping its floor), NOT a single rung - Auto degrades, the viewer
// picks. The switch test above proves the routing; this proves the shaping.
func TestProducerSwitch_SoftwareLadderCappedToHDR(t *testing.T) {
	t.Parallel()
	cache, err := apptranscoding.NewSegmentCache(t.TempDir() + "/seg")
	require.NoError(t, err)
	sw := apptranscoding.NewSoftwareProducer(cache, nil, nil, apptranscoding.SoftwareProducerOptions{}, nil)
	t.Cleanup(sw.Close)

	src := apptranscoding.SourceDims{Width: 3840, Height: 2160, Codec: "hevc", TenBit: true}
	capability := domaintranscoding.EncodeCapability{SoftwareMaxHeight: 1080, SoftwareHDRMaxHeight: 480}
	profiles := sw.Profiles(domaintranscoding.LadderFilmNormal, src, buildHDRDecision(), capability)

	require.NotEmpty(t, profiles)
	for _, p := range profiles {
		assert.LessOrEqual(t, p.Height, 480,
			"every software HDR rung obeys the honest SoftwareHDRMaxHeight ceiling")
	}
	assert.Equal(t, 480, profiles[len(profiles)-1].Height, "the HDR top rung is the ceiling")
}

// TestProfilesForDecision_HashInvariant locks the cache-hash invariant: the
// ladder profilesForDecision returns at manifest-time MUST be byte-identical to
// the one it returns at segment-time, so the DecisionFingerprint.Hash() agrees.
// Any drift = a permanent cache miss (every segment re-encoded). We assert the
// derived ProfileFingerprint slice hashes identically across two calls with the
// same inputs.
func TestProfilesForDecision_HashInvariant(t *testing.T) {
	t.Parallel()

	mkHandler := func() *hlsVODServe {
		h, _, _ := newSwitchHandler(false, hdrCapTestCapability) // software path, single-rendition
		h.complexityRepo = nil
		return h
	}
	item := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}
	streams := build4KHDRStreams()
	decision := buildHDRDecision()

	// Two independent handlers (separate selector instances) with the SAME
	// inputs: simulate manifest-time and segment-time on different goroutines /
	// requests. The shaped ladders must hash identically.
	hManifest := mkHandler()
	hSegment := mkHandler()
	pManifest := hManifest.profilesForDecision(context.Background(), item, streams, decision)
	pSegment := hSegment.profilesForDecision(context.Background(), item, streams, decision)

	assert.Equal(t, fingerprintProfiles(pManifest), fingerprintProfiles(pSegment),
		"profilesForDecision must be deterministic across manifest- and segment-time (cache-hash invariant)")
	assert.Equal(t, hashOfProfiles(t, pManifest), hashOfProfiles(t, pSegment),
		"the DecisionFingerprint hash derived from the ladder must match across the two calls")
}

// fingerprintProfiles mirrors buildFingerprint's HLSProfile→ProfileFingerprint
// projection so the test compares exactly what enters the hash.
func fingerprintProfiles(ps []domaintranscoding.HLSProfile) []apptranscoding.ProfileFingerprint {
	out := make([]apptranscoding.ProfileFingerprint, len(ps))
	for i, p := range ps {
		out[i] = apptranscoding.ProfileFingerprint{
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
func hashOfProfiles(t *testing.T, ps []domaintranscoding.HLSProfile) string {
	t.Helper()
	fp := apptranscoding.DecisionFingerprint{
		MediaID:    "m1",
		SourcePath: "/m.mkv",
		VideoCodec: "h264",
		AudioCodec: "aac",
		Profiles:   fingerprintProfiles(ps),
	}
	return fp.Hash()
}

// build1080pH264Streams is a 1080p H.264 8-bit SDR source.
func build1080pH264Streams() []*domainlibrary.MediaStream {
	w, h := 1920, 1080
	return []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "h264", Width: &w, Height: &h},
	}
}
