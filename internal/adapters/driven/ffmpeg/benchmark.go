package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/sysinfo"
)

// BenchmarkRunner measures the host's realtime encode ceiling for both
// the software (libx264 ultrafast) and hardware (NVENC / QSV / VAAPI)
// paths. Implementation of domain transcoding.BenchmarkRunner.
//
// Probes a fixed ladder of {720, 1080, 1440, 2160}p heights on the
// project's bundled sample (or a synthetic testsrc fallback when the
// sample is missing). For each height we encode ~5 s of video and
// measure the wall-clock; FPS = frames / wall. A height passes when
// FPS >= sampleRate × realtimeMargin (1.2 default — 20 % margin over
// realtime so the host doesn't crash on slightly heavier sources than
// the bench).
//
// Sample mire path is configurable to let tests use a tiny generated
// file instead of bundling 5 MB into the binary for unit tests.
type BenchmarkRunner struct {
	ffmpegPath  string
	samplePath  string
	hwDetector  *HWDetector
	logger      *slog.Logger
	durationSec int     // per-height bench duration; 5 s by default
	margin      float64 // FPS / realtime ratio required to pass (1.2 default)
}

// BenchmarkOpts tweaks the runner. Zero values fall back to defaults.
type BenchmarkOpts struct {
	FfmpegPath  string  // empty → "ffmpeg" on PATH
	SamplePath  string  // empty → "/usr/share/evey/bench.mkv" then testsrc fallback
	DurationSec int     // empty → 5
	Margin      float64 // empty → 1.2
}

func NewBenchmarkRunner(opts BenchmarkOpts, logger *slog.Logger) *BenchmarkRunner {
	if opts.FfmpegPath == "" {
		opts.FfmpegPath = "ffmpeg"
	}
	if opts.SamplePath == "" {
		opts.SamplePath = "/usr/share/evey/bench.mkv"
	}
	if opts.DurationSec == 0 {
		opts.DurationSec = 5
	}
	if opts.Margin == 0 {
		opts.Margin = 1.2
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &BenchmarkRunner{
		ffmpegPath:  opts.FfmpegPath,
		samplePath:  opts.SamplePath,
		hwDetector:  NewHWDetector(opts.FfmpegPath),
		logger:      logger,
		durationSec: opts.DurationSec,
		margin:      opts.Margin,
	}
}

// benchLadder is the resolution rungs we probe. Ordered low → high so
// we can short-circuit: as soon as a rung fails, the heavier ones can't
// possibly pass either, so we stop. Saves seconds on weak hosts.
var benchLadder = []int{720, 1080, 1440, 2160}

// Run executes the full benchmark and returns a fully-populated
// EncodeCapability. Best-effort: a partial failure (HW probe crashed,
// sample unreadable) downgrades the calibration rather than aborting it.
func (r *BenchmarkRunner) Run(ctx context.Context) (transcoding.EncodeCapability, error) {
	start := time.Now()
	r.logger.Info("benchmark: starting",
		slog.String("sample", r.samplePath),
		slog.Int("duration_per_rung_sec", r.durationSec),
		slog.Float64("margin", r.margin))

	sample, fallback := r.resolveSample()

	swMax := r.benchSoftware(ctx, sample, fallback)

	hwMax, hwBackend := r.benchHardware(ctx, sample, fallback)

	cap := transcoding.EncodeCapability{
		SoftwareMaxHeight:   swMax,
		HWMaxHeight:         hwMax,
		HWBackend:           hwBackend,
		CPUSignature:        sysinfo.CPUSignature(),
		BenchmarkedAt:       time.Now().UTC(),
		BenchmarkDurationMs: time.Since(start).Milliseconds(),
	}
	r.logger.Info("benchmark: complete",
		slog.Int("software_max_height", cap.SoftwareMaxHeight),
		slog.Int("hw_max_height", cap.HWMaxHeight),
		slog.String("hw_backend", cap.HWBackend),
		slog.Int64("elapsed_ms", cap.BenchmarkDurationMs))
	return cap, nil
}

// resolveSample picks the bench input. When the bundled sample isn't on
// disk (dev container without /usr/share/evey/bench.mkv installed), we
// fall back to a synthetic testsrc — flagged so the caller can note the
// caveat in logs (testsrc has regular patterns that libx264 compresses
// better than real footage, so the resulting cap may be slightly
// optimistic).
func (r *BenchmarkRunner) resolveSample() (string, bool) {
	if _, err := os.Stat(r.samplePath); err == nil {
		return r.samplePath, false
	}
	r.logger.Warn("benchmark: bundled sample missing, using synthetic testsrc",
		slog.String("expected_path", r.samplePath))
	return "", true // empty path signals "use lavfi testsrc"
}

// benchSoftware probes the libx264 ultrafast ceiling. Highest passing
// rung wins. Returns 720 minimum (universal floor) even if every probe
// fails — guarantees the host can at least serve a 720p stream.
func (r *BenchmarkRunner) benchSoftware(ctx context.Context, sample string, fallback bool) int {
	best := 0
	for _, h := range benchLadder {
		fps, ok := r.runOnce(ctx, sample, fallback, h, "libx264", nil)
		passing := ok && fps >= float64(r.sourceRate())*r.margin
		r.logger.Info("benchmark: software probe",
			slog.Int("height", h),
			slog.Float64("fps", fps),
			slog.Bool("passing", passing))
		if !passing {
			break // ladder is monotone — no point probing 1440p once 1080p fails
		}
		best = h
	}
	if best == 0 {
		// Even 720p failed (very weak host, or ffmpeg crashed on every
		// probe). Stay at the universal 720p floor so playback still
		// works — the host may overshoot realtime occasionally but the
		// runtime throttle will catch it.
		r.logger.Warn("benchmark: software probe failed on every rung, falling back to 720p floor")
		return 720
	}
	return best
}

// benchHardware picks the HW backend with the most encoders available
// and probes its ladder the same way. Returns (0, "") when no HW is
// usable — software-only operation, fully supported by the handler.
func (r *BenchmarkRunner) benchHardware(ctx context.Context, sample string, fallback bool) (int, string) {
	devices, err := r.hwDetector.DetectDevices(ctx)
	if err != nil || len(devices) == 0 {
		return 0, ""
	}
	for _, dev := range devices {
		// Pick the first device that advertises an h264 encoder. NVENC
		// preferred over VAAPI/QSV when both are present (production
		// box on a workstation), which the detector orders naturally.
		var encoder string
		for _, c := range dev.SupportedCodecs {
			if strings.HasPrefix(c, "h264_") {
				encoder = c
				break
			}
		}
		if encoder == "" {
			continue
		}
		extraInputArgs := hwInputArgs(dev.Type)
		best := 0
		for _, h := range benchLadder {
			fps, ok := r.runOnce(ctx, sample, fallback, h, encoder, extraInputArgs)
			passing := ok && fps >= float64(r.sourceRate())*r.margin
			r.logger.Info("benchmark: hardware probe",
				slog.String("backend", dev.Type),
				slog.String("encoder", encoder),
				slog.Int("height", h),
				slog.Float64("fps", fps),
				slog.Bool("passing", passing))
			if !passing {
				break
			}
			best = h
		}
		if best > 0 {
			return best, dev.Type
		}
	}
	return 0, ""
}

// hwInputArgs prepends the input-level HW init flags for each backend.
// Empty for NVENC (which works in pure copy-CPU pipeline too) — and the
// software fallback path inside HW encoders is forgiving.
func hwInputArgs(backend string) []string {
	switch backend {
	case string(transcoding.VAAPI):
		return []string{"-hwaccel", "vaapi", "-vaapi_device", "/dev/dri/renderD128"}
	case string(transcoding.NVENC):
		return []string{"-hwaccel", "cuda"}
	case string(transcoding.QSV):
		return []string{"-hwaccel", "qsv"}
	}
	return nil
}

// frameRE matches the running ffmpeg progress line. ffmpeg prints
// `frame=  72 fps=0.0 ...` once at the END of the encode (with
// -nostats the running updates are suppressed, but the terminal line
// is still emitted). We extract the frame count, divide by Go-side
// wall-clock = real FPS.
var frameRE = regexp.MustCompile(`frame=\s*([0-9]+)`)

// runOnce encodes durationSec of input at the requested height with
// the requested encoder; returns the measured encode FPS
// (= frame_count / wall_clock_seconds). Returns (0, false) on any
// error so the caller treats it as "rung failed".
//
// FPS is computed Go-side because `-nostats` disables ffmpeg's running
// `fps=N.N` field and the terminal value with the flag set is always
// `0.0`. The `frame=N` count is reliable in both modes.
//
// Output discarded to /dev/null — we measure throughput, not quality.
func (r *BenchmarkRunner) runOnce(ctx context.Context, sample string, fallback bool, height int, encoder string, extraInputArgs []string) (float64, bool) {
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(r.durationSec*3)*time.Second+10*time.Second)
	defer cancel()

	args := []string{"-hide_banner", "-nostats"}
	args = append(args, extraInputArgs...)
	if fallback {
		args = append(args, "-f", "lavfi", "-i",
			fmt.Sprintf("testsrc=duration=%d:size=3840x2160:rate=24", r.durationSec))
	} else {
		args = append(args, "-t", fmt.Sprintf("%d", r.durationSec), "-i", sample)
	}
	args = append(args,
		"-vf", fmt.Sprintf("scale=-2:%d", height),
		"-c:v", encoder,
		"-preset", "ultrafast",
		"-an", "-sn",
		"-f", "null", "-",
	)

	cmd := exec.CommandContext(runCtx, r.ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	startedAt := time.Now()
	err := cmd.Run()
	wall := time.Since(startedAt).Seconds()
	if err != nil {
		r.logger.Debug("benchmark: runOnce failed",
			slog.String("encoder", encoder),
			slog.Int("height", height),
			slog.Any("err", err),
			slog.String("stderr_tail", tailString(stderr.String(), 300)))
		return 0, false
	}
	if wall <= 0 {
		return 0, false
	}
	// Last `frame=N` line is the final count.
	matches := frameRE.FindAllStringSubmatch(stderr.String(), -1)
	if len(matches) == 0 {
		return 0, false
	}
	last := matches[len(matches)-1]
	if len(last) < 2 {
		return 0, false
	}
	frames, err := strconv.Atoi(last[1])
	if err != nil || frames <= 0 {
		return 0, false
	}
	return float64(frames) / wall, true
}

// sourceRate returns the assumed source frame-rate against which we
// compare encode FPS. testsrc is 24, the bundled mire is 24 — anything
// else would force us to probe the sample, which adds cost for no
// material improvement on the cap decision.
func (r *BenchmarkRunner) sourceRate() int {
	return 24
}
