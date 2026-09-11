package httpapi

import (
	"context"
	"testing"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

func TestProfilesForDecision_AV1MaxHeightCap(t *testing.T) {
	box := domaintranscoding.EncodeCapability{
		HWBackend: "nvenc", HWCodecs: []string{"av1"}, HWDecodeCodecs: []string{"h264"},
		HWMaxHeight: 2160, SoftwareMaxHeight: 2160,
	}
	w, hgt := 3840, 2160
	streams := []*domainlibrary.MediaStream{{StreamType: "video", Codec: "h264", Width: &w, Height: &hgt}}
	dec := domaintranscoding.PlaybackDecision{VideoCodec: "av1", VideoDecision: "transcode"}
	item := &domainlibrary.MediaItem{ID: "mi-av1"}

	mk := func(maxH int) *hlsVODServe {
		return &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return box },
			hwAccelEnabled: func() bool { return true },
			av1Enabled:     func() bool { return true },
			av1MaxHeight:   func() int { return maxH },
		}
	}
	top := func(ps []domaintranscoding.HLSProfile) int {
		m := 0
		for _, p := range ps {
			if p.Height > m {
				m = p.Height
			}
		}
		return m
	}

	uncapped := top(mk(0).profilesForDecision(context.Background(), item, streams, dec))
	if uncapped <= 720 {
		t.Fatalf("control: uncapped AV1 ladder should exceed 720p (got %dp) for the cap to be meaningful", uncapped)
	}
	capped := top(mk(720).profilesForDecision(context.Background(), item, streams, dec))
	if capped > 720 {
		t.Errorf("av1_max_height=720 must cap the AV1 ladder to 720p, got top rung %dp", capped)
	}
}

func TestResolveHW(t *testing.T) {
	nvenc := domaintranscoding.EncodeCapability{
		HWBackend:  "nvenc",
		HWCodecs:   []string{"h264", "hevc", "av1"},
		HWDevice:   "",
		HWFeatures: []string{"temporal_aq", "rc_lookahead"},
	}
	vaapi := domaintranscoding.EncodeCapability{
		HWBackend: "vaapi",
		HWCodecs:  []string{"h264", "hevc"},
		HWDevice:  "/dev/dri/renderD128",
	}

	t.Run("flag on, backend supports codec → HW", func(t *testing.T) {
		h := &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return nvenc },
			hwAccelEnabled: func() bool { return true },
		}
		accel, device, features, usable, _ := h.resolveHW("h264", "", false, false)
		if !usable || accel != "nvenc" || device != "" {
			t.Errorf("got (%q,%q,%v), want (nvenc,'',true)", accel, device, usable)
		}
		if len(features) != 2 || features[0] != "temporal_aq" {
			t.Errorf("probed features must be forwarded, got %v", features)
		}
	})

	t.Run("vaapi forwards device path", func(t *testing.T) {
		h := &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return vaapi },
			hwAccelEnabled: func() bool { return true },
		}
		accel, device, _, usable, _ := h.resolveHW("hevc", "", false, false)
		if !usable || accel != "vaapi" || device != "/dev/dri/renderD128" {
			t.Errorf("got (%q,%q,%v), want (vaapi,/dev/dri/renderD128,true)", accel, device, usable)
		}
	})

	t.Run("unsupported codec → software", func(t *testing.T) {
		h := &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return vaapi }, // no av1
			hwAccelEnabled: func() bool { return true },
		}
		if _, _, _, usable, _ := h.resolveHW("av1", "", false, false); usable {
			t.Error("av1 unsupported by vaapi caps → must be software")
		}
	})

	t.Run("source codec not HW-decodable → decoupled HW (old iGPU, HEVC)", func(t *testing.T) {
		// Encodes H.264 + HEVC but DECODES only H.264 - a pre-Skylake Intel iGPU.
		oldIGPU := domaintranscoding.EncodeCapability{
			HWBackend:      "vaapi",
			HWCodecs:       []string{"h264", "hevc"},
			HWDecodeCodecs: []string{"h264"},
			HWDevice:       "/dev/dri/renderD128",
		}
		h := &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return oldIGPU },
			hwAccelEnabled: func() bool { return true },
		}
		if _, _, _, usable, _ := h.resolveHW("h264", "h264", false, false); !usable {
			t.Error("h264 source is HW-decodable → must take the HW path")
		}
		accel, _, _, usable, softwareDecode := h.resolveHW("h264", "hevc", false, false)
		if !usable || accel != "vaapi" || !softwareDecode {
			t.Errorf("hevc source not HW-decodable on this GPU → software decode + hwupload, GPU keeps the encode; got (%q,%v,%v)", accel, usable, softwareDecode)
		}
	})

	t.Run("flag off → software", func(t *testing.T) {
		h := &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return nvenc },
			hwAccelEnabled: func() bool { return false },
		}
		if _, _, _, usable, _ := h.resolveHW("h264", "", false, false); usable {
			t.Error("flag off → must be software")
		}
	})

	t.Run("nil flag func → software", func(t *testing.T) {
		h := &hlsVODServe{
			capability: func() domaintranscoding.EncodeCapability { return nvenc },
		}
		if _, _, _, usable, _ := h.resolveHW("h264", "", false, false); usable {
			t.Error("nil hwAccelEnabled → must be software")
		}
	})

	t.Run("no calibrated backend → software", func(t *testing.T) {
		h := &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return domaintranscoding.SafeFallback() },
			hwAccelEnabled: func() bool { return true },
		}
		if _, _, _, usable, _ := h.resolveHW("h264", "", false, false); usable {
			t.Error("no HW backend → must be software")
		}
	})

	t.Run("10-bit source + VPP can't do 10-bit → software (i965 VEBOX)", func(t *testing.T) {
		// Decodes hevc AND encodes h264, but the calibration found the VPP aborts
		// on a 10-bit (p010) surface - the Intel i965 case. A Main10 source
		// must software-transcode so it never crashes the GPU pipeline.
		box := domaintranscoding.EncodeCapability{
			HWBackend:        "vaapi",
			HWCodecs:         []string{"h264"},
			HWDecodeCodecs:   []string{"h264", "hevc"},
			HW10BitTranscode: false,
			HWDevice:         "/dev/dri/renderD128",
		}
		h := &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return box },
			hwAccelEnabled: func() bool { return true },
		}
		if _, _, _, usable, _ := h.resolveHW("h264", "hevc", false, false); !usable {
			t.Error("8-bit hevc source → HW path (decode ok, no 10-bit VPP needed)")
		}
		if _, _, _, usable, _ := h.resolveHW("h264", "hevc", true, false); usable {
			t.Error("10-bit source + !HW10BitTranscode → must software-transcode (p010 aborts the VEBOX)")
		}
		// ...but when this request HW-tone-maps, tonemap_vaapi folds the 10-bit p010
		// itself, so the same 10-bit source CAN take the HW path without HW10BitTranscode.
		if _, _, _, usable, _ := h.resolveHW("h264", "hevc", true, true); !usable {
			t.Error("10-bit source + HW tonemap → HW path (tonemap_vaapi folds p010)")
		}
	})

	t.Run("10-bit source + VPP handles 10-bit → HW", func(t *testing.T) {
		box := domaintranscoding.EncodeCapability{
			HWBackend:        "nvenc",
			HWCodecs:         []string{"h264", "hevc"},
			HWDecodeCodecs:   []string{"h264", "hevc"},
			HW10BitTranscode: true,
		}
		h := &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return box },
			hwAccelEnabled: func() bool { return true },
		}
		if _, _, _, usable, _ := h.resolveHW("h264", "hevc", true, false); !usable {
			t.Error("10-bit source + HW10BitTranscode → must take the HW path")
		}
	})
}

func TestStreamIsTenBit(t *testing.T) {
	bd10, bd8 := 10, 8
	video := domainlibrary.StreamTypeVideo
	cases := []struct {
		name string
		s    *domainlibrary.MediaStream
		want bool
	}{
		{"nil", nil, false},
		{"Main 10 profile", &domainlibrary.MediaStream{StreamType: video, Profile: "Main 10"}, true},
		{"Main profile (8-bit)", &domainlibrary.MediaStream{StreamType: video, Profile: "Main"}, false},
		{"bit_depth 10", &domainlibrary.MediaStream{StreamType: video, BitDepth: &bd10}, true},
		{"bit_depth 8", &domainlibrary.MediaStream{StreamType: video, BitDepth: &bd8}, false},
	}
	for _, c := range cases {
		if got := streamIsTenBit(c.s); got != c.want {
			t.Errorf("%s: streamIsTenBit=%v want %v", c.name, got, c.want)
		}
	}

	// sourceVideoTenBit must pick the video stream, ignoring audio.
	streams := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeAudio, Profile: "LC"},
		{StreamType: video, Profile: "Main 10"},
	}
	if !sourceVideoTenBit(streams) {
		t.Error("sourceVideoTenBit must detect the Main 10 video stream")
	}
	if sourceVideoTenBit([]*domainlibrary.MediaStream{{StreamType: video, Profile: "Main"}}) {
		t.Error("8-bit Main video stream must not be flagged 10-bit")
	}
}

func TestEffectiveVideoCodec_AV1Gate(t *testing.T) {
	dec := func(codec string) domaintranscoding.PlaybackDecision {
		return domaintranscoding.PlaybackDecision{VideoCodec: codec}
	}

	t.Run("av1 + no HW + no AV1 sw ceiling falls back to h264", func(t *testing.T) {
		h := &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return domaintranscoding.SafeFallback() },
			hwAccelEnabled: func() bool { return true },
		}
		got, _, _, _, _, _ := h.effectiveVideoCodec(dec("av1"), nil)
		if got != "h264" {
			t.Errorf("av1 must fall back to h264 when not encodable here, got %q", got)
		}
	})

	t.Run("av1 + HW av1 + enabled stays av1", func(t *testing.T) {
		box := domaintranscoding.EncodeCapability{HWBackend: "nvenc", HWCodecs: []string{"av1"}, HWMaxHeight: 2160}
		h := &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return box },
			hwAccelEnabled: func() bool { return true },
			av1Enabled:     func() bool { return true },
		}
		got, _, _, _, usable, _ := h.effectiveVideoCodec(dec("av1"), nil)
		if got != "av1" || !usable {
			t.Errorf("av1 with HW av1 + admin opt-in must stay av1 (usable), got %q usable=%v", got, usable)
		}
	})

	t.Run("av1 + HW av1 but admin disabled falls back to h264", func(t *testing.T) {
		// The opt-in gate: even when the GPU CAN encode AV1, an admin who hasn't
		// turned AV1 on (default) must keep streams on H.264.
		box := domaintranscoding.EncodeCapability{HWBackend: "nvenc", HWCodecs: []string{"av1"}, HWMaxHeight: 2160}
		h := &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return box },
			hwAccelEnabled: func() bool { return true },
			av1Enabled:     func() bool { return false },
		}
		got, _, _, _, _, _ := h.effectiveVideoCodec(dec("av1"), nil)
		if got != "h264" {
			t.Errorf("av1 must fall back to h264 when the admin has not enabled it, got %q", got)
		}
	})

	t.Run("av1 + no HW falls back to h264 even with a calibrated AV1 sw ceiling", func(t *testing.T) {
		// AV1 is hardware-only for streaming: libsvtav1 can't sustain realtime
		// on the weak-NAS target (it starves → perpetual rebuffer), so a calibrated
		// software-AV1 ceiling must NOT keep a live session on software AV1.
		box := domaintranscoding.EncodeCapability{SoftwareMaxHeight: 1080, SoftwareAV1MaxHeight: 720}
		h := &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return box },
			hwAccelEnabled: func() bool { return true },
		}
		got, _, _, _, _, _ := h.effectiveVideoCodec(dec("av1"), nil)
		if got != "h264" {
			t.Errorf("av1 must fall back to h264 when the GPU can't encode it (HW-only policy), got %q", got)
		}
	})

	t.Run("h264 is unaffected", func(t *testing.T) {
		h := &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return domaintranscoding.SafeFallback() },
			hwAccelEnabled: func() bool { return true },
		}
		got, _, _, _, _, _ := h.effectiveVideoCodec(dec("h264"), nil)
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
	nvencH264 := domaintranscoding.EncodeCapability{
		HWBackend:      "nvenc",
		HWCodecs:       []string{"h264", "hevc"}, // no AV1 encode
		HWDecodeCodecs: []string{"h264", "hevc"},
	}
	h := &hlsVODServe{
		capability:     func() domaintranscoding.EncodeCapability { return nvencH264 },
		hwAccelEnabled: func() bool { return true },
		av1Enabled:     func() bool { return false },
	}
	decision := domaintranscoding.PlaybackDecision{VideoCodec: "av1", VideoDecision: "transcode"}
	src := apptranscoding.SourceDims{Codec: "h264"}

	_, accel, _, _, usable, _ := h.effectiveVideoCodec(decision, []*domainlibrary.MediaStream{{
		StreamType: domainlibrary.StreamTypeVideo, Codec: "h264",
	}})
	if !usable || accel != "nvenc" {
		t.Fatalf("fingerprint side: effective H.264 must be HW-usable, got (%q,%v)", accel, usable)
	}
	if !h.hwRouteUsable(decision, src) {
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
	nvencNoTonemap := domaintranscoding.EncodeCapability{
		HWBackend:        "nvenc",
		HWCodecs:         []string{"h264", "hevc"},
		HWDecodeCodecs:   []string{"h264", "hevc"},
		HW10BitTranscode: true,
		HWTonemap:        false,
		HWMaxHeight:      2160,
	}
	h := &hlsVODServe{
		capability:     func() domaintranscoding.EncodeCapability { return nvencNoTonemap },
		hwAccelEnabled: func() bool { return true },
		av1Enabled:     func() bool { return false },
	}
	src := apptranscoding.SourceDims{Codec: "hevc", TenBit: true}

	hdr := domaintranscoding.PlaybackDecision{
		VideoCodec:       "h264",
		VideoDecision:    "transcode",
		TranscodeReasons: []string{"hdr_tonemapping_needed"},
	}
	if !h.hwRouteUsable(hdr, src) {
		t.Error("HDR source + no GPU tone-map: the CPU tone-maps between the GPU decode and encode (the ladder is capped by the software HDR ceiling in ShapeLadder)")
	}
	// The bridge needs the GPU to decode: a source it cannot decode stays on the CPU.
	if h.hwRouteUsable(hdr, apptranscoding.SourceDims{Codec: "mpeg2video", TenBit: true}) {
		t.Error("HDR source the GPU cannot decode + no GPU tone-map must route SOFTWARE")
	}

	// Same host, an SDR source: nothing to tone-map, the GPU path stands.
	sdr := domaintranscoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"}
	if !h.hwRouteUsable(sdr, apptranscoding.SourceDims{Codec: "h264"}) {
		t.Error("an SDR source must still ride the GPU")
	}
}

// The rendition serves the copy rung and, when the encoder's reorder delay can
// be compensated, the transcoded ladder too (one AAC encode for every rung).
// Inverse telecine changes the frame rate, so it stays muxed.
func TestAudioRenditionsForCopyAndTranscode(t *testing.T) {
	probes := func(h *hlsVODServe) *hlsVODServe {
		h.frameRateProbe = func(context.Context, string) float64 { return 24 }
		h.encoderReorderProbe = func(context.Context, string, string, string, []string) (int, bool) { return 2, true }
		return h
	}
	h := probes(&hlsVODServe{audioRenditionEnabled: true})
	copyDec := domaintranscoding.PlaybackDecision{VideoDecision: "copy", AudioDecision: "transcode"}
	transcodeDec := domaintranscoding.PlaybackDecision{VideoDecision: "transcode", AudioDecision: "transcode"}
	if !h.audioRenditionsFor(copyDec, nil) {
		t.Error("video COPY + audio transcode is the hybrid rung: renditions must be on")
	}
	if !h.audioRenditionsFor(transcodeDec, nil) {
		t.Error("a transcoded ladder shares one audio rendition when the encoder delay is compensated")
	}
	tru := true
	telecine := []*domainlibrary.MediaStream{{StreamType: "video", Codec: "h264", IdetTelecine: &tru}}
	if h.audioRenditionsFor(transcodeDec, telecine) {
		t.Error("inverse telecine changes the frame rate: keep the audio muxed")
	}
	if (&hlsVODServe{audioRenditionEnabled: true}).audioRenditionsFor(transcodeDec, nil) {
		t.Error("without the encoder probes a transcode keeps its audio muxed")
	}
	if probes(&hlsVODServe{audioRenditionEnabled: false}).audioRenditionsFor(copyDec, nil) {
		t.Error("the operator opt-out must still win")
	}
}
