package ffmpeg

import (
	"github.com/eveyproject/evey/internal/domain/transcoding"
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
		"qsv":   {"-init_hw_device", "qsv=hw,child_device=/dev/dri/renderD128", "-filter_hw_device", "hw"},
		"nvenc": nil, // NVENC HW-decodes every codec → never needs the decoupled path
	}
	for backend, want := range cases {
		got := hwDecoupledInputArgs(backend, "")
		if !slices.Equal(got, want) {
			t.Errorf("hwDecoupledInputArgs(%q) = %v, want %v", backend, got, want)
		}
		// The decoupled path MUST software-decode - no -hwaccel, the very flag
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
		"libx264":    "", // software encoder - no GPU upload
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
	args := hwInputArgs("vaapi", "")
	if !slices.Contains(args, "-hwaccel") || !strings.Contains(strings.Join(args, " "), "vaapi") {
		t.Errorf("hwInputArgs(vaapi) should keep the HW-decode flags, got %v", args)
	}
	// The bench binds the SAME device args the production builder emits, on the
	// named device - a second render node / GPU reaches every probe and rung.
	if got := hwInputArgs("vaapi", "/dev/dri/renderD129"); !slices.Equal(got, hwInitArgs(transcoding.VAAPI, "/dev/dri/renderD129")) {
		t.Errorf("hwInputArgs(vaapi, renderD129) = %v", got)
	}
	if got := hwInputArgs("nvenc", "1"); !slices.Contains(got, "-hwaccel_device") || !slices.Contains(got, "1") {
		t.Errorf("hwInputArgs(nvenc, 1) must bind the CUDA index: %v", got)
	}
	if got := hwDecoupledInputArgs("qsv", "/dev/dri/renderD129"); !slices.Contains(got, "qsv=hw,child_device=/dev/dri/renderD129") {
		t.Errorf("hwDecoupledInputArgs(qsv, renderD129) = %v", got)
	}
}

// probeRungs decides the pipeline shape the calibration bench measures. It MUST
// mirror the runtime orchestrator: single-variant encodes one rung on demand,
// multi-variant encodes the whole ladder in parallel. Benching the wrong shape
// pins the HDR cap far below (multi-variant probe) or above (over-optimistic)
// what the host actually serves - the root cause of the 360p HDR fallback when
// the runtime ran single-variant but the bench measured the parallel ladder.
func TestProbeRungs_SingleVariantIsolatesOneRung(t *testing.T) {
	ladder := []int{720, 1080, 1440}

	// Single-variant: each probe is exactly one rung - the shape an on-demand
	// session runs (decode + 1 encode at h).
	for i, h := range ladder {
		got := probeRungs(ladder, i, true)
		if !slices.Equal(got, []int{h}) {
			t.Errorf("probeRungs(single, i=%d) = %v, want [%d]", i, got, h)
		}
	}

	// Multi-variant: each probe is the cumulative ladder up to and including i
	// (decode + N parallel encodes) - unchanged legacy behaviour.
	wantMulti := [][]int{{720}, {720, 1080}, {720, 1080, 1440}}
	for i := range ladder {
		got := probeRungs(ladder, i, false)
		if !slices.Equal(got, wantMulti[i]) {
			t.Errorf("probeRungs(multi, i=%d) = %v, want %v", i, got, wantMulti[i])
		}
	}
}

// The multi-variant slice must not alias swBenchLadder's backing array such that
// a later append could corrupt the shared ladder. The 3-index slice caps cap==len.
func TestProbeRungs_MultiVariantSliceCannotMutateLadder(t *testing.T) {
	ladder := []int{720, 1080, 1440}
	got := probeRungs(ladder, 0, false) // {720}, but must not have spare capacity
	got = append(got, 9999)             // would clobber ladder[1] if cap allowed it
	if ladder[1] != 1080 {
		t.Fatalf("append to probeRungs result mutated the shared ladder: ladder[1]=%d", ladder[1])
	}
	_ = got
}
