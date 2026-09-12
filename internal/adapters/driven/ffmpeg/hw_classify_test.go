package ffmpeg

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

func TestClassifyHWFailure_Stderr(t *testing.T) {
	// A generic non-signal, non-context exit error so only the stderr tail
	// drives the classification.
	exit := errors.New("exit status 1")

	cases := []struct {
		name   string
		stderr string
		want   transcoding.FailureClass
	}{
		{"nvenc vram oom", "[h264_nvenc @ 0x55] CUDA_ERROR_OUT_OF_MEMORY: out of memory", transcoding.FailureHWDeviceMemory},
		{"qsv no surfaces", "Error during encoding: no free surfaces", transcoding.FailureHWDeviceMemory},
		{"vaapi alloc", "Failed to allocate memory for the encoder", transcoding.FailureHWDeviceMemory},

		{"encoder missing", "Unknown encoder 'hevc_nvenc'", transcoding.FailureHWEncoderMissing},
		{"auto enc select", "Automatic encoder selection failed Default encoder not available", transcoding.FailureHWEncoderMissing},

		{"vaapi device", "Failed to create a VAAPI device: -1 (unknown libva error)", transcoding.FailureHWInit},
		{"no va display", "[AVHWDeviceContext] No VA display found for device /dev/dri/renderD128", transcoding.FailureHWInit},
		{"cuda load", "Cannot load nvcuda: shared object not found", transcoding.FailureHWInit},
		{"qsv init", "Error creating a MFX session / QSV device creation failed", transcoding.FailureHWInit},

		{"unrecognized option", "Unrecognized option 'temporal_aq'.", transcoding.FailureHWUnsupportedOption},
		{"qsv set option", "[h264_qsv @ 0x7f] Error setting option forced_idr to value 1", transcoding.FailureHWUnsupportedOption},
		// A bare EINVAL is how every filter-graph failure ends its thread; it is
		// the media/route, not a refused option, and never touches HWFeatures.
		{"invalid arg", "[hevc_nvenc @ 0x55] Invalid argument", transcoding.FailureRouteIncompatible},
		{"nvenc feature refused", "[h264_nvenc @ 0x55] Temporal AQ not supported by the device", transcoding.FailureHWUnsupportedOption},

		{"missing input", "input.mkv: No such file or directory", transcoding.FailureTransient},
		{"bad data", "Invalid data found when processing input", transcoding.FailureTransient},
		{"broken pipe", "av_interleaved_write_frame(): Broken pipe", transcoding.FailureTransient},

		{"unrecognised", "Some entirely unfamiliar failure mode", transcoding.FailureUnknown},
		{"empty", "", transcoding.FailureUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClassifyHWFailure(exit, c.stderr); got != c.want {
				t.Errorf("ClassifyHWFailure(%q) = %q, want %q", c.stderr, got, c.want)
			}
		})
	}
}

// The four tails below are verbatim ffmpeg 7.1.4-Jellyfin stderr captured on an
// RTX 4070 Ti (2026-09-12) for sessions the production builder emitted: they
// are media/route incompatibilities that used to be read as a host-wide
// "unsupported option" (persisted, HWFeatures wiped) or as a GPU init failure.
func TestClassifyHWFailure_MeasuredRouteIncompatibilities(t *testing.T) {
	exit := errors.New("exit status 218")
	cases := []struct {
		name   string
		stderr string
		want   transcoding.FailureClass
	}{
		{"Hi10P / 4:2:2 on the zero-copy chain", `Impossible to convert between the formats supported by the filter 'graph -1 input from stream 0:0' and the filter 'auto_scale_0'
[vf#0:0 @ 0x61a8278c7c00] Error reinitializing filters!
[vf#0:0 @ 0x61a8278c7c00] Task finished with error code: -38 (Function not implemented)
[vost#0:0/h264_nvenc @ 0x61a8278c77c0] Task finished with error code: -22 (Invalid argument)
[vost#0:0/h264_nvenc @ 0x61a8278c77c0] Terminating thread with return code -22 (Invalid argument)
[out#0/null @ 0x61a8278c8e40] Nothing was written into output file, because at least one of its streams received no packets.`, transcoding.FailureRouteIncompatible},
		{"progressive HDR through yadif_cuda then hwdownload", `[hwdownload @ 0x7d1594003cc0] Input frame is not the in the configured hwframe context.
[vf#0:0 @ 0x56170e65c240] Error while filtering: Invalid argument
[vf#0:0 @ 0x56170e65c240] Task finished with error code: -22 (Invalid argument)
[vost#0:0/h264_nvenc @ 0x56170e6bd3c0] Terminating thread with return code -22 (Invalid argument)
[out#0/null @ 0x56170e727000] Nothing was written into output file, because at least one of its streams received no packets.`, transcoding.FailureRouteIncompatible},
		{"stream NVDEC refuses, software fallback then filter mismatch", `[hevc @ 0x64521e428740] Failed setup for format cuda: hwaccel initialisation returned error.
Impossible to convert between the formats supported by the filter 'graph -1 input from stream 0:0' and the filter 'auto_scale_0'
[vf#0:0 @ 0x64521d79ee40] Error reinitializing filters!
[vost#0:0/wrapped_avframe @ 0x64521d7a0c00] Terminating thread with return code -22 (Invalid argument)`, transcoding.FailureRouteIncompatible},
		{"dead driver prints the same setup line after the device failure", `[AVHWDeviceContext @ 0x5f] Cannot load libcuda.so.1
[AVHWDeviceContext @ 0x5f] Could not dynamically load CUDA
Device creation failed: -1.
[hevc @ 0x60] No device available for decoder: device type cuda needed for codec hevc.
[hevc @ 0x60] Failed setup for format cuda: hwaccel initialisation returned error.`, transcoding.FailureHWInit},
		{"8-bit encoder fed a 10-bit surface", `[h264_nvenc @ 0x55] 10 bit encode not supported
[h264_nvenc @ 0x55] Provided device doesn't support required NVENC features
Error initializing output stream 0:0 -- Error while opening encoder for output stream #0:0`, transcoding.FailureRouteIncompatible},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClassifyHWFailure(exit, c.stderr); got != c.want {
				t.Errorf("ClassifyHWFailure = %q, want %q", got, c.want)
			}
		})
	}
}

func TestClassifyHWFailure_NilIsNone(t *testing.T) {
	if got := ClassifyHWFailure(nil, "CUDA_ERROR_OUT_OF_MEMORY"); got != transcoding.FailureNone {
		t.Errorf("nil error must classify as FailureNone, got %q", got)
	}
}

func TestClassifyHWFailure_ContextCancelIsCancelled(t *testing.T) {
	// Even with a HW-looking stderr, an explicit cancel/deadline is ours.
	if got := ClassifyHWFailure(context.Canceled, "Failed to create a VAAPI device"); got != transcoding.FailureCancelled {
		t.Errorf("context.Canceled must be Cancelled, got %q", got)
	}
	if got := ClassifyHWFailure(context.DeadlineExceeded, "out of memory"); got != transcoding.FailureCancelled {
		t.Errorf("context.DeadlineExceeded must be Cancelled, got %q", got)
	}
}

func TestClassifyHWFailure_KilledSignalIsCancelled(t *testing.T) {
	// A SIGKILL (our resource monitor / seek-cancel) must not be read as a HW
	// fault, even with a HW-looking stderr tail. Spawn a real process and kill
	// it to obtain a genuine signalled *exec.ExitError.
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start sleep: %v", err)
	}
	_ = cmd.Process.Kill()
	werr := cmd.Wait()
	if werr == nil {
		t.Fatal("expected a non-nil wait error after Kill")
	}
	if got := ClassifyHWFailure(werr, "CUDA_ERROR_OUT_OF_MEMORY"); got != transcoding.FailureCancelled {
		t.Errorf("SIGKILL must classify as Cancelled, got %q", got)
	}
}
