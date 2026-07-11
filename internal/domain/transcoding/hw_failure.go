package transcoding

import "strings"

// FailureClass categorises WHY a hardware transcode process failed, so the
// orchestrator can react proportionally: how far to downgrade the encode, and
// whether to demote the host capability permanently or only for this one job.
// The classification itself (ffmpeg stderr → FailureClass) lives in the ffmpeg
// adapter, which owns the encoder's stderr vocabulary; this package owns only
// the policy that turns a class into an action.
type FailureClass string

const (
	// FailureNone: clean exit, nothing to react to.
	FailureNone FailureClass = ""
	// FailureHWUnsupportedOption: the encoder rejected a tuning option (e.g.
	// -temporal_aq on pre-Turing NVENC). Deterministic → drop tuning.
	FailureHWUnsupportedOption FailureClass = "hw_unsupported_option"
	// FailureHWEncoderMissing: the HW encoder for this codec isn't available
	// (e.g. hevc_nvenc on a GPU without HEVC). Deterministic → this codec
	// can't use HW at all.
	FailureHWEncoderMissing FailureClass = "hw_encoder_missing"
	// FailureHWInit: the HW device/context failed to initialise (driver gone,
	// render node missing, VAAPI/CUDA init error). HW unusable on this host.
	FailureHWInit FailureClass = "hw_init"
	// FailureHWDeviceMemory: out of VRAM / device memory. Usually transient
	// (contention) → fall back for this job, do NOT permanently demote.
	FailureHWDeviceMemory FailureClass = "hw_device_memory"
	// FailureTransient: not HW-related and likely retryable (I/O, broken
	// pipe). Don't downgrade HW.
	FailureTransient FailureClass = "transient"
	// FailureCancelled: we killed it on purpose (seek/cancel, resource-monitor
	// SIGKILL). Not a failure to react to.
	FailureCancelled FailureClass = "cancelled"
	// FailureUnknown: unrecognised error. Conservatively fall back to software
	// for this job but never permanently demote on a fluke.
	FailureUnknown FailureClass = "unknown"
)

// IsHWCapability reports whether the class indicates a hardware-encode
// capability problem (vs a transient / cancelled / unknown cause).
func (c FailureClass) IsHWCapability() bool {
	switch c {
	case FailureHWUnsupportedOption, FailureHWEncoderMissing,
		FailureHWInit, FailureHWDeviceMemory:
		return true
	}
	return false
}

// Deterministic reports whether the failure will recur identically on the same
// host + args, so the capability demotion is worth persisting. False for
// transient / resource causes that may clear on their own.
func (c FailureClass) Deterministic() bool {
	switch c {
	case FailureHWUnsupportedOption, FailureHWEncoderMissing, FailureHWInit:
		return true
	}
	return false
}

// RetryTier is the encode strategy to (re-)spawn a job with.
type RetryTier string

const (
	// TierHWFull: hardware encode with all probed tuning options.
	TierHWFull RetryTier = "hw_full"
	// TierHWMinimal: hardware encode, no tuning options (the safe HW baseline).
	TierHWMinimal RetryTier = "hw_minimal"
	// TierSoftware: libx264 / libx265 - the universal floor that always works.
	TierSoftware RetryTier = "software"
)

// Demotion is the policy decision for a failed encode: which tier to retry the
// job at, and the (possibly demoted) host capability to apply.
type Demotion struct {
	// RetryTier is the strategy to re-spawn THIS job with.
	RetryTier RetryTier
	// Capability is the (possibly) demoted host capability.
	Capability EncodeCapability
	// Persist is true when Capability should be written back to the store
	// (deterministic failure), false when the change is only for this job.
	Persist bool
	// Changed reports whether Capability actually differs from the input - lets
	// the caller skip a no-op write / alert.
	Changed bool
}

// PlanDemotion decides how to react to a failed HARDWARE encode of `codec`,
// given the current host capability and the classified failure. Smart,
// minimal-blast-radius rules (see each FailureClass doc):
//
//   - unsupported option (deterministic): retry HW WITHOUT tuning; persist by
//     clearing HWFeatures (keep HW + the codec).
//   - encoder missing (deterministic): this codec can't HW-encode; retry in
//     software; persist by removing the codec from HWCodecs (other codecs keep
//     HW; if none remain the backend is disabled).
//   - HW init failure (deterministic): the device is unusable; retry in
//     software; persist by disabling HW for the whole host.
//   - device OOM (transient): software for THIS job only; do NOT persist (HW
//     may work again once memory frees).
//   - transient: retry the same tier; no capability change.
//   - cancelled / none: no reaction.
//   - unknown: software for this job; no persistent demotion.
func PlanDemotion(cap EncodeCapability, class FailureClass, codec string) Demotion {
	switch class {
	case FailureHWUnsupportedOption:
		d := cap
		changed := len(d.HWFeatures) > 0
		d.HWFeatures = nil
		return Demotion{RetryTier: TierHWMinimal, Capability: d, Persist: true, Changed: changed}

	case FailureHWEncoderMissing:
		d := cap
		changed := removeHWCodec(&d, codec)
		return Demotion{RetryTier: TierSoftware, Capability: d, Persist: true, Changed: changed}

	case FailureHWInit:
		d := cap
		changed := d.HWBackend != "" || d.HWMaxHeight != 0 ||
			len(d.HWCodecs) > 0 || len(d.HWFeatures) > 0 || d.HWDevice != ""
		disableHW(&d)
		return Demotion{RetryTier: TierSoftware, Capability: d, Persist: true, Changed: changed}

	case FailureHWDeviceMemory:
		// Transient: fall back for this job but keep the capability intact.
		return Demotion{RetryTier: TierSoftware, Capability: cap, Persist: false}

	case FailureTransient, FailureCancelled, FailureNone:
		return Demotion{RetryTier: TierHWFull, Capability: cap, Persist: false}

	default: // FailureUnknown
		return Demotion{RetryTier: TierSoftware, Capability: cap, Persist: false}
	}
}

// removeHWCodec drops codec from c.HWCodecs (case-insensitive, "" ⇒ h264). If
// no HW codecs remain the backend is disabled. Returns whether anything changed.
func removeHWCodec(c *EncodeCapability, codec string) bool {
	codec = strings.ToLower(strings.TrimSpace(codec))
	if codec == "" {
		codec = "h264"
	}
	kept := make([]string, 0, len(c.HWCodecs))
	removed := false
	for _, x := range c.HWCodecs {
		if strings.EqualFold(strings.TrimSpace(x), codec) {
			removed = true
			continue
		}
		kept = append(kept, x)
	}
	if !removed {
		return false
	}
	if len(kept) == 0 {
		disableHW(c)
		return true
	}
	c.HWCodecs = kept
	return true
}

// disableHW zeroes every hardware field, leaving software-only capability.
func disableHW(c *EncodeCapability) {
	c.HWBackend = ""
	c.HWMaxHeight = 0
	c.HWCodecs = nil
	c.HWDevice = ""
	c.HWFeatures = nil
}
