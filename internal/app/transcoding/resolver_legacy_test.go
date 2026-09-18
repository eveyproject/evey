package transcoding

// resolver_legacy_test.go - the single-derivation entry points of the old
// resolver (codec, backend, route gate, ladder, class, fingerprint, producer,
// pin) kept for THESE tests only. Production resolves a whole PlaybackPlan
// (Resolve) and projects plan.Route; nothing outside this package reaches a
// sub-derivation (contractguard). Each wrapper is the same code path Resolve
// runs, so a test here still pins the production behaviour.

import (
	"context"
	"slices"
	"strings"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

// (moved from resolver_route.go)
// EffectiveVideoCodec applies the capability gate to the decision's chosen
// output codec and returns the codec the segments will ACTUALLY carry together
// with the HW resolution for that codec. AV1 software (libsvtav1) is several
// times heavier than H.264 and the weak-NAS target can't sustain it, so when AV1
// is requested but the host has no HW AV1 encoder AND no admin opt-in, fall
// back to H.264 rather than starve. Shared by the fingerprint (the cache key)
// and the manifest builder (the advertised CODECS) so the master playlist can
// never advertise a codec the segments don't carry. Re-resolves HW for the
// fallback codec (H.264 may be HW-encodable where AV1 was not).
func (pr *PlanResolver) EffectiveVideoCodec(decision transcoding.PlaybackDecision, streams []*library.MediaStream) (codec, accel, device string, features []string, usable, softwareDecode bool) {
	return pr.effectiveVideoCodec(pr.hwPolicy(), decision, streams)
}

// (moved from resolver_route.go)
// ResolveHW decides the hardware encode backend for a chosen output codec.
// Returns ("", "", false) - the software path - unless the flag is on, the
// host calibrated a HW backend, and that backend can encode this codec. The
// boolean is both the ladder's hwAccelUsable (taller HW ceiling) and the
// signal that tags the fingerprint so the orchestrator runs the GPU pipeline.
// A legacy entry point: no burn-in need and no passthrough are declared, so
// the builder's port judges the plain topology only.
func (pr *PlanResolver) ResolveHW(outputCodec, sourceCodec string, sourceTenBit, hwTonemap bool) (accel, device string, features []string, usable, softwareDecode bool) {
	return pr.resolveHW(pr.hwPolicy(), outputCodec, hwSource{Codec: sourceCodec, TenBit: sourceTenBit}, hwTonemap)
}

// (moved from resolver_route.go)
// HWRouteUsable is the selector's routing gate: does the HARDWARE producer
// take this request? It must answer exactly like EffectiveVideoCodec tags the
// fingerprint - the route decides which physical resource the session burns
// (GPU vs CPU pools), so a gate/fingerprint disagreement makes admission price
// one pool while the spawn consumes the other. In particular it replays the
// AV1 fallback: an AV1-preferring client on an AV1-disabled (or AV1-incapable)
// host is ENCODED as H.264, so the route must be resolved for H.264 too.
func (pr *PlanResolver) HWRouteUsable(decision transcoding.PlaybackDecision, src SourceDims) bool {
	pol := pr.hwPolicy()
	hwTonemap := pr.hwTonemapWanted(pol, decision, src.HDRRange)
	// No GPU tone-mapper: the CPU tone-maps between the GPU decode and encode
	// (hwTonemapBridge). That needs the GPU to decode the source; a software
	// decode keeps the whole job on the CPU. Same answer as EffectiveVideoCodec.
	bridge := !hwTonemap && slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed")
	needs := pr.routeNeeds(pol, decision)
	resolve := func(c string) bool {
		hs := hwSource{Codec: src.Codec, TenBit: src.TenBit && !bridge, PixelFormat: src.PixelFormat, Interlaced: src.Interlaced,
			WantHeight: wantedRung(decision, src.Height), Bridge: bridge, Needs: needs,
			Passthrough: c == "hevc" && pr.passthroughWanted(pol, decision, src.HDRRange)}
		_, _, _, usable, softwareDecode := pr.resolveHW(pol, c, hs, hwTonemap)
		return usable && !pr.hwRouteRefused(src, bridge, softwareDecode)
	}
	codec := pr.outputCodecFor(pol, decision, nil)
	usable := resolve(codec)
	if strings.EqualFold(codec, "av1") && (!pol.AV1 || !usable) {
		usable = resolve("h264")
	}
	if codec == "hevc" && decision.VideoCodec != "hevc" && !usable {
		usable = resolve("h264")
	}
	return usable
}

// (moved from resolver_route.go)
// HWTonemapWanted reports whether this request should tone-map HDR->SDR on the
// GPU: the decision needs tone-mapping, the toggle is on, the host probed
// HWTonemap, and the backend has a wired HW tone-map filter (VAAPI OpenCL/VPP,
// NVENC tonemap_cuda; QSV's vpp is ineligible). When false the software
// tonemap path runs, byte-identical to a host with no GPU tone-mapping.
func (pr *PlanResolver) HWTonemapWanted(decision transcoding.PlaybackDecision, hdrRange string) bool {
	return pr.hwTonemapWanted(pr.hwPolicy(), decision, hdrRange)
}

// (moved from resolver_route.go)
// HWDeinterlaceWanted reports whether the resolved GPU session should run the
// backend's SESSION-level deinterlacer: the source is known interlaced (flag or
// idet) and the backend is one whose deinterlacer is session-level and proven
// (QSV). Per-frame backends answer false - their filter is always emitted.
func (pr *PlanResolver) HWDeinterlaceWanted(streams []*library.MediaStream, accel string) bool {
	return pr.hwDeinterlaceWanted(SourceInterlaced(streams), accel)
}

// (moved from resolver_route.go)
// HDRPassthroughWanted: an HDR source transcoded for an HDR client that decodes
// HEVC, on a GPU probed for Main10 encoding - keep the HDR instead of tone-mapping.
func (pr *PlanResolver) HDRPassthroughWanted(decision transcoding.PlaybackDecision, streams []*library.MediaStream) bool {
	return pr.hdrPassthroughWanted(pr.hwPolicy(), decision, streams)
}

// (moved from resolver_route.go)
// HWOverlayWanted reports whether this request should burn its bitmap subtitle
// in ON THE GPU: the decision actually burns one, HW accel is on, and the host
// probed a working overlay compositor for its backend.
//
// It exists because a burn-in used to disqualify the hardware path outright, so
// one flagged-default forced subtitle sent an entire 4K HDR session - decode,
// tone-map and four encodes - to the CPU on a host whose GPU did all three
// (prod, 2026-09-09). Every backend is eligible here: unlike the tone-map, the
// compositor is wired for vaapi, qsv AND cuda, and the calibration probe is what
// says whether this host's driver honours it.
func (pr *PlanResolver) HWOverlayWanted(decision transcoding.PlaybackDecision) bool {
	return pr.hwOverlayWanted(pr.hwPolicy(), decision)
}

// (moved from resolver_ladder.go)
// LadderClassFor is the class of the NATURAL route (before the per-title
// block and the admission spill) - what admission is offered first. The
// billed class of a plan is plan.Route.Class(); this stays for the adapter's
// wrappers and tests.
func (pr *PlanResolver) LadderClassFor(decision transcoding.PlaybackDecision, streams []*library.MediaStream) LadderClass {
	return pr.naturalRoute(PlaybackRequest{Streams: streams}, decision, false).Class()
}

// (moved from resolver_ladder.go)
// ShapeLadder picks the ABR ladder for a given (decision, media, streams)
// triple. Reads the per-title complexity class persisted by the background
// ComplexityWorker; falls back to LadderFilmNormal when the item hasn't been
// classified yet (safe: matches pre-refactor behaviour). Caps the ladder to
// the source resolution so we never advertise a 4K rung for a 1080p source.
func (pr *PlanResolver) ShapeLadder(ctx context.Context, req PlaybackRequest, decision transcoding.PlaybackDecision) []transcoding.HLSProfile {
	isCopy := pr.IsCopy(decision, req.Streams, req.SourceBytes)
	route := pr.resolveRoute(req, decision, isCopy)
	profiles, _, _ := pr.shapeLadderFor(ctx, req, decision, route, pr.pinKeyFor(req, decision, route))
	return profiles
}

// (moved from resolver_ladder.go)
// PinKey identifies a stream for the admission shaper - the (request, route)
// form for callers outside Resolve; Resolve itself uses pinKeyFor on the route
// it just resolved.
func (pr *PlanResolver) PinKey(req PlaybackRequest, decision transcoding.PlaybackDecision) string {
	return pr.pinKeyFor(req, decision, pr.resolveRoute(req, decision, false))
}

// (moved from resolver_producer.go)
// ProducerFor picks the producer for an already-resolved decision, so the
// producer chosen for a segment matches the one the ladder was shaped for
// (and thus the cache hash). Falls back to the orchestrator when no selector
// is wired (hand-built test handlers); returns nil only when neither is set.
func (pr *PlanResolver) ProducerFor(req PlaybackRequest, decision transcoding.PlaybackDecision, isCopy bool) StreamProducer {
	return pr.producerForRoute(pr.routeWithPin(req, decision, isCopy))
}

// (moved from resolver_fingerprint.go)
// Fingerprint assembles the DecisionFingerprint - the cache-key object - from
// the resolved (decision, profiles) pair. Must be byte-identical between the
// manifest-time and segment-time resolutions of the same request shape, or
// the cache lookup misses on every segment.
func (pr *PlanResolver) Fingerprint(ctx context.Context, req PlaybackRequest, decision transcoding.PlaybackDecision, profiles []transcoding.HLSProfile) DecisionFingerprint {
	isCopy := pr.IsCopy(decision, req.Streams, req.SourceBytes)
	route := pr.resolveRoute(req, decision, isCopy)
	return pr.fingerprintFor(ctx, req, decision, pr.withPin(req, decision, route), profiles, pr.pinKeyFor(req, decision, route))
}
