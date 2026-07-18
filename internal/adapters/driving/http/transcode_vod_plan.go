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
	"errors"
	"net/http"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
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
}

// planResolver assembles the app-layer resolver from the handler's live
// fields, per call: hand-built test handlers mutate fields after
// construction, and dynamic toggles (capability, hw flags) must be read
// fresh. Assembly is a plain struct literal - no allocation beyond the
// resolver itself.
func (h *hlsVODServe) planResolver() *apptranscoding.PlanResolver {
	pr := &apptranscoding.PlanResolver{
		Capability:            h.capability,
		HWAccelEnabled:        h.hwAccelEnabled,
		AV1Enabled:            h.av1Enabled,
		AV1MaxHeight:          h.av1MaxHeight,
		HWTonemapEnabled:      h.hwTonemapEnabled,
		AudioNormalize:        h.audioNormalizeGetter,
		CapOverride:           h.capOverride,
		ComplexityRepo:        h.complexityRepo,
		ClassLookup:           h.classLookup,
		ClassLookupTTL:        mediaLookupTTL,
		Selector:              h.selector,
		CopyProducer:          h.copyProducer,
		AdmissionRoute:        h.admissionRoute,
		AdmissionPinnedClass:  h.admissionPinnedClass,
		AdmissionBind:         h.admissionBind,
		AdmissionFrozenLadder: h.admissionFrozenLadder,
		AdmissionFreezeLadder: h.admissionFreezeLadder,
		UseFMP4:               h.useFMP4,
		AudioRenditionEnabled: h.audioRenditionEnabled,
	}
	if h.cache != nil {
		pr.FreeBytes = h.cache.FreeBytes
	}
	// Assign the interface field only from a non-nil pointer: a typed-nil
	// *SegmentOrchestrator boxed into the interface would defeat the
	// resolver's nil check.
	if h.orchestrator != nil {
		pr.Orchestrator = h.orchestrator
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
		Container:           containerOf(item),
		Streams:             streams,
		Client:              parseClientCapabilities(r, r.URL.Query().Get("hdr") == "1"),
		Prefs:               prefs,
		AudioTrack:          audioTrack,
		SubtitleBurnInTrack: parseIntQuery(r, "subtitleTrack", -1),
		AudioOffsetMs:       audioOffsetMsQuery(r),
		PinnedVideoDecision: r.URL.Query().Get("vd"),
		PinnedAudioDecision: r.URL.Query().Get("ad"),
		WasmAudio:           wasmAudioRequested(r),
		SourceBytes:         sourceBytesOf(item),
	}
}

// resolvePlan marshals the request and resolves the whole per-request plan
// through the app-layer resolver.
func (h *hlsVODServe) resolvePlan(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) (*requestPlan, error) {
	if item == nil || item.File == nil {
		return nil, errors.New("resolvePlan: nil item")
	}
	claims := HTTPClaimsFromContext(r.Context())
	prefs := loadProfilePrefs(r.Context(), h.profilePrefs, claims.ProfileID)
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
		IsCopy:     plan.Packager == apptranscoding.PackagerCopyFFmpeg,
	}, nil
}

// itemID is string(item.ID), nil-safe - the MediaID the resolver keys the
// admission pin and complexity lookup on.
func itemID(item *domainlibrary.MediaItem) string {
	if item == nil {
		return ""
	}
	return string(item.ID)
}
