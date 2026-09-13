package ffmpeg

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/procfork"
)

// hw_readiness.go - a CALIBRATION-ONLY retry for HW init/readiness transients.
//
// The 2026-09-10 prod-host cold boots proved the failure is a GLOBAL Intel/VAAPI
// readiness transient at container start, not an OpenCL-specific bug: the same
// boots that lost the OpenCL derive also wrote no VAAPI encode init (watchdog)
// and failed the native VPP probe, then recovered by the next re-bench on the
// identical image. So this is not an OpenCL workaround - it is: when a probe
// fails during calibration with an init/readiness/transient class, replay the
// SAME probe on a bounded schedule (no restart, no fixed warm-up) and, if it
// recovers, proceed with the full calibration instead of persisting the first
// failure as "unsupported". Deterministic classes (route/format/unsupported/
// encoder-missing/corrupt) are never retried. Retries exhausted -> fail-close,
// exactly as today. Every attempt and any FAIL->OK is logged, and the first
// failure and retry count are kept so a future bad boot can be diagnosed.

// hwReadinessRetryDelays replays a failed init/readiness probe at these offsets.
// It fires ONLY after a retryable first failure and stops the instant one
// attempt succeeds - it is not a warm-up that runs on the happy path.
var hwReadinessRetryDelays = []time.Duration{
	500 * time.Millisecond,
	1500 * time.Millisecond,
	3 * time.Second,
	5 * time.Second,
}

// hwProbeFn runs one probe attempt. It returns whether it succeeded, the argv
// it ran (for the diagnostic log) and the tail of ffmpeg's stderr plus the raw
// error for classification.
type hwProbeFn func() (ok bool, argv, stderr string, err error)

// runHWReadinessProbe runs probe once; on a retryable (init/readiness/transient)
// failure it replays the SAME probe at hwReadinessRetryDelays until one succeeds
// or the schedule is exhausted. A deterministic failure returns false at once
// (no retry). Returns whether the probe ultimately succeeded.
func (r *BenchmarkRunner) runHWReadinessProbe(ctx context.Context, step, backend, device string, classify func(error, string) transcoding.FailureClass, probe hwProbeFn) bool {
	ok, argv, stderr, err := probe()
	if ok {
		return true
	}
	first := classify(err, stderr)
	r.logHWInitFailure(step, backend, device, 0, first, argv, stderr, err)
	if !first.RetryableInit() {
		return false // deterministic - fail-close, never retried
	}
	for i, d := range hwReadinessRetryDelays {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(d):
		}
		ok, argv, stderr, err = probe()
		if ok {
			r.logger.Warn("benchmark: HW probe recovered after readiness retry (transient cold-boot init, NOT persisted as unsupported)",
				slog.String("step", step), slog.String("backend", backend),
				slog.Int("retries", i+1), slog.Duration("after_delay", d),
				slog.String("first_failure", string(first)))
			return true
		}
		c := classify(err, stderr)
		r.logHWInitFailure(step, backend, device, i+1, c, argv, stderr, err)
		if !c.RetryableInit() {
			return false // turned deterministic mid-retry - stop
		}
	}
	r.logger.Warn("benchmark: HW probe still failing after readiness retries exhausted - fail-close (route/backend not certified)",
		slog.String("step", step), slog.String("backend", backend),
		slog.Int("retries", len(hwReadinessRetryDelays)), slog.String("first_failure", string(first)))
	return false
}

// logHWInitFailure records one failed calibration probe with everything needed
// to prove, at the next bad boot, whether the render node becomes usable after
// N seconds: the step, backend, attempt index, failure class, the argv, the
// full stderr tail, and a device/ICD environment snapshot.
func (r *BenchmarkRunner) logHWInitFailure(step, backend, device string, attempt int, class transcoding.FailureClass, argv, stderr string, err error) {
	attrs := []any{
		slog.String("step", step),
		slog.String("backend", backend),
		slog.Int("attempt", attempt),
		slog.String("class", string(class)),
		slog.Bool("retryable", class.RetryableInit()),
	}
	if step == "opencl_tonemap" {
		attrs = append(attrs, slog.String("opencl_substep", openCLSubstep(stderr)))
	}
	if err != nil {
		attrs = append(attrs, slog.String("err", err.Error()))
	}
	attrs = append(attrs, hwInitDiagnostics(device)...)
	attrs = append(attrs, slog.String("argv", argv), slog.String("stderr", stderr))
	r.logger.Warn("benchmark: HW init/probe failed", attrs...)
}

// hwInitDiagnostics snapshots the host state a HW init failure needs explaining:
// the render node, the /dev/dri nodes with their permissions and owning uid/gid,
// this process's uid/gid, and the OpenCL ICD vendor files present. Read-only and
// best-effort - a field that cannot be read is reported, never fatal.
func hwInitDiagnostics(device string) []any {
	node := device
	if node == "" {
		node = "/dev/dri/renderD128"
	}
	attrs := []any{
		slog.String("render_node", node),
		slog.Int("proc_uid", os.Getuid()),
		slog.Int("proc_gid", os.Getgid()),
	}
	if entries, derr := os.ReadDir("/dev/dri"); derr == nil {
		var lines []string
		for _, e := range entries {
			info, ierr := e.Info()
			if ierr != nil {
				lines = append(lines, e.Name()+"=?")
				continue
			}
			uid, gid := "?", "?"
			if st, ok := info.Sys().(*syscall.Stat_t); ok {
				uid = strconv.FormatUint(uint64(st.Uid), 10)
				gid = strconv.FormatUint(uint64(st.Gid), 10)
			}
			lines = append(lines, fmt.Sprintf("%s(%s uid=%s gid=%s)", e.Name(), info.Mode().Perm().String(), uid, gid))
		}
		attrs = append(attrs, slog.String("dev_dri", strings.Join(lines, ",")))
	} else {
		attrs = append(attrs, slog.String("dev_dri_err", derr.Error()))
	}
	if icds, gerr := filepath.Glob("/etc/OpenCL/vendors/*.icd"); gerr == nil {
		attrs = append(attrs, slog.String("opencl_icd_vendors", strings.Join(icds, ",")))
	}
	return attrs
}

// openCLSubstep names which stage of the VAAPI->OpenCL tone-map probe failed,
// so the three OpenCL categories the operator must tell apart are explicit in
// the log: the OpenCL runtime/device not coming up, the VAAPI->OpenCL derive/
// interop not initialising, or (elsewhere, in probeHWRoutes) the real route.
func openCLSubstep(stderr string) string {
	s := strings.ToLower(stderr)
	switch {
	case stderrContainsAny(s, "not supported by the device type", "target frames context", "hwmap", "derive"):
		return "derive_vaapi_to_opencl"
	case stderrContainsAny(s, "failed to create opencl", "no opencl", "no platform", "cannot create opencl", "opencl device"):
		return "runtime_device_init"
	default:
		return "unknown"
	}
}

// classifyOpenCLProbe classifies a failure of the VAAPI->OpenCL tone-map probe.
// Unlike a media route this probe runs on a SYNTHETIC frame, so a failure is
// never a (media, route) incompatibility: it is the OpenCL runtime/device not
// coming up or the VAAPI->OpenCL derive/interop not initialising - both
// init/readiness conditions a cold GPU clears on replay (the generic classifier
// would read the derive's trailing "Invalid argument" as route_incompatible and
// refuse to retry, which is exactly the bug this avoids). Only a NAMED
// unsupported option is deterministic here.
func classifyOpenCLProbe(err error, stderr string) transcoding.FailureClass {
	if err == nil {
		return transcoding.FailureNone
	}
	s := strings.ToLower(stderr)
	// Only a NAMED option error is deterministic here. Note "not supported by
	// the device type 'VAAPI'" is NOT in this list: on this fixed synthetic
	// chain that string is the VAAPI->OpenCL derive/init failing, an init/
	// readiness condition, not an option the caller could drop.
	if stderrContainsAny(s,
		"unrecognized option", "unknown option", "option not found",
		"error setting option", "value out of range") {
		return transcoding.FailureHWUnsupportedOption
	}
	return transcoding.FailureHWInit
}

// probeHWEncodeBaseE is the base HW-encode readiness probe: the minimal
// media-independent encode through the backend's H.264 encoder, returning the
// argv and stderr so the readiness retry can classify and log it. This is the
// "base VAAPI/init" gate - if it recovers, the full calibration proceeds.
func (r *BenchmarkRunner) probeHWEncodeBaseE(ctx context.Context, backend, encoder, device string) (bool, string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, hwProbeTimeout)
	defer cancel()
	args := []string{"-hide_banner", "-v", "error", "-nostdin"}
	switch backend {
	case "vaapi":
		if device == "" {
			return false, "", "", fmt.Errorf("vaapi: no render node")
		}
		args = append(args, "-vaapi_device", device, "-f", "lavfi",
			"-i", "testsrc2=size="+hwProbeMire+":rate=10:duration=0.2", "-vf", "format=nv12,hwupload")
	case "qsv":
		args = append(args, "-init_hw_device", "qsv=hw", "-filter_hw_device", "hw", "-f", "lavfi",
			"-i", "testsrc2=size="+hwProbeMire+":rate=10:duration=0.2", "-vf", "hwupload=extra_hw_frames=16,format=qsv")
	default:
		args = append(args, "-f", "lavfi", "-i", "testsrc2=size="+hwProbeMire+":rate=10:duration=0.2")
	}
	args = append(args, "-c:v", encoder, "-f", "null", "-")
	out, err := procfork.CombinedOutput(ctx, procfork.Options{Class: procfork.Probe, Site: "hwdetect.hw_readiness"}, r.ffmpegPath, args...)
	argv := strings.Join(args, " ")
	if err != nil {
		return false, argv, tailString(strings.TrimSpace(string(out)), 400), err
	}
	return true, argv, "", nil
}
