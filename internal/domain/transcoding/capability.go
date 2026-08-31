package transcoding

import (
	"context"
	"slices"
	"strings"
	"time"
)

// EncodeCapability is the calibrated upper bound on what the host can
// software-encode (and HW-encode if a GPU/iGPU is available) in realtime.
// Computed at first boot by the BenchmarkRunner and persisted in the
// server_settings table so we don't re-bench on every restart.
//
// SoftwareMaxHeight / HWMaxHeight default to a safe 720p when the
// benchmark hasn't run yet or returned no usable result - every modern
// CPU tolerates 720p x264 ultrafast in realtime, so the fallback is
// always playable.
type EncodeCapability struct {
	// SoftwareMaxHeight is the tallest H.264 software encode (libx264
	// ultrafast) the host can sustain at >= 1.2× realtime on the bench
	// sample. Always one of {720, 1080, 1440, 2160}.
	SoftwareMaxHeight int `json:"software_max_height"`

	// SoftwareHDRMaxHeight is the tallest software encode the host sustains for
	// HDR->SDR content: the SAME multi-variant pipeline PLUS the zscale tonemap on
	// a real 10-bit HDR sample. It captures the two costs SoftwareMaxHeight (an
	// 8-bit, no-tonemap bench) is blind to - 10-bit HEVC decode and the heavy
	// tonemap - so it is the honest ceiling for HDR on a weak NAS (and inherently
	// reflects decode-bound hosts). 0 until benched; the HDR ladder caps to it.
	SoftwareHDRMaxHeight int `json:"software_hdr_max_height"`

	// SoftwareAV1MaxHeight is the tallest libsvtav1 (AV1 software) encode the host
	// sustains at >= realtime. AV1 software is several times heavier than libx264,
	// so it gets its OWN ceiling instead of borrowing SoftwareMaxHeight (which is
	// the H.264 number and would massively over-state the box for AV1). 0 until an
	// AV1 bench passes - and 0 is meaningful: it means "AV1 software not viable
	// here", so the handler routes AV1 output to hardware only and otherwise falls
	// back to H.264 rather than starving on libsvtav1 (see effectiveVideoCodec).
	SoftwareAV1MaxHeight int `json:"software_av1_max_height"`

	// HWMaxHeight is the same upper bound for the hardware encode path
	// when an accelerator (NVENC / QSV / VAAPI) is available. Zero when
	// no HW was probed or none passed the realtime threshold.
	HWMaxHeight int `json:"hw_max_height"`

	// HWBackend names the accelerator used to compute HWMaxHeight
	// (`nvenc`, `qsv`, `vaapi`, or empty). The handler reads this to
	// pick which value to apply per request - software-only requests
	// fall back to SoftwareMaxHeight even on a HW-capable box.
	HWBackend string `json:"hw_backend,omitempty"`

	// HWCodecs lists the domain video codecs ("h264", "hevc", "av1") the
	// calibrated HW backend can actually encode. Empty when no HW. The
	// handler gates the hardware path per-codec on this so e.g. a old iGPU
	// QSV box (H.264 only) still software-encodes HEVC instead of failing.
	HWCodecs []string `json:"hw_codecs,omitempty"`

	// HWDecodeCodecs lists the domain video codecs the HW backend can actually
	// DECODE (probed separately from encode - a pre-Skylake Intel iGPU encodes
	// H.264 but cannot decode HEVC). The pipeline only puts `-hwaccel` decode on
	// a source whose codec is here, software-decoding the rest so an undecodable
	// source (e.g. HEVC on old iGPU) never stalls. Empty when no HW.
	HWDecodeCodecs []string `json:"hw_decode_codecs,omitempty"`

	// HW10BitTranscode reports whether the calibrated HW backend can run the
	// transcode VPP (scale + p010→nv12 conversion) on a 10-bit source. Probed
	// because encode/decode capability does NOT imply it: the Intel i965 driver
	// on old iGPU (Gen7.5 VEBOX) aborts with a fourcc assertion the moment a
	// 10-bit (p010) surface reaches scale_vaapi, so HEVC Main10 / yuv420p10le
	// sources (ubiquitous at 2160p) crash the HW pipeline and demote to software.
	// False (the conservative default for un-recalibrated hosts) makes the VOD
	// gate route 10-bit sources straight to software instead of crashing first.
	// NO omitempty: a false bool must still be serialized so the admin calibration
	// view can show the gate is active (false is the case that matters on a weak
	// host - omitempty would hide it and the operator couldn't tell "probed, gate
	// on" from "never benched").
	HW10BitTranscode bool `json:"hw_10bit_transcode"`

	// HWTonemap reports whether the calibrated HW backend can tone-map HDR->SDR
	// on the GPU (tonemap_vaapi / vpp_qsv tonemap / libplacebo). Probed because
	// it's a distinct capability: Gen9+ Intel / capable VAAPI-QSV / a Vulkan
	// libplacebo build do it, while old-iGPU-class VEBOX and OpenCL/Vulkan-less
	// hosts (incl. WSL) fail the probe. When true AND the hw-tonemap toggle is on,
	// an HDR source is tone-mapped on the GPU instead of the heavy software zscale
	// path. False -> software tonemap (the universally-correct fallback). No
	// omitempty: a false bool must still surface in the admin calibration view.
	HWTonemap bool `json:"hw_tonemap"`

	// GPUClass labels the calibrated accelerator for the admin panel:
	// "integrated" (Intel iGPU / QSV), "discrete" (NVIDIA NVENC), or "" when it
	// can't be told (AMD VAAPI - iGPU vs dGPU share a PCI vendor - or a host
	// without the sysfs node, e.g. a WSL/d3d12 virtual render node).
	GPUClass string `json:"gpu_class,omitempty"`

	// HWDevice is the DRM render node for VAAPI/QSV (e.g. /dev/dri/renderD128),
	// empty for NVENC (CUDA selects the device). Forwarded to the ffmpeg
	// hardware pipeline as -hwaccel_device / -qsv_device.
	HWDevice string `json:"hw_device,omitempty"`

	// HWFeatures lists the optional encoder tuning options the calibration
	// probe confirmed the HW encoder ACCEPTS on this host (e.g. "forced_idr",
	// "temporal_aq", "b_ref_mode", "rc_lookahead", "spatial_aq", "bframes").
	// The arg builder emits a tuning flag only if its name is present here, so
	// an option an older NVENC/QSV chip rejects is never passed in production.
	// Empty ⇒ HW encode with no tuning (the safe baseline). Demoted to empty
	// on a runtime unsupported-option failure (the level-2 safety net).
	HWFeatures []string `json:"hw_features,omitempty"`

	// DetectedBackends lists every HW accelerator the host advertises
	// ("nvenc"/"vaapi"/"qsv"), recorded at bench time BEFORE the active
	// mode's backend pin is applied. It is purely informational: the admin
	// panel reads it to offer only accelerators that actually exist, even when
	// the current mode forces software or a single backend. Empty on a host
	// with no GPU/iGPU.
	DetectedBackends []string `json:"detected_backends,omitempty"`

	// CPUSignature is a stable hash of the host CPU identity (model
	// name + core count from /proc/cpuinfo). If the host migrates to a
	// different machine - e.g. container restored on another node - the
	// signature changes and the calibration is auto-invalidated on the
	// next boot.
	CPUSignature string `json:"cpu_signature"`

	// BenchmarkedAt records when the calibration was last run so the
	// admin UI can surface "last calibrated 3 days ago" and the
	// Phase C feedback worker can age out CMCD overrides.
	BenchmarkedAt time.Time `json:"benchmarked_at"`

	// BenchmarkDurationMs is the wall-clock spent inside the
	// benchmark - surfaced for diagnostics so an admin re-running the
	// calibration manually sees "took 22 s" instead of a spinner that
	// just disappears.
	BenchmarkDurationMs int64 `json:"benchmark_duration_ms"`

	// BenchSamples records every resolution rung the last benchmark probed, per
	// ladder, with the measured fps vs the realtime threshold it had to clear.
	// Pure observability: the admin calibration API surfaces it so an operator
	// (or a remote debugger with only HTTPS access) sees WHY a ceiling landed
	// where it did - e.g. "720p HDR = 14 fps, needed 28.8 → failed → floored to
	// 480p" - instead of guessing from a bare cap or grepping WARN-level logs.
	// Empty when loaded from a calibration persisted before this field existed
	// (re-bench to populate). omitempty so old rows round-trip unchanged.
	BenchSamples []BenchRungSample `json:"bench_samples,omitempty"`

	// HWDemotedAt is when a runtime failure last DISABLED the HW backend, stamped
	// by the calibration service. Lets the demotion expire and be re-probed
	// instead of outliving its cause (one hw_init failure cost prod ~27h of GPU).
	HWDemotedAt time.Time `json:"hw_demoted_at,omitzero"`

	// DecodeSamples is DECODE-only throughput per source codec. Decode is the half
	// of the pipeline the encode rungs cannot see: same output rung, a 4K HEVC and
	// a 1080p H.264 source measured 2.12x vs 8.43x realtime. Empty on hosts
	// calibrated before it existed - pricing then falls back to the rung alone.
	DecodeSamples []DecodeRungSample `json:"decode_samples,omitempty"`

	// BenchSource is the codec+height BenchSamples were DECODED from, probed from
	// the file rather than assumed from its name.
	BenchSource SourceClass `json:"bench_source,omitzero"`

	// SourceRate is the source frame-rate (fps) the BenchSamples were measured
	// against - PROOF, not policy. Derive needs it to RE-derive the pass/fail of
	// each rung under a possibly-different policy margin (required = SourceRate ×
	// margin) instead of the bench-time one baked into BenchRungSample.RequiredFPS.
	// 0 on rows persisted before this field existed: Derive then trusts the stored
	// Passing flag, reproducing the old ceiling exactly. omitempty for round-trip.
	SourceRate int `json:"source_rate,omitempty"`

	// PolicyVersion is the transcoding.PolicyVersion of the Derive policy that
	// produced this capability's DERIVED fields (the ceilings). It is stamped by
	// Derive, never by the raw benchmark. On boot, a persisted capability whose
	// PolicyVersion differs from the current one is re-derived from its own proof
	// (BenchSamples) without re-benching, so a policy change in a release re-caps
	// every host deterministically. 0 on pre-Derive rows (treated as stale ⇒
	// re-derived on next load). omitempty for round-trip.
	PolicyVersion int `json:"policy_version,omitempty"`

	// BenchContended records that the host was already busy when the benchmark
	// started - CPU busy above the contention threshold before a single bench
	// encode ran. The samples then measure a SHARED box, not this box, and every
	// ceiling derived from them is provisional: seen live on a first boot during
	// a library import, a 4K-capable host benched 720p@5fps with "no usable GPU"
	// and armed a capacity of one stream. The service re-benches such a
	// capability by itself once the host is idle (RebenchIfContended) and the
	// admin calibration view says so. omitempty: absent from every row written
	// before the field existed, which correctly reads as "not known contended".
	BenchContended bool `json:"bench_contended,omitempty"`

	// BenchHostBusyPercent is the host-wide CPU busy share (0-100) sampled right
	// before the benchmark: the evidence behind BenchContended, shown to the
	// operator so "provisional" is a number, not a mood.
	BenchHostBusyPercent int `json:"bench_host_busy_percent,omitempty"`
}

// BenchRungSample is one resolution rung the boot benchmark measured: the
// encode throughput it achieved and whether it cleared the realtime margin that
// decides the ceiling. The honest evidence behind a SoftwareMaxHeight /
// SoftwareHDRMaxHeight value.
type BenchRungSample struct {
	Ladder          string  `json:"ladder"` // "software" | "software-hdr" | "hw"
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

// Pixels is the frame area decode throughput scales against. Height alone:
// carrying width would split one title into several source classes for no
// measurable difference in decode cost.
func (c SourceClass) Pixels() int {
	if c.Height <= 0 {
		return 0
	}
	return c.Height * c.Height
}

// DecodeRungSample is measured DECODE-only throughput for one codec at one
// height, taken by decoding a real sample to null.
type DecodeRungSample struct {
	Codec  string  `json:"codec"`
	Height int     `json:"height"`
	FPS    float64 `json:"fps"`
}

// SafeFallback returns the capability used when no calibration has run
// yet (first boot in progress, or DB unreachable). 720p software is the
// universal floor that every supported deployment can sustain.
func SafeFallback() EncodeCapability {
	return EncodeCapability{
		SoftwareMaxHeight: 720,
		HWMaxHeight:       0,
		HWBackend:         "",
	}
}

// MaxHeightFor returns the effective cap to apply for a given codec +
// source-side hwAccel preference. When the source can be HW-decoded AND
// a HW encode backend is calibrated, HWMaxHeight wins - software-only
// paths always fall back to SoftwareMaxHeight regardless of HW being
// present (e.g. unsupported source codec, sidecar subtitle burn-in
// forcing software).
//
// hwAccelUsable is the handler's view of "can this request take the HW
// path end-to-end" - false ⇒ stay software even if HWMaxHeight is set.
func (c EncodeCapability) MaxHeightFor(hwAccelUsable bool) int {
	if hwAccelUsable && c.HWMaxHeight > 0 {
		return c.HWMaxHeight
	}
	if c.SoftwareMaxHeight > 0 {
		return c.SoftwareMaxHeight
	}
	return 720
}

// AV1SoftwareViable reports whether the host has a calibrated AV1 software
// ceiling. False (the default until an AV1 bench passes) means libsvtav1 is too
// slow here, so AV1 output must stay hardware-only - see effectiveVideoCodec.
func (c EncodeCapability) AV1SoftwareViable() bool {
	return c.SoftwareAV1MaxHeight > 0
}

// SoftwareCeilingFor returns the software height ceiling for an OUTPUT codec.
// AV1 is far heavier than H.264, so it uses its own SoftwareAV1MaxHeight rather
// than the H.264 SoftwareMaxHeight; every other codec uses SoftwareMaxHeight.
// Falls back to the H.264 ceiling if AV1 has no calibrated value (defensive -
// the handler should not route AV1 to software in that case).
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
// domain codec. False whenever no backend is calibrated. An empty codec is
// treated as h264 (the universal default), matching resolveVideoEncoder.
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
// source codec. Encode capability does NOT imply it: a pre-Skylake Intel iGPU
// encodes H.264 but can't decode HEVC, so feeding such a source to `-hwaccel`
// fails (ENOSYS) and stalls the player. The VOD path consults this to fall back
// to software for an undecodable source instead of risking the stall.
func (c EncodeCapability) HWCanDecode(codec string) bool {
	if c.HWBackend == "" {
		return false
	}
	return slices.Contains(c.HWDecodeCodecs, normaliseSourceCodec(codec))
}

// HWHasFeature reports whether the calibration probe confirmed the named
// encoder tuning option is accepted on this host. The arg builder gates every
// optional tuning flag on this so an unsupported option is never emitted.
func (c EncodeCapability) HWHasFeature(feature string) bool {
	return slices.Contains(c.HWFeatures, feature)
}

// CapabilityRepo persists EncodeCapability across restarts. Backed by
// the existing server_settings table (single JSON-encoded value under a
// reserved key), so no schema migration is needed for Phase A.
type CapabilityRepo interface {
	Get(ctx context.Context) (EncodeCapability, bool, error)
	Set(ctx context.Context, cap EncodeCapability) error
}

// BenchmarkRunner measures the host's realtime encode ceiling on a
// bundled sample, separately for the software (x264 ultrafast) and
// hardware (if available) paths. Returns a fully-populated
// EncodeCapability with CPUSignature, BenchmarkedAt and timing fields
// already set - callers persist it as-is via CapabilityRepo.
//
// Implementations MUST be cancellable: a long benchmark on a weak host
// can take ~60 s and an admin clicking "recalibrate" twice in a row
// shouldn't pile up requests.
type BenchmarkRunner interface {
	// Run benches the host under the given preference: pref.Backend pins the
	// hardware probe to one accelerator (empty = auto-detect) and
	// pref.DisableHW skips hardware probing entirely (software-only). The
	// preference is the calibration-facing projection of the admin HW mode.
	Run(ctx context.Context, pref HWPreference) (EncodeCapability, error)
	// HWBackendUsable confirms the named HW backend ("nvenc"/"vaapi"/"qsv") can
	// actually encode on this host right now. Used to re-validate a persisted
	// capability before trusting it, since a matching CPU signature does not
	// guarantee the GPU/encoder is present and working (shared DB across HW and
	// non-HW containers, a moved disk, or a driver regression).
	HWBackendUsable(ctx context.Context, backend string) bool
}
