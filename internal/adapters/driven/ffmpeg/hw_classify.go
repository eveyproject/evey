package ffmpeg

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"syscall"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// ClassifyHWFailure maps an ffmpeg exit error + the tail of its stderr to a
// transcoding.FailureClass. It is deliberately CONSERVATIVE: only stderr
// signatures we recognise as a hardware-capability problem return an
// IsHWCapability class. Anything we can't attribute stays Cancelled (we killed
// it) / Transient / Unknown, so the safety net never permanently demotes the
// host on a fluke. A nil error classifies as FailureNone.
//
// Ordering matters — most specific signatures first, because some substrings
// overlap (an OOM also prints "failed to allocate").
func ClassifyHWFailure(exitErr error, stderrTail string) transcoding.FailureClass {
	if exitErr == nil {
		return transcoding.FailureNone
	}
	// Our own cancellation (ctx deadline/cancel from a seek, GC, shutdown).
	if errors.Is(exitErr, context.Canceled) || errors.Is(exitErr, context.DeadlineExceeded) {
		return transcoding.FailureCancelled
	}
	// A kill-by-signal is, by default, attributed to us (seek-cancel or the
	// resource monitor's SIGKILL) — we don't demote the host on a kill we
	// can't confirm came from the GPU.
	var ee *exec.ExitError
	if errors.As(exitErr, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			switch ws.Signal() {
			case syscall.SIGKILL, syscall.SIGTERM, syscall.SIGINT:
				return transcoding.FailureCancelled
			}
		}
	}

	s := strings.ToLower(stderrTail)
	switch {
	case stderrContainsAny(s,
		"out of memory", "cuda_error_out_of_memory", "cannot allocate memory",
		"failed to allocate", "no free surfaces", "cm_out_of_memory"):
		return transcoding.FailureHWDeviceMemory

	case stderrContainsAny(s,
		"unknown encoder", "encoder not found", "could not find encoder",
		"automatic encoder selection failed", "unsupported codec"):
		return transcoding.FailureHWEncoderMissing

	case stderrContainsAny(s,
		"failed to create", "device creation failed", "no va display",
		"failed to initialise", "failed to initialize", "cannot load nvcuda",
		"cannot load libcuda", "no device available", "cannot open the drm",
		"could not open the device", "hwaccel initialisation",
		"hwaccel initialization", "qsv device creation",
		"cannot create mfx session", "mfxinit"):
		return transcoding.FailureHWInit

	case stderrContainsAny(s,
		"unrecognized option", "unknown option", "option not found",
		"error setting option", "invalid argument", "value out of range"):
		return transcoding.FailureHWUnsupportedOption

	case stderrContainsAny(s,
		"no such file", "invalid data found", "broken pipe",
		"connection reset", "input/output error", "end of file"):
		return transcoding.FailureTransient
	}
	return transcoding.FailureUnknown
}

func stderrContainsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
