package transcoding

import (
	"context"
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
	Run(ctx context.Context) (EncodeCapability, error)
}
