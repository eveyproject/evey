package transcoding

import "strings"

// HWMode is the admin-selectable hardware-acceleration policy. It sits ABOVE
// calibration: the mode decides which backend (if any) calibration is allowed
// to pick, so an operator can pin a specific accelerator, force software, or
// leave it on auto - all from the admin panel, without a redeploy.
//
//   - HWModeAuto: calibration picks the best accelerator it detects (the
//     historical default - nothing changes for existing installs).
//   - HWModeOff: software only, EVEN IF a usable GPU is present. The escape
//     hatch when HW misbehaves (driver bug, thermal, a TDR-prone box) or when
//     the operator simply prefers CPU.
//   - HWModeNVENC / HWModeVAAPI / HWModeQSV / HWModeRKMPP: force that backend. If it turns
//     out unusable on this host, calibration strips it and falls back to
//     software (the admin GET then shows the active backend so the UI can say
//     "requested X, running software").
type HWMode string

const (
	HWModeAuto  HWMode = "auto"
	HWModeOff   HWMode = "off"
	HWModeNVENC HWMode = "nvenc"
	HWModeVAAPI HWMode = "vaapi"
	HWModeQSV   HWMode = "qsv"
	HWModeRKMPP HWMode = "rkmpp"
)

// ParseHWMode validates an operator-supplied string. Case/space-insensitive.
// Returns (mode, true) on a known value, (HWModeAuto, false) otherwise so
// callers can reject bad input rather than silently defaulting.
func ParseHWMode(s string) (HWMode, bool) {
	switch HWMode(strings.ToLower(strings.TrimSpace(s))) {
	case HWModeAuto:
		return HWModeAuto, true
	case HWModeOff:
		return HWModeOff, true
	case HWModeNVENC:
		return HWModeNVENC, true
	case HWModeVAAPI:
		return HWModeVAAPI, true
	case HWModeQSV:
		return HWModeQSV, true
	case HWModeRKMPP:
		return HWModeRKMPP, true
	default:
		return HWModeAuto, false
	}
}

// HWPreference is the calibration-facing projection of a mode: the knobs the
// BenchmarkRunner actually honours. Backend == "" means "no backend pin"
// (auto); DisableHW short-circuits the hardware bench entirely.
type HWPreference struct {
	// Backend pins the bench to one accelerator ("nvenc"/"vaapi"/"qsv"); empty
	// lets the detector pick. Ignored when DisableHW is set.
	Backend string
	// DisableHW skips hardware probing - the host is calibrated software-only.
	DisableHW bool
	// Device pins the bench to one device of the backend(s): a DRM render node
	// ("/dev/dri/renderD129") or a CUDA index ("1"). Empty = every detected
	// device is certified and the best one per backend is selected. The knob
	// that lets a multi-GPU host run one Evey per GPU, or choose the second
	// render node (EVEY_TRANSCODING_HW_DEVICE).
	Device string
}

// Preference derives the bench knobs from the mode.
func (m HWMode) Preference() HWPreference {
	switch m {
	case HWModeOff:
		return HWPreference{DisableHW: true}
	case HWModeNVENC, HWModeVAAPI, HWModeQSV, HWModeRKMPP:
		return HWPreference{Backend: string(m)}
	default: // HWModeAuto (and any unexpected value, treated as auto)
		return HWPreference{}
	}
}

// ResolveHWMode is the single precedence rule used at boot AND by the admin GET:
// an explicit mode string (env EVEY_TRANSCODING_HW_MODE or a persisted admin
// override) wins; otherwise fall back to the legacy two-knob derivation. An
// unparseable explicit value is ignored (treated as unset) rather than failing.
func ResolveHWMode(explicit string, accelEnabled bool, backend string) HWMode {
	if m, ok := ParseHWMode(explicit); ok {
		return m
	}
	return HWModeFromEnv(accelEnabled, backend)
}

// HWModeFromEnv derives the bootstrap mode from the two legacy env knobs
// (EVEY_TRANSCODING_HW_ACCEL_ENABLED + EVEY_TRANSCODING_HW_BACKEND) so existing
// deployments keep their behaviour and the admin override layers on top:
//   - accel disabled              -> off
//   - enabled, no backend pinned  -> auto
//   - enabled, backend pinned      -> that backend (validated; unknown -> auto)
func HWModeFromEnv(accelEnabled bool, backend string) HWMode {
	if !accelEnabled {
		return HWModeOff
	}
	if b := strings.ToLower(strings.TrimSpace(backend)); b != "" {
		if m, ok := ParseHWMode(b); ok && m != HWModeAuto && m != HWModeOff {
			return m
		}
	}
	return HWModeAuto
}

// HWGateEnabled reports whether the legacy master gate (Transcoding.HWAccelEnabled,
// which the VOD handler routes on) should be on, given the PERSISTED mode and the
// bootstrap env knobs.
//
// The admin panel persists only transcoding.hw_mode - the toggle's "on" is the mode
// leaving "off" - so the gate has to be derived from it at boot. Reading the two
// independently lets them disagree: an operator who enabled the GPU sees the panel
// still checked (the mode IS persisted) while every restart quietly resets the gate
// to its env default and sends all transcodes back to software.
func HWGateEnabled(explicitMode string, envAccelEnabled bool, backend string) bool {
	return ResolveHWMode(explicitMode, envAccelEnabled, backend) != HWModeOff
}
