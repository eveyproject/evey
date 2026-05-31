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
		{"invalid arg", "[hevc_nvenc @ 0x55] Invalid argument", transcoding.FailureHWUnsupportedOption},

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
