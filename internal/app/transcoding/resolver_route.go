package transcoding

// resolver_route.go - the ONE derivation of the route (ADR-0030): the effective
// codec and backend, the pipeline topology, the tone-map placement, the HDR
// passthrough, the per-title block, and the operator's hardware policy.
// Everything below Resolve reads plan.Route; nothing here is called by the
// projections.

import (
	"log/slog"
	"math"
	"slices"
	"strings"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/metrics"
)

// resolveRoute is the one derivation of the route: the effective codec and
// backend (EffectiveVideoCodec: capability gate, backend pick, codec
// fallbacks, whole-session refusals), the tone-map placement, the HDR
// passthrough, the GPU runtime options, then the per-title block. Admission
// may still spill it (Resolve).
func (pr *PlanResolver) resolveRoute(req PlaybackRequest, decision transcoding.PlaybackDecision, isCopy bool) ResolvedRoute {
	route := pr.naturalRoute(req, decision, isCopy)
	if route.HW() && pr.titleBlocked(req.MediaID) {
		route = route.spill(SpillTitleBlocked, decision)
	}
	return route
}

// naturalRoute is the route before the per-title block and the admission
// spill: what the host would serve this request with, all else being free.
func (pr *PlanResolver) naturalRoute(req PlaybackRequest, decision transcoding.PlaybackDecision, isCopy bool) ResolvedRoute {
	snap := snapshot(req)
	// The operator's toggles were read ONCE by Resolve (req.Policy) and are
	// threaded down: a toggle flipped mid-request cannot allow the GPU at the
	// pick and refuse its features a line later.
	pol := pr.policy(req)
	codec, accel, device, features, usable, softwareDecode := pr.effectiveVideoCodec(pol, decision, req.Streams)
	hwUsable := usable && accel != ""
	passWanted := pr.hdrPassthroughWanted(pol, decision, req.Streams)
	// The HDR passthrough only exists on the GPU: wanted but not served by
	// one (the source the GPU cannot decode, a spill, a blocked title), the
	// CPU tone-maps, and the ladder takes the HDR floor the session really
	// runs under.
	tonemap := slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed") || (!hwUsable && passWanted)
	er := transcoding.EncodeRoute{
		Codec:          codec,
		Accel:          transcoding.HWAccelType(accel),
		HWUsable:       hwUsable,
		SoftwareDecode: softwareDecode,
		Tonemap:        tonemap,
		HWTonemap:      hwUsable && pr.hwTonemapWanted(pol, decision, snap.HDRRange),
		HDRPassthrough: hwUsable && passWanted,
	}
	route := ResolvedRoute{
		Codec:       codec,
		Topology:    er.Topology(),
		Copy:        isCopy,
		Deinterlace: hwUsable && pr.hwDeinterlaceWanted(snap.Interlaced, accel),
		Overlay:     hwUsable && pr.hwOverlayWanted(pol, decision),
	}
	if hwUsable {
		route.Backend = BackendSelection{Name: accel, Device: device, Features: features}
		pr.shadowExpressibility(route, decision)
	}
	return route
}

// shadowExpressibility asks the builder whether it can emit the route the
// resolver just picked. A refusal here is a planner/builder disagreement:
// counted and logged, never acted on yet - the builder still falls back on
// its own gates, and the two must read zero disagreements before it refuses.
func (pr *PlanResolver) shadowExpressibility(route ResolvedRoute, decision transcoding.PlaybackDecision) {
	if pr.Expressibility == nil {
		return
	}
	needs := transcoding.RouteNeeds{BurnInBitmap: decision.SubtitleDecision == "burn", GPUOverlay: route.Overlay}
	if err := pr.Expressibility.Expressible(route.Topology, transcoding.HWAccelType(route.Backend.Name), route.Backend.Device, route.Backend.Features, needs); err != nil {
		metrics.HLSVODRouteNotExpressible.Inc()
		slog.Warn("plan resolver: resolved GPU route the builder cannot express (planner/builder disagreement, ADR-0030)",
			slog.String("topology", route.Topology.String()), slog.String("accel", route.Backend.Name), slog.Any("err", err))
	}
}

// routeWithPin is the resolved route as the LIVE pin has it: a caller that
// reaches a sub-derivation without Resolve (the adapter's wrappers, tests)
// still sees the admission spill of a stream already pinned to the CPU.
func (pr *PlanResolver) routeWithPin(req PlaybackRequest, decision transcoding.PlaybackDecision, isCopy bool) ResolvedRoute {
	return pr.withPin(req, decision, pr.resolveRoute(req, decision, isCopy))
}

// withPin applies the LIVE pin's spill to a resolved route.
func (pr *PlanResolver) withPin(req PlaybackRequest, decision transcoding.PlaybackDecision, route ResolvedRoute) ResolvedRoute {
	if route.HW() && pr.AdmissionPinnedClass != nil {
		if cls, ok := pr.AdmissionPinnedClass(pr.pinKeyFor(req, decision, route)); ok && cls != LadderClassHW {
			route = route.spill(SpillPoolFull, decision)
		}
	}
	return route
}

// HWPolicy is the operator's hardware policy as ONE value: what the toggles
// say, read through one accessor instead of at eleven sites (ADR-0030, 8).
// A nil toggle keeps its historical default.
type HWPolicy struct {
	// Accel: the GPU may be used at all (nil toggle = false: no GPU).
	Accel bool
	// Tonemap: the GPU may tone-map (nil toggle = true).
	Tonemap bool
	// LowPower: the Intel low-power (VDEnc) features may be used (nil = false).
	LowPower bool
	// AV1: AV1 output may be served (nil toggle = true, historical).
	AV1 bool
	// AV1MaxHeight caps AV1 output (0 = none).
	AV1MaxHeight int
	taken        bool
}

// Taken reports whether the policy was read (a zero PlaybackRequest.Policy
// means the caller skipped Resolve; the resolver then reads it itself).
func (p HWPolicy) Taken() bool { return p.taken }

// policy returns the request's policy snapshot, reading it when a caller
// reached a sub-derivation without going through Resolve.
func (pr *PlanResolver) policy(req PlaybackRequest) HWPolicy {
	if req.Policy.Taken() {
		return req.Policy
	}
	return pr.hwPolicy()
}

// hwPolicy reads the toggles. The capability is a separate question (a policy
// that allows the GPU on a host that has none allows nothing).
func (pr *PlanResolver) hwPolicy() HWPolicy {
	pol := HWPolicy{Tonemap: true, AV1: true, taken: true}
	if pr.HWAccelEnabled != nil {
		pol.Accel = pr.HWAccelEnabled()
	}
	if pr.HWTonemapEnabled != nil {
		pol.Tonemap = pr.HWTonemapEnabled()
	}
	if pr.HWLowPowerEnabled != nil {
		pol.LowPower = pr.HWLowPowerEnabled()
	}
	if pr.AV1Enabled != nil {
		pol.AV1 = pr.AV1Enabled()
	}
	if pr.AV1MaxHeight != nil {
		pol.AV1MaxHeight = pr.AV1MaxHeight()
	}
	return pol
}

// gpuAllowed: the policy allows the GPU and the host calibrated one.
func (pr *PlanResolver) gpuAllowed(pol HWPolicy) bool {
	return pol.Accel && pr.Capability != nil
}

// capabilityOrFallback resolves the calibrated capability, nil-safe.
func (pr *PlanResolver) capabilityOrFallback() transcoding.EncodeCapability {
	if pr.Capability == nil {
		return transcoding.SafeFallback()
	}
	return pr.Capability()
}

// hwSource is what the GPU decode gate reads about the source.
type hwSource struct {
	Codec       string
	TenBit      bool
	PixelFormat string
	// Interlaced: a backend whose deinterlacer is session-level (QSV) must
	// have proven it correct to take this source at all (DeinterlacesOnGPU).
	Interlaced bool
	// WantHeight is the tallest rung the session would ask for (wantedRung);
	// 0 = unknown. A backend certified under it loses to one that reaches it.
	WantHeight int
	// Bridge: the CPU tone-maps between the GPU decode and encode. The backend
	// must have proven that route and must decode the source itself.
	Bridge bool
}

// ResolveHW decides the hardware encode backend for a chosen output codec.
// Returns ("", "", false) - the software path - unless the flag is on, the
// host calibrated a HW backend, and that backend can encode this codec. The
// boolean is both the ladder's hwAccelUsable (taller HW ceiling) and the
// signal that tags the fingerprint so the orchestrator runs the GPU pipeline.
// HDR tonemap / bitmap burn-in are not gated here: the orchestrated VOD path
// does neither today, and the ffmpeg builder independently falls back to
// software for those, so the two stay consistent.
func (pr *PlanResolver) ResolveHW(outputCodec, sourceCodec string, sourceTenBit, hwTonemap bool) (accel, device string, features []string, usable, softwareDecode bool) {
	return pr.resolveHW(pr.hwPolicy(), outputCodec, hwSource{Codec: sourceCodec, TenBit: sourceTenBit}, hwTonemap)
}

// wantedRung is the tallest rung a session for this source and client would
// ask for: the source height capped by the client's ceiling. 0 = unknown (no
// rung filter on the backend pick).
func wantedRung(decision transcoding.PlaybackDecision, sourceHeight int) int {
	h := sourceHeight
	if decision.MaxHeight > 0 && (h == 0 || decision.MaxHeight < h) {
		h = decision.MaxHeight
	}
	return h
}

// resolveHW is ResolveHW with the full source facts. Every GPU shape it can
// answer with has to be PROVEN by the calibration's route probes
// (EncodeCapability.HWRoutes): the filter-level facts say what the GPU can
// do, the route proof says the builder's session for it actually runs here.
func (pr *PlanResolver) resolveHW(pol HWPolicy, outputCodec string, src hwSource, hwTonemap bool) (accel, device string, features []string, usable, softwareDecode bool) {
	if !pr.gpuAllowed(pol) {
		return "", "", nil, false, false
	}
	// Choose the backend for THIS route and THIS source (not one global "best
	// host backend"): among the accelerators that are correct, prove the route,
	// support the codec AND serve this source (decode matrix, decoupled path,
	// 10-bit VPP, chroma layout, bridge), the one reaching the wanted rung at
	// the lowest measured cost. On a box with both, HDR->SDR takes VAAPI's
	// OpenCL tone-map while HEVC/SDR take a faster QSV; a QSV that cannot decode
	// this source hands it to a VAAPI that can, never to the CPU.
	bc, softwareDecode, ok := pr.pickBackend(outputCodec, src, hwTonemap)
	if !ok {
		return "", "", nil, false, false
	}
	return bc.Backend, bc.Device, effectiveHWFeatures(pol, bc.Features), true, softwareDecode
}

// backendServes reports whether backend bc can run THIS source on the route
// (the per-source gates that used to run AFTER the pick, so a backend chosen
// on cost could fail them and drop the whole request to software), and whether
// it would software-decode. The zero-copy pipeline HW-decodes the source. If
// this backend can't decode it (chroma/depth/codec), the source takes the
// DECOUPLED path (software decode + hwupload + GPU scale/encode), unless that
// is not expressible or honest: no upload chain / unproven decoupled route, a
// GPU tone-map (needs the 10-bit surface the upload folds away), a 10-bit
// source (its software decode is the expensive half - price it as CPU, not
// GPU), or a bridge (the download needs a GPU frame). A 10-bit source needs the
// HW VPP to fold p010->nv12 for the 8-bit encoder (proven), unless a GPU
// tone-map folds p010 itself. A chroma layout outside 4:2:0 8/10-bit rides the
// zero-copy chain only once that chain ran on it end-to-end for this backend.
func backendServes(cap transcoding.EncodeCapability, bc transcoding.HWBackendCap, src hwSource, hwTonemap bool) (softwareDecode, ok bool) {
	backend := transcoding.HWAccelType(bc.Backend)
	decodable := bc.CanDecodeSource(src.Codec, src.TenBit, src.PixelFormat, cap.ProbeVersion)
	softwareDecode = src.Codec != "" && !decodable
	if softwareDecode {
		if hwTonemap || src.Bridge || src.TenBit || (backend != transcoding.VAAPI && backend != transcoding.QSV && backend != transcoding.RKMPP) ||
			!bc.Proves(transcoding.HWRouteDecoupled) {
			return false, false
		}
	}
	if src.Bridge && !bc.Proves(transcoding.HWRouteBridge) {
		return false, false
	}
	if src.TenBit && !hwTonemap && (!bc.HW10Bit || !bc.Proves(transcoding.HWRouteSource10Bit)) {
		return false, false
	}
	if !softwareDecode && !transcoding.CanonicalDecodeFormat(src.PixelFormat) && !bc.Proves(transcoding.HWRouteSourceFormat(src.PixelFormat)) {
		return false, false
	}
	return softwareDecode, true
}

// pickBackend chooses the accelerator for a route and a source (ADR-0029), in this order:
//  1. COMPATIBILITY: the certified backends that prove the route's encode path,
//     support the codec and serve THIS source (backendServes: decode matrix,
//     decoupled path, 10-bit VPP, chroma layout, bridge proof, deinterlacer);
//  2. QUALITY: among them, those whose certified ceiling (the bridge ceiling
//     on a bridge) reaches the rung the session wants (src.WantHeight; when
//     none does, the tallest ceilings - the request is capped there anyway);
//  3. COST: the cheapest by measured route cost (RouteCosts) at that rung -
//     the larger of its accelerator and CPU pool fractions -
//     with a <5% hysteresis tie-break (the primary, then alphabetical) so the
//     pick does not flip on measurement noise across recalibrations.
//
// Filtering on the source BEFORE ranking is what lets a QSV that cannot decode
// a source hand it to a VAAPI that can: the incompatible backend is simply not
// in the field, so the next one is tried instead of the whole request dropping
// to software. A 720p-only accelerator never beats a 2160p one for a 2160p
// request however cheap it measures; it may win a 720p request. A tone-map
// route needs a backend proving HWRouteTonemap (only a GPU tone-mapper - QSV's
// vpp is ineligible - so HDR lands on VAAPI/OpenCL); everything else needs SDR
// (+HEVC / +AV1 for that output). nil RouteCosts or a pre-Backends capability
// falls back to the primary backend. Returns the pick and whether it will
// software-decode (the decoupled route).
func (pr *PlanResolver) pickBackend(outputCodec string, src hwSource, hwTonemap bool) (transcoding.HWBackendCap, bool, bool) {
	cap := pr.Capability()
	caps := cap.BackendCaps()
	if len(caps) == 0 {
		return transcoding.HWBackendCap{}, false, false
	}
	hevc := transcoding.CanonicalVideoCodec(outputCodec) == "hevc"
	av1 := transcoding.CanonicalVideoCodec(outputCodec) == "av1"
	qualifies := func(b transcoding.HWBackendCap) bool {
		if !b.SupportsCodec(outputCodec) || !b.Proves(transcoding.HWRouteSDR) {
			return false
		}
		if hevc && !b.Proves(transcoding.HWRouteHEVC) {
			return false
		}
		if av1 && !b.Proves(transcoding.HWRouteAV1) {
			return false
		}
		// An interlaced source on a session-level deinterlacer (QSV) needs the
		// probed+proven filter; a QSV that never proved it would serve combing.
		if src.Interlaced && !b.DeinterlacesOnGPU() {
			return false
		}
		if hwTonemap && !b.Proves(transcoding.HWRouteTonemap) {
			return false
		}
		return true
	}
	var model *RouteCostModel
	if pr.RouteCosts != nil {
		model = pr.RouteCosts()
	}
	// 1. Compatibility: route + codec + source.
	type candidate struct {
		b              transcoding.HWBackendCap
		softwareDecode bool
	}
	// routeOf is the cost route this candidate would really run for the source.
	routeOf := func(c candidate) transcoding.RouteCostClass {
		switch {
		case hwTonemap:
			return transcoding.RouteCostTonemap
		case src.Bridge:
			return transcoding.RouteCostBridge
		case c.softwareDecode:
			return transcoding.RouteCostDecoupled
		}
		return transcoding.RouteCostSDR
	}
	keyOf := func(c candidate) transcoding.RouteCostKey {
		return transcoding.RouteCostKeyFor(transcoding.HWAccelType(c.b.Backend), transcoding.RouteEncoderName(outputCodec, c.b.Backend), routeOf(c))
	}
	// ceilingOf is the quality a session on this candidate is really PROVEN
	// for: the backend's encode ceiling (the effective bridge ceiling on a
	// bridge - the software HDR ceiling when its bridge cleared no rung; the
	// CPU ceiling when it software-decodes, a decode the GPU ladder was never
	// benched behind), bounded by the tallest rung THIS route produced in the
	// route evidence. A QSV certified 2160p whose HEVC route only produced
	// 1080p is a 1080p HEVC backend here, and loses a 2160p request to a
	// VAAPI whose HEVC route produced 1440p. known=false only without a
	// route-aware model (legacy: the generic ceiling ranks, 0 = unknown).
	ceilingOf := func(c candidate) (h int, known bool) {
		h = cap.HWCeilingFor(c.b.Backend)
		if src.Bridge {
			h = cap.EffectiveBridgeCeilingFor(c.b.Backend)
		}
		if c.softwareDecode && cap.SoftwareMaxHeight > 0 && (h == 0 || cap.SoftwareMaxHeight < h) {
			h = cap.SoftwareMaxHeight
		}
		if model == nil {
			return h, h > 0
		}
		proven, _ := model.TallestRung(keyOf(c)) // 0 = the route produced nothing
		if h == 0 || proven < h {
			h = proven
		}
		return h, true
	}
	var field []candidate
	tallest := 0
	for _, b := range caps {
		if !qualifies(b) {
			continue
		}
		sd, ok := backendServes(cap, b, src, hwTonemap)
		if !ok {
			continue
		}
		c := candidate{b, sd}
		// A host WITH route evidence prices a named route ONLY on it: admission
		// refuses a (backend, encoder, route) it has no sample for. Picking it
		// here would send the session to the limiter to be refused and spilled
		// to software - so it is not in the field, and the codec fallback
		// (HEVC -> H.264) re-resolves onto a measured GPU route instead.
		if model != nil {
			if proven, _ := model.TallestRung(keyOf(c)); proven == 0 {
				continue
			}
		}
		field = append(field, c)
		h, _ := ceilingOf(c)
		tallest = max(tallest, h)
	}
	// 2. Quality: reach the wanted rung, else the tallest PROVEN quality (the
	// request is capped there anyway). A ceiling the legacy path cannot tell
	// (unknown) is never a reason to drop a backend.
	target := tallest
	if src.WantHeight > 0 && src.WantHeight < tallest {
		target = src.WantHeight
	}
	reaches := func(c candidate) bool {
		h, known := ceilingOf(c)
		return !known || h >= target
	}
	// 3. Cost: the route priced at the rung it would serve - the wanted rung,
	// bounded by the proven ceiling, 1080 when unknown - at film cadence. The
	// RELATIVE order is what matters. The price is the route's pressure on
	// its most constrained pool (max of the accelerator and CPU fractions),
	// the same figure MaxStreams caps on: ranking on the accelerator alone
	// preferred a decoupled/bridged route whose software decode or CPU
	// tone-map eats cores over one slightly dearer on the GPU and nearly free
	// on the CPU. Missing evidence => +inf (never preferred on cost).
	costOf := func(c candidate) float64 {
		if model == nil {
			return 0 // no cost model: every qualifying backend ties, tie-break decides
		}
		rung := 1080
		if src.WantHeight > 0 {
			rung = src.WantHeight
		}
		if h, _ := ceilingOf(c); h > 0 && h < rung {
			rung = h
		}
		if rc, ok := model.Cost(keyOf(c), rung, 24); ok {
			return math.Max(rc.Accel, rc.CPU)
		}
		return 1e9
	}
	var best candidate
	var bestCost float64
	found := false
	for _, c := range field {
		if !reaches(c) {
			continue
		}
		cost := costOf(c)
		if !found || betterBackend(c.b, cost, best.b, bestCost, cap.HWBackend) {
			best, bestCost, found = c, cost, true
		}
	}
	return best.b, best.softwareDecode, found
}

// betterBackend is pickBackend's ordering: strictly cheaper wins beyond a 5%
// hysteresis band; within the band the PRIMARY backend wins, then the
// alphabetically-smaller one - both stable across recalibrations.
func betterBackend(cand transcoding.HWBackendCap, candCost float64, cur transcoding.HWBackendCap, curCost float64, primary string) bool {
	if hi := max(candCost, curCost); hi > 0 && absFloat(candCost-curCost)/hi >= 0.05 {
		return candCost < curCost
	}
	if strings.EqualFold(cand.Backend, primary) != strings.EqualFold(cur.Backend, primary) {
		return strings.EqualFold(cand.Backend, primary)
	}
	return cand.Backend < cur.Backend
}

func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// burnInStaysSoftware
// burnInStaysSoftware mirrors the builder's own refusal (hwUsable): a bitmap
// subtitle burn-in without a probed GPU compositor is built as a SOFTWARE
// encode, so the route, the class and the pool must say software too - or a
// CPU x264 session runs under the GPU orchestrator, priced on the GPU pool,
// capped at the GPU ceiling and blamed on the GPU when it fails.
func (pr *PlanResolver) burnInStaysSoftware(pol HWPolicy, decision transcoding.PlaybackDecision) bool {
	return decision.SubtitleDecision == "burn" && !pr.hwOverlayWanted(pol, decision)
}

// titleBlocked reads the per-title GPU route block (nil-safe).
func (pr *PlanResolver) titleBlocked(mediaID string) bool {
	return pr.HWRouteBlocked != nil && mediaID != "" && pr.HWRouteBlocked(mediaID)
}

// effectiveHWFeatures drops the low-power features unless the admin opted in.
func effectiveHWFeatures(pol HWPolicy, features []string) []string {
	if pol.LowPower {
		return features
	}
	out := make([]string, 0, len(features))
	for _, f := range features {
		if !strings.HasPrefix(f, "low_power_") {
			out = append(out, f)
		}
	}
	return out
}

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

func (pr *PlanResolver) effectiveVideoCodec(pol HWPolicy, decision transcoding.PlaybackDecision, streams []*library.MediaStream) (codec, accel, device string, features []string, usable, softwareDecode bool) {
	dims := SourceDimsOf(streams)
	hwTonemap := pr.hwTonemapWanted(pol, decision, dims.HDRRange)
	// CPU tone-map bridged between GPU decode and encode: the GPU never sees the
	// 10-bit surface, but it must be the one decoding (the bridge downloads).
	bridge := !hwTonemap && slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed")
	resolve := func(c string) {
		accel, device, features, usable, softwareDecode = pr.resolveHW(pol, c, hwSource{Codec: dims.Codec, TenBit: dims.TenBit && !bridge, PixelFormat: dims.PixelFormat, Interlaced: dims.Interlaced, WantHeight: wantedRung(decision, dims.Height), Bridge: bridge}, hwTonemap)
		if pr.hwRouteRefused(pol, decision, dims, bridge, softwareDecode) {
			accel, device, features, usable, softwareDecode = "", "", nil, false, false
		}
	}
	codec = pr.codecWithPassthrough(pol, pr.outputCodecFor(pol, decision, streams), decision, streams)
	resolve(codec)
	if codec == "hevc" && decision.VideoCodec != "hevc" && !usable {
		codec = "h264"
		resolve(codec)
	}
	if strings.EqualFold(codec, "av1") && (!pol.AV1 || !usable) {
		codec = "h264"
		resolve(codec)
	}
	return codec, accel, device, features, usable, softwareDecode
}

// hwRouteRefused gathers the whole-session reasons a resolved GPU encode still
// stays on the CPU, shared by EffectiveVideoCodec and HWRouteUsable so the
// fingerprint, the class and the producer can never disagree: inverse
// telecine (a software-only filter), a bridge whose decode would be software
// (the download needs a GPU frame) or whose route the calibration did not
// prove, and a burn-in the builder refuses to composite on the GPU.
func (pr *PlanResolver) hwRouteRefused(pol HWPolicy, decision transcoding.PlaybackDecision, src SourceDims, bridge, softwareDecode bool) bool {
	if src.Telecine || pr.burnInStaysSoftware(pol, decision) {
		return true
	}
	// The bridge's own gates (a proven bridge route on the PICKED backend, a
	// GPU decode) are enforced per backend in pickBackend; this keeps the
	// belt for a caller that resolved without the bridge flag.
	if bridge && softwareDecode {
		return true
	}
	return false
}

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
	resolve := func(c string) bool {
		_, _, _, usable, softwareDecode := pr.resolveHW(pol, c, hwSource{Codec: src.Codec, TenBit: src.TenBit && !bridge, PixelFormat: src.PixelFormat, Interlaced: src.Interlaced, WantHeight: wantedRung(decision, src.Height), Bridge: bridge}, hwTonemap)
		return usable && !pr.hwRouteRefused(pol, decision, src, bridge, softwareDecode)
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

// outputCodecFor upgrades the decision's H.264 to HEVC when the client decodes it
// and the GPU encodes it; an explicit preference is never overridden. streams may be nil.
func (pr *PlanResolver) outputCodecFor(pol HWPolicy, decision transcoding.PlaybackDecision, streams []*library.MediaStream) string {
	codec := decision.VideoCodec
	if decision.VideoDecision != "transcode" || slices.Contains(decision.TranscodeReasons, "preferred_video_codec") ||
		slices.Contains(decision.TranscodeReasons, "video_codec_pinned") {
		return codec
	}
	if codec != "" && !strings.EqualFold(codec, transcoding.DefaultVideoCodec) {
		return codec
	}
	if !pr.clientDecodes(decision, "hevc") || !pr.gpuAllowed(pol) {
		return codec
	}
	if pr.Capability().AnyBackendSupports("hevc") {
		return "hevc"
	}
	return codec
}

// outputCodecForPassthrough: the HDR passthrough is HEVC by construction.
func (pr *PlanResolver) codecWithPassthrough(pol HWPolicy, codec string, decision transcoding.PlaybackDecision, streams []*library.MediaStream) string {
	if pr.hdrPassthroughWanted(pol, decision, streams) {
		return "hevc"
	}
	return codec
}

func (pr *PlanResolver) clientDecodes(decision transcoding.PlaybackDecision, codec string) bool {
	for _, c := range decision.ClientVideoCodecs {
		if transcoding.CanonicalVideoCodec(c) == codec {
			return true
		}
	}
	return false
}

// HDRPassthroughWanted: an HDR source transcoded for an HDR client that decodes
// HEVC, on a GPU probed for Main10 encoding - keep the HDR instead of tone-mapping.
func (pr *PlanResolver) HDRPassthroughWanted(decision transcoding.PlaybackDecision, streams []*library.MediaStream) bool {
	return pr.hdrPassthroughWanted(pr.hwPolicy(), decision, streams)
}

func (pr *PlanResolver) hdrPassthroughWanted(pol HWPolicy, decision transcoding.PlaybackDecision, streams []*library.MediaStream) bool {
	if decision.VideoDecision != "transcode" || slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed") {
		return false
	}
	if SourceTranscodeVideoRange(streams) == "" || !pr.clientDecodes(decision, "hevc") {
		return false
	}
	if !pr.gpuAllowed(pol) {
		return false
	}
	if _, ok := pr.Capability().PassthroughBackend(); !ok {
		return false
	}
	if decision.VideoCodec != "" && !strings.EqualFold(decision.VideoCodec, "h264") && !strings.EqualFold(decision.VideoCodec, "hevc") {
		return false
	}
	return true
}

// HWTonemapWanted reports whether this request should tone-map HDR->SDR on the
// GPU: the decision needs tone-mapping, the toggle is on, the host probed
// HWTonemap, and the backend has a wired HW tone-map filter (VAAPI OpenCL/VPP,
// NVENC tonemap_cuda; QSV's vpp is ineligible). When false the software
// tonemap path runs, byte-identical to a host with no GPU tone-mapping.
func (pr *PlanResolver) HWTonemapWanted(decision transcoding.PlaybackDecision, hdrRange string) bool {
	return pr.hwTonemapWanted(pr.hwPolicy(), decision, hdrRange)
}

func (pr *PlanResolver) hwTonemapWanted(pol HWPolicy, decision transcoding.PlaybackDecision, hdrRange string) bool {
	if !pol.Tonemap || !pr.gpuAllowed(pol) {
		return false
	}
	// A backend that tone-maps this range on the GPU (VAAPI OpenCL / NVENC CUDA
	// for any range, PQ-only for the fixed VPP; QSV vpp is ineligible). Picked PER ROUTE, so a
	// QSV-primary host still routes HDR->SDR to VAAPI's OpenCL.
	if _, ok := pr.Capability().TonemapBackend(hdrRange); !ok {
		return false
	}
	return slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed")
}

// HWDeinterlaceWanted reports whether the resolved GPU session should run the
// backend's SESSION-level deinterlacer: the source is known interlaced (flag or
// idet) and the backend is one whose deinterlacer is session-level and proven
// (QSV). Per-frame backends answer false - their filter is always emitted.
func (pr *PlanResolver) HWDeinterlaceWanted(streams []*library.MediaStream, accel string) bool {
	return pr.hwDeinterlaceWanted(SourceInterlaced(streams), accel)
}

func (pr *PlanResolver) hwDeinterlaceWanted(interlaced bool, accel string) bool {
	if accel != string(transcoding.QSV) || pr.Capability == nil || !interlaced {
		return false
	}
	bc, ok := pr.Capability().BackendCap(accel)
	return ok && bc.DeinterlacesOnGPU()
}

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

func (pr *PlanResolver) hwOverlayWanted(pol HWPolicy, decision transcoding.PlaybackDecision) bool {
	if !pr.gpuAllowed(pol) {
		return false
	}
	if _, ok := pr.Capability().OverlayBackend(); !ok {
		return false
	}
	return decision.SubtitleDecision == "burn"
}
