package transcoding

// playback_plan_resolver.go - the concrete PlaybackPlanResolver: the playback
// "brain" absorbed from the http adapter (decision pins, ladder shaping,
// hardware routing, fingerprint assembly, copy-rung routing, producer pick).
// The adapter now only marshals a *http.Request into a PlaybackRequest and
// serves off the returned PlaybackPlan; everything decision-shaped lives here,
// testable without net/http. Field-by-field the dependencies mirror what the
// handler carried - all nil-safe, so a partially-wired resolver (hand-built
// tests) degrades exactly like the partially-wired handler did.

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
	pkgcache "github.com/evey-project/evey/internal/pkg/cache"
	"github.com/evey-project/evey/internal/pkg/metrics"
)

// PlanResolver resolves a PlaybackRequest into an immutable PlaybackPlan. It
// implements PlaybackPlanResolver. Zero value is usable: every dependency is
// optional and its absence reproduces the safe fallback the http handler had
// (720p software capability, no admission, no copy rung, no complexity class).
type PlanResolver struct {
	// Capability reports the host's calibrated encode ceiling; nil → SafeFallback.
	Capability func() transcoding.EncodeCapability
	// RouteCosts is the per-route cost model (nil-safe) the backend-per-route
	// pick reads to choose the CHEAPEST correct+proven backend for a route.
	// nil → the pick falls back to the primary backend (single-backend behaviour).
	RouteCosts func() *RouteCostModel
	// HWAccelEnabled / AV1Enabled / AV1MaxHeight / HWTonemapEnabled are the
	// operator toggles, read dynamically (admin changes apply without restart).
	HWAccelEnabled   func() bool
	AV1Enabled       func() bool
	AV1MaxHeight     func() int
	HWTonemapEnabled func() bool
	// HWLowPowerEnabled: admin opt-in for the Intel low-power (VDEnc) encoder.
	// Off (nil) keeps the probed low_power_* features out of the encode.
	HWLowPowerEnabled func() bool
	// Expressibility is the ffmpeg builder's own knowledge of the routes it
	// can emit (ADR-0030). Consulted on every resolved GPU route; in this
	// stage a disagreement is logged and counted (shadow), the resolver's own
	// mirrors of the builder's rules still decide. nil = not consulted.
	Expressibility transcoding.RouteExpressibility
	// MaxVideoBitrateKbps caps the served video bitrate (transcoding.max_video_bitrate_kbps, 0 = source).
	MaxVideoBitrateKbps func() int
	// AudioNormalize reports the loudnorm admin setting (forces audio re-encode).
	AudioNormalize func(ctx context.Context) bool
	// CapOverride is the CMCD-feedback adaptive ceiling (never raises, only lowers).
	CapOverride func(sourceCodec string, sourceHeight int) (transcoding.CapOverride, bool)

	// ComplexityRepo + ClassLookup resolve the per-title ladder class;
	// ClassLookupTTL bounds the hot-path cache entry. It must be > 0 to cache at
	// all: TTLCache.Set stamps expiresAt = now + ttl and Get treats a passed
	// expiry as a miss, so 0 stores an already-expired entry and every segment
	// re-queries ComplexityRepo. (The http adapter wires 10s.)
	ComplexityRepo transcoding.ComplexityRepo
	ClassLookup    *pkgcache.TTLCache[transcoding.ComplexityEntry]
	ClassLookupTTL time.Duration

	// Selector routes a transcode to the software or hardware producer;
	// CopyProducer serves the ffmpeg copy rung (nil = rung off); Orchestrator
	// is the no-selector fallback (hand-built test handlers). FreeBytes is the
	// segment cache's disk probe for the copy-rung disk guard.
	Selector     *ProducerSelector
	CopyProducer StreamProducer
	Orchestrator StreamProducer
	FreeBytes    func() (int64, error)

	// Admission closures - all optional, TRUE-nil when the shaper/limiter is
	// not wired (see the adapter's admissionRouteOf and friends).
	// AdmissionRoute carries the source class so the shed probe prices the decode
	// like the spawn does.
	AdmissionRoute        func(key string, req RouteAdmission) RouteGrant
	AdmissionPinnedClass  func(key string) (LadderClass, bool)
	AdmissionBind         func(key, fingerprintHash string)
	AdmissionFrozenLadder func(key string) ([]transcoding.HLSProfile, bool)
	AdmissionFreezeLadder func(key string, ladder []transcoding.HLSProfile)

	// HWRouteBlocked reports whether this title's GPU route failed recently
	// (HWRouteBlocklist): the title is then shaped, fingerprinted and produced
	// exactly like an admission spill - the CPU route, every delivery cap kept
	// - so the client's retry plays instead of replaying the failure. nil
	// blocks nothing.
	HWRouteBlocked func(mediaID string) bool

	// AudioEncoderID returns the concrete ffmpeg AAC implementation this host
	// will encode with ("aac" / "libfdk_aac"). nil → native, i.e. the historical
	// bytes. Threaded into the FINGERPRINT and from there into the process opts,
	// so the cache key and the command line read the SAME value.
	AudioEncoderID func() string

	// UseFMP4 selects the segment container (project default true);
	// AudioRenditionEnabled mirrors the audio-rendition feature flag.
	UseFMP4               bool
	AudioRenditionEnabled bool
	// TranscodeAudioRendition extends the audio rendition to transcoded ladders
	// (the http adapter sets it when it can compensate the encoder's delay).
	TranscodeAudioRendition bool
	// Ladder shapes the rungs (nil = the static class table); Quality turns
	// each rung into an encoder target (nil = the static policy).
	Ladder  transcoding.LadderBuilder
	Quality transcoding.QualityPolicy
}

var _ PlaybackPlanResolver = (*PlanResolver)(nil)

// Resolve computes the whole plan. Side-effect order is the contract: the
// decision first, then the ladder shaping (which creates/refreshes the
// admission pin and freezes the ladder), then the fingerprint (which binds
// its hash to that pin).
func (pr *PlanResolver) Resolve(ctx context.Context, req PlaybackRequest) (PlaybackPlan, error) {
	// The source is read ONCE here; every derivation below reads req.Source.
	req.Source = SnapshotOf(req)
	decision := pr.Decide(ctx, req)
	isCopy := pr.IsCopy(decision, req.Streams, req.SourceBytes)
	// The route is resolved ONCE (ADR-0030): the natural route with the
	// per-title block applied, then admission may spill it to the CPU. The
	// ladder, the fingerprint, the class and the producer project it.
	route := pr.resolveRoute(req, decision, isCopy)
	profiles, grant, spilled := pr.shapeLadderFor(ctx, req, decision, route)
	if spilled {
		route = route.spill(SpillPoolFull, decision)
	}
	fp := pr.fingerprintFor(ctx, req, decision, route, profiles)
	packager := PackagerTranscodeFFmpeg
	if isCopy {
		packager = PackagerCopyFFmpeg
	}
	return PlaybackPlan{
		Decision:        decision,
		Class:           route.Class(),
		Ladder:          profiles,
		VideoCodec:      route.Codec,
		HWAccel:         route.Backend.Name,
		Packager:        packager,
		FingerprintHash: fp.Hash(),
		WasmAudio:       req.WasmAudio,
		Fingerprint:     fp,
		Route:           route,
		Admission:       grant,
	}, nil
}

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
	codec, accel, device, features, usable, softwareDecode := pr.EffectiveVideoCodec(decision, req.Streams)
	hwUsable := usable && accel != ""
	passWanted := pr.HDRPassthroughWanted(decision, req.Streams)
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
		HWTonemap:      hwUsable && pr.HWTonemapWanted(decision, snap.HDRRange),
		HDRPassthrough: hwUsable && passWanted,
	}
	route := ResolvedRoute{
		Codec:       codec,
		Topology:    er.Topology(),
		Copy:        isCopy,
		Deinterlace: hwUsable && pr.hwDeinterlaceWanted(snap.Interlaced, accel),
		Overlay:     hwUsable && pr.HWOverlayWanted(decision),
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
	route := pr.resolveRoute(req, decision, isCopy)
	if route.HW() && pr.AdmissionPinnedClass != nil {
		if cls, ok := pr.AdmissionPinnedClass(pr.PinKey(req, decision)); ok && cls != LadderClassHW {
			route = route.spill(SpillPoolFull, decision)
		}
	}
	return route
}

// Decide is the per-track copy/transcode decision with the request-time pins
// applied: the pure MakePlaybackDecisionWithPrefs core, then the audio-resync
// override, the vd=/ad= mid-play pins, and the HDR tonemap re-add for any HDR
// source that will not be served as a raw copy.
func (pr *PlanResolver) Decide(ctx context.Context, req PlaybackRequest) transcoding.PlaybackDecision {
	prefs := req.Prefs
	// Inject the selected subtitle track so the decision only burns (and so only
	// denies the copy rung) when the user actually plays this bitmap subtitle -
	// not whenever the file merely carries one. Mirrors the subtitleTrack pin
	// the fingerprint and the burn-in filter already consume.
	prefs.SubtitleBurnInTrack = req.SubtitleBurnInTrack
	// Inject the audio track the request will actually serve, so the audio
	// copy-vs-transcode decision is made on THAT track (the default may be DTS
	// while the user plays an AAC track, or vice-versa). Mirrors the audioTrack
	// resolution the fingerprint and the ffmpeg -map already use.
	prefs.AudioTrack = req.AudioTrack
	prefs.SourceBitrateKbps = req.SourceBitrateKbps
	d := MakePlaybackDecisionWithPrefs(ctx, req.Streams, req.Container, req.Client, req.MaxBitrateKbps, prefs, req.SourcePath)
	if d == nil {
		return transcoding.PlaybackDecision{}
	}
	// User audio resync: shifting audio requires re-encoding it from a
	// time-shifted input, so a non-zero offset forces the audio decision to
	// transcode (a copied track cannot be shifted). The offset itself rides
	// the fingerprint (cache fork) and every baked URL.
	if req.AudioOffsetMs != 0 && d.AudioDecision != "transcode" {
		d.AudioDecision = "transcode"
		d.TranscodeReasons = append(d.TranscodeReasons, "audio_resync_requested")
		if d.AudioCodec == "" {
			d.AudioCodec = "aac"
		}
	}
	// Mid-play decision pinning. The first playlist of a playback stamps the
	// then-current video decision into every child URL (vd=..., see
	// buildManifestInput/querySuffix); this honors it on the follow-up polls.
	// Background enrichment can flip copy-eligibility BETWEEN two polls of a
	// live EVENT playlist - measured in prod (2026-07-07): the
	// keyframe backfill indexed the file 22 min into a TRANSCODE playback,
	// the same playlist URL switched to the copy plan, and the source-h264
	// segments under the transcode init rendered as green garbage on the
	// Chromecast's hardware decoder. Pinning "transcode" always holds (a
	// re-encode is always serveable). Pinning "copy" cannot be forced - if
	// the fresh decision no longer says copy the plan is structurally gone
	// (keyframes wiped) and a re-encode is the only serveable output - so a
	// broken copy pin flips WITH a warning instead of 500ing the session.
	switch req.PinnedVideoDecision {
	case "transcode":
		if d.VideoDecision == "copy" {
			d.VideoDecision = "transcode"
			d.TranscodeReasons = append(d.TranscodeReasons, "decision_pinned")
			if d.VideoCodec == "" {
				d.VideoCodec = "h264"
			}
		}
	case "copy":
		if d.VideoDecision != "copy" {
			slog.WarnContext(ctx, "hls-vod: pinned copy decision no longer serveable, session flips to transcode mid-play (player may need a restart)",
				slog.String("media_id", req.MediaID),
				slog.String("reasons", strings.Join(d.TranscodeReasons, ",")))
		}
	}
	// Output-codec pin (vc=): the fallback rung of a hybrid copy master keeps
	// the copy rung's codec family, so the player's emergency drop is a rung
	// switch and not an H.264->HEVC decoder swap in the same buffer under
	// starvation. A codec this client cannot decode is ignored, never forced.
	// The reason is stamped even when the decision already says the pinned
	// codec: it is what stops outputCodecFor's HEVC upgrade downstream.
	if pin := transcoding.CanonicalVideoCodec(req.PinnedVideoCodec); (pin == "h264" || pin == "hevc" || pin == "av1") &&
		d.VideoDecision == "transcode" && pr.clientDecodes(*d, pin) {
		d.VideoCodec = pin
		d.TranscodeReasons = append(d.TranscodeReasons, "video_codec_pinned")
	}
	// Audio-side pin, same contract as vd: "transcode" always holds; a broken
	// "copy" pin (loudnorm toggled on, codec no longer passthrough-eligible)
	// flips WITH a warning - the session's audio codec changes mid-play and
	// the player may need a restart, but a re-encode is always serveable.
	switch req.PinnedAudioDecision {
	case "transcode":
		if d.AudioDecision != "transcode" {
			d.AudioDecision = "transcode"
			d.TranscodeReasons = append(d.TranscodeReasons, "decision_pinned")
			if d.AudioCodec == "" {
				d.AudioCodec = "aac"
			}
		}
	case "copy":
		if d.AudioDecision != "copy" {
			slog.WarnContext(ctx, "hls-vod: pinned audio copy decision no longer serveable, session flips to transcoded audio mid-play",
				slog.String("media_id", req.MediaID),
				slog.String("reasons", strings.Join(d.TranscodeReasons, ",")))
		}
	}
	// A transcoded rung is SDR BY CONTRACT - the master only advertises
	// VIDEO-RANGE on the COPY rung. An HDR-capable client (hdr=1) legitimately
	// suppresses the decision-level tonemap for the raw-serve paths, but when
	// the copy rung is UNAVAILABLE (keyframes/HDR truth not yet backfilled,
	// non-wired codec/profile, copy producer off) this request still
	// re-encodes - and a PQ source x264-encoded without tonemapping renders
	// washed-out (proven on prod bytes: SATAVG 3.1 untonemapped vs the same
	// segment tonemapped). Re-add the tonemap for any HDR source that will
	// NOT be served as a raw copy. The disk guard can still flip copy→ladder
	// at serve time without re-adding it - accepted bounded edge (hashing
	// FreeBytes would be worse).
	snap := snapshot(req)
	servedAsRawCopy := d.VideoDecision == "copy" && pr.CopyProducer != nil && snap.CopyVideoCodec != ""
	if !servedAsRawCopy &&
		!slices.Contains(d.TranscodeReasons, "hdr_tonemapping_needed") &&
		snap.IsHDR {
		// Judge the passthrough on the transcode this will be, not on the copy
		// verdict it started as: a copy-blocked title (keyframes pending, VFR,
		// HDR truth unknown) on an HDR client and a Main10 GPU keeps its HDR.
		asTranscode := *d
		asTranscode.VideoDecision = "transcode"
		if pr.HDRPassthroughWanted(asTranscode, req.Streams) {
			if d.VideoDecision != "transcode" {
				d.TranscodeReasons = append(d.TranscodeReasons, "hdr_copy_unavailable")
			}
		} else {
			d.TranscodeReasons = append(d.TranscodeReasons, "hdr_tonemapping_needed")
		}
		d.VideoDecision = "transcode"
		if d.VideoCodec == "" {
			d.VideoCodec = "h264"
		}
	}
	return *d
}

// snapshot returns the request's source snapshot, taking it when a caller
// reached a sub-derivation without going through Resolve.
func snapshot(req PlaybackRequest) SourceSnapshot {
	if req.Source.Taken() {
		return req.Source
	}
	return SnapshotOf(req)
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
	return pr.resolveHW(outputCodec, hwSource{Codec: sourceCodec, TenBit: sourceTenBit}, hwTonemap)
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
func (pr *PlanResolver) resolveHW(outputCodec string, src hwSource, hwTonemap bool) (accel, device string, features []string, usable, softwareDecode bool) {
	if pr.HWAccelEnabled == nil || !pr.HWAccelEnabled() || pr.Capability == nil {
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
	return bc.Backend, bc.Device, pr.effectiveHWFeatures(bc.Features), true, softwareDecode
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
func (pr *PlanResolver) burnInStaysSoftware(decision transcoding.PlaybackDecision) bool {
	return decision.SubtitleDecision == "burn" && !pr.HWOverlayWanted(decision)
}

// titleBlocked reads the per-title GPU route block (nil-safe).
func (pr *PlanResolver) titleBlocked(mediaID string) bool {
	return pr.HWRouteBlocked != nil && mediaID != "" && pr.HWRouteBlocked(mediaID)
}

// effectiveHWFeatures drops the low-power features unless the admin opted in.
func (pr *PlanResolver) effectiveHWFeatures(features []string) []string {
	if pr.HWLowPowerEnabled != nil && pr.HWLowPowerEnabled() {
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
	dims := SourceDimsOf(streams)
	hwTonemap := pr.HWTonemapWanted(decision, dims.HDRRange)
	// CPU tone-map bridged between GPU decode and encode: the GPU never sees the
	// 10-bit surface, but it must be the one decoding (the bridge downloads).
	bridge := !hwTonemap && slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed")
	resolve := func(c string) {
		accel, device, features, usable, softwareDecode = pr.resolveHW(c, hwSource{Codec: dims.Codec, TenBit: dims.TenBit && !bridge, PixelFormat: dims.PixelFormat, Interlaced: dims.Interlaced, WantHeight: wantedRung(decision, dims.Height), Bridge: bridge}, hwTonemap)
		if pr.hwRouteRefused(decision, dims, bridge, softwareDecode) {
			accel, device, features, usable, softwareDecode = "", "", nil, false, false
		}
	}
	codec = pr.codecWithPassthrough(pr.outputCodecFor(decision, streams), decision, streams)
	resolve(codec)
	if codec == "hevc" && decision.VideoCodec != "hevc" && !usable {
		codec = "h264"
		resolve(codec)
	}
	if strings.EqualFold(codec, "av1") && ((pr.AV1Enabled != nil && !pr.AV1Enabled()) || !usable) {
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
func (pr *PlanResolver) hwRouteRefused(decision transcoding.PlaybackDecision, src SourceDims, bridge, softwareDecode bool) bool {
	if src.Telecine || pr.burnInStaysSoftware(decision) {
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
	hwTonemap := pr.HWTonemapWanted(decision, src.HDRRange)
	// No GPU tone-mapper: the CPU tone-maps between the GPU decode and encode
	// (hwTonemapBridge). That needs the GPU to decode the source; a software
	// decode keeps the whole job on the CPU. Same answer as EffectiveVideoCodec.
	bridge := !hwTonemap && slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed")
	resolve := func(c string) bool {
		_, _, _, usable, softwareDecode := pr.resolveHW(c, hwSource{Codec: src.Codec, TenBit: src.TenBit && !bridge, PixelFormat: src.PixelFormat, Interlaced: src.Interlaced, WantHeight: wantedRung(decision, src.Height), Bridge: bridge}, hwTonemap)
		return usable && !pr.hwRouteRefused(decision, src, bridge, softwareDecode)
	}
	codec := pr.outputCodecFor(decision, nil)
	usable := resolve(codec)
	if strings.EqualFold(codec, "av1") && ((pr.AV1Enabled != nil && !pr.AV1Enabled()) || !usable) {
		usable = resolve("h264")
	}
	if codec == "hevc" && decision.VideoCodec != "hevc" && !usable {
		usable = resolve("h264")
	}
	return usable
}

// outputCodecFor upgrades the decision's H.264 to HEVC when the client decodes it
// and the GPU encodes it; an explicit preference is never overridden. streams may be nil.
func (pr *PlanResolver) outputCodecFor(decision transcoding.PlaybackDecision, streams []*library.MediaStream) string {
	codec := decision.VideoCodec
	if decision.VideoDecision != "transcode" || slices.Contains(decision.TranscodeReasons, "preferred_video_codec") ||
		slices.Contains(decision.TranscodeReasons, "video_codec_pinned") {
		return codec
	}
	if codec != "" && !strings.EqualFold(codec, transcoding.DefaultVideoCodec) {
		return codec
	}
	if !pr.clientDecodes(decision, "hevc") || pr.HWAccelEnabled == nil || !pr.HWAccelEnabled() || pr.Capability == nil {
		return codec
	}
	if pr.Capability().AnyBackendSupports("hevc") {
		return "hevc"
	}
	return codec
}

// outputCodecForPassthrough: the HDR passthrough is HEVC by construction.
func (pr *PlanResolver) codecWithPassthrough(codec string, decision transcoding.PlaybackDecision, streams []*library.MediaStream) string {
	if pr.HDRPassthroughWanted(decision, streams) {
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
	if decision.VideoDecision != "transcode" || slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed") {
		return false
	}
	if SourceTranscodeVideoRange(streams) == "" || !pr.clientDecodes(decision, "hevc") {
		return false
	}
	if pr.HWAccelEnabled == nil || !pr.HWAccelEnabled() || pr.Capability == nil {
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
	if pr.HWTonemapEnabled != nil && !pr.HWTonemapEnabled() {
		return false
	}
	if pr.HWAccelEnabled == nil || !pr.HWAccelEnabled() || pr.Capability == nil {
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

// audioChannelsFor is the channel count of the track the VIDEO segments carry:
// the selected track's, or 0 (unknown, guard on) when the audio is served as
// its own rendition (videoAudioTrack -1).
func audioChannelsFor(snap SourceSnapshot, videoAudioTrack, selected int) int {
	if videoAudioTrack != selected {
		return 0
	}
	return snap.AudioChannels
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
	if pr.HWAccelEnabled == nil || !pr.HWAccelEnabled() || pr.Capability == nil {
		return false
	}
	if _, ok := pr.Capability().OverlayBackend(); !ok {
		return false
	}
	return decision.SubtitleDecision == "burn"
}

// PinKey identifies a stream for the admission shaper. Coarser than the
// decision-fingerprint hash on purpose: burn-in/audio variants of the same
// title share one pinned ceiling (their video ladder is the same shape), while
// distinct titles or codecs never collide. It must be derivable identically at
// manifest- and segment-time from the request alone.
//
// It also carries every REQUEST-scoped ladder input (screen height, bitrate
// ceiling): the frozen ladder is read back by key, so two viewers of one title
// with different caps must not share a pin or the second inherits the first's.
func (pr *PlanResolver) PinKey(req PlaybackRequest, decision transcoding.PlaybackDecision) string {
	ec, _, _, _, _, _ := pr.EffectiveVideoCodec(decision, req.Streams)
	key := fmt.Sprintf("%s|%s|%s|sh=%d|mh=%d|bw=%d|lvl=%d", req.MediaID, ec, decision.VideoDecision,
		req.ScreenHeight, req.Client.MaxHeight, pr.videoBitrateCeilingKbps(req, decision), req.Client.MaxH264Level)
	// A title whose GPU route just failed gets a fresh pin: the ladder frozen
	// for its GPU session must not be served to its CPU session.
	if pr.titleBlocked(req.MediaID) {
		key += "|rb=1"
	}
	return key
}

// ShapeLadder picks the ABR ladder for a given (decision, media, streams)
// triple. Reads the per-title complexity class persisted by the background
// ComplexityWorker; falls back to LadderFilmNormal when the item hasn't been
// classified yet (safe: matches pre-refactor behaviour). Caps the ladder to
// the source resolution so we never advertise a 4K rung for a 1080p source.
func (pr *PlanResolver) ShapeLadder(ctx context.Context, req PlaybackRequest, decision transcoding.PlaybackDecision) []transcoding.HLSProfile {
	isCopy := pr.IsCopy(decision, req.Streams, req.SourceBytes)
	profiles, _, _ := pr.shapeLadderFor(ctx, req, decision, pr.resolveRoute(req, decision, isCopy))
	return profiles
}

// shapeLadderFor shapes the ladder for the resolved route and runs admission
// on it. Returns the ladder, the grant, and whether admission spilled the
// stream to the CPU route (the caller rewrites the route; the ladder returned
// is already the CPU shape).
func (pr *PlanResolver) shapeLadderFor(ctx context.Context, req PlaybackRequest, decision transcoding.PlaybackDecision, route ResolvedRoute) (profiles []transcoding.HLSProfile, grant AdmissionGrant, spilled bool) {
	// Read-verbatim (Étape 2): a live stream's ABR ladder is shaped ONCE and
	// frozen on its admission pin; return it unchanged so the manifest and every
	// segment shape the SAME ladder BY CONSTRUCTION - immune to a mid-stream drift
	// of the mutable inputs below (complexity class, capability, CMCD/av1 caps)
	// that would otherwise desync a player's URLs from the cached bytes. Copy
	// rungs take no admission pin (never frozen); their single source-res rung is
	// stable regardless. streamKey is set only on the freezable (transcode) path,
	// and gates the freeze at the tail.
	isCopy := route.Copy
	var streamKey string
	if !isCopy && pr.AdmissionFrozenLadder != nil {
		streamKey = pr.PinKey(req, decision)
		if frozen, ok := pr.AdmissionFrozenLadder(streamKey); ok {
			// The pin governs: a stream pinned to the CPU while the GPU pool was
			// full stays there for its whole life (the URLs address CPU bytes).
			if route.HW() && pr.AdmissionPinnedClass != nil {
				if cls, ok := pr.AdmissionPinnedClass(streamKey); ok && cls != LadderClassHW {
					spilled = true
				}
			}
			return frozen, AdmissionGrant{PinKey: streamKey, Granted: true}, spilled
		}
	}

	complexity := pr.complexityOf(ctx, req.MediaID)
	in := transcoding.LadderInput{
		Source:     pr.sourceFacts(req),
		Complexity: complexity,
		Route:      route.Encode(),
		Delivery:   pr.deliveryConstraints(req, decision),
	}
	laddered := pr.ladderBuilder().Build(in)

	if pr.AdmissionRoute != nil && len(laddered) > 0 && !isCopy {
		// The class admission prices: the route's - a blocked title IS a
		// software stream now (its route was spilled), so admission prices the
		// CPU pool and never offers the GPU as a fallback.
		natural := route.Class()
		// The spill ladder (GPU pool full, CPU pool free) is the same request
		// shaped for the CPU route - every delivery cap included. It used to be
		// the bare producer shape, so a spilled viewer lost the screen, level and
		// bitrate caps the master had negotiated.
		spillRoute := route.spill(SpillPoolFull, decision)
		softwareShape := func() []transcoding.HLSProfile {
			if pr.Selector == nil || pr.Selector.Software() == nil {
				return nil
			}
			sw := in
			sw.Route = spillRoute.Encode()
			return pr.ladderBuilder().Build(sw)
		}
		var fallback func() []int
		if natural == LadderClassHW && pr.Selector != nil {
			fallback = func() []int { return profileHeights(softwareShape()) }
		}
		snap := snapshot(req)
		key := pr.PinKey(req, decision)
		g := pr.AdmissionRoute(key, RouteAdmission{
			Natural:         natural,
			Heights:         profileHeights(laddered),
			Src:             snap.Class(),
			Route:           route.CostKey(),
			SourceFPS:       snap.FPS,
			FallbackHeights: fallback,
			FallbackRoute:   spillRoute.CostKey(),
		})
		grant = AdmissionGrant{PinKey: key, Granted: g.OK, Ceiling: g.Ceiling}
		if g.OK {
			if g.Class != natural && g.Class == LadderClassSoftware {
				if swLadder := softwareShape(); len(swLadder) > 0 {
					laddered = swLadder
				}
				spilled = natural == LadderClassHW
			}
			if g.Ceiling > 0 {
				laddered = transcoding.CapLadderToHeight(laddered, g.Ceiling)
			}
		}
	}
	// Freeze this shaped ladder on the admission pin (created by AdmissionRoute
	// just above) so every later manifest-/segment-time call reads it verbatim.
	// No-op when no pin was created (nothing fit ⇒ refused, or admission unwired):
	// FreezeLadder finds no pin and does nothing, and the next call re-derives.
	if streamKey != "" && pr.AdmissionFreezeLadder != nil {
		pr.AdmissionFreezeLadder(streamKey, laddered)
	}
	return laddered, grant, spilled
}

// profileHeights lists the heights of a shaped ladder, in order.
// videoBitrateCeilingKbps is the tightest of the request cap, the profile cap,
// a bitrate_exceeded decision and the server setting; 0 = follow the source.
func (pr *PlanResolver) videoBitrateCeilingKbps(req PlaybackRequest, decision transcoding.PlaybackDecision) int64 {
	var ceiling int64
	tighten := func(v int64) {
		if v > 0 && (ceiling == 0 || v < ceiling) {
			ceiling = v
		}
	}
	tighten(int64(req.MaxBitrateKbps))
	tighten(int64(req.Prefs.EffectiveMaxBitrateKbps()))
	if slices.Contains(decision.TranscodeReasons, "bitrate_exceeded") {
		tighten(decision.VideoBitrate)
	}
	if pr.MaxVideoBitrateKbps != nil {
		tighten(int64(pr.MaxVideoBitrateKbps()))
	}
	return ceiling
}

func profileHeights(profiles []transcoding.HLSProfile) []int {
	out := make([]int, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, p.Height)
	}
	return out
}

// LadderClassFor is the class of the NATURAL route (before the per-title
// block and the admission spill) - what admission is offered first. The
// billed class of a plan is plan.Route.Class(); this stays for the adapter's
// wrappers and tests.
func (pr *PlanResolver) LadderClassFor(decision transcoding.PlaybackDecision, streams []*library.MediaStream) LadderClass {
	return pr.naturalRoute(PlaybackRequest{Streams: streams}, decision, false).Class()
}

func (pr *PlanResolver) ladderBuilder() transcoding.LadderBuilder {
	if pr.Ladder != nil {
		return pr.Ladder
	}
	return transcoding.StaticLadderBuilder{}
}

// complexityOf reads the title's persisted complexity (class + metrics, the
// cost model's input), through the short-TTL cache on the segment hot path.
// An unclassified title resolves to the film_normal class and no metrics.
func (pr *PlanResolver) complexityOf(ctx context.Context, mediaID string) transcoding.ComplexityVector {
	entry := transcoding.ComplexityEntry{Class: transcoding.ClassFilmNormal}
	if pr.ComplexityRepo == nil || mediaID == "" {
		return transcoding.ComplexityVector{Class: entry.Class, Metrics: entry.Metrics}
	}
	if pr.ClassLookup != nil {
		if e, ok := pr.ClassLookup.Get(mediaID); ok {
			return transcoding.ComplexityVector{Class: e.Class, Metrics: e.Metrics}
		}
	}
	if e, found, err := pr.ComplexityRepo.Get(ctx, mediaID); err == nil && found {
		if e.Class.IsValid() {
			entry.Class = e.Class
		}
		entry.Metrics = e.Metrics
	}
	if pr.ClassLookup != nil {
		// Cache the RESOLVED entry (default included) so an unclassified
		// title doesn't re-query on every segment either.
		pr.ClassLookup.Set(mediaID, entry, pr.ClassLookupTTL)
	}
	return transcoding.ComplexityVector{Class: entry.Class, Metrics: entry.Metrics}
}

// QualityPolicy is the rate-control policy the encoder plans read.
func (pr *PlanResolver) QualityPolicy() transcoding.QualityPolicy {
	if pr.Quality != nil {
		return pr.Quality
	}
	return transcoding.StaticQualityPolicy{}
}

// sourceFacts projects the request's streams onto the ladder engine's input.
func (pr *PlanResolver) sourceFacts(req PlaybackRequest) transcoding.SourceFacts {
	return snapshot(req).Facts()
}

// deliveryConstraints gathers every cap the request, the operator, the QoE
// feedback and the host put on the ladder.
func (pr *PlanResolver) deliveryConstraints(req PlaybackRequest, decision transcoding.PlaybackDecision) transcoding.DeliveryConstraints {
	dc := transcoding.DeliveryConstraints{
		ScreenHeight:            req.ScreenHeight,
		MaxHeight:               req.Client.MaxHeight,
		MaxH264Level:            req.Client.MaxH264Level,
		VideoBitrateCeilingKbps: pr.videoBitrateCeilingKbps(req, decision),
		Capability:              pr.capabilityOrFallback(),
	}
	if pr.CapOverride != nil {
		if ov, ok := pr.CapOverride(snapshot(req).Codec, snapshot(req).Height); ok {
			dc.QoEMaxHeight = ov.MaxHeight
		}
	}
	if pr.AV1MaxHeight != nil {
		dc.AV1MaxHeight = pr.AV1MaxHeight()
	}
	return dc
}

// Fingerprint assembles the DecisionFingerprint - the cache-key object - from
// the resolved (decision, profiles) pair. Must be byte-identical between the
// manifest-time and segment-time resolutions of the same request shape, or
// the cache lookup misses on every segment.
func (pr *PlanResolver) Fingerprint(ctx context.Context, req PlaybackRequest, decision transcoding.PlaybackDecision, profiles []transcoding.HLSProfile) DecisionFingerprint {
	isCopy := pr.IsCopy(decision, req.Streams, req.SourceBytes)
	return pr.fingerprintFor(ctx, req, decision, pr.routeWithPin(req, decision, isCopy), profiles)
}

// fingerprintFor PROJECTS the resolved route and the shaped ladder onto the
// cache-key object: it decides nothing about codec, backend or pipeline.
func (pr *PlanResolver) fingerprintFor(ctx context.Context, req PlaybackRequest, decision transcoding.PlaybackDecision, route ResolvedRoute, profiles []transcoding.HLSProfile) DecisionFingerprint {
	streams := req.Streams
	audioTrack := req.AudioTrack
	subBurnIn := req.SubtitleBurnInTrack

	// When the audio-rendition feature is on, the audio is served as a SEPARATE
	// HLS rendition (audio_T.m3u8 / audio_T_*.m4s) and the video segments MUST be
	// video-only. Otherwise stream_0 keeps muxing the audio track while the master
	// also advertises the separate audio group with DEFAULT=YES - hls.js then
	// appends audio twice (muxed-in-stream_0 + audio_0), which throws
	// bufferAppendError on any seek. Signalling "no audio" with track -1 reuses the
	// tested video-only ladder (hasAudio) and forks the cache (atrack=-1 in Hash())
	// so muxed and video-only segments for the same media never alias.
	videoAudioTrack := audioTrack
	// Same video-only requirement for the wasm-audio path: the AC-3/E-AC-3 audio
	// is served as a SEPARATE rendition (mkvgo od_audio* or ffmpeg audio_*) and
	// decoded client-side, so the ffmpeg video segments must carry no audio track
	// (atrack=-1 forks the cache off the muxed ladder). Harmless when the video is
	// served by mkvgo instead (this ffmpeg fingerprint is then unused).
	//
	// GATED ON THE TRACK CODEC, like the master and the audio handler: since
	// wasm ON, the cast receiver rides ?wasmAudio=1 on EVERY cast, and for a
	// plain-AAC track the server refuses the wasm master and serves the NORMAL
	// muxed one - stripping the audio here anyway shipped audio-free segments
	// under a master advertising audio (CAF 104/905, prod 2026-07-29).
	// Same predicate as the master handler and the decision: the codec alone is
	// not the condition, the channel gate is part of it.
	wasmAudioApplies := req.WasmAudio && WasmAudioApplies(streams, audioTrack, req.Prefs.MaxAudioChannels)
	snap := snapshot(req)
	renditionAudio := pr.AudioRenditionEnabled && !snap.Telecine &&
		(decision.VideoDecision == "copy" || (decision.VideoDecision == "transcode" && pr.TranscodeAudioRendition))
	if renditionAudio || wasmAudioApplies {
		videoAudioTrack = -1
	}

	fpProfiles := make([]ProfileFingerprint, len(profiles))
	for i, p := range profiles {
		// Single, reflection-guarded projection (TestProfileFingerprintOf_CopiesEveryField).
		fpProfiles[i] = ProfileFingerprintOf(p)
	}
	// Each rung's rate control, from the policy the ladder was shaped under.
	encode := route.Encode()
	quality := make([]transcoding.QualityTarget, len(profiles))
	for i, p := range profiles {
		quality[i] = pr.QualityPolicy().TargetFor(p, encode)
	}
	// The route's projections: the codec the segments carry (capability
	// gate, spill, codec fallback already applied), the frozen backend, the
	// pipeline shape. A spilled stream reads as software here because its
	// route IS software - LadderClassOf(fp) then bills the CPU pool.
	hwUsable := route.HW()
	topology := route.Topology
	normalize := pr.AudioNormalize != nil && pr.AudioNormalize(ctx)
	srcAudio := snap.AudioCodec
	audioCopy := AudioCopyEligible(decision, normalize, len(fpProfiles), srcAudio, req.Client)
	fp := DecisionFingerprint{
		MediaID:    req.MediaID,
		SourcePath: req.SourcePath,
		VideoCodec: route.Codec,
		// The SOURCE codec (excluded from Hash - derivable from MediaID): the
		// copy builder tags the fMP4 sample entry from it (-tag:v hvc1).
		SourceVideoCodec: snap.Codec,
		// The decode workload admission prices (excluded from Hash, as the codec).
		SourceVideoHeight: snap.Height,
		SourceFPS:         snap.FPS,
		// Legacy-codec corrections (B3): idet content verdict + anamorphic SAR,
		// both hashed conditionally so progressive/square titles keep their
		// cache. No-ops on modern progressive sources.
		DeinterlaceAll: snap.DeinterlaceAll,
		Telecine:       snap.Telecine,
		SourceSAR:      snap.SAR,
		// The codec the segments actually carry (source codec on passthrough).
		// The builder only reads it on the transcode branch; here it keeps the
		// hash truthful per served bytes.
		AudioCodec:          ServedAudioCodec(audioCopy, srcAudio, decision),
		AudioTrack:          videoAudioTrack,
		SubtitleBurnInTrack: subBurnIn,
		SubtitleBurnInCodec: snap.SubtitleCodec,
		// HDR->SDR tonemap is needed whenever the decision flagged it (HDR source +
		// non-HDR client, or Dolby Vision). Baked into the fingerprint so it (a)
		// enters the cache hash - tonemapped vs not are different bytes - and (b)
		// reaches the orchestrator/builder, which has no source streams to detect it.
		HDRTonemap:       topology.ToneMap != transcoding.ToneMapNone,
		HDRPassthrough:   topology.Output == transcoding.OutputHDRPassthrough,
		HDRTransfer:      strings.ToLower(snap.HDRRange),
		MaxH264Level:     req.Client.MaxH264Level,
		AudioNormalize:   normalize,
		AudioEncoder:     pr.audioEncoderID(),
		MaxAudioChannels: decision.MaxAudioChannels,
		// Gates the fold guard on a real fold; wasm (-1) reads as unknown, guard on.
		SourceAudioChannels: audioChannelsFor(snap, videoAudioTrack, audioTrack),
		// Audio passthrough eligibility - see AudioCopyEligible (shared with
		// the manifest input so the master CODECS always matches the bytes).
		AudioCopy: audioCopy,
		// User audio resync (ms) - hashed conditionally (non-zero only) and
		// threaded to the builder as a net input shift with the source-delay
		// cancel. Decide already forced audio transcode when non-zero.
		AudioOffsetMs:    req.AudioOffsetMs,
		ForceSidecarSubs: decision.SubtitleDecision == "external",
		UseFMP4:          pr.UseFMP4,
		HWAccel:          route.Backend.Name,
		HWDevice:         route.Backend.Device,
		HWFeatures:       route.Backend.Features,
		// Decoupled route (software decode + hwupload), only when the HW path is
		// actually taken. Excluded from Hash() like HWAccel.
		HWSoftwareDecode: topology.SoftwareDecode(),
		// GPU tone-mapping for this request (only when the HW path is actually
		// taken). Excluded from Hash() like HWAccel - a HW- vs software-tonemapped
		// segment is interchangeable in the addressable cache.
		HWTonemap: topology.ToneMap == transcoding.ToneMapGPU,
		// GPU subtitle compositing, same contract as HWTonemap: only when the HW
		// path is actually taken, and excluded from Hash() - the same subtitle on
		// the same frames, whoever blended it.
		HWOverlay:      route.Overlay && hwUsable,
		HWDeinterlace:  route.Deinterlace && hwUsable,
		Profiles:       fpProfiles,
		QualityTargets: quality,
	}
	// Tell the admission shaper which session this pin shaped: while that
	// session holds a limiter slot, the pinned ceiling must not move (see
	// VideoAdmissionShaper - the cache-hash invariant, live edition).
	if pr.AdmissionBind != nil {
		pr.AdmissionBind(pr.PinKey(req, decision), fp.Hash())
	}
	return fp
}

// IsCopy reports whether this request should be served as a video-COPY
// (remux) rung rather than transcoded. Three conditions, all required: the
// copy path is wired (CopyProducer non-nil), the decision did NOT require a
// video transcode, and the source is copy-eligible (SourceCopyVideoCodec).
// When false, the request routes through the selector exactly as before.
func (pr *PlanResolver) IsCopy(decision transcoding.PlaybackDecision, streams []*library.MediaStream, sourceBytes int64) bool {
	if pr.CopyProducer == nil || !VideoCopyEligible(decision, streams) {
		return false
	}
	// Disk guard: the copy rung's segments duplicate ~the source size on disk over
	// the session (video copied 1:1 + a smaller AAC track), and the unpaced copy
	// produces them well ahead of the viewer. If the cache filesystem can't fit it
	// with headroom, fall back to the segment-bounded transcode ladder rather than
	// risk ENOSPC on a small box. Fail-OPEN when the size is unknown or
	// statfs fails - keeps today's behaviour and the mux still fails gracefully
	// downstream. (master- and segment-time both reach this with the same
	// item.File.Size; a free-space flip between them is a rare, bounded 503-retry,
	// not a cache aliasing - the copy hash never collides with a transcode hash.)
	if sourceBytes > 0 && pr.FreeBytes != nil {
		if free, err := pr.FreeBytes(); err == nil && free < sourceBytes+sourceBytes/10 {
			return false
		}
	}
	return true
}

// ProducerFor picks the producer for an already-resolved decision, so the
// producer chosen for a segment matches the one the ladder was shaped for
// (and thus the cache hash). Falls back to the orchestrator when no selector
// is wired (hand-built test handlers); returns nil only when neither is set.
func (pr *PlanResolver) ProducerFor(req PlaybackRequest, decision transcoding.PlaybackDecision, isCopy bool) StreamProducer {
	return pr.producerForRoute(pr.routeWithPin(req, decision, isCopy))
}

// ProducerForPlan picks the producer a resolved plan spawns on: the copy
// producer for the remux rung, else the producer of the route's side - a
// PROJECTION of plan.Route, which already carries the spill and the block.
func (pr *PlanResolver) ProducerForPlan(plan PlaybackPlan) StreamProducer {
	return pr.producerForRoute(plan.Route)
}

func (pr *PlanResolver) producerForRoute(route ResolvedRoute) StreamProducer {
	if pr.Selector != nil {
		if route.Copy {
			return pr.CopyProducer
		}
		return pr.Selector.forRoute(route.HW())
	}
	return pr.Orchestrator
}

// audioEncoderID is the nil-safe read of the injected probe: an unwired
// resolver keeps the native encoder, which is exactly the pre-feature
// behaviour, bytes and hash alike.
func (pr *PlanResolver) audioEncoderID() string {
	if pr.AudioEncoderID == nil {
		return ""
	}
	return pr.AudioEncoderID()
}
