package httpapi

// transcode_vod_plan.go - the http side of the PlaybackPlanResolver seam
// (internal/app/transcoding/playback_plan.go): marshal the *http.Request into
// a transport-free PlaybackRequest, hand it to the app-layer PlanResolver,
// and serve off the returned immutable PlaybackPlan. Every consumer of a
// request (manifest input, shed probe, stale-URL seal, producer pick, segment
// key) reads the same single resolution - the cache-hash invariant within a
// request is a data flow, not a discipline.
//
// It is also the segment hot path's diet: one request used to re-derive the
// decision up to three times (bound check + fingerprint + producer pick),
// each re-loading the profile prefs from Postgres - measured at 5 prefs
// fetches per segment and 6 per master; the plan does one.

import (
	"context"
	"errors"
	"github.com/eveyproject/evey/internal/pkg/sysinfo"
	"log/slog"
	"net/http"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/eveyproject/evey/internal/pkg/network"
)

// requestPlan is the resolved plan: the app-layer PlaybackPlan plus the
// convenience views the serving path reads directly.
type requestPlan struct {
	// Plan is the app-layer resolution - decision, class, shaped ladder,
	// effective codec/hw route, packager, fingerprint (+hash). Note: the
	// ladder path only ever bills the two ffmpeg packagers; the zero-process
	// mkvgo paths (od_/cc_ resources, plan master) are dispatched before this
	// resolver runs.
	Plan apptranscoding.PlaybackPlan

	// Decision / Profiles / AudioTrack / FP / FPHash are views of the same
	// single resolution (Decision and Ladder aliased off Plan, AudioTrack the
	// adapter-resolved request pin the marshalling fed in).
	Decision   domaintranscoding.PlaybackDecision
	Profiles   []domaintranscoding.HLSProfile
	AudioTrack int
	FP         apptranscoding.DecisionFingerprint
	FPHash     string
	// IsCopy is the ffmpeg copy-rung routing bit: every consumer (bound
	// check, shed, producer pick) reads it here instead of re-deriving.
	IsCopy bool
	// Request is the marshalled input the plan was resolved from (the producer
	// pick keys the admission pin off it).
	Request apptranscoding.PlaybackRequest
}

// planResolver assembles the app-layer resolver from the handler's live
// fields, per call: hand-built test handlers mutate fields after
// construction, and dynamic toggles (capability, hw flags) must be read
// fresh. Assembly is a plain struct literal - no allocation beyond the
// resolver itself.
func (h *hlsVODServe) planResolver() *apptranscoding.PlanResolver {
	pr := &apptranscoding.PlanResolver{
		Capability: h.capability,
		// The per-route cost model the backend-per-route pick reads (cheapest
		// correct+proven backend for each route). Built from the live capability;
		// nil-safe (a pre-v7 capability yields a model with no route evidence,
		// and the pick falls back to the primary backend).
		RouteCosts: func() *apptranscoding.RouteCostModel {
			if h.capability == nil {
				return nil
			}
			return apptranscoding.NewRouteCostModel(h.capability().RouteSamples, sysinfo.CPUCapacity())
		},
		HWAccelEnabled:         h.hwAccelEnabled,
		AV1Enabled:             h.av1Enabled,
		AV1MaxHeight:           h.av1MaxHeight,
		HWTonemapEnabled:       h.hwTonemapEnabled,
		HWLowPowerEnabled:      h.hwLowPowerEnabled,
		Expressibility:         h.expressibility,
		MaxVideoBitrateKbps:    h.maxVideoBitrateKbps,
		AudioNormalize:         h.audioNormalizeGetter,
		AudioEncoderID:         h.audioEncoderIDGetter,
		CapOverride:            h.capOverride,
		ComplexityRepo:         h.complexityRepo,
		ClassLookup:            h.classLookup,
		ClassLookupTTL:         mediaLookupTTL,
		Selector:               h.selector,
		CopyProducer:           h.copyProducer,
		AdmissionRoute:         h.admissionRoute,
		AdmissionPinnedClass:   h.admissionPinnedClass,
		AdmissionBind:          h.admissionBind,
		AdmissionFrozenLadder:  h.admissionFrozenLadder,
		AdmissionFreezeLadder:  h.admissionFreezeLadder,
		AdmissionNoteRoute:     h.admissionNoteRoute,
		UseFMP4:                h.useFMP4,
		AudioRenditionEnabled:  h.audioRenditionEnabled,
		OnDemandPackager:       h.onDemand != nil,
		OnDemandTrackTrim:      h.onDemandTrackTrim,
		AudioRenditionPipeline: h.audio != nil,
		// Same predicate as audioRenditionsFor's transcode branch (minus the
		// telecine gate the resolver applies itself).
		TranscodeAudioRendition: h.encoderReorderProbe != nil && h.frameRateProbe != nil,
	}
	if h.cache != nil {
		pr.FreeBytes = h.cache.FreeBytes
	}
	// Assign the interface field only from a non-nil pointer: a typed-nil
	// *SegmentOrchestrator boxed into the interface would defeat the
	// resolver's nil check.
	if h.orchestrator != nil {
		pr.Orchestrator = h.orchestrator
		pr.HWRouteBlocked = h.orchestrator.HWRouteBlocked
	}
	return pr
}

// marshalPlaybackRequest reads everything decision-relevant off the
// *http.Request + resolved media into the transport-free PlaybackRequest.
// This is the ONLY place query params become plan inputs - anything read
// here must also be baked into the child URLs (the cache-hash invariant
// across requests).
func (h *hlsVODServe) marshalPlaybackRequest(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, prefs apptranscoding.TranscodingPreferences, audioTrack int) apptranscoding.PlaybackRequest {
	return apptranscoding.PlaybackRequest{
		MediaID:             mediaID,
		SourcePath:          sourcePathOf(item),
		SourceFingerprint:   sourceFingerprintOf(item),
		Container:           containerOf(item),
		Streams:             streams,
		Client:              parseClientCapabilities(r, hdrQuerySupported(r)),
		Prefs:               prefs,
		AudioTrack:          audioTrack,
		SubtitleBurnInTrack: parseIntQuery(r, "subtitleTrack", -1),
		AudioOffsetMs:       audioOffsetMsQuery(r),
		PinnedVideoDecision: r.URL.Query().Get("vd"),
		PinnedAudioDecision: r.URL.Query().Get("ad"),
		PinnedVideoCodec:    r.URL.Query().Get("vc"),
		WasmAudio:           wasmAudioEffective(r, h.castWasmAudioEnabled),
		MaxBitrateKbps:      h.requestBitrateCapKbps(r),
		SourceBytes:         sourceBytesOf(item),
		SourceDurationMs:    item.File.Duration.Milliseconds(),
		SourceBitrateKbps:   sourceBitrateKbpsOf(item, streams),
		SourceNeedsRemux:    item.File.NeedsRemux(),
		SourcePeakKbps:      copyPeakKbpsOf(item),
		ScreenHeight:        parseIntQuery(r, "screenHeight", 0),
		LineKbps:            maxKbpsRequested(r),
		ABRFloor:            floorRequested(r),
	}
}

// resolvePlan marshals the request and resolves the whole per-request plan
// through the app-layer resolver, for a request that is about to SERVE that
// plan: it also records the stream's move onto this fingerprint and reclaims
// the one it left.
//
// A caller that resolves a plan it may throw away, or a plan for media this
// request is not serving, must use probePlan instead - see its doc.
func (h *hlsVODServe) resolvePlan(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) (*requestPlan, error) {
	rp, err := h.probePlan(r, mediaID, item, streams)
	if err != nil {
		return nil, err
	}
	h.reclaimSupersededHash(r, mediaID, rp)
	return rp, nil
}

// probePlan is resolvePlan WITHOUT the registry bind and the superseded-hash
// reclaim - a pure resolution, no side effect on any other stream.
//
// Binding is destructive: it moves this token onto the resolved hash and hands
// the one it left to reclaimHash, which kills that hash's ffmpeg. A speculative
// resolution (a fallback rung that may not be advertised, a prewarm fingerprint
// for the NEXT episode) must never do that - it killed the encoder of the
// stream actually playing. The segment fetch is what binds for real.
func (h *hlsVODServe) probePlan(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) (*requestPlan, error) {
	if item == nil || item.File == nil {
		return nil, errors.New("resolvePlan: nil item")
	}
	claims := HTTPClaimsFromContext(r.Context())
	prefs := h.profilePrefsFor(r.Context(), claims.ProfileID)
	prefs.NetworkType = networkTypeQuery(r)
	streams = h.warmKeyframesInline(r, mediaID, item, streams)
	audioTrack := h.resolveAudioTrackWith(r, streams, prefs)
	req := h.marshalPlaybackRequest(r, mediaID, item, streams, prefs, audioTrack)
	plan, err := h.planResolver().Resolve(r.Context(), req)
	if err != nil {
		return nil, err
	}
	return &requestPlan{
		Plan:       plan,
		Decision:   plan.Decision,
		Profiles:   plan.Ladder,
		AudioTrack: audioTrack,
		FP:         plan.Fingerprint,
		FPHash:     plan.FingerprintHash,
		IsCopy:     plan.Route.Copy,
		Request:    req,
	}, nil
}

// warmKeyframesInline builds the keyframe index at the first play of a file
// that has none yet, when that index is all that stands between the source
// and the copy rung. The scan does not build it (only the ingest plan does,
// off by default) and the paced backfill yields to playback, so a freshly
// added, copy-eligible file used to be TRANSCODED for its whole first viewing
// - measured 2026-09-24 on dev: first master at 23:38 as transcode, index
// landed at 23:46, once the session had idled out. The warmer is bounded
// (KeyframeOnDemand.TryIndex: a Cues read answers in milliseconds, a cue-less
// walk detaches past its budget) and stands down while someone is watching,
// so this never stalls a player. On success the streams are re-read so this
// very plan resolves as copy.
func (h *hlsVODServe) warmKeyframesInline(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) []*domainlibrary.MediaStream {
	if h.keyframeWarmer == nil || !apptranscoding.SourceIndexOnlyBlocksCopy(streams) {
		return streams
	}
	if !h.keyframeWarmer(r.Context(), mediaID, sourcePathOf(item)) {
		return streams
	}
	fresh, err := h.libSvc.GetMediaStreams(r.Context(), mediaID)
	if err != nil || len(fresh) == 0 {
		return streams
	}
	return fresh
}

// reclaimSupersededHash hands back the video-encode slot of the hash THIS
// stream just left. A track switch, a subtitle burn-in change or an audio
// offset are all Hash() dimensions, so they re-fingerprint the same viewing
// session and the server sees a brand-new stream: the limiter bills both, and
// the abandoned encoder keeps its slot until the paused/idle sweeps reap it
// minutes later. On a box calibrated at one concurrent encode that made the
// switch fail with "too many streams" - refused by the viewer's own dead
// encoder (observed live 2026-07-25: two fingerprints for one media, a:0 then
// a:1, the second shed while the first held the only slot for 58 s).
//
// Only fires for a hash NO other stream is on (see StreamHashRegistry), and
// only on the encoding path: a copy rung takes no slot, so there is nothing to
// reclaim and nothing to gain from tearing it down.
func (h *hlsVODServe) reclaimSupersededHash(r *http.Request, mediaID string, rp *requestPlan) {
	if h.streamHashes == nil || rp.IsCopy || rp.FPHash == "" {
		return
	}
	superseded, ok := h.streamHashes.Bind(RawTokenFromContext(r.Context()), mediaID, rp.FPHash)
	if !ok || h.reclaimHash == nil {
		return
	}
	if killed := h.reclaimHash(superseded, "stream_superseded"); killed > 0 {
		slog.InfoContext(r.Context(), "hls-vod: reclaimed the encoder this stream just left",
			slog.String("media_id", mediaID),
			slog.String("superseded_hash", superseded),
			slog.String("now_on_hash", rp.FPHash),
			slog.Int("killed", killed))
	}
}

// requestBitrateCapKbps is the tightest of the admin per-user limit and the
// remote ceiling (a client outside the LAN). 0 = none. Both are stable for a
// session, and the resolver keys the admission pin on the result.
func (h *hlsVODServe) requestBitrateCapKbps(r *http.Request) int {
	cap := 0
	tighten := func(v int) {
		if v > 0 && (cap == 0 || v < cap) {
			cap = v
		}
	}
	if claims := HTTPClaimsFromContext(r.Context()); claims != nil && claims.UserID != "" && h.userBitrate != nil {
		tighten(h.userBitrateLimitFor(r.Context(), claims.UserID))
	}
	if h.maxVideoBitrateRemoteKbps != nil && !lanClient(r, h.lanNetworksList()) {
		tighten(h.maxVideoBitrateRemoteKbps())
	}
	// The device's data saver: without it a title that transcodes for its
	// codec alone (no bitrate_exceeded) got a ladder above the asked ceiling.
	tighten(parseIntQuery(r, "maxBitrate", 0))
	return cap
}

func (h *hlsVODServe) userBitrateLimitFor(ctx context.Context, userID string) int {
	if h.userBitrateLookup != nil {
		if v, ok := h.userBitrateLookup.Get(userID); ok {
			return v
		}
	}
	limit, err := h.userBitrate.GetUserBitrateLimit(ctx, userID)
	if err != nil {
		return 0
	}
	if h.userBitrateLookup != nil {
		h.userBitrateLookup.Set(userID, limit, mediaLookupTTL)
	}
	return limit
}

// lanClient reports whether the request comes from a private, loopback or
// link-local address, or from one of the admin's LAN Networks - the same rule
// Stream info applies (grpcapi playback_handler), so both agree on who is capped.
// An unknown address counts as LAN: the cap must never bite on a missing signal.
func lanClient(r *http.Request, lanNetworks []string) bool {
	ip := network.ClientIP(r)
	if ip == nil {
		return true
	}
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return true
	}
	return network.NewDetector(lanNetworks).DetectIP(ip) == network.LAN
}

func (h *hlsVODServe) lanNetworksList() []string {
	if h.lanNetworks == nil {
		return nil
	}
	return h.lanNetworks()
}

// itemID is string(item.ID), nil-safe - the MediaID the resolver keys the
// admission pin and complexity lookup on.
func itemID(item *domainlibrary.MediaItem) string {
	if item == nil {
		return ""
	}
	return string(item.ID)
}
