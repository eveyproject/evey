package ffmpeg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/pkg/procfork"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/hdr"
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
// FPS >= sampleRate × realtimeMargin (1.2 default - 20 % headroom over realtime).
// The cap need not be over-conservative: the auto-degrade ABR ladder is the real
// safety net (Auto starts at the top rung and steps DOWN on a genuine stall), so
// a small margin is enough and a higher one would needlessly drop a rung the box
// actually sustains (e.g. 1080p HDR runs ~1.37x realtime here - comfortably).
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
	// benchSamples is how many times each SOFTWARE rung is probed; the MEDIAN fps
	// decides pass/fail. A single probe is fragile: a recalibration triggered while
	// the box is momentarily busy (a background scan tick, a GC pause) under-measures
	// and pins the cap a rung too low (the "recalibrate dropped me to 720p" bug). The
	// median of an odd N rejects a lone slow outlier. Default 3; tests set 1 to keep
	// the synthetic-clip ladder fast. HW rungs stay single-probe (GPU throughput is
	// stable). The idle-gate on Recalibrate is the complementary guard against
	// SUSTAINED load (median can't rescue a box that is busy for every sample).
	benchSamples int
	// singleVariant mirrors the runtime orchestrator mode. When true the
	// software ladder is probed one rung at a time (decode + tonemap + ONE
	// encode at height h) - the shape a single-variant session actually runs
	// - instead of the cumulative parallel ladder. The two shapes have very
	// different CPU pressure: an old box that can't sustain decode + 4 parallel
	// encodes at 1080p HDR can comfortably sustain decode + ONE 1080p encode.
	// Benching the wrong shape pins the cap far below what the host serves.
	singleVariant bool

	// sdrSrc / hdrSrc locate the bench inputs (operator override > pinned
	// download > synthetic generator). Empty zero values keep the legacy
	// behaviour: generate the synthetic clip on first run.
	sdrSrc benchSampleSource
	hdrSrc benchSampleSource
}

// benchSampleSource locates one calibration input. Resolution order (see
// resolveBenchSample): an operator-provided file (overridePath), then a
// previously-cached file, then a SHA256-pinned download (url+sha256), then the
// on-the-fly synthetic clip. A real-content clip calibrates far more honestly
// than the synthetic - the synthetic over-states decode+encode cost - so a
// pinned reference clip is preferred when configured, with the synthetic as the
// always-available airgapped fallback.
type benchSampleSource struct {
	overridePath string // EVEY_BENCH_*_SAMPLE: a real file the operator dropped in
	url          string // EVEY_BENCH_*_SAMPLE_URL: pinned reference clip (https)
	sha256       string // EVEY_BENCH_*_SAMPLE_SHA256: required when url is set
}

// BenchmarkOpts tweaks the runner. Zero values fall back to defaults.
type BenchmarkOpts struct {
	FfmpegPath  string  // empty → "ffmpeg" on PATH
	SamplePath  string  // empty → "/usr/share/evey/bench.mkv" then testsrc fallback
	DurationSec int     // empty → 5
	Margin      float64 // empty → 1.2
	// BenchSamples is how many times each software rung is probed; the median fps
	// decides pass/fail (robustness against a momentarily-loaded recalibration).
	// Empty → 3. Set 1 in tests to keep the ladder fast.
	BenchSamples int
	// SingleVariant tells the bench to model single-rung on-demand encoding
	// (the runtime default, EVEY_TRANSCODING_SINGLE_VARIANT). Pass
	// cfg.Transcoding.SingleVariant so the cap is derived against the same
	// pipeline shape the runtime executes. Defaults to false (legacy parallel
	// ladder) to stay byte-identical for callers that don't set it.
	SingleVariant bool

	// Bench sample sources: point the bench at a real reference clip instead of
	// the synthetic. Zero values fall back to synthetic generation. Wire from
	// cfg.Transcoding.Bench* so an operator can calibrate against real content
	// (their own file, or a pinned license-clean reference clip).
	//   *SamplePath   EVEY_BENCH_*_SAMPLE        - a real file dropped in
	//   *SampleURL    EVEY_BENCH_*_SAMPLE_URL    - pinned reference clip (https)
	//   *SampleSHA256 EVEY_BENCH_*_SAMPLE_SHA256 - required when URL is set
	SDRSamplePath, SDRSampleURL, SDRSampleSHA256 string
	HDRSamplePath, HDRSampleURL, HDRSampleSHA256 string
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
	if opts.BenchSamples <= 0 {
		opts.BenchSamples = 3
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &BenchmarkRunner{
		ffmpegPath:    opts.FfmpegPath,
		samplePath:    opts.SamplePath,
		hwDetector:    NewHWDetector(opts.FfmpegPath),
		logger:        logger,
		durationSec:   opts.DurationSec,
		margin:        opts.Margin,
		benchSamples:  opts.BenchSamples,
		singleVariant: opts.SingleVariant,
		sdrSrc: benchSampleSource{
			overridePath: opts.SDRSamplePath,
			url:          opts.SDRSampleURL,
			sha256:       opts.SDRSampleSHA256,
		},
		hdrSrc: benchSampleSource{
			overridePath: opts.HDRSamplePath,
			url:          opts.HDRSampleURL,
			sha256:       opts.HDRSampleSHA256,
		},
	}
}

// hwBenchLadder is the resolution rungs we probe for hardware-encode
// capability. Goes all the way to 2160p - NVENC / QSV / VAAPI can
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

// swBenchLadderSpec is one declarative software probe ladder: the class label
// (shared with the domain's Derive grouping - transcoding.LadderSoftware...), the
// encoder, the realtime-margin multiplier applied to the base margin, the floor to
// fall back to when no rung passes, and whether it runs on the 10-bit HDR sample
// through the production tonemap. The boot benchmark WALKS this table instead of a
// hand-written bench function per class, so adding a ladder, changing a margin, or
// swapping an encoder is a DATA edit here (the plan's "ladder déclaratif").
type swBenchLadderSpec struct {
	label      string
	encoder    string
	marginMult float64 // effective margin = base margin × marginMult
	floor      int     // returned height when no rung passes
	hdr        bool    // true ⇒ probe on the HDR sample with the tonemap pre-filter
}

// swBenchLadderSpecs is the software half of the bench ladder, as data. HDR is
// heavier (10-bit decode + tonemap) so it floors higher (480); AV1 (libsvtav1) is
// several times heavier than x264 and must clear a higher margin, with floor 0
// meaning "not viable ⇒ hardware-only". The hardware ladder stays separate
// (benchHardware) because it probes per-device, not per-declared-encoder.
var swBenchLadderSpecs = []swBenchLadderSpec{
	{label: transcoding.LadderSoftware, encoder: "libx264", marginMult: 1, floor: 720},
	{label: transcoding.LadderSoftwareHDR, encoder: "libx264", marginMult: 1, floor: 480, hdr: true},
	{label: transcoding.LadderSoftwareAV1, encoder: "libsvtav1", marginMult: av1RealtimeMarginMultiplier, floor: 0},
}

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

	sample, fallback := r.resolveSample(ctx)

	// Walk the declarative software ladder table (plain SDR, HDR->SDR tonemap,
	// AV1). Each spec names its encoder, margin and floor; the HDR spec pulls the
	// 10-bit sample + tonemap pre-filter. Probing HDR separately keeps the HDR cap
	// honest on a weak NAS (10-bit decode + tonemap it can't hide), and AV1 gets
	// its own far-higher margin so it stays hardware-only unless the box has real
	// spare capacity. Returns one ceiling per label + every rung's samples.
	swCeilings, swSamples := r.benchSoftwareLadders(ctx, sample, fallback)

	// detected lists every accelerator the host advertises. Skipped entirely
	// when HW is disabled (mode=off): probing the GPU - including the real-encode
	// verification probe - wastes CPU and spams "hwdetect: probe failed" on a
	// box whose operator explicitly turned HW off (e.g. an old iGPU that
	// can't HEVC-decode). No detection, no probe, no noise when off.
	var detected []string
	if !pref.DisableHW {
		detected = r.detectBackends(ctx)
	}

	var (
		hwMax     int
		hwBackend string
		hwCodecs  []string
		hwDevice  string
		hwSamples []transcoding.BenchRungSample
	)
	// pref.DisableHW (mode=off) calibrates software-only: skip the hardware
	// probe entirely so the host never selects a GPU the operator disabled.
	if !pref.DisableHW {
		hwMax, hwBackend, hwCodecs, hwDevice, hwSamples = r.benchHardware(ctx, sample, fallback, pref.Backend)
	}

	// Probe which optional encoder tuning options the HW encoder accepts, so
	// the arg builder only emits validated ones (level-1 safety). Only when a
	// HW backend passed the realtime bench.
	var hwFeatures []string
	var hwDecodeCodecs []string
	var hwRoutes []string
	var hw10Bit, hwTonemap, hwOverlay bool
	var hwHDR10 bool
	if hwMax > 0 {
		hwFeatures = r.probeHWFeatures(ctx, hwBackend, hwDevice, hwDomainCodecs(hwCodecs))
		// Probe DECODE support separately - encode-capable ≠ decode-capable
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
		if hwTonemap && probeHWTonemapOpenCL(ctx, r.ffmpegPath, hwBackend, hwDevice) {
			hwFeatures = append(hwFeatures, transcoding.FeatureTonemapOpenCL)
			r.logger.Info("benchmark: HW feature supported",
				slog.String("backend", hwBackend), slog.String("feature", transcoding.FeatureTonemapOpenCL))
		}
		// Probe the GPU subtitle compositor last, and separately again: a backend
		// that decodes, folds 10-bit and tone-maps can still lack a working
		// overlay VPP. It is the capability that decides whether a burn-in stays
		// on the GPU at all - see EncodeCapability.HWOverlay.
		hwOverlay = probeHWOverlay(ctx, r.ffmpegPath, hwBackend, hwDevice)
		if slices.Contains(hwDomainCodecs(hwCodecs), "hevc") {
			hwHDR10 = probeHWHDR10Encode(ctx, r.ffmpegPath, hwBackend, hwDevice)
		}
		// No GPU tone-mapper: HDR runs the CPU tone-map bridged between the GPU
		// decode and encode. Measure that shape so its ladder gets its own ceiling.
		if !hwTonemap {
			hwSamples = append(hwSamples, r.benchHWBridge(ctx, hwBackend, hwDomainCodecs(hwCodecs))...)
		}
		// Last: every route the builder can emit for this backend, run for real
		// (BuildFFmpegArgs + the HLS muxer) on a clip of its class. The filter
		// probes above say what the GPU can do; this says what a SESSION does.
		hwRoutes = r.probeHWRoutes(ctx, hwBackend, hwDevice, hwDomainCodecs(hwCodecs), hwFeatures, hwTonemap, hw10Bit, hwHDR10, hwDecodeCodecs)
	}

	// What the encode rungs above were DECODED from, and how fast this host
	// decodes each codec. Together they let admission price a session by the
	// source the viewer asked for instead of by the output rung alone - the
	// dimension that separates a 1080p H.264 stream from a 4K HEVC one.
	benchSource, decodeSamples := r.decodeEvidence(ctx, sample, fallback)

	cap := transcoding.EncodeCapability{
		SoftwareMaxHeight:    swCeilings[transcoding.LadderSoftware],
		SoftwareHDRMaxHeight: swCeilings[transcoding.LadderSoftwareHDR],
		SoftwareAV1MaxHeight: swCeilings[transcoding.LadderSoftwareAV1],
		HWMaxHeight:          hwMax,
		HWBackend:            hwBackend,
		HWCodecs:             hwCodecs,
		HWDecodeCodecs:       hwDecodeCodecs,
		HW10BitTranscode:     hw10Bit,
		HWTonemap:            hwTonemap,
		HWOverlay:            hwOverlay,
		HWHDR10Encode:        hwHDR10,
		ProbeVersion:         transcoding.CapabilityProbeVersion,
		HWDevice:             hwDevice,
		GPUClass:             classifyGPU(hwBackend, hwDevice),
		HWFeatures:           hwFeatures,
		HWRoutes:             hwRoutes,
		DetectedBackends:     detected,
		CPUSignature:         sysinfo.CPUSignature(),
		BenchmarkedAt:        time.Now().UTC(),
		BenchmarkDurationMs:  time.Since(start).Milliseconds(),
		// SourceRate is the fps the samples were measured against - proof the
		// domain's Derive needs to RE-derive each rung's pass/fail under a
		// (possibly changed) policy margin, instead of the bench-time one.
		SourceRate: r.sourceRate(),
		// Per-rung evidence (fps vs realtime threshold) behind the ceilings above,
		// surfaced in the admin calibration API so the operator can see WHY the
		// HDR cap floored to 480p instead of guessing. Includes the hardware ladder
		// (Ladder=="hw") when a GPU backend was probed, so the panel shows the HW
		// fps too, not just software.
		BenchSamples: append(swSamples, hwSamples...),
		// The decode half of the same proof (see EncodeCapability.DecodeSamples).
		BenchSource:   benchSource,
		DecodeSamples: decodeSamples,
	}
	// Per-route single-session cost evidence: what each route the host serves
	// actually cost, warm and route-faithful. Admission prices on this instead
	// of the cumulative HW rung above (which stays, diagnostic only). Measured
	// last, on the samples already resolved, so it adds no clip generation.
	cap.RouteSamples = r.measureRouteEvidence(ctx, sample, fallback, &cap)
	sortRouteSamples(cap.RouteSamples)
	r.logger.Info("benchmark: complete",
		slog.Int("software_max_height", cap.SoftwareMaxHeight),
		slog.Int("software_hdr_max_height", cap.SoftwareHDRMaxHeight),
		slog.Int("software_av1_max_height", cap.SoftwareAV1MaxHeight),
		slog.Int("hw_max_height", cap.HWMaxHeight),
		slog.String("hw_backend", cap.HWBackend),
		slog.Bool("hw_10bit_transcode", cap.HW10BitTranscode),
		slog.Bool("hw_tonemap", cap.HWTonemap),
		slog.Bool("hw_overlay", cap.HWOverlay),
		slog.String("gpu_class", cap.GPUClass),
		slog.Any("detected_backends", cap.DetectedBackends),
		slog.Any("hw_features", cap.HWFeatures),
		slog.Any("hw_routes", cap.HWRoutes),
		slog.Any("hw_decode_codecs", cap.HWDecodeCodecs),
		slog.String("bench_source_codec", cap.BenchSource.Codec),
		slog.Int("bench_source_height", cap.BenchSource.Height),
		slog.Int("decode_samples", len(cap.DecodeSamples)),
		slog.Int("route_samples", len(cap.RouteSamples)),
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
// hwEncoderQualityArgs - a feature is emitted in production iff it is BOTH
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
// v1 probes NVENC only - a device-free encode from a synthetic source the
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

// HWEncodeUsable is the media-independent proof a runtime failure must clear
// before the host is demoted: a synthetic encode through the backend's encoder
// for codec, with the named tuning features rendered exactly as production
// renders them (hwEncoderQualityArgs). Fail-closed.
func (r *BenchmarkRunner) HWEncodeUsable(ctx context.Context, backend, device, codec string, features []string) bool {
	codec = strings.ToLower(strings.TrimSpace(codec))
	if codec == "" {
		codec = "h264"
	}
	enc := resolveHWEncoder(backend, codec)
	if enc == "" || !checkEncoderAvailable(ctx, r.ffmpegPath, enc) {
		return false
	}
	if device == "" {
		device = hwDevicePath(backend)
	}
	return probeHWEncodeWith(ctx, r.ffmpegPath, enc, backend, device, hwEncoderQualityArgs(transcoding.HWAccelType(backend), codec, features))
}

// ConfirmHWFeatures re-probes each feature on its own and keeps the ones the
// backend still accepts, so a runtime "unsupported option" narrows the host to
// what it just proved instead of wiping every tuning flag on one session's word.
func (r *BenchmarkRunner) ConfirmHWFeatures(ctx context.Context, backend, device, codec string, features []string) []string {
	kept := make([]string, 0, len(features))
	for _, f := range features {
		if r.HWEncodeUsable(ctx, backend, device, codec, []string{f}) {
			kept = append(kept, f)
		}
	}
	return kept
}

func (r *BenchmarkRunner) probeHWFeatures(ctx context.Context, backend, device string, codecs []string) []string {
	switch backend {
	case "nvenc":
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
	case "vaapi", "qsv":
		return r.probeIntelFeatures(ctx, backend, device, codecs)
	}
	return nil
}

// probeIntelFeatures: low-power (VDEnc, needs i915.enable_guc=2) per codec, then B-frames on top.
func (r *BenchmarkRunner) probeIntelFeatures(ctx context.Context, backend, device string, codecs []string) []string {
	var ok []string
	for _, codec := range codecs {
		enc := resolveHWEncoder(backend, codec)
		if enc == "" {
			continue
		}
		lowPower := probeHWEncodeWith(ctx, r.ffmpegPath, enc, backend, device, []string{"-low_power", "1"})
		if lowPower {
			ok = append(ok, transcoding.LowPowerFeature(codec))
			r.logger.Info("benchmark: HW feature supported",
				slog.String("backend", backend), slog.String("feature", transcoding.LowPowerFeature(codec)))
		}
		// B-frames are probed per codec AND per encoder mode: on Gen9 the normal
		// encoder has them, the low-power one does not, and ffmpeg never fails.
		if backend == "vaapi" {
			modes := []bool{false}
			if lowPower {
				modes = append(modes, true)
			}
			for _, lp := range modes {
				var mode []string
				if lp {
					mode = []string{"-low_power", "1"}
				}
				if probeHWBFrames(ctx, r.ffmpegPath, enc, backend, device, mode) {
					ok = append(ok, transcoding.BFramesFeature(codec, lp))
					r.logger.Info("benchmark: HW feature supported",
						slog.String("backend", backend), slog.String("feature", transcoding.BFramesFeature(codec, lp)))
				}
			}
		}
	}
	return ok
}

// probeOption runs one feature-probe encode; it is supported iff ffmpeg exits 0.
func (r *BenchmarkRunner) probeOption(ctx context.Context, args []string) bool {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return procfork.Run(ctx, procfork.Options{Class: procfork.Probe, Site: "benchmark.probe_option"}, r.ffmpegBin(), args...) == nil
}

// resolveSample picks the bench input. When the bundled sample isn't on
// disk, we *generate* a real 4K HEVC mire on the fly via ffmpeg
// (testsrc → libx265 ultrafast, ~5 s of video, ~5 MB) and cache it.
//
// Why a real HEVC sample matters: the original implementation used
// lavfi testsrc as input. lavfi generates frames in-memory, so the
// decoder never runs - the bench measured pure encode throughput.
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
// once generation succeeds - kept as a compatibility seam for
// callers that need to know "are we on synthetic" (legacy callers
// passed true to switch to lavfi). With on-demand generation we
// never need the lavfi branch anymore unless generation itself
// fails.
func (r *BenchmarkRunner) resolveSample(ctx context.Context) (string, bool) {
	return r.resolveBenchSample(ctx, "SDR", r.sdrSrc, r.samplePath, r.generateSample)
}

// resolveBenchSample produces the path to a bench input, in priority order:
//  1. src.overridePath - a real file the operator dropped in (used verbatim);
//  2. defaultPath - a clip cached on a previous run (download or generate);
//  3. a SHA256-pinned download from src.url (cached to defaultPath);
//  4. the synthetic clip produced by generate(defaultPath).
//
// A real-content clip calibrates far more honestly than the synthetic (the
// synthetic over-states decode+encode cost, pinning the cap low), so a pinned
// reference clip wins when configured; the synthetic is the always-available
// airgapped fallback. Returns (path, fallback): fallback=true only when every
// step fails, signalling the caller to drop to the in-memory lavfi testsrc.
func (r *BenchmarkRunner) resolveBenchSample(ctx context.Context, label string, src benchSampleSource, defaultPath string, generate func(dst string) error) (string, bool) {
	if src.overridePath != "" {
		if _, err := os.Stat(src.overridePath); err == nil {
			r.logger.Info("benchmark: using operator-provided "+label+" sample",
				slog.String("path", src.overridePath))
			return src.overridePath, false
		}
		r.logger.Warn("benchmark: "+label+" sample override not found, falling through",
			slog.String("path", src.overridePath))
	}
	if _, err := os.Stat(defaultPath); err == nil {
		if r.cachedSampleIsSound(ctx, label, src, defaultPath) {
			return defaultPath, false
		}
		// Ours, and wrong: drop it and fall through to regenerate below.
		if err := os.Remove(defaultPath); err != nil {
			r.logger.Warn("benchmark: could not remove the unusable "+label+" sample, calibrating against it anyway",
				slog.String("path", defaultPath), slog.Any("err", err))
			return defaultPath, false
		}
	}
	if src.url != "" && src.sha256 != "" {
		if err := r.downloadSample(src.url, src.sha256, defaultPath); err != nil {
			r.logger.Warn("benchmark: "+label+" reference clip download failed, falling back to synthetic",
				slog.String("url", src.url), slog.Any("err", err))
		} else {
			r.logger.Info("benchmark: downloaded "+label+" reference clip",
				slog.String("url", src.url), slog.String("path", defaultPath))
			return defaultPath, false
		}
	}
	if err := generate(defaultPath); err != nil {
		r.logger.Warn("benchmark: "+label+" sample generation failed, using synthetic testsrc fallback",
			slog.String("path", defaultPath), slog.Any("err", err))
		return "", true
	}
	r.logger.Info("benchmark: generated synthetic "+label+" sample", slog.String("path", defaultPath))
	return defaultPath, false
}

// downloadSample fetches a SHA256-pinned reference clip to dst. The pin is
// mandatory (callers pass it only when both url and sha are set), so a swapped
// or corrupted file at the URL is rejected rather than calibrated against - the
// URL is operator-configured and the content is verified, so this is not an
// open SSRF/remote-content sink. Streams to a temp file with a size cap and
// atomically renames on a verified hash.
func (r *BenchmarkRunner) downloadSample(url, wantSHA, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir bench dir: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // G107: url is operator-configured + SHA256-pinned below
	if err != nil {
		return fmt.Errorf("get %q: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %q: http %d", url, resp.StatusCode)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), "sample-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() //nolint:errcheck // best-effort temp cleanup; rename consumes it on success
	const maxBytes = 256 << 20                // 256 MiB cap - a calibration clip is tens of MB
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, maxBytes+1))
	_ = tmp.Close()
	if err != nil {
		return fmt.Errorf("download body: %w", err)
	}
	if n > maxBytes {
		return fmt.Errorf("reference clip exceeds %d byte cap", maxBytes)
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, wantSHA) {
		return fmt.Errorf("sha256 mismatch: got %s want %s", got, wantSHA)
	}
	return os.Rename(tmpName, dst)
}

// synthBenchSource builds the lavfi source for the synthetic clip: testsrc2
// (gradients + moving geometry - real motion vectors and detail, unlike the near
// static original testsrc) softened by a light gaussian blur (testsrc2's razor
// edges are unnaturally hard for x264; the blur brings the residual/encode cost
// toward real film) plus a light film-grain noise (so x265 can't compress it to
// a trivial size - keeps the decode non-trivial - without the pathological
// incompressibility of the old noise=alls=20, which made x264 re-encode pure
// noise and pinned the cap far below what real content sustains). pixFmt is
// yuv420p (SDR) or yuv420p10le (HDR).
func synthBenchSource(pixFmt string) string {
	return "testsrc2=duration=5:size=3840x2160:rate=24," +
		"gblur=sigma=1.5,noise=alls=4:allf=t+u,format=" + pixFmt
}

// generateSample produces a ~5 s 3840x2160 HEVC clip at dst, used when no real
// reference clip is configured. Encoded with libx265 `medium` + a B-frame chain
// so the decoder exercises reordering + multi-reference motion comp (the load
// profile of a real rip). Bitrate targets ~5 Mbit/s - typical 4K streaming /
// "4KLight" content, NOT a 25 Mbit/s remux: the source bitrate dominates decode
// cost, so an over-fat sample understates the host. Slow on weak hosts (~30-60 s
// at 4K) but runs once, reused across reboots/recalibrations.
func (r *BenchmarkRunner) generateSample(dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir bench dir: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	out, err := procfork.CombinedOutput(ctx, procfork.Options{Class: procfork.Production, Site: "benchmark.generate_sample", UnboundedThreads: true}, r.ffmpegBin(),
		"-hide_banner", "-nostats", "-y",
		"-f", "lavfi", "-i", synthBenchSource("yuv420p"),
		"-c:v", "libx265", "-preset", "medium",
		"-x265-params", "bframes=4:ref=3:b-adapt=2:rc-lookahead=20",
		"-b:v", "5M", "-maxrate", "6M", "-bufsize", "10M",
		"-tag:v", "hvc1",
		"-an",
		dst,
	)
	if err != nil {
		return fmt.Errorf("ffmpeg generate sample: %w (tail: %s)", err, tailString(string(out), 200))
	}
	return nil
}

// hdrSamplePath is the on-disk path for the 10-bit HDR10 bench sample, next to
// the SDR one.
func (r *BenchmarkRunner) hdrSamplePath() string {
	return filepath.Join(filepath.Dir(r.samplePath), "sample-hdr10.hevc")
}

// resolveHDRSample resolves the 10-bit HDR10 bench input through the same
// cascade as the SDR one (operator file > cached > pinned download > synthetic).
// A real HDR clip is strongly preferred: the synthetic understates the 10-bit
// decode + tonemap cost, which is exactly what pins the HDR cap too low.
func (r *BenchmarkRunner) resolveHDRSample(ctx context.Context) (string, bool) {
	return r.resolveBenchSample(ctx, "HDR", r.hdrSrc, r.hdrSamplePath(), r.generateHDRSample)
}

// generateHDRSample produces a ~5 s 3840x2160 HEVC Main10 HDR10 clip (bt2020 /
// PQ, 10-bit) at dst - the heavy-decode counterpart of generateSample, used to
// bench the real HDR->SDR transcode cost (10-bit decode + tonemap + encode) when
// no real HDR reference clip is configured. Same content recipe and ~5 Mbit/s
// target as the SDR sample; the tonemap compute cost is luminance-independent,
// so a representative-bitrate 10-bit clip exercises it honestly.
func (r *BenchmarkRunner) generateHDRSample(dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir bench dir: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	out, err := procfork.CombinedOutput(ctx, procfork.Options{Class: procfork.Production, Site: "benchmark.generate_hdr_sample", UnboundedThreads: true}, r.ffmpegBin(),
		"-hide_banner", "-nostats", "-y",
		"-f", "lavfi", "-i", synthBenchSource("yuv420p10le"),
		"-c:v", "libx265", "-preset", "medium", "-pix_fmt", "yuv420p10le",
		// Main10 + HDR10 metadata so the decoder runs the real 10-bit path and
		// the tonemap filter sees bt2020/PQ - same b-frame chain as the SDR sample.
		"-x265-params",
		"bframes=4:ref=3:b-adapt=2:rc-lookahead=20:hdr10=1:"+
			"colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc",
		"-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc",
		"-b:v", "5M", "-maxrate", "6M", "-bufsize", "10M",
		"-tag:v", "hvc1", "-an",
		dst,
	)
	if err != nil {
		return fmt.Errorf("ffmpeg generate HDR sample: %w (tail: %s)", err, tailString(string(out), 200))
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

// benchSoftwareLadders walks the declarative swBenchLadderSpecs table and probes
// each software ladder (plain SDR, HDR->SDR, AV1) under the SAME pipeline shape the
// production HLS transcoder runs. That shape depends on the orchestrator mode:
//   - single-variant (the runtime default): decode the source once + encode ONE
//     rung at height h. Each ABR rung is its own on-demand session, so the
//     per-stream pressure is decode + 1 encode.
//   - multi-variant (legacy, opt-out): one ffmpeg decodes once, splits N ways via
//     filter_complex, and encodes every rung at or below h in parallel.
//
// Benching the wrong shape skews the cap hard, so r.singleVariant selects the
// matching shape (handled inside benchSoftwareLadder). Returns one ceiling per
// spec label (the highest passing rung, or the spec's floor) plus every rung's
// samples concatenated in table order. The HDR spec is probed on the 10-bit HDR
// sample with the production tonemap pre-filter (resolved lazily, once).
func (r *BenchmarkRunner) benchSoftwareLadders(ctx context.Context, sdrSample string, sdrFallback bool) (map[string]int, []transcoding.BenchRungSample) {
	ceilings := make(map[string]int, len(swBenchLadderSpecs))
	var samples []transcoding.BenchRungSample
	var hdrSample string
	var hdrFallback, hdrResolved bool

	for _, spec := range swBenchLadderSpecs {
		sample, fallback := sdrSample, sdrFallback
		preFilter := func(int) string { return "" }
		if spec.hdr {
			// HDR->SDR is much heavier than the plain 8-bit bench (10-bit decode +
			// tonemap): probe it on the real 10-bit sample so the cap is honest on a
			// weak NAS. Resolve the HDR sample lazily and once.
			if !hdrResolved {
				hdrSample, hdrFallback = r.resolveHDRSample(ctx)
				hdrResolved = true
			}
			sample, fallback = hdrSample, hdrFallback
			// Tonemap at the ladder-TOP resolution (downscale folded into the
			// linearizing zscale), matching the production scale-first runtime.
			useTonemapx := tonemapxAvailable(r.ffmpegPath)
			preFilter = func(topH int) string {
				o := hdr.DefaultToneMapOptions()
				o.DstWidth = topH * 16 / 9
				o.UseTonemapx = useTonemapx
				return hdr.BuildToneMapFilter(&hdr.HDRInfo{IsHDR: true}, o)
			}
		}
		ceiling, s := r.benchSoftwareLadder(ctx, sample, fallback, preFilter, spec.label, spec.encoder, r.margin*spec.marginMult, spec.floor)
		ceilings[spec.label] = ceiling
		samples = append(samples, s...)
	}
	return ceilings, samples
}

// av1RealtimeMarginMultiplier raises the realtime margin the AV1 software ladder
// must clear, on top of the base margin used for x264. Belt-and-suspenders for
// the cold-start: the -readrate_initial_burst masks SVT-AV1's first-segment
// latency only in proportion to throughput headroom, so a rung is offered only
// when the host encodes it with real spare capacity (≈2.4x realtime at the
// default 1.2 base). A box that merely scrapes realtime AV1 is kept hardware-only
// rather than handed an AV1 rung that stutters on every fresh play/seek.
const av1RealtimeMarginMultiplier = 2.0

// probeRungs returns the rungs a software probe encodes when the ladder top is
// at index i. In single-variant mode that is just rung i (decode + 1 encode at
// h, the shape an on-demand session runs); in multi-variant mode it is every
// rung at or below i (decode + N parallel encodes, the legacy multi-output
// pipeline). Pure so the shape selection is unit-tested without ffmpeg.
func probeRungs(ladder []int, i int, singleVariant bool) []int {
	if singleVariant {
		return ladder[i : i+1]
	}
	return ladder[: i+1 : i+1]
}

// benchSoftwareLadder walks swBenchLadder under the production pipeline shape,
// optionally prefixing a filter on the decoded stream before encoding
// (preFilter, e.g. the HDR tonemap). In single-variant mode each rung is probed
// in isolation (1 decode + 1 encode at h); in multi-variant mode rung h is
// probed alongside every lighter rung (1 decode + N parallel encodes). Highest
// rung sustaining >= sourceRate×margin wins; floor when none pass.
func (r *BenchmarkRunner) benchSoftwareLadder(ctx context.Context, sample string, fallback bool, preFilter func(topHeight int) string, label, encoder string, margin float64, floor int) (int, []transcoding.BenchRungSample) {
	best := 0
	required := float64(r.sourceRate()) * margin
	var samples []transcoding.BenchRungSample
	for i, h := range swBenchLadder {
		laddered := probeRungs(swBenchLadder, i, r.singleVariant)
		fps, ok := r.medianMultiVariant(ctx, sample, fallback, laddered, encoder, nil, preFilter(h))
		passing := ok && fps >= required
		samples = append(samples, transcoding.BenchRungSample{
			Ladder:          label,
			Height:          h,
			FPS:             fps,
			RequiredFPS:     required,
			Passing:         passing,
			ParallelEncodes: len(laddered),
		})
		r.logger.Info("benchmark: "+label+" probe",
			slog.Int("height", h),
			slog.Int("parallel_encodes", len(laddered)),
			slog.Bool("single_variant", r.singleVariant),
			slog.Float64("fps", fps),
			slog.Float64("required_fps", required),
			slog.Bool("passing", passing))
		if !passing {
			break // ladder is monotone - no point probing 1440p once 1080p fails
		}
		best = h
	}
	if best == 0 {
		// Every rung failed (very weak host, or ffmpeg crashed on every probe).
		// Stay at the floor so playback still works - the host may overshoot
		// realtime occasionally but the runtime throttle catches it.
		r.logger.Warn("benchmark: "+label+" probe failed on every rung, falling back to floor",
			slog.Int("floor", floor))
		return floor, samples
	}
	return best, samples
}

// benchHardware calibrates the host's hardware encode. It no longer trusts the
// detector's fixed order (VAAPI before NVENC before QSV): among every detected
// accelerator that PROVES its ladder AND passes the visual-correctness gate, it
// picks the one whose measured single-encode is FASTEST (cost). Proved+correct
// first, cost second - a QSV that outruns the box's VAAPI (measured on an UHD
// 630: hevc 75 vs 30 fps) is chosen over it, where detector order used to pin
// VAAPI. A backend pin (mode=vaapi/nvenc/qsv) still restricts the field to one.
// Returns (0, "") when no HW is usable - software-only, fully supported.
func (r *BenchmarkRunner) benchHardware(ctx context.Context, sample string, fallback bool, pinBackend string) (int, string, []string, string, []transcoding.BenchRungSample) {
	devices, err := r.hwDetector.DetectDevices(ctx)
	if err != nil || len(devices) == 0 {
		return 0, "", nil, "", nil
	}
	pinBackend = strings.ToLower(strings.TrimSpace(pinBackend))

	type hwCandidate struct {
		backend, encoder, device string
		best                     int
		topFPS                   float64
		codecs                   []string
		samples                  []transcoding.BenchRungSample
	}
	var candidates []hwCandidate

	for _, dev := range devices {
		// Honour the active mode's backend pin: when set, only the matching
		// backend is considered, so a forced nvenc/vaapi/qsv (or a shared host
		// with one Evey-per-GPU) doesn't pick over the one the operator selected.
		if pinBackend != "" && strings.ToLower(dev.Type) != pinBackend {
			continue
		}
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
		runLadder := func(extraInputArgs []string, uploadChain string) (int, []transcoding.BenchRungSample) {
			best := 0
			var samples []transcoding.BenchRungSample
			for i, h := range hwBenchLadder {
				laddered := hwBenchLadder[:i+1]
				fps, ok := r.runMultiVariant(ctx, sample, fallback, laddered, encoder, extraInputArgs, uploadChain)
				required := float64(r.sourceRate()) * r.margin
				passing := ok && fps >= required
				r.logger.Info("benchmark: hardware probe",
					slog.String("backend", dev.Type),
					slog.String("encoder", encoder),
					slog.Int("height", h),
					slog.Int("parallel_encodes", len(laddered)),
					slog.Float64("fps", fps),
					slog.Bool("passing", passing),
					slog.Bool("decoupled", uploadChain != ""))
				samples = append(samples, transcoding.BenchRungSample{
					Ladder:          "hw",
					Height:          h,
					FPS:             fps,
					RequiredFPS:     required,
					Passing:         passing,
					ParallelEncodes: len(laddered),
				})
				if !passing {
					break
				}
				best = h
			}
			return best, samples
		}

		// Production zero-copy path first; decoupled (software decode + hwupload)
		// only if the GPU cannot HW-decode the sample codec (VAAPI/QSV).
		usedInput, usedChain := hwInputArgs(dev.Type), ""
		best, samples := runLadder(usedInput, usedChain)
		if best == 0 {
			if up := hwUploadChain(encoder); up != "" {
				usedInput, usedChain = hwDecoupledInputArgs(dev.Type), up
				best, samples = runLadder(usedInput, usedChain)
			}
		}
		if best == 0 {
			continue
		}
		// Correctness gate: a path that exits 0 at full speed can still emit
		// garbage pixels (buggy driver/GPU). Only a MEASURED divergence refuses it.
		verdict, verr := r.validateHWVisualOutput(ctx, sample, fallback, encoder, usedInput, usedChain)
		if verr != nil {
			r.logger.Warn("benchmark: hw visual validation inconclusive - certifying on speed alone",
				slog.String("backend", dev.Type), slog.String("encoder", encoder), slog.Any("err", verr))
		} else if !verdict.OK {
			r.logger.Warn("benchmark: hardware encoder output is visually corrupt - backend NOT certified",
				slog.String("backend", dev.Type), slog.String("encoder", encoder),
				slog.Int("worst_hamming", verdict.WorstHamming), slog.Int("threshold", hwCorruptHammingThreshold),
				slog.String("failure_class", string(transcoding.FailureHWCorruptOutput)))
			continue // corrupt: never a candidate
		}
		candidates = append(candidates, hwCandidate{
			backend: dev.Type, encoder: encoder, device: hwDevicePath(dev.Type),
			best: best, topFPS: topPassingFPS(samples), codecs: hwDomainCodecs(dev.SupportedCodecs), samples: samples,
		})
	}

	if len(candidates) == 0 {
		return 0, "", nil, "", nil
	}
	// Proved + correct, now cost: the fastest single-encode wins, whatever the
	// detector's order was. Ties break on the taller ceiling, then a stable
	// backend name so a re-bench is deterministic.
	bestC := candidates[0]
	for _, c := range candidates[1:] {
		if c.topFPS > bestC.topFPS ||
			(c.topFPS == bestC.topFPS && c.best > bestC.best) ||
			(c.topFPS == bestC.topFPS && c.best == bestC.best && c.backend < bestC.backend) {
			bestC = c
		}
	}
	if len(candidates) > 1 {
		r.logger.Info("benchmark: multiple accelerators certified, picked the fastest",
			slog.String("picked", bestC.backend), slog.Float64("top_fps", bestC.topFPS), slog.Int("top_rung", bestC.best))
	}
	return bestC.best, bestC.backend, bestC.codecs, bestC.device, bestC.samples
}

// topPassingFPS is the fps of the tallest passing rung in a hw ladder - the
// throughput that ranks a certified candidate by cost. 0 when none passed.
func topPassingFPS(samples []transcoding.BenchRungSample) float64 {
	fps, top := 0.0, 0
	for _, s := range samples {
		if s.Passing && s.Height >= top {
			top, fps = s.Height, s.FPS
		}
	}
	return fps
}

// benchHWBridge walks hwBenchLadder through hwTonemapBridge (GPU decode,
// CPU tone-map at the rung width, GPU encode) on the HDR sample. Nil when the
// backend has no bridge chain or no H.264 encoder.
func (r *BenchmarkRunner) benchHWBridge(ctx context.Context, backend string, codecs []string) []transcoding.BenchRungSample {
	if !slices.Contains(codecs, "h264") {
		return nil
	}
	accel := transcoding.HWAccelType(backend)
	encoder := "h264_" + backend
	sample, fallback := r.resolveHDRSample(ctx)
	useTonemapx := tonemapxAvailable(r.ffmpegPath)
	required := float64(r.sourceRate()) * r.margin
	var samples []transcoding.BenchRungSample
	for i, h := range hwBenchLadder {
		o := hdr.DefaultToneMapOptions()
		o.DstWidth = h * 16 / 9
		o.UseTonemapx = useTonemapx
		chain := hwTonemapBridge(accel, hdr.BuildToneMapFilter(&hdr.HDRInfo{IsHDR: true}, o), deinterlaceFilter)
		if chain == "" {
			return nil
		}
		laddered := hwBenchLadder[:i+1]
		fps, ok := r.runMultiVariant(ctx, sample, fallback, laddered, encoder, hwInputArgs(backend), chain)
		passing := ok && fps >= required
		r.logger.Info("benchmark: hw tone-map bridge probe",
			slog.String("backend", backend), slog.Int("height", h),
			slog.Float64("fps", fps), slog.Float64("required_fps", required), slog.Bool("passing", passing))
		samples = append(samples, transcoding.BenchRungSample{
			Ladder: transcoding.LadderHWBridge, Height: h, FPS: fps, RequiredFPS: required,
			Passing: passing, ParallelEncodes: len(laddered),
		})
		if !passing {
			break
		}
	}
	return samples
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
// Empty for NVENC (which works in pure copy-CPU pipeline too) - and the
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
// "rung failed". Output goes to /dev/null per branch - we measure
// throughput, not segments.
// medianFloat returns the median of vals (sorted copy, mean of the two middles
// for an even count). Empty → 0. Pure, so the outlier-rejection logic is
// unit-tested without ffmpeg.
func medianFloat(vals []float64) float64 {
	n := len(vals)
	if n == 0 {
		return 0
	}
	s := make([]float64, n)
	copy(s, vals)
	sort.Float64s(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// medianMultiVariant probes a rung benchSamples times and returns the MEDIAN fps,
// so a single under-measuring run (a momentarily-busy host) can't pin the cap a
// rung too low. ok is true when a MAJORITY of probes succeeded - a rung that
// mostly fails to encode is a genuine fail, not noise. Logs min/median/max so a
// noisy calibration is visible in the admin log rather than silently lowering the
// ceiling.
func (r *BenchmarkRunner) medianMultiVariant(ctx context.Context, sample string, fallback bool, heights []int, encoder string, extraInputArgs []string, uploadChain string) (float64, bool) {
	n := r.benchSamples
	if n <= 0 {
		n = 1
	}
	fpsList := make([]float64, 0, n)
	for range n {
		if ctx.Err() != nil {
			break
		}
		if fps, ok := r.runMultiVariant(ctx, sample, fallback, heights, encoder, extraInputArgs, uploadChain); ok {
			fpsList = append(fpsList, fps)
		}
	}
	// Majority must have produced a reading; otherwise this rung genuinely can't
	// encode (ffmpeg crashing / timing out), not a noisy outlier.
	if len(fpsList) <= n/2 {
		return 0, false
	}
	med := medianFloat(fpsList)
	if n > 1 {
		lo, hi := fpsList[0], fpsList[0]
		for _, v := range fpsList {
			if v < lo {
				lo = v
			}
			if v > hi {
				hi = v
			}
		}
		r.logger.Debug("benchmark: rung median probe",
			slog.String("encoder", encoder),
			slog.Any("heights", heights),
			slog.Int("samples", len(fpsList)),
			slog.Float64("fps_min", lo),
			slog.Float64("fps_median", med),
			slog.Float64("fps_max", hi))
	}
	return med, true
}

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
	// decode→scale→encode stays on the GPU - exactly the production zero-copy
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
			fmt.Fprintf(&fcBuf, ";[v%d]%s[o%d]", i, hwScaleToHeight(hwScale, h), i)
		} else {
			fmt.Fprintf(&fcBuf, ";[v%d]scale=-2:%d[o%d]", i, h, i)
		}
	}
	fc := fcBuf.String()
	args = append(args, "-filter_complex", fc)
	for i := range heights {
		args = append(args, "-map", fmt.Sprintf("[o%d]", i), fmt.Sprintf("-c:v:%d", i), encoder)
		switch {
		case hwScale != "":
			if hwPreset != "" {
				args = append(args, "-preset", hwPreset)
			}
		case encoder == "libsvtav1":
			// libsvtav1 takes the numeric preset (it rejects "veryfast") - bench at
			// the SAME svtAV1Preset the production builder emits, or the cap is
			// measured against a speed the runtime never uses.
			args = append(args, "-preset", svtAV1Preset, "-pix_fmt", "yuv420p")
		default:
			// veryfast (NOT ultrafast) to match the production encoders
			// (hls_transcoder buildSingle/MultiVariantArgs). Benching a faster
			// preset than the runtime uses overstates the ceiling, so the host
			// gets offered a rung it then starves on.
			args = append(args, "-preset", "veryfast", "-pix_fmt", "yuv420p")
		}
	}
	// Single null output for the whole graph - ffmpeg muxes all the
	// mapped streams together (one per rung). We don't care about the
	// container, just throughput.
	args = append(args, "-an", "-sn", "-f", "null", "-")

	cmd, finish, err := procfork.Command(runCtx, procfork.Options{Class: procfork.Production, Site: "benchmark.encode", UnboundedThreads: true}, r.ffmpegPath, args...)
	if err != nil {
		return 0, false
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	startedAt := time.Now()
	err = finish(cmd.Run(), stderr.Bytes())
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
// compare encode FPS. testsrc is 24, the bundled mire is 24 - anything
// else would force us to probe the sample, which adds cost for no
// material improvement on the cap decision.
func (r *BenchmarkRunner) sourceRate() int {
	return 24
}
