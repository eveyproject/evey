package httpapi

// transcode_vod_profiles.go resolves the effective codec/HW route and
// builds the HLS profile ladder for a given decision.

import (
	"context"
	"slices"
	"strings"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// resolveHW decides the hardware encode backend for a chosen output codec.
// Returns ("", "", false) — the software path — unless the flag is on, the
// host calibrated a HW backend, and that backend can encode this codec. The
// boolean is both the ladder's hwAccelUsable (taller HW ceiling) and the
// signal that tags the fingerprint so the orchestrator runs the GPU pipeline.
// HDR tonemap / bitmap burn-in are not gated here: the orchestrated VOD path
// does neither today, and the ffmpeg builder independently falls back to
// software for those, so the two stay consistent.
// effectiveVideoCodec applies the capability gate to the decision's chosen
// output codec and returns the codec the segments will ACTUALLY carry together
// with the HW resolution for that codec. AV1 software (libsvtav1) is several
// times heavier than H.264 and the weak-NAS target can't sustain it, so when AV1
// is requested but the host has no HW AV1 encoder AND no calibrated AV1 software
// ceiling (SoftwareAV1MaxHeight == 0), fall back to H.264 rather than starve.
// Shared by buildFingerprint (the cache key) and the manifest builder (the
// advertised CODECS) so the master playlist can never advertise a codec the
// segments don't carry. Re-resolves HW for the fallback codec (H.264 may be
// HW-encodable where AV1 was not).
func (h *hlsVODServe) effectiveVideoCodec(decision domaintranscoding.PlaybackDecision, streams []*domainlibrary.MediaStream) (codec, accel, device string, features []string, usable bool) {
	hwTonemap := h.hwTonemapWanted(decision)
	srcCodec := sourceVideoCodec(streams)
	src10 := sourceVideoTenBit(streams)
	codec = decision.VideoCodec
	accel, device, features, usable = h.resolveHW(codec, srcCodec, src10, hwTonemap)
	// AV1 output must clear TWO gates, else fall back to H.264:
	//   1. the admin opt-in (transcoding.av1_enabled) — off by default; and
	//   2. a usable HW AV1 encoder. AV1 is hardware-only for streaming because
	//      libsvtav1 (software AV1) is several times heavier than H.264 and the
	//      weak-NAS target can't sustain it in realtime — it starves and the
	//      player rebuffers forever. The software-AV1 calibration stays for
	//      non-streaming/experimental use but never feeds a live HLS session.
	if strings.EqualFold(codec, "av1") && ((h.av1Enabled != nil && !h.av1Enabled()) || !usable) {
		codec = "h264"
		accel, device, features, usable = h.resolveHW(codec, srcCodec, src10, hwTonemap)
	}
	return codec, accel, device, features, usable
}

func (h *hlsVODServe) resolveHW(outputCodec, sourceCodec string, sourceTenBit, hwTonemap bool) (accel, device string, features []string, usable bool) {
	if h.hwAccelEnabled == nil || !h.hwAccelEnabled() || h.capability == nil {
		return "", "", nil, false
	}
	cap := h.capability()
	if !cap.HWSupports(outputCodec) {
		return "", "", nil, false
	}
	// The orchestrated pipeline HW-decodes the source (-hwaccel). If this GPU
	// can't decode the source codec — a pre-Skylake Intel iGPU decodes H.264 but
	// not HEVC — that decode fails with ENOSYS and the player stalls on rebuffer.
	// Fall back to full software for such a source rather than risk the stall.
	// H.264 sources (the common case) keep the full HW path. (A future decoupled
	// software-decode + hwupload path could keep HW *encode* even here.)
	if sourceCodec != "" && !cap.HWCanDecode(sourceCodec) {
		return "", "", nil, false
	}
	// A 10-bit source (HEVC Main10, ubiquitous at 2160p HDR) needs the HW VPP to
	// fold p010 down to the 8-bit nv12 the h264 encoder takes (hwScaleFilter's
	// format=nv12). Some drivers — Intel i965 VEBOX — abort (SIGABRT)
	// the moment a 10-bit surface reaches scale_vaapi. Unless the calibration
	// proved the 10-bit VPP works, software-transcode the source instead of
	// crashing the GPU pipeline and churning through a demotion.
	// Exception: when this request HW-tone-maps (hwTonemap), the GPU tone-mapper
	// (tonemap_vaapi / vpp_qsv) folds the 10-bit p010 down to 8-bit nv12 itself,
	// so the scale_vaapi-on-p010 hazard the gate guards against never arises —
	// the 10-bit source can take the HW path even without HW10BitTranscode.
	if sourceTenBit && !cap.HW10BitTranscode && !hwTonemap {
		return "", "", nil, false
	}
	return cap.HWBackend, cap.HWDevice, cap.HWFeatures, true
}

// hwRouteUsable is the selector's routing gate: does the HARDWARE producer
// take this request? It must answer exactly like effectiveVideoCodec tags the
// fingerprint - the route decides which physical resource the session burns
// (GPU vs CPU pools), so a gate/fingerprint disagreement makes admission price
// one pool while the spawn consumes the other. In particular it replays the
// AV1 fallback: an AV1-preferring client on an AV1-disabled (or AV1-incapable)
// host is ENCODED as H.264, so the route must be resolved for H.264 too.
func (h *hlsVODServe) hwRouteUsable(decision domaintranscoding.PlaybackDecision, src apptranscoding.SourceDims) bool {
	hwTonemap := h.hwTonemapWanted(decision)
	codec := decision.VideoCodec
	_, _, _, usable := h.resolveHW(codec, src.Codec, src.TenBit, hwTonemap)
	if strings.EqualFold(codec, "av1") && ((h.av1Enabled != nil && !h.av1Enabled()) || !usable) {
		_, _, _, usable = h.resolveHW("h264", src.Codec, src.TenBit, hwTonemap)
	}
	return usable
}

// hwTonemapWanted reports whether this request should tone-map HDR->SDR on the
// GPU: the decision needs tone-mapping, the toggle is on, the host probed
// HWTonemap, and the backend has a wired HW tone-map filter (vaapi/qsv — NVENC's
// libplacebo path is not modelled, so it tone-maps in software). When false the
// software tonemap path runs, byte-identical to a host with no GPU tone-mapping.
func (h *hlsVODServe) hwTonemapWanted(decision domaintranscoding.PlaybackDecision) bool {
	if h.hwTonemapEnabled != nil && !h.hwTonemapEnabled() {
		return false
	}
	if h.hwAccelEnabled == nil || !h.hwAccelEnabled() || h.capability == nil {
		return false
	}
	cap := h.capability()
	if !cap.HWTonemap || (cap.HWBackend != "vaapi" && cap.HWBackend != "qsv") {
		return false
	}
	return slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed")
}

// profilesForDecision picks the ABR ladder for a given (decision, mediaItem,
// streams) triple. Reads the per-title complexity class persisted by the
// background ComplexityWorker; falls back to LadderFilmNormal when the
// item hasn't been classified yet (safe: matches pre-refactor behaviour).
// Caps the ladder to the source resolution so we never advertise a 4K rung
// for a 1080p source.
func (h *hlsVODServe) profilesForDecision(ctx context.Context, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, decision domaintranscoding.PlaybackDecision) []domaintranscoding.HLSProfile {
	class := domaintranscoding.ClassFilmNormal
	if h.complexityRepo != nil && item != nil {
		resolved := false
		if h.classLookup != nil {
			if c, ok := h.classLookup.Get(string(item.ID)); ok {
				class, resolved = c, true
			}
		}
		if !resolved {
			if entry, found, err := h.complexityRepo.Get(ctx, string(item.ID)); err == nil && found && entry.Class.IsValid() {
				class = entry.Class
			}
			if h.classLookup != nil {
				// Cache the RESOLVED class (default included) so an unclassified
				// title doesn't re-query on every segment either.
				h.classLookup.Set(string(item.ID), class, mediaLookupTTL)
			}
		}
	}
	// COMMON HEAD (shared by both producers): pick the per-title ladder. The
	// PRODUCER's pure Profiles() owns the tail — source cap + height ceiling +
	// (software) single-rendition collapse — so the manifest-time and
	// segment-time shaping is identical and routes through the SAME producer
	// (the cache-hash invariant: drift here = permanent cache miss).
	base := domaintranscoding.LadderFor(class)
	src := sourceDimsOf(streams)

	capability := domaintranscoding.SafeFallback()
	if h.capability != nil {
		capability = h.capability()
	}

	var laddered []domaintranscoding.HLSProfile
	switch {
	case h.isCopyRequest(decision, streams, sourceBytesOf(item)):
		// Copy/remux rung: a single source-resolution rung, no transcode ladder
		// (the player never ABR-switches a copy stream). Same producer the
		// segment path resolves, so manifest- and segment-time shaping agree.
		laddered = h.copyProducer.Profiles(base, src, decision, capability)
	case h.selector != nil:
		// Production path: the wired selector routes to the software or hardware
		// producer (reusing resolveHW), and the producer's pure Profiles() owns
		// the shaping tail. This is the single source of truth for ladder shape;
		// manifest-time and segment-time both reach it via the same selector.
		laddered = h.selector.For(decision, src).Profiles(base, src, decision, capability)
	default:
		// No selector wired (hand-constructed test handlers). Reproduce the
		// legacy shaping so unit tests that probe profilesForDecision directly
		// keep their contract without standing up a full producer pair.
		laddered = h.legacyProfileShaping(base, src, streams, decision, capability)
	}

	// CMCD-feedback adaptive override (Phase C). Variant-neutral: it only lowers
	// the ceiling on a source-class showing sustained starvation (MIN with the
	// benchmark cap), never raises it. Kept in the handler (not the pure
	// Profiles) because the override is mutable runtime state read from
	// playback_qoe_events. Applied AFTER the producer shaped the ladder so it
	// trims the result identically at manifest- and segment-time (the override
	// value is stable across a request pair).
	if h.capOverride != nil && src.Height > 0 {
		if ov, ok := h.capOverride(sourceVideoCodec(streams), src.Height); ok && ov.MaxHeight > 0 {
			laddered = domaintranscoding.CapLadderToHeight(laddered, ov.MaxHeight)
		}
	}

	// AV1 max-quality cap: when AV1 is the EFFECTIVE output codec, an admin can
	// hold its ladder below the source/GPU ceiling (transcoding.av1_max_height,
	// 0 = no cap). `decision` here still carries the raw preferred codec, so
	// recompute the post-gate codec. Applied here (like the CMCD override) so the
	// manifest- and segment-time ladders trim identically — the cache-hash
	// invariant holds because both reach this shared head with the same inputs.
	if h.av1MaxHeight != nil {
		if m := h.av1MaxHeight(); m > 0 {
			if ec, _, _, _, _ := h.effectiveVideoCodec(decision, streams); strings.EqualFold(ec, "av1") {
				laddered = domaintranscoding.CapLadderToHeight(laddered, m)
			}
		}
	}

	// Load-forced route and ceiling (the third cap, same shared-head
	// discipline as the two above): when the host cannot afford this stream's
	// top rung but CAN afford a lower one, serve that one instead of refusing
	// the viewer - and when a HARDWARE stream finds its whole GPU pool full
	// while the CPU pool has room, spill it to the software route entirely.
	// Both the route and the ceiling are PINNED per stream by the admission
	// shaper, so the manifest a player holds and every segment it fetches
	// shape the same ladder - the cache-hash invariant that rules this whole
	// function. Copy requests are exempt: they never take a capacity slot.
	if h.admissionRoute != nil && len(laddered) > 0 &&
		!h.isCopyRequest(decision, streams, sourceBytesOf(item)) {
		natural := h.ladderClassFor(decision, streams)
		softwareShape := func() []domaintranscoding.HLSProfile {
			if h.selector == nil {
				return nil
			}
			sw := h.selector.Software()
			if sw == nil {
				return nil
			}
			return sw.Profiles(base, src, decision, capability)
		}
		var fallback func() []int
		if natural == apptranscoding.LadderClassHW && h.selector != nil {
			fallback = func() []int { return profileHeights(softwareShape()) }
		}
		g := h.admissionRoute(h.admissionPinKey(item, streams, decision), natural, profileHeights(laddered), fallback)
		if g.OK {
			if g.Class != natural && g.Class == apptranscoding.LadderClassSoftware {
				if swLadder := softwareShape(); len(swLadder) > 0 {
					laddered = swLadder
				}
			}
			if g.Ceiling > 0 {
				laddered = domaintranscoding.CapLadderToHeight(laddered, g.Ceiling)
			}
		}
	}
	return laddered
}

// profileHeights lists the heights of a shaped ladder, in order.
func profileHeights(profiles []domaintranscoding.HLSProfile) []int {
	out := make([]int, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, p.Height)
	}
	return out
}

// ladderClassFor derives, from the request alone, the LadderClass this
// stream's session will be billed as. It MUST agree with
// apptranscoding.LadderClassOf on the fingerprint the same request builds -
// the producers charge LadderClassOf(fp) at spawn, so a disagreement here
// makes admission price one resource pool while the spawn consumes another.
// TestLadderClassFor_AgreesWithFingerprint locks the equivalence.
func (h *hlsVODServe) ladderClassFor(decision domaintranscoding.PlaybackDecision, streams []*domainlibrary.MediaStream) apptranscoding.LadderClass {
	ec, accel, _, _, usable := h.effectiveVideoCodec(decision, streams)
	switch {
	case usable && accel != "":
		return apptranscoding.LadderClassHW
	case strings.EqualFold(ec, "av1"):
		return apptranscoding.LadderClassSoftwareAV1
	case slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed"):
		return apptranscoding.LadderClassSoftwareHDR
	default:
		return apptranscoding.LadderClassSoftware
	}
}

// legacyProfileShaping reproduces the ladder tail for hand-built test handlers
// that have no wired selector. Production NEVER takes this path — it routes
// through h.selector to the real software/hardware producer (whose pure
// Profiles owns the shaping). This mirrors SoftwareProducer.Profiles exactly
// (single-rendition collapses to ONE rung at the sustainable software ceiling,
// maxH), so the test-only fallback can't drift from the production shape.
func (h *hlsVODServe) legacyProfileShaping(base []domaintranscoding.HLSProfile, src apptranscoding.SourceDims, streams []*domainlibrary.MediaStream, decision domaintranscoding.PlaybackDecision, capability domaintranscoding.EncodeCapability) []domaintranscoding.HLSProfile {
	capped := domaintranscoding.CapLadderToSource(base, src.Width, src.Height)

	hwUsable := false
	maxH := 720
	if h.capability != nil {
		_, _, _, hwUsable = h.resolveHW(decision.VideoCodec, sourceVideoCodec(streams), src.TenBit, h.hwTonemapWanted(decision))
		maxH = capability.MaxHeightFor(hwUsable)
		if !hwUsable && capability.SoftwareHDRMaxHeight > 0 &&
			capability.SoftwareHDRMaxHeight < maxH &&
			slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed") {
			maxH = capability.SoftwareHDRMaxHeight
		}
	}
	// Mirrors SoftwareProducer.Profiles: the natural ladder capped to the
	// sustainable ceiling (with its 360p floor), no single-rendition collapse,
	// and the top surviving rung inheriting the first dropped rung's bitrate
	// budget when the ceiling (not the source) trimmed the ladder.
	heightCapped := domaintranscoding.CapLadderToHeight(capped, maxH)
	return domaintranscoding.BoostCappedTopRung(capped, heightCapped)
}
