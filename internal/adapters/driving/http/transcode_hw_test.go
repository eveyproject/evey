package httpapi

import (
	"testing"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

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
		accel, device, features, usable := h.resolveHW("h264", "", false, false)
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
		accel, device, _, usable := h.resolveHW("hevc", "", false, false)
		if !usable || accel != "vaapi" || device != "/dev/dri/renderD128" {
			t.Errorf("got (%q,%q,%v), want (vaapi,/dev/dri/renderD128,true)", accel, device, usable)
		}
	})

	t.Run("unsupported codec → software", func(t *testing.T) {
		h := &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return vaapi }, // no av1
			hwAccelEnabled: func() bool { return true },
		}
		if _, _, _, usable := h.resolveHW("av1", "", false, false); usable {
			t.Error("av1 unsupported by vaapi caps → must be software")
		}
	})

	t.Run("source codec not HW-decodable → software (old iGPU, HEVC)", func(t *testing.T) {
		// Encodes H.264 + HEVC but DECODES only H.264 — a pre-Skylake Intel iGPU.
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
		if _, _, _, usable := h.resolveHW("h264", "h264", false, false); !usable {
			t.Error("h264 source is HW-decodable → must take the HW path")
		}
		if _, _, _, usable := h.resolveHW("h264", "hevc", false, false); usable {
			t.Error("hevc source not HW-decodable on this GPU → must fall back to software (no stall)")
		}
	})

	t.Run("flag off → software", func(t *testing.T) {
		h := &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return nvenc },
			hwAccelEnabled: func() bool { return false },
		}
		if _, _, _, usable := h.resolveHW("h264", "", false, false); usable {
			t.Error("flag off → must be software")
		}
	})

	t.Run("nil flag func → software", func(t *testing.T) {
		h := &hlsVODServe{
			capability: func() domaintranscoding.EncodeCapability { return nvenc },
		}
		if _, _, _, usable := h.resolveHW("h264", "", false, false); usable {
			t.Error("nil hwAccelEnabled → must be software")
		}
	})

	t.Run("no calibrated backend → software", func(t *testing.T) {
		h := &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return domaintranscoding.SafeFallback() },
			hwAccelEnabled: func() bool { return true },
		}
		if _, _, _, usable := h.resolveHW("h264", "", false, false); usable {
			t.Error("no HW backend → must be software")
		}
	})

	t.Run("10-bit source + VPP can't do 10-bit → software (i965 VEBOX)", func(t *testing.T) {
		// Decodes hevc AND encodes h264, but the calibration found the VPP aborts
		// on a 10-bit (p010) surface — the Intel i965 case. A Main10 source
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
		if _, _, _, usable := h.resolveHW("h264", "hevc", false, false); !usable {
			t.Error("8-bit hevc source → HW path (decode ok, no 10-bit VPP needed)")
		}
		if _, _, _, usable := h.resolveHW("h264", "hevc", true, false); usable {
			t.Error("10-bit source + !HW10BitTranscode → must software-transcode (p010 aborts the VEBOX)")
		}
		// ...but when this request HW-tone-maps, tonemap_vaapi folds the 10-bit p010
		// itself, so the same 10-bit source CAN take the HW path without HW10BitTranscode.
		if _, _, _, usable := h.resolveHW("h264", "hevc", true, true); !usable {
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
		if _, _, _, usable := h.resolveHW("h264", "hevc", true, false); !usable {
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
		got, _, _, _, _ := h.effectiveVideoCodec(dec("av1"), nil)
		if got != "h264" {
			t.Errorf("av1 must fall back to h264 when not encodable here, got %q", got)
		}
	})

	t.Run("av1 + HW av1 stays av1", func(t *testing.T) {
		box := domaintranscoding.EncodeCapability{HWBackend: "nvenc", HWCodecs: []string{"av1"}, HWMaxHeight: 2160}
		h := &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return box },
			hwAccelEnabled: func() bool { return true },
		}
		got, _, _, _, usable := h.effectiveVideoCodec(dec("av1"), nil)
		if got != "av1" || !usable {
			t.Errorf("av1 with HW av1 must stay av1 (usable), got %q usable=%v", got, usable)
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
		got, _, _, _, _ := h.effectiveVideoCodec(dec("av1"), nil)
		if got != "h264" {
			t.Errorf("av1 must fall back to h264 when the GPU can't encode it (HW-only policy), got %q", got)
		}
	})

	t.Run("h264 is unaffected", func(t *testing.T) {
		h := &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return domaintranscoding.SafeFallback() },
			hwAccelEnabled: func() bool { return true },
		}
		got, _, _, _, _ := h.effectiveVideoCodec(dec("h264"), nil)
		if got != "h264" {
			t.Errorf("h264 must be unchanged, got %q", got)
		}
	})
}
