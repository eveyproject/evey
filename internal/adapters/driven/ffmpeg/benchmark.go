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
func (r *BenchmarkRunner) Run(ctx context.Context, pref transcoding.HWPreference) (transcoding.EncodeCapability, error) {
	start := time.Now()
	r.logger.Info("benchmark: starting",
		slog.String("sample", r.samplePath),
		slog.Int("duration_per_rung_sec", r.durationSec),
		slog.Float64("margin", r.margin),
		slog.String("pin_backend", pref.Backend),
		slog.Bool("hw_disabled", pref.DisableHW))

	sample, fallback := r.resolveSample()

	swMax := r.benchSoftware(ctx, sample, fallback)

	// detected lists every accelerator the host advertises, recorded before the
	// preference's backend pin / disable is applied — so the admin can still see
	// "nvenc present" even while the active mode forces software or vaapi.
	detected := r.detectBackends(ctx)

	var (
		hwMax     int
		hwBackend string
		hwCodecs  []string
		hwDevice  string
	)
	// pref.DisableHW (mode=off) calibrates software-only: skip the hardware
	// probe entirely so the host never selects a GPU the operator disabled.
	if !pref.DisableHW {
		hwMax, hwBackend, hwCodecs, hwDevice = r.benchHardware(ctx, sample, fallback, pref.Backend)
	}

	// Probe which optional encoder tuning options the HW encoder accepts, so
	// the arg builder only emits validated ones (level-1 safety). Only when a
	// HW backend passed the realtime bench.
	var hwFeatures []string
	var hwDecodeCodecs []string
	var hw10Bit, hwTonemap bool
	if hwMax > 0 {
		hwFeatures = r.probeHWFeatures(ctx, hwBackend)
		// Probe DECODE support separately — encode-capable ≠ decode-capable
		// (old iGPU encodes H.264 but can't decode HEVC). Drives the pipeline's
		// per-source -hwaccel gating so an undecodable source software-decodes
		// instead of stalling the player.
		hwDecodeCodecs = probeHWDecodeCodecs(ctx, r.ffmpegPath, hwBackend, hwDevice)
		// Probe the 10-bit VPP path separately too: a backend that encodes h264
		// fine can still abort on the p010→nv12 scale a 10-bit (HEVC Main10)
		// source needs (Intel i965 old iGPU VEBOX). Gates 10-bit sources to
		// software so they never crash the HW pipeline.
		hw10Bit = probeHW10BitTranscode(ctx, r.ffmpegPath, hwBackend, hwDevice)
		// Probe GPU tone-mapping (tonemap_vaapi / vpp_qsv / libplacebo). Distinct
		// from the 10-bit VPP: a backend may fold 10-bit yet lack a tone-map filter
		// (or the Vulkan/OpenCL ICD it needs). False -> HDR tone-maps in software.
		hwTonemap = probeHWTonemap(ctx, r.ffmpegPath, hwBackend, hwDevice)
	}

	cap := transcoding.EncodeCapability{
		SoftwareMaxHeight:   swMax,
		HWMaxHeight:         hwMax,
		HWBackend:           hwBackend,
		HWCodecs:            hwCodecs,
		HWDecodeCodecs:      hwDecodeCodecs,
		HW10BitTranscode:    hw10Bit,
		HWTonemap:           hwTonemap,
		HWDevice:            hwDevice,
		GPUClass:            classifyGPU(hwBackend, hwDevice),
		HWFeatures:          hwFeatures,
		DetectedBackends:    detected,
		CPUSignature:        sysinfo.CPUSignature(),
		BenchmarkedAt:       time.Now().UTC(),
		BenchmarkDurationMs: time.Since(start).Milliseconds(),
	}
	r.logger.Info("benchmark: complete",
		slog.Int("software_max_height", cap.SoftwareMaxHeight),
		slog.Int("hw_max_height", cap.HWMaxHeight),
		slog.String("hw_backend", cap.HWBackend),
		slog.Bool("hw_10bit_transcode", cap.HW10BitTranscode),
		slog.Bool("hw_tonemap", cap.HWTonemap),
		slog.String("gpu_class", cap.GPUClass),
		slog.Any("detected_backends", cap.DetectedBackends),
		slog.Any("hw_features", cap.HWFeatures),
		slog.Int64("elapsed_ms", cap.BenchmarkDurationMs))
	return cap, nil
}

// detectBackends returns the accelerator types the host advertises
// ("nvenc"/"vaapi"/"qsv"), independent of the active mode. Best-effort: a
// detector error yields an empty list (the host is treated as software-only
// for display purposes).
func (r *BenchmarkRunner) detectBackends(ctx context.Context) []string {
	devices, err := r.hwDetector.DetectDevices(ctx)
	if err != nil || len(devices) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(devices))
	var out []string
	for _, d := range devices {
		t := strings.ToLower(d.Type)
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// nvencFeatureProbes are the optional NVENC tuning flags we want to use, each
// tested with a tiny throwaway encode. Names MUST match the has-checks in
// hwEncoderQualityArgs — a feature is emitted in production iff it is BOTH
// listed here (probed OK) and gated there. Kept in sync by TestNVENCProbeNames.
var nvencFeatureProbes = []struct {
	name string
	args []string
}{
	{"rc_lookahead", []string{"-rc-lookahead", "20"}},
	{"spatial_aq", []string{"-spatial-aq", "1"}},
	{"temporal_aq", []string{"-temporal-aq", "1"}},
	{"b_ref_mode", []string{"-b_ref_mode", "middle"}},
	{"bframes", []string{"-bf", "3"}},
}

// nvencProbeArgs builds the ffmpeg argv that encodes a fraction of a second of
// a synthetic black source through h264_nvenc WITH the candidate option. No
// real input file and no -hwaccel: NVENC accepts system-memory frames and
// uploads them itself, so the encoder still opens with the option and rejects
// it (non-zero exit) exactly as it would in production.
func nvencProbeArgs(optionArgs []string) []string {
	args := []string{
		"-hide_banner", "-nostats", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=black:s=256x144:r=24:d=0.2",
		"-c:v", "h264_nvenc", "-preset", "p4",
	}
	args = append(args, optionArgs...)
	return append(args, "-f", "null", "-")
}

// probeHWFeatures returns the tuning options the host's HW encoder accepts.
// v1 probes NVENC only — a device-free encode from a synthetic source the
// caller's GPU box can validate directly. VAAPI/QSV option probing needs the
// real device set up like production and is a hardware-validated follow-up, so
// it returns nil here (⇒ the safe HW baseline, no tuning).
// HWBackendUsable confirms the named HW backend can actually encode h264 on this
// host: the encoder must be compiled into ffmpeg AND a tiny real encode must
// succeed. Cheap re-validation guard for a persisted capability whose backend
// may no longer be usable here.
func (r *BenchmarkRunner) HWBackendUsable(ctx context.Context, backend string) bool {
	enc := resolveHWEncoder(backend, "h264")
	if enc == "" {
		return false
	}
	device := ""
	if backend == string(transcoding.VAAPI) || backend == string(transcoding.QSV) {
		device = "/dev/dri/renderD128"
	}
	return checkEncoderAvailable(ctx, r.ffmpegPath, enc) && probeHWEncode(ctx, r.ffmpegPath, enc, backend, device)
}

func (r *BenchmarkRunner) probeHWFeatures(ctx context.Context, backend string) []string {
	if backend != "nvenc" {
		return nil
	}
	var ok []string
	for _, p := range nvencFeatureProbes {
		if r.probeOption(ctx, nvencProbeArgs(p.args)) {
			ok = append(ok, p.name)
			r.logger.Info("benchmark: HW feature supported",
				slog.String("backend", backend), slog.String("feature", p.name))
		} else {
			r.logger.Debug("benchmark: HW feature rejected",
				slog.String("backend", backend), slog.String("feature", p.name))
		}
	}
	return ok
}

// probeOption runs one feature-probe encode; it is supported iff ffmpeg exits 0.
func (r *BenchmarkRunner) probeOption(ctx context.Context, args []string) bool {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, r.ffmpegBin(), args...).Run() == nil
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
		fps, ok := r.runMultiVariant(ctx, sample, fallback, laddered, "libx264", nil, "")
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
func (r *BenchmarkRunner) benchHardware(ctx context.Context, sample string, fallback bool, pinBackend string) (int, string, []string, string) {
	devices, err := r.hwDetector.DetectDevices(ctx)
	if err != nil || len(devices) == 0 {
		return 0, "", nil, ""
	}
	pinBackend = strings.ToLower(strings.TrimSpace(pinBackend))
	for _, dev := range devices {
		// Honour the active mode's backend pin: when set, only the matching
		// backend is considered, so a forced nvenc/vaapi/qsv (or a shared host
		// with one Evey-per-GPU) doesn't pick the detector's preferred backend
		// over the one the operator selected.
		if pinBackend != "" && strings.ToLower(dev.Type) != pinBackend {
			continue
		}
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
		// runLadder probes each rung under the multi-variant pipeline and
		// returns the tallest passing height (0 if none).
		runLadder := func(extraInputArgs []string, uploadChain string) int {
			best := 0
			for i, h := range hwBenchLadder {
				laddered := hwBenchLadder[:i+1]
				fps, ok := r.runMultiVariant(ctx, sample, fallback, laddered, encoder, extraInputArgs, uploadChain)
				passing := ok && fps >= float64(r.sourceRate())*r.margin
				r.logger.Info("benchmark: hardware probe",
					slog.String("backend", dev.Type),
					slog.String("encoder", encoder),
					slog.Int("height", h),
					slog.Int("parallel_encodes", len(laddered)),
					slog.Float64("fps", fps),
					slog.Bool("passing", passing),
					slog.Bool("decoupled", uploadChain != ""))
				if !passing {
					break
				}
				best = h
			}
			return best
		}

		// First measure the production zero-copy path: HW decode → scale → encode.
		best := runLadder(hwInputArgs(dev.Type), "")
		// If nothing passed, the GPU may simply be unable to HW-DECODE the sample
		// codec while still being able to ENCODE — pre-Skylake Intel iGPUs decode
		// H.264 but NOT HEVC, and our sample is HEVC, so `-hwaccel vaapi` fails with
		// ENOSYS before any encode runs. Retry measuring encode only: software-
		// decode + hwupload, the exact shape probeHWEncode already validated.
		// VAAPI/QSV only — NVENC HW-decodes every codec, so it never needs this.
		if best == 0 {
			if up := hwUploadChain(encoder); up != "" {
				best = runLadder(hwDecoupledInputArgs(dev.Type), up)
			}
		}
		if best > 0 {
			return best, dev.Type, hwDomainCodecs(dev.SupportedCodecs), hwDevicePath(dev.Type)
		}
	}
	return 0, "", nil, ""
}

// hwDomainCodecs maps probed encoder names (h264_nvenc, hevc_vaapi, ...) back
// to the domain codec names the handler gates on (h264, hevc, av1), preserving
// order and de-duplicating.
func hwDomainCodecs(supported []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, enc := range supported {
		base := enc
		if i := strings.IndexByte(enc, '_'); i > 0 {
			base = enc[:i]
		}
		if base != "" && !seen[base] {
			seen[base] = true
			out = append(out, base)
		}
	}
	return out
}

// hwDevicePath returns the DRM render node ffmpeg needs for VAAPI/QSV; NVENC
// selects its device through CUDA and needs none.
func hwDevicePath(backend string) string {
	switch transcoding.HWAccelType(backend) {
	case transcoding.VAAPI, transcoding.QSV:
		return "/dev/dri/renderD128"
	}
	return ""
}

// hwInputArgs prepends the input-level HW init flags for each backend.
// Empty for NVENC (which works in pure copy-CPU pipeline too) — and the
// software fallback path inside HW encoders is forgiving.
func hwInputArgs(backend string) []string {
	switch backend {
	case string(transcoding.VAAPI):
		return []string{"-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi", "-vaapi_device", "/dev/dri/renderD128"}
	case string(transcoding.NVENC):
		return []string{"-hwaccel", "cuda", "-hwaccel_output_format", "cuda"}
	case string(transcoding.QSV):
		return []string{"-hwaccel", "qsv", "-hwaccel_output_format", "qsv"}
	}
	return nil
}

// hwDecoupledInputArgs returns the device-binding input flags WITHOUT -hwaccel,
// so ffmpeg software-decodes the sample and the encode path is exercised via an
// explicit hwupload (see hwUploadChain). Used as a fallback when the GPU can't
// HW-decode the sample codec (e.g. pre-Skylake Intel + an HEVC sample). Mirrors
// the device setup probeHWEncode validated. NVENC is absent: it HW-decodes every
// codec, so the zero-copy path always works for it.
func hwDecoupledInputArgs(backend string) []string {
	switch backend {
	case string(transcoding.VAAPI):
		return []string{"-vaapi_device", "/dev/dri/renderD128"}
	case string(transcoding.QSV):
		return []string{"-init_hw_device", "qsv=hw", "-filter_hw_device", "hw"}
	}
	return nil
}

// hwUploadChain is the filter prefix that uploads software-decoded frames to the
// GPU for the decoupled bench path, per encoder family. Empty for NVENC (its
// HW-decode path covers every codec) and for software encoders.
func hwUploadChain(encoder string) string {
	switch {
	case strings.HasSuffix(encoder, "_vaapi"):
		return "format=nv12,hwupload"
	case strings.HasSuffix(encoder, "_qsv"):
		return "hwupload=extra_hw_frames=16,format=qsv"
	}
	return ""
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
func (r *BenchmarkRunner) runMultiVariant(ctx context.Context, sample string, fallback bool, heights []int, encoder string, extraInputArgs []string, uploadChain string) (float64, bool) {
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
	// HW encoders need the matching GPU scaler + a valid preset (NVENC has no
	// x264 'ultrafast') and must NOT pin a host pixel format (frames are GPU
	// surfaces). hwInputArgs already added -hwaccel_output_format for these, so
	// decode→scale→encode stays on the GPU — exactly the production zero-copy
	// path the bench is meant to measure.
	hwScale, hwPreset := "", ""
	switch {
	case strings.HasSuffix(encoder, "_nvenc"):
		hwScale, hwPreset = "scale_cuda", "p4"
	case strings.HasSuffix(encoder, "_vaapi"):
		hwScale = "scale_vaapi" // encoder-default rate control, no preset
	case strings.HasSuffix(encoder, "_qsv"):
		hwScale, hwPreset = "scale_qsv", "veryfast"
	}

	var fcBuf strings.Builder
	// Decoupled path (uploadChain != "") software-decodes then uploads to the GPU
	// before the split, so scale_*/encode run on GPU surfaces without HW decode.
	head := "[0:v]"
	if uploadChain != "" {
		head = "[0:v]" + uploadChain + ","
	}
	fmt.Fprintf(&fcBuf, "%ssplit=%d%s", head, len(heights), strings.Join(splitOutputs, ""))
	for i, h := range heights {
		if hwScale != "" {
			fmt.Fprintf(&fcBuf, ";[v%d]%s=-2:%d[o%d]", i, hwScale, h, i)
		} else {
			fmt.Fprintf(&fcBuf, ";[v%d]scale=-2:%d[o%d]", i, h, i)
		}
	}
	fc := fcBuf.String()
	args = append(args, "-filter_complex", fc)
	for i := range heights {
		args = append(args, "-map", fmt.Sprintf("[o%d]", i), fmt.Sprintf("-c:v:%d", i), encoder)
		if hwScale != "" {
			if hwPreset != "" {
				args = append(args, "-preset", hwPreset)
			}
		} else {
			args = append(args, "-preset", "ultrafast", "-pix_fmt", "yuv420p")
		}
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
