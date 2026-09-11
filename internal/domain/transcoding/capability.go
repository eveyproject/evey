package transcoding

import (
	"context"
	"slices"
	"strings"
	"time"
)

// EncodeCapability is the calibrated upper bound on what the host can encode
// in realtime (software, and hardware when a GPU is available), computed by the
// BenchmarkRunner and persisted so boots do not re-bench. Ceilings default to
// a safe 720p until a benchmark ran.
type EncodeCapability struct {
	// SoftwareMaxHeight is the tallest H.264 software encode the host sustains
	// at >= 1.2× realtime. One of {720, 1080, 1440, 2160}.
	SoftwareMaxHeight int `json:"software_max_height"`

	// SoftwareHDRMaxHeight is the same for HDR->SDR content (10-bit decode +
	// tonemap, the costs the SDR bench cannot see). 0 until benched.
	SoftwareHDRMaxHeight int `json:"software_hdr_max_height"`

	// SoftwareAV1MaxHeight is the libsvtav1 ceiling. 0 is meaningful: AV1
	// software is not viable, so AV1 output stays hardware-only.
	SoftwareAV1MaxHeight int `json:"software_av1_max_height"`

	// HWMaxHeight is the hardware encode ceiling; 0 when no HW passed the bench.
	HWMaxHeight int `json:"hw_max_height"`

	// HWBridgeMaxHeight is the ceiling of the CPU tone-map bridged between a GPU
	// decode and a GPU encode (LadderHWBridge). 0 = not measured: the HDR ladder
	// then caps at SoftwareHDRMaxHeight, the conservative bound.
	HWBridgeMaxHeight int `json:"hw_bridge_max_height,omitempty"`

	// HWBackend names the accelerator behind HWMaxHeight (nvenc/qsv/vaapi, "").
	HWBackend string `json:"hw_backend,omitempty"`

	// HWCodecs lists the domain codecs the backend encodes ("h264", "hevc",
	// "av1"): an old QSV box encodes H.264 only and software-encodes HEVC.
	HWCodecs []string `json:"hw_codecs,omitempty"`

	// HWDecodeCodecs lists what the backend DECODES (probed separately: a
	// pre-Skylake iGPU encodes H.264 but cannot decode HEVC).
	HWDecodeCodecs []string `json:"hw_decode_codecs,omitempty"`

	// HW10BitTranscode: the VPP folds a 10-bit source (p010 -> nv12). The Intel
	// i965 driver aborts the moment a p010 surface reaches scale_vaapi, so false
	// routes 10-bit sources to software. No omitempty: the admin view must show
	// the gate is active.
	HW10BitTranscode bool `json:"hw_10bit_transcode"`

	// HWTonemap: the backend tone-maps HDR->SDR on the GPU (tonemap_vaapi /
	// vpp_qsv / libplacebo). False -> software tonemap. No omitempty.
	HWTonemap bool `json:"hw_tonemap"`

	// HWOverlay: the backend composites a bitmap subtitle on the GPU; false
	// keeps a burn-in on the software path (a forced-subtitle rip used to send
	// a whole 4K HDR session to the CPU, prod 2026-09-09). No omitempty.
	HWOverlay bool `json:"hw_overlay"`

	// HWHDR10Encode: the backend encodes HEVC Main10 from a 10-bit surface, HDR metadata kept.
	HWHDR10Encode bool `json:"hw_hdr10_encode"`

	// ProbeVersion is the CapabilityProbeVersion the HW probes ran under; a
	// persisted capability below it is re-benched at boot.
	ProbeVersion int `json:"probe_version,omitempty"`

	// GPUClass labels the accelerator for the admin panel: "integrated",
	// "discrete", or "" when it cannot be told (AMD VAAPI, a WSL render node).
	GPUClass string `json:"gpu_class,omitempty"`

	// HWDevice is the DRM render node for VAAPI/QSV, empty for NVENC.
	HWDevice string `json:"hw_device,omitempty"`

	// HWFeatures lists the encoder tuning options the probe confirmed the
	// backend accepts; the arg builder emits a flag only when named here.
	// Demoted to empty on a runtime unsupported-option failure.
	HWFeatures []string `json:"hw_features,omitempty"`

	// DetectedBackends lists every accelerator the host advertises, recorded
	// before the mode's pin: informational, for the admin picker.
	DetectedBackends []string `json:"detected_backends,omitempty"`

	// CPUSignature identifies the host CPU; a changed signature (container
	// restored elsewhere) invalidates the calibration at boot.
	CPUSignature string `json:"cpu_signature"`

	BenchmarkedAt       time.Time `json:"benchmarked_at"`
	BenchmarkDurationMs int64     `json:"benchmark_duration_ms"`

	// BenchSamples is every rung the benchmark probed (fps vs threshold), so
	// the operator sees WHY a ceiling landed where it did. Empty on rows
	// persisted before it existed.
	BenchSamples []BenchRungSample `json:"bench_samples,omitempty"`

	// HWDemotedAt is when a runtime failure last disabled the HW backend, so
	// the demotion expires instead of outliving its cause.
	HWDemotedAt time.Time `json:"hw_demoted_at,omitzero"`

	// DecodeSamples is DECODE-only throughput per source codec (a 4K HEVC and a
	// 1080p H.264 source measured 2.12x vs 8.43x at the same output rung).
	DecodeSamples []DecodeRungSample `json:"decode_samples,omitempty"`

	// BenchSource is the codec+height BenchSamples were decoded from.
	BenchSource SourceClass `json:"bench_source,omitzero"`

	// SourceRate is the fps the samples were measured against, so Derive can
	// re-derive pass/fail under a different margin. 0 on old rows: the stored
	// Passing flag is trusted.
	SourceRate int `json:"source_rate,omitempty"`

	// PolicyVersion is the Derive policy that produced the ceilings; an older
	// one is re-derived from the proof at boot without re-benching.
	PolicyVersion int `json:"policy_version,omitempty"`

	// BenchContended records that the host was already busy when the benchmark
	// ran (a first boot during a library import benched 720p@5fps): the ceilings
	// are provisional and the service re-benches once idle.
	BenchContended bool `json:"bench_contended,omitempty"`

	// BenchHostBusyPercent is the host CPU busy share sampled before the bench.
	BenchHostBusyPercent int `json:"bench_host_busy_percent,omitempty"`
}

// BenchRungSample is one resolution rung the benchmark measured.
type BenchRungSample struct {
	Ladder          string  `json:"ladder"` // "software" | "software-hdr" | "hw" | "hw-bridge"
	Height          int     `json:"height"`
	FPS             float64 `json:"fps"`          // measured encode fps for this rung
	RequiredFPS     float64 `json:"required_fps"` // sourceRate × realtime margin it had to beat
	Passing         bool    `json:"passing"`      // FPS >= RequiredFPS
	ParallelEncodes int     `json:"parallel_encodes"`
}

// SourceClass is a decode workload: codec + pixel height. The zero value means
// "unknown source", which callers price the old way rather than as free.
type SourceClass struct {
	Codec  string `json:"codec"`
	Height int    `json:"height"`
}

// Known reports whether c carries enough information to price a decode.
func (c SourceClass) Known() bool {
	return c.Codec != "" && c.Height > 0
}

// Pixels is the frame area decode throughput scales against (height alone).
func (c SourceClass) Pixels() int {
	if c.Height <= 0 {
		return 0
	}
	return c.Height * c.Height
}

// DecodeRungSample is measured DECODE-only throughput for one codec at one height.
type DecodeRungSample struct {
	Codec  string  `json:"codec"`
	Height int     `json:"height"`
	FPS    float64 `json:"fps"`
}

// CapabilityProbeVersion bumps whenever a new HW probe is added (1: HDR10
// encode + Intel low-power features; 2: OpenCL tone-map + per-codec B-frames;
// 3: the OpenCL chain fixed, 2026-09-10; 4: the tone-map bridge ladder).
const CapabilityProbeVersion = 4

// FeatureTonemapOpenCL: tonemap_opencl runs on this GPU (bt2390 curve, HLG and
// Dolby Vision RPU handled) - the VPP tone-mapper is the fallback.
const FeatureTonemapOpenCL = "tonemap_opencl"

// BFramesFeature names the probed B-frame support of a codec; "_lp" suffixed
// for the low-power encoder, whose support differs (VDEnc has none on Gen9).
func BFramesFeature(codec string, lowPower bool) string {
	codec = strings.ToLower(strings.TrimSpace(codec))
	if codec == "" {
		codec = "h264"
	}
	if lowPower {
		return "bframes_" + codec + "_lp"
	}
	return "bframes_" + codec
}

// SafeFallback is the capability used before any calibration ran: 720p
// software, the universal floor.
func SafeFallback() EncodeCapability {
	return EncodeCapability{
		SoftwareMaxHeight: 720,
		HWMaxHeight:       0,
		HWBackend:         "",
	}
}

// MaxHeightFor returns the effective cap: HWMaxHeight when the request takes
// the HW path end-to-end (hwAccelUsable), else the software ceiling.
func (c EncodeCapability) MaxHeightFor(hwAccelUsable bool) int {
	if hwAccelUsable && c.HWMaxHeight > 0 {
		return c.HWMaxHeight
	}
	if c.SoftwareMaxHeight > 0 {
		return c.SoftwareMaxHeight
	}
	return 720
}

// AV1SoftwareViable reports whether the host has a calibrated AV1 software ceiling.
func (c EncodeCapability) AV1SoftwareViable() bool {
	return c.SoftwareAV1MaxHeight > 0
}

// SoftwareCeilingFor returns the software height ceiling for an OUTPUT codec:
// AV1 has its own (far heavier), every other codec uses SoftwareMaxHeight.
func (c EncodeCapability) SoftwareCeilingFor(codec string) int {
	if strings.EqualFold(strings.TrimSpace(codec), "av1") && c.SoftwareAV1MaxHeight > 0 {
		return c.SoftwareAV1MaxHeight
	}
	if c.SoftwareMaxHeight > 0 {
		return c.SoftwareMaxHeight
	}
	return 720
}

// HWSupports reports whether the calibrated HW backend can encode the given
// domain codec. An empty codec is treated as h264, matching resolveVideoEncoder.
func (c EncodeCapability) HWSupports(codec string) bool {
	if c.HWBackend == "" {
		return false
	}
	codec = strings.ToLower(strings.TrimSpace(codec))
	if codec == "" {
		codec = "h264"
	}
	return slices.Contains(c.HWCodecs, codec)
}

// HWCanDecode reports whether the calibrated HW backend can DECODE the given
// source codec (encode capability does not imply it).
func (c EncodeCapability) HWCanDecode(codec string) bool {
	if c.HWBackend == "" {
		return false
	}
	return slices.Contains(c.HWDecodeCodecs, normaliseSourceCodec(codec))
}

// HWHasFeature reports whether the calibration probe confirmed the named
// encoder tuning option is accepted on this host.
func (c EncodeCapability) HWHasFeature(feature string) bool {
	return slices.Contains(c.HWFeatures, feature)
}

// LowPowerFeature names the probed Intel low-power (VDEnc) encoder feature of a codec.
func LowPowerFeature(codec string) string {
	codec = strings.ToLower(strings.TrimSpace(codec))
	if codec == "" {
		codec = "h264"
	}
	return "low_power_" + codec
}

func (c EncodeCapability) HWHDRPassthroughUsable() bool {
	return c.HWBackend != "" && c.HWHDR10Encode && c.HWSupports("hevc")
}

// CapabilityRepo persists EncodeCapability across restarts (server_settings, one JSON value).
type CapabilityRepo interface {
	Get(ctx context.Context) (EncodeCapability, bool, error)
	Set(ctx context.Context, cap EncodeCapability) error
}

// BenchmarkRunner measures the host's realtime encode ceilings on a bundled
// sample. Implementations MUST be cancellable (a weak host benches ~60 s).
type BenchmarkRunner interface {
	// Run benches under pref: Backend pins the HW probe to one accelerator
	// (empty = auto), DisableHW skips hardware probing entirely.
	Run(ctx context.Context, pref HWPreference) (EncodeCapability, error)
	// HWBackendUsable confirms the named backend can encode on this host right
	// now: a matching CPU signature does not guarantee the GPU is still there.
	HWBackendUsable(ctx context.Context, backend string) bool
}
