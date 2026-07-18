package httpapi

// transcode_vod_plan.go resolves the per-request playback plan ONCE - the
// single entry point in front of what used to be a scattered makeDecision /
// profilesForDecision / buildFingerprint dance re-run by every consumer.
// This is the first, delegating step of the PlaybackPlanResolver seam
// (internal/app/transcoding/playback_plan.go): the resolution still lives in
// the http adapter and delegates to the existing helpers, but there is now
// exactly one place that runs it, and every consumer (manifest input, shed
// probe, stale-URL seal, producer pick, segment key) reads the same values
// by construction. Within a request the cache-hash invariant stops being a
// discipline and becomes a data flow; absorbing the helpers into an
// app-layer resolver is now a move, not a rewrite.
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

// requestPlan is the resolved plan: the app-layer PlaybackPlan projection
// plus the adapter-side values the serving path still consumes directly.
type requestPlan struct {
	// Plan is the app-layer seam object - the projection a future app-layer
	// PlaybackPlanResolver will return. Populated here (from the same single
	// resolution) so the eventual absorption changes where it is computed,
	// not what. Note: the ladder path only ever bills the two ffmpeg
	// packagers; the zero-process mkvgo paths (od_/cc_ resources, plan
	// master) are dispatched before this resolver runs.
	Plan apptranscoding.PlaybackPlan

	// Decision / Profiles / AudioTrack are the raw resolution inputs shared
	// verbatim with the fingerprint and the manifest input.
	Decision   domaintranscoding.PlaybackDecision
	Profiles   []domaintranscoding.HLSProfile
	AudioTrack int
	// FP is the full decision fingerprint (the cache key object); FPHash its
	// hash, computed once - the value sealed into URLs, probed by admission,
	// and addressing the segment cache.
	FP     apptranscoding.DecisionFingerprint
	FPHash string
	// IsCopy is the ffmpeg copy-rung routing bit (isCopyRequest): every
	// consumer reads it here instead of re-deriving decision + disk guard.
	IsCopy bool
}

// resolvePlan computes the whole per-request plan. Side-effect order matches
// the legacy call sequence exactly: the decision first, then the ladder
// shaping (which creates/refreshes the admission pin and freezes the ladder),
// then the fingerprint (which binds its hash to that pin).
func (h *hlsVODServe) resolvePlan(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) (*requestPlan, error) {
	if item == nil || item.File == nil {
		return nil, errors.New("resolvePlan: nil item")
	}
	claims := HTTPClaimsFromContext(r.Context())
	prefs := loadProfilePrefs(r.Context(), h.profilePrefs, claims.ProfileID)
	audioTrack := h.resolveAudioTrackWith(r, streams, prefs)
	decision := h.makeDecisionWith(r.Context(), streams, item.File.ContainerFormat, item.File.Path, r, prefs, audioTrack)
	profiles := h.profilesForDecision(r.Context(), item, streams, decision)
	fp, err := h.buildFingerprintWith(r, mediaID, item, streams, decision, profiles, audioTrack)
	if err != nil {
		return nil, err
	}
	fpHash := fp.Hash()
	isCopy := h.isCopyRequest(decision, streams, sourceBytesOf(item))

	packager := apptranscoding.PackagerTranscodeFFmpeg
	if isCopy {
		packager = apptranscoding.PackagerCopyFFmpeg
	}
	return &requestPlan{
		Plan: apptranscoding.PlaybackPlan{
			Decision: decision,
			// The class the spawn will be billed as - read from the
			// fingerprint, whose hw fields already follow the pinned route
			// (a spilled stream is tagged software).
			Class:           apptranscoding.LadderClassOf(fp),
			Ladder:          profiles,
			VideoCodec:      fp.VideoCodec,
			HWAccel:         fp.HWAccel,
			Packager:        packager,
			FingerprintHash: fpHash,
			WasmAudio:       wasmAudioRequested(r),
		},
		Decision:   decision,
		Profiles:   profiles,
		AudioTrack: audioTrack,
		FP:         fp,
		FPHash:     fpHash,
		IsCopy:     isCopy,
	}, nil
}
