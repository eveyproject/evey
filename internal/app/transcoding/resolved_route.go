package transcoding

import (
	"strings"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// resolved_route.go - the ONE derivation of the route (ADR-0030). Resolve
// computes a ResolvedRoute once; the ladder, the admission key, the
// fingerprint, the billed class and the producer pick are projections of it
// and decide nothing themselves.

// SpillReason says why a GPU-eligible request is served from the CPU. "" =
// the route is the natural one.
type SpillReason string

const (
	SpillNone         SpillReason = ""
	SpillPoolFull     SpillReason = "pool_full"     // admission pinned the stream to the CPU route
	SpillTitleBlocked SpillReason = "title_blocked" // the title's GPU route failed recently (HWRouteBlocklist)
)

// BackendSelection is the FROZEN accelerator pick the builder needs: the
// backend, its device node and the features the operator allows on it.
// Zero when the CPU encodes.
type BackendSelection struct {
	Name     string
	Device   string
	Features []string
}

// ResolvedRoute is what the plan resolved about HOW the session encodes: the
// codec the segments carry, the pipeline shape, the accelerator, whether it
// was spilled off the GPU, and the two GPU filtergraph options that are not
// topology (a session-level deinterlacer, a GPU subtitle compositor - they
// change the filters, not the pools or the calibration shape).
type ResolvedRoute struct {
	Codec    string
	Topology transcoding.Topology
	Backend  BackendSelection
	Spill    SpillReason
	// Copy: the single remux rung; no encode, never priced.
	Copy bool
	// Deinterlace / Overlay: GPU runtime options, only meaningful when the
	// accelerator encodes.
	Deinterlace bool
	Overlay     bool
}

// HW reports whether the accelerator encodes.
func (r ResolvedRoute) HW() bool { return r.Topology.HW() }

// Spilled reports whether the route was rewritten for the CPU.
func (r ResolvedRoute) Spilled() bool { return r.Spill != SpillNone }

// Class is the ladder class the session is billed as - the same answer
// LadderClassOf gives on the fingerprint this route projects to (locked by
// TestResolvedRoute_ProjectionsAgree).
func (r ResolvedRoute) Class() LadderClass {
	switch {
	case r.HW():
		return LadderClassHW
	case strings.EqualFold(strings.TrimSpace(r.Codec), "av1"):
		return LadderClassSoftwareAV1
	case r.Topology.ToneMap != transcoding.ToneMapNone:
		return LadderClassSoftwareHDR
	}
	return LadderClassSoftware
}

// CostKey is the route admission prices and the calibration measured.
func (r ResolvedRoute) CostKey() transcoding.RouteCostKey {
	return transcoding.RouteCostKeyFor(transcoding.HWAccelType(r.Backend.Name), transcoding.RouteEncoderName(r.Codec, r.Backend.Name), r.Topology.CostClass())
}

// Encode projects the route onto the ladder engine's EncodeRoute.
func (r ResolvedRoute) Encode() transcoding.EncodeRoute {
	t := r.Topology
	return transcoding.EncodeRoute{
		Codec:          r.Codec,
		Accel:          transcoding.HWAccelType(r.Backend.Name),
		HWUsable:       t.HW(),
		SoftwareDecode: t.SoftwareDecode(),
		Tonemap:        t.ToneMap != transcoding.ToneMapNone,
		HWTonemap:      t.ToneMap == transcoding.ToneMapGPU,
		Copy:           r.Copy,
		HDRPassthrough: t.Output == transcoding.OutputHDRPassthrough,
	}
}

// spill is the CPU route a hardware route falls back to: the codec the
// software encoder will carry (AV1 rides hardware only; an auto-upgraded HEVC
// goes back to H.264), no accelerator, the tone-map on the CPU - an HDR
// passthrough spilled becomes a tone-map, since no GPU keeps the range.
func (r ResolvedRoute) spill(why SpillReason, decision transcoding.PlaybackDecision) ResolvedRoute {
	out := r
	out.Spill = why
	out.Backend = BackendSelection{}
	out.Deinterlace, out.Overlay = false, false
	tonemap := transcoding.ToneMapNone
	if r.Topology.ToneMap != transcoding.ToneMapNone || r.Topology.Output == transcoding.OutputHDRPassthrough {
		tonemap = transcoding.ToneMapCPU
	}
	out.Topology = transcoding.MustTopology(transcoding.StageCPU, transcoding.StageCPU, tonemap, transcoding.OutputSDR)
	if strings.EqualFold(r.Codec, "av1") || (strings.EqualFold(r.Codec, "hevc") && !strings.EqualFold(decision.VideoCodec, "hevc")) {
		out.Codec = "h264"
	}
	return out
}

// HWFailureReport is what a failed HARDWARE session reports upward (ADR-0030,
// 7): the failure class AND the exact route that failed - backend, encoder,
// cost class - so a host verdict can target the proof of that route rather
// than "the codec". Codec is the route's output codec, kept explicit for the
// demotion rules that are still keyed on it.
type HWFailureReport struct {
	Class transcoding.FailureClass
	Codec string
	Route transcoding.RouteCostKey
}

// AdmissionGrant is what admission answered for this plan's stream. Nothing
// here is derivable from the route: the billed class is Route.Class(), a
// spill is Route.Spill.
type AdmissionGrant struct {
	// PinKey identifies the stream's pin in the shaper ("" = no admission wired).
	PinKey string
	// Granted: the stream may spawn. False = both pools refused it (the caller
	// sheds); also false when no admission is wired.
	Granted bool
	// Ceiling is the tallest rung admission allowed (0 = none applied, or the
	// ladder was read frozen off the pin, which already carries it).
	Ceiling int
}
