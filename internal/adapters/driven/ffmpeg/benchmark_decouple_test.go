package ffmpeg

import (
	"slices"
	"strings"
	"testing"
)

// The bench's HW ladder first tries the zero-copy path (HW decode → scale →
// encode). When the GPU can't HW-decode the sample codec (pre-Skylake Intel
// iGPUs decode H.264 but not HEVC, and the sample is HEVC), that path fails with
// ENOSYS before any encode runs, and benchHardware retries with the decoupled
// path: software-decode + hwupload. These tests lock the two shapes so the
// retry keeps mirroring probeHWEncode's validated incantation.

func TestHwDecoupledInputArgs(t *testing.T) {
	cases := map[string][]string{
		"vaapi": {"-vaapi_device", "/dev/dri/renderD128"},
		"qsv":   {"-init_hw_device", "qsv=hw", "-filter_hw_device", "hw"},
		"nvenc": nil, // NVENC HW-decodes every codec → never needs the decoupled path
	}
	for backend, want := range cases {
		got := hwDecoupledInputArgs(backend)
		if !slices.Equal(got, want) {
			t.Errorf("hwDecoupledInputArgs(%q) = %v, want %v", backend, got, want)
		}
		// The decoupled path MUST software-decode — no -hwaccel, the very flag
		// that fails on a GPU lacking the sample's decoder.
		if slices.Contains(got, "-hwaccel") {
			t.Errorf("hwDecoupledInputArgs(%q) must not contain -hwaccel, got %v", backend, got)
		}
	}
}

func TestHwUploadChain(t *testing.T) {
	cases := map[string]string{
		"h264_vaapi": "format=nv12,hwupload",
		"hevc_vaapi": "format=nv12,hwupload",
		"h264_qsv":   "hwupload=extra_hw_frames=16,format=qsv",
		"h264_nvenc": "", // zero-copy HW decode handles every codec
		"libx264":    "", // software encoder — no GPU upload
	}
	for encoder, want := range cases {
		if got := hwUploadChain(encoder); got != want {
			t.Errorf("hwUploadChain(%q) = %q, want %q", encoder, got, want)
		}
	}
}

// hwInputArgs (the zero-copy first pass) must still request HW decode for VAAPI,
// so a capable GPU keeps the production decode→scale→encode path.
func TestHwInputArgs_ZeroCopyStillHWDecodes(t *testing.T) {
	args := hwInputArgs("vaapi")
	if !slices.Contains(args, "-hwaccel") || !strings.Contains(strings.Join(args, " "), "vaapi") {
		t.Errorf("hwInputArgs(vaapi) should keep the HW-decode flags, got %v", args)
	}
}
