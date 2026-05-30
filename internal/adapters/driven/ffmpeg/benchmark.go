package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
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
		// /data is the project's writable volume in all deployment
		// flavours (docker, k8s, bare). /usr/share is read-only on
		// most container images so we can't generate there.
		opts.SamplePath = "/data/bench/sample.hevc"
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

// hwBenchLadder is the resolution rungs we probe for hardware-encode
// capability. Goes all the way to 2160p — NVENC / QSV / VAAPI can
// reasonably hit it on consumer hardware.
var hwBenchLadder = []int{720, 1080, 1440, 2160}

// swBenchLadder caps software encoding at 1440p. The 4K rung is
// intentionally excluded: even on a CPU that can encode 2160p in
// the synthetic mire (testsrc + noise), real 4K HEVC blu-ray sources
// carry decoder load (B-frame chains, multi-reference motion comp,
// long GOPs) that our generated sample can't fully approximate
// without shipping a copyrighted reference clip. We've seen the
// runtime gap empirically: bench says "2160p OK" but Ghostbusters
// 4K HEVC stalls in production. Until we have HW decode wired (or
// a license-clean real-content sample bundled), 2160p stays a
// HW-only capability. Phase C runtime overrides will lower further
// if even 1440p turns out to be too aggressive on some host.
var swBenchLadder = []int{720, 1080, 1440}

// Ordered low → high so we can short-circuit: as soon as a rung
// fails, the heavier ones can't possibly pass either. Saves
// seconds on weak hosts.

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
// disk, we *generate* a real 4K HEVC mire on the fly via ffmpeg
// (testsrc → libx265 ultrafast, ~5 s of video, ~5 MB) and cache it.
//
// Why a real HEVC sample matters: the original implementation used
// lavfi testsrc as input. lavfi generates frames in-memory, so the
// decoder never runs — the bench measured pure encode throughput.
// But the actual production bottleneck on a host without HW is
// software HEVC *decode* (4K HEVC = ~1× realtime on a CPU), which
// can plummet the effective realtime budget for the encoder. Pure
// encode benches systematically overstate the cap (we saw it
// dev-side: testsrc says 2160p OK, real Ghostbusters 4K HEVC stalls
// at 1080p decode-bound).
//
// First call generates the file; subsequent runs (Recalibrate, next
// boot) reuse it. The file is small enough to ship in the image's
// /var/lib/evey/, but we still generate at first run so the binary
// is self-contained.
//
// Returns the path to use; second value (fallback) is always false
// once generation succeeds — kept as a compatibility seam for
// callers that need to know "are we on synthetic" (legacy callers
// passed true to switch to lavfi). With on-demand generation we
// never need the lavfi branch anymore unless generation itself
// fails.
func (r *BenchmarkRunner) resolveSample() (string, bool) {
	if _, err := os.Stat(r.samplePath); err == nil {
		return r.samplePath, false
	}
	if err := r.generateSample(); err != nil {
		r.logger.Warn("benchmark: HEVC sample generation failed, falling back to synthetic testsrc",
			slog.String("path", r.samplePath),
			slog.Any("err", err))
		return "", true
	}
	r.logger.Info("benchmark: generated HEVC mire sample",
		slog.String("path", r.samplePath))
	return r.samplePath, false
}

// generateSample produces a ~5 s 3840x2160 HEVC file at samplePath.
// Encoded with libx265 `medium` preset + explicit B-frame chain so
// the decoder side exercises B-frame reordering + multi-reference
// motion comp — the load profile a real Blu-ray rip would have.
// (An earlier draft used `ultrafast` here, which produced a file with
// no B-frames; the decoder ran 3-5× faster than on real content and
// the bench cap was systematically optimistic.)
//
// Slow on weak hosts (~30-60 s at preset medium 4K) but only runs
// once per machine — the file is reused across reboots and
// recalibrations until Recalibrate is forced.
func (r *BenchmarkRunner) generateSample() error {
	if err := os.MkdirAll(filepath.Dir(r.samplePath), 0o755); err != nil {
		return fmt.Errorf("mkdir bench dir: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	// We layer noise on top of testsrc so x265 can't compress to a
	// trivial size — without `geq` the lavfi pattern is too regular
	// and the resulting .hevc is < 100 KB (x265 finds it fully
	// predictable), which means the decoder runs in pure cache and
	// the bench measures nothing meaningful. With noise the file
	// lands around 10-20 MB at the target bitrate, similar to a
	// real 4K Blu-ray.
	cmd := exec.CommandContext(ctx, r.ffmpegBin(),
		"-hide_banner", "-nostats", "-y",
		"-f", "lavfi", "-i",
		"testsrc=duration=5:size=3840x2160:rate=24,format=yuv420p,"+
			"noise=alls=20:allf=t+u",
		"-c:v", "libx265", "-preset", "medium",
		// bframes=4 + ref=3 = decoder load profile of a typical 4K
		// Blu-ray encoder. -b:v with a real cap forces x265 to spend
		// the bits even on the easy parts of the input.
		"-x265-params", "bframes=4:ref=3:b-adapt=2:rc-lookahead=20",
		"-b:v", "20M", "-maxrate", "25M", "-bufsize", "50M",
		"-tag:v", "hvc1",
		"-an",
		r.samplePath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg generate sample: %w (tail: %s)", err, tailString(string(out), 200))
	}
	return nil
}

// ffmpegBin returns the resolved ffmpeg binary path, falling back to
// "ffmpeg" on PATH when none configured.
func (r *BenchmarkRunner) ffmpegBin() string {
	if r.ffmpegPath != "" {
		return r.ffmpegPath
	}
	return "ffmpeg"
}

// benchSoftware probes the libx264 ultrafast ceiling under the same
// pipeline shape the production HLS transcoder uses: a single ffmpeg
// instance decodes the HEVC source once, splits the decoded stream N
// ways via filter_complex, and encodes every active rung in parallel
// (`buildMultiVariantArgs` in hls_transcoder.go uses var_stream_map).
// Probing only "decode + one encode at rung H" would systematically
// overshoot — the real CPU pressure is decode + N encodes, where N is
// the number of rungs at or below H.
//
// Highest passing rung wins. Returns 720 minimum (universal floor) even
// if every probe fails — guarantees the host can at least serve a 720p
// stream.
func (r *BenchmarkRunner) benchSoftware(ctx context.Context, sample string, fallback bool) int {
	best := 0
	for i, h := range swBenchLadder {
		// Heights for every rung at or below h — exactly what the
		// production pipeline encodes when the ladder top is h.
		laddered := swBenchLadder[:i+1]
		fps, ok := r.runMultiVariant(ctx, sample, fallback, laddered, "libx264", nil)
		passing := ok && fps >= float64(r.sourceRate())*r.margin
		r.logger.Info("benchmark: software probe",
			slog.Int("height", h),
			slog.Int("parallel_encodes", len(laddered)),
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
// and probes its ladder under the same multi-variant pipeline as the
// software path (1 decode + N parallel encodes, matching prod).
// Returns (0, "") when no HW is usable — software-only operation,
// fully supported by the handler.
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
		for i, h := range hwBenchLadder {
			laddered := hwBenchLadder[:i+1]
			fps, ok := r.runMultiVariant(ctx, sample, fallback, laddered, encoder, extraInputArgs)
			passing := ok && fps >= float64(r.sourceRate())*r.margin
			r.logger.Info("benchmark: hardware probe",
				slog.String("backend", dev.Type),
				slog.String("encoder", encoder),
				slog.Int("height", h),
				slog.Int("parallel_encodes", len(laddered)),
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

// runMultiVariant runs ffmpeg with the same pipeline shape as the
// production HLS transcoder: 1 decode + filter_complex split into N
// branches + N parallel scaler+encoder chains. The FPS we report is
// the OVERALL throughput (`frame=` count from ffmpeg's progress, which
// is the encoded frames in the slowest output, divided by wall-clock).
// That number is what realistically caps the production pipeline:
// the parallel encodes share decode bandwidth, scale CPU, and core
// allocation, so a host that can do 1×4K encode in realtime won't
// necessarily do 1 decode + 4 parallel encodes in realtime.
//
// Returns (0, false) on any failure so the caller treats it as
// "rung failed". Output goes to /dev/null per branch — we measure
// throughput, not segments.
func (r *BenchmarkRunner) runMultiVariant(ctx context.Context, sample string, fallback bool, heights []int, encoder string, extraInputArgs []string) (float64, bool) {
	if len(heights) == 0 {
		return 0, false
	}
	// Generous timeout: a host close to realtime might take
	// (durationSec / margin) wall-clock to encode, plus startup
	// overhead. Triple the duration leaves room for jitter without
	// running forever on a fully-stuck host.
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

	// Build filter_complex: [0:v]split=N[v0][v1]...; [v0]scale=-2:H0[o0]; ...
	// Mirrors buildMultiVariantArgs in hls_transcoder.go so the bench
	// shape matches production exactly.
	splitOutputs := make([]string, len(heights))
	for i := range heights {
		splitOutputs[i] = fmt.Sprintf("[v%d]", i)
	}
	var fcBuf strings.Builder
	fmt.Fprintf(&fcBuf, "[0:v]split=%d%s", len(heights), strings.Join(splitOutputs, ""))
	for i, h := range heights {
		fmt.Fprintf(&fcBuf, ";[v%d]scale=-2:%d[o%d]", i, h, i)
	}
	fc := fcBuf.String()
	args = append(args, "-filter_complex", fc)
	for i := range heights {
		args = append(args,
			"-map", fmt.Sprintf("[o%d]", i),
			fmt.Sprintf("-c:v:%d", i), encoder,
			"-preset", "ultrafast",
			"-pix_fmt", "yuv420p",
		)
	}
	// Single null output for the whole graph — ffmpeg muxes all the
	// mapped streams together (one per rung). We don't care about the
	// container, just throughput.
	args = append(args, "-an", "-sn", "-f", "null", "-")

	cmd := exec.CommandContext(runCtx, r.ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	startedAt := time.Now()
	err := cmd.Run()
	wall := time.Since(startedAt).Seconds()
	if err != nil {
		r.logger.Debug("benchmark: runMultiVariant failed",
			slog.String("encoder", encoder),
			slog.Any("heights", heights),
			slog.Any("err", err),
			slog.String("stderr_tail", tailString(stderr.String(), 300)))
		return 0, false
	}
	if wall <= 0 {
		return 0, false
	}
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
