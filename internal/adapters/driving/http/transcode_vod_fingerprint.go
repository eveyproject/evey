package httpapi

// transcode_vod_fingerprint.go builds the DecisionFingerprint / manifest
// input and turns it into a PlaybackDecision.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// resolveAudioTrackWith returns the audio-index to use for the given request
// and stream list, with the profile prefs already loaded (the plan resolution
// loads them once and threads them here). An explicit ?audioTrack= is honoured
// as-is (the user or player has already chosen); otherwise the best match by
// the profile's language preferences (AudioLanguages → Locale → server
// language → IsDefault → 0).
func (h *hlsVODServe) resolveAudioTrackWith(r *http.Request, streams []*domainlibrary.MediaStream, prefs apptranscoding.TranscodingPreferences) int {
	if t, ok := explicitAudioTrack(r, streams); ok {
		return t
	}
	return h.audioTrackFromPrefs(streams, prefs)
}

// explicitAudioTrack honours a valid ?audioTrack= choice, bounded against the
// real audio-stream count: an out-of-range ?audioTrack=99 would otherwise
// become `-map 0:a:99` and fail the whole ffmpeg session per request.
// Out-of-range reports false so the caller falls through to the
// language-based default (a valid index).
func explicitAudioTrack(r *http.Request, streams []*domainlibrary.MediaStream) (int, bool) {
	if !r.URL.Query().Has("audioTrack") {
		return 0, false
	}
	t := parseIntQuery(r, "audioTrack", 0)
	nAudio := 0
	for _, s := range streams {
		if s.StreamType == domainlibrary.Audio {
			nAudio++
		}
	}
	if t >= 0 && t < nAudio {
		return t, true
	}
	return 0, false
}

func (h *hlsVODServe) audioTrackFromPrefs(streams []*domainlibrary.MediaStream, prefs apptranscoding.TranscodingPreferences) int {
	srvLang := "en"
	if h.serverLang != nil {
		srvLang = h.serverLang()
	}
	langs := buildPrefLangs(prefs.AudioLanguages, prefs.Locale, srvLang)
	return resolveDefaultAudioTrack(streams, langs)
}

// buildManifestInputWith turns an already-resolved plan + the request's
// token into the ManifestInput the generator consumes: the decision, ladder,
// audio track and sealed fingerprint hash all come from the SAME resolution
// the caller's other steps consumed, and the query-parsed fields it threads
// into the manifest's URLs are the ones the segment requests hash back from.
func (h *hlsVODServe) buildManifestInputWith(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, rp *requestPlan) (apptranscoding.ManifestInput, error) {
	if item == nil || item.File == nil {
		return apptranscoding.ManifestInput{}, errors.New("buildManifestInput: nil item")
	}

	if item.File.Duration.Milliseconds() <= 0 {
		return apptranscoding.ManifestInput{}, &durationMissingError{mediaID: mediaID}
	}

	decision := rp.Decision
	profiles := rp.Profiles
	// The plan fitted to the client's line advertises ONE rung of its ladder;
	// the variant keeps its LADDER index (the child request re-derives the
	// full ladder, any other name would serve the wrong profile).
	videoPlaylistURI := ""
	if fit := rp.Plan.Fit; fit.Single && fit.Rung >= 0 && fit.Rung < len(profiles) {
		profiles = profiles[fit.Rung : fit.Rung+1]
		videoPlaylistURI = fmt.Sprintf("stream_%d.m3u8", fit.Rung)
	}

	audioTrack := rp.AudioTrack
	subBurnIn := parseIntQuery(r, "subtitleTrack", -1)

	token := RawTokenFromContext(r.Context())

	// `?resume=N` is intentionally OUTSIDE the decisionHash inputs so two
	// users on the same DecisionFingerprint don't shard the segment cache
	// just because they resume at different positions. ManifestInput.Validate
	// rejects negative/garbage; sec or ms accepted (caller convention: sec).
	resumeSec := 0.0
	if r != nil {
		if raw := r.URL.Query().Get("resume"); raw != "" {
			if n, err := strconv.ParseFloat(raw, 64); err == nil && n > 0 {
				resumeSec = n
			}
		}
	}

	// Audio renditions are populated only when the feature flag is on.
	// Empty slice preserves the muxed audio+video manifest bit-for-bit -
	// every existing client keeps working without change. Shared
	// buildAudioRenditions helper guarantees the master + the
	// audio playlist handlers agree on the same SourceTrackIdx
	// numbering (audio-index, matches `ffmpeg -map 0:a:N`).
	var audioRenditions []apptranscoding.AudioRendition
	if h.audioRenditionsFor(decision, streams) {
		audioRenditions = buildAudioRenditions(streams)
		// Each rendition advertises the codec it will really carry: the source
		// codec when it is copied, AAC when it is re-encoded.
		for i := range audioRenditions {
			audioRenditions[i].Codec = "aac"
			if c := h.audioRenditionCopyCodec(r, item, streams, audioRenditions[i].SourceTrackIdx); c != "" {
				audioRenditions[i].Codec = c
			}
		}
	}

	// Advertise the codec the segments will ACTUALLY carry: the RESOLVED
	// plan's (plan.Route.Codec, what the spawn hands the encoder), never a
	// re-derivation - a spill or a per-title GPU block turns the GPU HEVC
	// into a software H.264, and the master would read hvc1 over avc1 bytes.
	// Empty on a plan with no video encode (the copy rung advertises the
	// source's own codec through the copy manifest).
	chosenVideoCodec := rp.FP.VideoCodec

	return apptranscoding.ManifestInput{
		MediaID:             mediaID,
		DurationSec:         float64(item.File.Duration.Milliseconds()) / 1000.0,
		Profiles:            profiles,
		StreamToken:         token,
		AudioTrack:          audioTrack,
		SubtitleBurnInTrack: subBurnIn,
		VideoCodecs:         splitCSVQuery(r, "videoCodecs"),
		AudioCodecs:         splitCSVQuery(r, "audioCodecs"),
		Containers:          splitCSVQuery(r, "containers"),
		// Re-baked into every child URI for the same reason as the lists above:
		// parseClientCapabilities reads it back on the segment request.
		FMP4AudioPassthrough: splitCSVQuery(r, "fmp4AudioPassthrough"),
		UseFMP4:              h.useFMP4,
		ChosenVideoCodec:     chosenVideoCodec,
		// The codec the segments will ACTUALLY carry: the source codec on
		// audio passthrough (ac-3/ec-3 for Safari-class clients, mp3 for a
		// client that opted into FMP4AudioPassthrough), the decision's encoder
		// otherwise. Read straight off the RESOLVED fingerprint rather than
		// re-derived: this used to recompute audioCopyEligible from the request
		// with its own arguments, so the master could disagree with the bytes
		// the moment one input drifted (the client profile now being one of
		// them). One resolution, one answer.
		ChosenAudioCodec: servedAudioCodec(
			rp.FP.AudioCopy, sourceAudioCodec(streams, audioTrack), decision),
		ResumeOffsetSec: resumeSec,
		AudioRenditions: audioRenditions,
		CopyVideoCodec:  sourceCopyVideoCodec(streams),
		CopyVideoRange:  sourceCopyVideoRange(streams),
		TranscodeVideoRange: func() string {
			if rp.FP.HDRPassthrough {
				return sourceCopyVideoRange(streams)
			}
			return ""
		}(),
		// hdr=1 is a decision input (copy rung vs tonemap) exactly like the
		// codec lists: it MUST be re-baked into the variant/segment URLs or the
		// segment-time makeDecision silently downgrades an HDR copy rung to a
		// tonemapped h264 transcode that contradicts the master's CODECS.
		HDRCapable: r != nil && r.URL.Query().Get("hdr") == "1",
		ScreenHeight: func() int {
			if r == nil {
				return 0
			}
			return parseIntQuery(r, "screenHeight", 0)
		}(),
		MaxHeight: func() int {
			if r == nil {
				return 0
			}
			return parseIntQuery(r, "maxHeight", 0)
		}(),
		MaxH264Level: func() int {
			if r == nil {
				return 0
			}
			return parseIntQuery(r, "maxH264Level", 0)
		}(),
		// The channel ceiling is a decision input on the audio rendition the
		// same way hdr=1 is on the video rung: the EXT-X-MEDIA URIs must carry
		// it or the rendition re-derives the SOURCE layout (see field doc).
		MaxAudioChannels: maxAudioChannelsQuery(r),
		// wasm-audio path: propagate wasmAudio=1 into every variant/segment URI.
		// The segment's fingerprint is sealed VIDEO-ONLY (videoAudioTrack=-1 when
		// wasmAudio), so a segment request that lost the flag would re-derive a
		// MUXED fingerprint, mismatch the sealed fp, and 410 - breaking the ffmpeg
		// video-transcode wasm path (the video never loads). Baking it here keeps
		// the variant playlists' segment URIs self-consistent with the master.
		WasmAudio:     wasmAudioEffective(r, h.castWasmAudioEnabled),
		AudioOffsetMs: audioOffsetMsQuery(r),
		// The playback's FIRST playlist stamps its decision; children inherit
		// the stamp verbatim so the whole session stays on one plan even if
		// background enrichment flips copy-eligibility mid-play.
		PinnedVideoDecision: pinnedVideoDecision(r, decision),
		PinnedAudioDecision: pinnedAudioDecision(r, decision),
		PinnedVideoCodec:    r.URL.Query().Get("vc"),
		// The client's line constraints are plan inputs: baked like the pins.
		LineKbps:         maxKbpsRequested(r),
		ABRFloor:         floorRequested(r),
		VideoPlaylistURI: videoPlaylistURI,
		// Seal this moment's decision hash into every URI the manifest emits.
		// stream_N is a ladder INDEX; if the ladder reshapes mid-play the same
		// URL silently maps to another rendition (the 2026-07-15 rainbow). A
		// later request whose fp no longer matches is refused with 410 instead
		// of serving a stranger's bytes. The plan's hash IS this request's
		// fingerprint hash - sealed and compared values are one resolution.
		FingerprintHash: rp.FPHash,
	}, nil
}

// audioOffsetMsQuery parses the user's audio resync request (ms; positive =
// audio later), clamped to a sane +/-30s so a crafted URL cannot demand an
// absurd shift.
func audioOffsetMsQuery(r *http.Request) int {
	if r == nil {
		return 0
	}
	v := parseIntQuery(r, "audioOffsetMs", 0)
	const capMs = 30_000
	if v > capMs {
		v = capMs
	} else if v < -capMs {
		v = -capMs
	}
	// Quantize to the UI's 100ms step. Each distinct value forks the segment
	// cache into its own ffmpeg session, so snapping to the grid the player
	// actually emits caps the distinct-session count at 601 instead of 60001
	// - a crafted sweep of adjacent millisecond offsets can no longer spawn an
	// unbounded fleet of encoders. Round half away from zero so the sign holds.
	const stepMs = 100
	if v >= 0 {
		return ((v + stepMs/2) / stepMs) * stepMs
	}
	return -(((-v + stepMs/2) / stepMs) * stepMs)
}

// pinnedAudioDecision mirrors pinnedVideoDecision for the audio side.
func pinnedAudioDecision(r *http.Request, decision domaintranscoding.PlaybackDecision) string {
	if r != nil {
		if v := r.URL.Query().Get("ad"); v == "copy" || v == "transcode" {
			return v
		}
	}
	if decision.AudioDecision == "copy" {
		return "copy"
	}
	return "transcode"
}

// pinnedVideoDecision returns the vd value to bake into this playback's URLs:
// the request's own pin when it carries one (pass-through - children must
// inherit the ORIGINAL stamp, not re-derive it), else the decision this very
// request just computed (the first playlist of the playback mints the pin).
func pinnedVideoDecision(r *http.Request, decision domaintranscoding.PlaybackDecision) string {
	if r != nil {
		if v := r.URL.Query().Get("vd"); v == "copy" || v == "transcode" {
			return v
		}
	}
	if decision.VideoDecision == "copy" {
		return "copy"
	}
	return "transcode"
}

// buildFingerprint reproduces the DecisionFingerprint from a segment-time
// request. Must match exactly what was produced at manifest time, or the
// cache lookup misses on every segment. Hence one shared resolution: this
// wrapper resolves the whole plan and hands back its fingerprint - kept for
// the call sites (prewarm, tests) that want the fingerprint alone.
//
// probePlan, not resolvePlan: both callers are the next-episode prewarms, which
// fingerprint ANOTHER media under the current playback's token. Binding there
// killed the encoder of the episode being watched, in its last minute.
func (h *hlsVODServe) buildFingerprint(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) (apptranscoding.DecisionFingerprint, error) {
	if item == nil || item.File == nil {
		return apptranscoding.DecisionFingerprint{}, errors.New("buildFingerprint: nil item")
	}
	rp, err := h.probePlan(r, mediaID, item, streams)
	if err != nil {
		return apptranscoding.DecisionFingerprint{}, err
	}
	return rp.FP, nil
}

// makeDecision is the decision wrapper for the call sites without a resolved
// plan (on-demand master paths, prewarm, tests): it loads the prefs, resolves
// the audio track, marshals the request and delegates to the app-layer
// resolver's Decide - the same core the plan resolution runs.
func (h *hlsVODServe) makeDecision(ctx context.Context, streams []*domainlibrary.MediaStream, container, sourcePath string, r *http.Request) domaintranscoding.PlaybackDecision {
	return h.decide(ctx, streams, container, sourcePath, apptranscoding.SourceVideoBitrateKbps(streams), r)
}

// decisionFor is makeDecision for a resolved item: the container, path and
// bitrate (video stream, else the file) all come from it.
func (h *hlsVODServe) decisionFor(ctx context.Context, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, r *http.Request) domaintranscoding.PlaybackDecision {
	return h.decide(ctx, streams, containerOf(item), sourcePathOf(item), sourceBitrateKbpsOf(item, streams), r)
}

// decisionForTrack is decisionFor judged on the audioTrack-th audio track
// instead of the one the request selected: the master advertises EVERY
// rendition, and each one is copied or re-encoded on its own merits.
func (h *hlsVODServe) decisionForTrack(ctx context.Context, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, r *http.Request, audioTrack int) domaintranscoding.PlaybackDecision {
	return h.decideWith(ctx, streams, containerOf(item), sourcePathOf(item), sourceBitrateKbpsOf(item, streams), r, &audioTrack)
}

func (h *hlsVODServe) decide(ctx context.Context, streams []*domainlibrary.MediaStream, container, sourcePath string, sourceKbps int64, r *http.Request) domaintranscoding.PlaybackDecision {
	return h.decideWith(ctx, streams, container, sourcePath, sourceKbps, r, nil)
}

func (h *hlsVODServe) decideWith(ctx context.Context, streams []*domainlibrary.MediaStream, container, sourcePath string, sourceKbps int64, r *http.Request, audioTrack *int) domaintranscoding.PlaybackDecision {
	claims := HTTPClaimsFromContext(r.Context())
	prefs := h.profilePrefsFor(ctx, claims.ProfileID)
	selected := h.resolveAudioTrackWith(r, streams, prefs)
	// The ad= pin is minted ONCE, from the verdict of the track the request
	// selected, and then rides every child URL. A judgement about ANOTHER
	// track - the master advertises every rendition, each copied or re-encoded
	// on its own merits - is a different question, and the pin has no answer
	// to it. Two things went wrong while it was applied anyway (prod,
	// 2026-09-19, a 4K remux: one TrueHD track beside three AC-3 ones):
	//
	//   - ad=copy made the resolver log "pinned audio copy decision no longer
	//     serveable, session flips to transcoded audio mid-play" on every poll
	//     of the TrueHD rendition. Nothing was flipping; the noise buried the
	//     one case where a pin really does break.
	//   - ad=transcode is worse, because that pin is FORCED: select the TrueHD
	//     track and every AC-3 rendition beside it gets re-encoded to AAC, for
	//     no reason other than the verdict of a track nobody is listening to.
	pinnedAudioDec := r.URL.Query().Get("ad")
	if audioTrack != nil {
		if *audioTrack != selected {
			pinnedAudioDec = ""
		}
		selected = *audioTrack
	}
	req := apptranscoding.PlaybackRequest{
		SourcePath:          sourcePath,
		Container:           container,
		Streams:             streams,
		Client:              parseClientCapabilities(r, hdrQuerySupported(r)),
		Prefs:               prefs,
		AudioTrack:          selected,
		SubtitleBurnInTrack: parseIntQuery(r, "subtitleTrack", -1),
		AudioOffsetMs:       audioOffsetMsQuery(r),
		PinnedVideoDecision: r.URL.Query().Get("vd"),
		PinnedAudioDecision: pinnedAudioDec,
		PinnedVideoCodec:    r.URL.Query().Get("vc"),
		WasmAudio:           wasmAudioEffective(r, h.castWasmAudioEnabled),
		// The same caps the plan applies: the mkvgo copy path decides off this
		// call, and a copy under a bitrate ceiling is exactly what it must refuse.
		MaxBitrateKbps:    h.requestBitrateCapKbps(r),
		SourceBitrateKbps: sourceKbps,
	}
	return h.planResolver().Decide(ctx, req)
}
