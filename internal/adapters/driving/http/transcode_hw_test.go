package httpapi

import (
	"testing"

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
		accel, device, features, usable := h.resolveHW("h264")
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
		accel, device, _, usable := h.resolveHW("hevc")
		if !usable || accel != "vaapi" || device != "/dev/dri/renderD128" {
			t.Errorf("got (%q,%q,%v), want (vaapi,/dev/dri/renderD128,true)", accel, device, usable)
		}
	})

	t.Run("unsupported codec → software", func(t *testing.T) {
		h := &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return vaapi }, // no av1
			hwAccelEnabled: func() bool { return true },
		}
		if _, _, _, usable := h.resolveHW("av1"); usable {
			t.Error("av1 unsupported by vaapi caps → must be software")
		}
	})

	t.Run("flag off → software", func(t *testing.T) {
		h := &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return nvenc },
			hwAccelEnabled: func() bool { return false },
		}
		if _, _, _, usable := h.resolveHW("h264"); usable {
			t.Error("flag off → must be software")
		}
	})

	t.Run("nil flag func → software", func(t *testing.T) {
		h := &hlsVODServe{
			capability: func() domaintranscoding.EncodeCapability { return nvenc },
		}
		if _, _, _, usable := h.resolveHW("h264"); usable {
			t.Error("nil hwAccelEnabled → must be software")
		}
	})

	t.Run("no calibrated backend → software", func(t *testing.T) {
		h := &hlsVODServe{
			capability:     func() domaintranscoding.EncodeCapability { return domaintranscoding.SafeFallback() },
			hwAccelEnabled: func() bool { return true },
		}
		if _, _, _, usable := h.resolveHW("h264"); usable {
			t.Error("no HW backend → must be software")
		}
	})
}
