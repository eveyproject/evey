package ffmpeg

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
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
// Ordering matters - most specific signatures first, because some substrings
// overlap (an OOM also prints "failed to allocate"; a dead driver prints both
// "Device creation failed" and the "Failed setup for format" a mere
// unsupported stream prints too).
func ClassifyHWFailure(exitErr error, stderrTail string) transcoding.FailureClass {
	if exitErr == nil {
		return transcoding.FailureNone
	}
	// Our own cancellation (ctx deadline/cancel from a seek, GC, shutdown).
	if errors.Is(exitErr, context.Canceled) || errors.Is(exitErr, context.DeadlineExceeded) {
		return transcoding.FailureCancelled
	}
	// A kill-by-signal is, by default, attributed to us (seek-cancel or the
	// resource monitor's SIGKILL) - we don't demote the host on a kill we
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

	s := strings.ToLower(stripStderrNoise(stderrTail))
	switch {
	case stderrContainsAny(s,
		"out of memory", "cuda_error_out_of_memory", "cannot allocate memory",
		"failed to allocate", "no free surfaces", "cm_out_of_memory"):
		return transcoding.FailureHWDeviceMemory

	case stderrContainsAny(s,
		"unknown encoder", "encoder not found", "could not find encoder",
		"automatic encoder selection failed", "unsupported codec"):
		return transcoding.FailureHWEncoderMissing

	// A GPU tone-map interop that fails INSIDE a session (the Vulkan/libplacebo
	// or OpenCL device derived from the decoder's node, the map into it, the
	// filter itself) is a verdict on that route for that title, never on the
	// accelerator: the same VAAPI/CUDA device keeps encoding every SDR title.
	// Classified before the device-init tier because those failures word
	// themselves like a dead device ("Failed to create Vulkan instance",
	// "device creation failed" on the derived device). The title falls back
	// to the CPU bridge/software; the host's features and pools stay.
	case isInteropFailure(s):
		return transcoding.FailureRouteIncompatible

	// DEVICE-level signatures only: the accelerator itself could not be opened.
	// "hwaccel initialisation returned error" is deliberately NOT here - ffmpeg
	// prints it for a stream the decoder cannot hand to the GPU (a profile or a
	// frame size NVDEC refuses) exactly as for a dead driver, and the two are
	// told apart by the device lines below being present or not.
	case stderrContainsAny(s,
		"device creation failed", "failed to create a cuda device",
		"failed to create a vaapi device", "no va display",
		"failed to initialise vaapi", "cannot load nvcuda", "cannot load libcuda",
		"cannot load libnvidia-encode", "no device available", "cannot open the drm",
		"could not open the device", "qsv device creation",
		"cannot create mfx session", "mfxinit"):
		return transcoding.FailureHWInit

	// The media and the route disagree (measured 2026-09-12 on an RTX 4070 Ti,
	// prod image): a 10-bit H.264 / 4:2:2 HEVC source the GPU decoder does not
	// take ("Impossible to convert between the formats"), a filter that hands
	// hwdownload a frame from another pool ("not the in the configured hwframe
	// context"), a stream NVDEC refuses ("Failed setup for format"), an encoder
	// fed a surface it cannot take. The host encodes everything else; only this
	// (media, route) pair is wrong.
	case stderrContainsAny(s,
		"impossible to convert between the formats",
		"configured hwframe context", "failed setup for format",
		"nothing was written into output file",
		"10 bit encode not supported", "doesn't support required nvenc features",
		"does not support required nvenc features",
		"function not implemented"):
		return transcoding.FailureRouteIncompatible

	// An encoder option this device refuses - NAMED as such by ffmpeg. A bare
	// "Invalid argument" is not one: every filter-graph failure ends its thread
	// on EINVAL, so that string alone used to wipe the host's tuning features
	// on any broken session.
	case stderrContainsAny(s,
		"unrecognized option", "unknown option", "option not found",
		"error setting option", "not supported by the device",
		"not supported by this device", "value out of range"):
		return transcoding.FailureHWUnsupportedOption

	case stderrContainsAny(s,
		"failed to create", "device creation", "failed to initialise", "failed to initialize"):
		return transcoding.FailureHWInit

	case stderrContainsAny(s, "invalid argument"):
		return transcoding.FailureRouteIncompatible

	case stderrContainsAny(s,
		"no such file", "invalid data found", "broken pipe",
		"connection reset", "input/output error", "end of file"):
		return transcoding.FailureTransient
	}
	return transcoding.FailureUnknown
}

// interopLogContext is the log prefix of an interop filter instance, as ffmpeg
// prints it ("[Parsed_hwmap_1 @", "[libplacebo @", "[Parsed_tonemap_opencl_2 @").
var interopLogContext = regexp.MustCompile(`\[(parsed_)?(libplacebo|hwmap|\w+_opencl|\w+_vulkan)(_\d+)? @`)

// interopFailurePhrases are ffmpeg's own wordings of a Vulkan/OpenCL device or
// mapping failure (measured on jellyfin-ffmpeg 7.1.4), not the bare word, which
// a media path or a title would carry just as well.
var interopFailurePhrases = []string{
	"vk_error_", "failed to create vulkan", "failed initializing vulkan",
	"opencl platforms", "opencl device creation failed", "failed to create opencl",
	"failed to set value 'vulkan", "failed to set value 'opencl",
	"filter 'libplacebo'", "filter 'hwmap'", "filter 'tonemap_opencl'",
	"failed to map frame", "failed to derive device", "failed to create derived device",
}

func isInteropFailure(s string) bool {
	return interopLogContext.MatchString(s) || stderrContainsAny(s, interopFailurePhrases...)
}

// bannerLine matches the banner lines and a configuration fragment the 8 KiB
// stderr ring may have cut mid-line.
var bannerLine = regexp.MustCompile(`^(ffmpeg version |built with |configuration:|lib\w+ +\d+\.)|--(enable|disable)-`)

// stripStderrNoise drops what ffmpeg prints that is not a log message: the
// banner (its configuration line lists --enable-vulkan/opencl/libplacebo) and
// the indented Input/Output/Stream/Metadata dump that carries paths and titles.
func stripStderrNoise(stderr string) string {
	lines := strings.Split(stderr, "\n")
	kept := lines[:0]
	for _, l := range lines {
		if l == "" || l[0] == ' ' || l[0] == '\t' || bannerLine.MatchString(l) ||
			strings.HasPrefix(l, "Input #") || strings.HasPrefix(l, "Output #") {
			continue
		}
		kept = append(kept, l)
	}
	return strings.Join(kept, "\n")
}

func stderrContainsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
