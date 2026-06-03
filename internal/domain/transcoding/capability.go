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
// benchmark hasn't run yet or returned no usable result — every modern
// CPU tolerates 720p x264 ultrafast in realtime, so the fallback is
// always playable.
type EncodeCapability struct {
	// SoftwareMaxHeight is the tallest H.264 software encode (libx264
	// ultrafast) the host can sustain at >= 1.2× realtime on the bench
	// sample. Always one of {720, 1080, 1440, 2160}.
	SoftwareMaxHeight int `json:"software_max_height"`

	// HWMaxHeight is the same upper bound for the hardware encode path
	// when an accelerator (NVENC / QSV / VAAPI) is available. Zero when
	// no HW was probed or none passed the realtime threshold.
	HWMaxHeight int `json:"hw_max_height"`

	// HWBackend names the accelerator used to compute HWMaxHeight
	// (`nvenc`, `qsv`, `vaapi`, or empty). The handler reads this to
	// pick which value to apply per request — software-only requests
	// fall back to SoftwareMaxHeight even on a HW-capable box.
	HWBackend string `json:"hw_backend,omitempty"`

	// HWCodecs lists the domain video codecs ("h264", "hevc", "av1") the
	// calibrated HW backend can actually encode. Empty when no HW. The
	// handler gates the hardware path per-codec on this so e.g. a old iGPU
	// QSV box (H.264 only) still software-encodes HEVC instead of failing.
	HWCodecs []string `json:"hw_codecs,omitempty"`

	// HWDecodeCodecs lists the domain video codecs the HW backend can actually
	// DECODE (probed separately from encode — a pre-Skylake Intel iGPU encodes
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
	HW10BitTranscode bool `json:"hw_10bit_transcode,omitempty"`

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
	// different machine — e.g. container restored on another node — the
	// signature changes and the calibration is auto-invalidated on the
	// next boot.
	CPUSignature string `json:"cpu_signature"`

	// BenchmarkedAt records when the calibration was last run so the
	// admin UI can surface "last calibrated 3 days ago" and the
	// Phase C feedback worker can age out CMCD overrides.
	BenchmarkedAt time.Time `json:"benchmarked_at"`

	// BenchmarkDurationMs is the wall-clock spent inside the
	// benchmark — surfaced for diagnostics so an admin re-running the
	// calibration manually sees "took 22 s" instead of a spinner that
	// just disappears.
	BenchmarkDurationMs int64 `json:"benchmark_duration_ms"`
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
// a HW encode backend is calibrated, HWMaxHeight wins — software-only
// paths always fall back to SoftwareMaxHeight regardless of HW being
// present (e.g. unsupported source codec, sidecar subtitle burn-in
// forcing software).
//
// hwAccelUsable is the handler's view of "can this request take the HW
// path end-to-end" — false ⇒ stay software even if HWMaxHeight is set.
func (c EncodeCapability) MaxHeightFor(hwAccelUsable bool) int {
	if hwAccelUsable && c.HWMaxHeight > 0 {
		return c.HWMaxHeight
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
	for _, supported := range c.HWCodecs {
		if supported == codec {
			return true
		}
	}
	return false
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
	for _, f := range c.HWFeatures {
		if f == feature {
			return true
		}
	}
	return false
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
// already set — callers persist it as-is via CapabilityRepo.
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
