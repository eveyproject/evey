package httpapi

// transcode_vod_fingerprint.go builds the DecisionFingerprint / manifest
// input and turns it into a PlaybackDecision.

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// resolveAudioTrack returns the audio-index to use for the given request and
// stream list. When the client supplies an explicit ?audioTrack= query param,
// that value is honoured as-is (the user or player has already chosen). When
// absent, the server resolves the best match using the profile's language
// preferences (AudioLanguages → Locale → server language → IsDefault → 0).
func (h *hlsVODServe) resolveAudioTrack(r *http.Request, streams []*domainlibrary.MediaStream) int {
	if r.URL.Query().Has("audioTrack") {
		t := parseIntQuery(r, "audioTrack", 0)
		// Honour the explicit choice, but bound it against the real audio-stream
		// count: an out-of-range ?audioTrack=99 would otherwise become `-map 0:a:99`
		// and fail the whole ffmpeg session per request. Out-of-range falls through
		// to the language-based default (a valid index).
		nAudio := 0
		for _, s := range streams {
			if s.StreamType == domainlibrary.Audio {
				nAudio++
			}
		}
		if t >= 0 && t < nAudio {
			return t
		}
	}
	claims := HTTPClaimsFromContext(r.Context())
	prefs := loadProfilePrefs(r.Context(), h.profilePrefs, claims.ProfileID)
	srvLang := "en"
	if h.serverLang != nil {
		srvLang = h.serverLang()
	}
	langs := buildPrefLangs(prefs.AudioLanguages, prefs.Locale, srvLang)
	return resolveDefaultAudioTrack(streams, langs)
}

// buildManifestInput resolves the media + decision + token into the
// ManifestInput the generator consumes. Threads the same query string-parsed
// fields into both the manifest's URLs and the DecisionFingerprint, so segment
// requests can be hashed back to the same cache bucket.
func (h *hlsVODServe) buildManifestInput(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) (apptranscoding.ManifestInput, error) {
	if item == nil || item.File == nil {
		return apptranscoding.ManifestInput{}, errors.New("buildManifestInput: nil item")
	}

	if item.File.Duration.Milliseconds() <= 0 {
		return apptranscoding.ManifestInput{}, &durationMissingError{mediaID: mediaID}
	}

	decision := h.makeDecision(r.Context(), streams, item.File.ContainerFormat, item.File.Path, r)
	profiles := h.profilesForDecision(r.Context(), item, streams, decision)

	audioTrack := h.resolveAudioTrack(r, streams)
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
	if h.audioRenditionsFor(decision) {
		audioRenditions = buildAudioRenditions(streams)
	}

	// Advertise the codec the segments will ACTUALLY carry, after the capability
	// gate (AV1 → H.264 when AV1 isn't encodable here). Same helper as
	// buildFingerprint, so the master CODECS can't disagree with the bytes.
	chosenVideoCodec, _, _, _, _ := h.effectiveVideoCodec(decision, streams)

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
		UseFMP4:             h.useFMP4,
		ChosenVideoCodec:    chosenVideoCodec,
		// The codec the segments will ACTUALLY carry: the source codec on
		// audio passthrough (ac-3/ec-3 CODECS for Safari-class clients), the
		// decision's encoder otherwise. Must mirror buildFingerprint or the
		// master would advertise codecs the segments don't carry.
		ChosenAudioCodec: servedAudioCodec(
			audioCopyEligible(decision,
				h.audioNormalizeGetter != nil && h.audioNormalizeGetter(r.Context()),
				len(profiles), sourceAudioCodec(streams, audioTrack)),
			sourceAudioCodec(streams, audioTrack), decision),
		ResumeOffsetSec: resumeSec,
		AudioRenditions: audioRenditions,
		CopyVideoCodec:  sourceCopyVideoCodec(streams),
		CopyVideoRange:  sourceCopyVideoRange(streams),
		// hdr=1 is a decision input (copy rung vs tonemap) exactly like the
		// codec lists: it MUST be re-baked into the variant/segment URLs or the
		// segment-time makeDecision silently downgrades an HDR copy rung to a
		// tonemapped h264 transcode that contradicts the master's CODECS.
		HDRCapable: r != nil && r.URL.Query().Get("hdr") == "1",
		// wasm-audio path: propagate wasmAudio=1 into every variant/segment URI.
		// The segment's fingerprint is sealed VIDEO-ONLY (videoAudioTrack=-1 when
		// wasmAudio), so a segment request that lost the flag would re-derive a
		// MUXED fingerprint, mismatch the sealed fp, and 410 - breaking the ffmpeg
		// video-transcode wasm path (the video never loads). Baking it here keeps
		// the variant playlists' segment URIs self-consistent with the master.
		WasmAudio:     wasmAudioRequested(r),
		AudioOffsetMs: audioOffsetMsQuery(r),
		// The playback's FIRST playlist stamps its decision; children inherit
		// the stamp verbatim so the whole session stays on one plan even if
		// background enrichment flips copy-eligibility mid-play.
		PinnedVideoDecision: pinnedVideoDecision(r, decision),
		PinnedAudioDecision: pinnedAudioDecision(r, decision),
		// Seal this moment's decision hash into every URI the manifest emits.
		// stream_N is a ladder INDEX; if the ladder reshapes mid-play the same
		// URL silently maps to another rendition (the 2026-07-15 rainbow). A
		// later request whose fp no longer matches is refused with 410 instead
		// of serving a stranger's bytes. Best-effort: an unbuildable
		// fingerprint leaves the param off (no seal, pre-feature behaviour).
		FingerprintHash: h.currentFPHash(r, mediaID, item, streams),
	}, nil
}

// currentFPHash is the decision hash of this very request - the value the
// manifests seal into their URIs and the stale-URL check compares against.
func (h *hlsVODServe) currentFPHash(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) string {
	fp, err := h.buildFingerprint(r, mediaID, item, streams)
	if err != nil {
		return ""
	}
	return fp.Hash()
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
// cache lookup misses on every segment. Hence shared helpers.
func (h *hlsVODServe) buildFingerprint(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) (apptranscoding.DecisionFingerprint, error) {
	if item == nil || item.File == nil {
		return apptranscoding.DecisionFingerprint{}, errors.New("buildFingerprint: nil item")
	}
	decision := h.makeDecision(r.Context(), streams, item.File.ContainerFormat, item.File.Path, r)
	profiles := h.profilesForDecision(r.Context(), item, streams, decision)

	audioTrack := h.resolveAudioTrack(r, streams)
	subBurnIn := parseIntQuery(r, "subtitleTrack", -1)

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
	if h.audioRenditionsFor(decision) || wasmAudioRequested(r) {
		videoAudioTrack = -1
	}

	fpProfiles := make([]apptranscoding.ProfileFingerprint, len(profiles))
	for i, p := range profiles {
		// Single, reflection-guarded projection (TestProfileFingerprintOf_CopiesEveryField).
		fpProfiles[i] = apptranscoding.ProfileFingerprintOf(p)
	}

	hwTonemap := h.hwTonemapWanted(decision)
	// Capability gate: AV1 falls back to H.264 when the host can't encode it (no
	// HW AV1, no calibrated AV1 software ceiling). videoCodec is what the segments
	// actually carry - the cache key MUST use it, not the raw decision codec.
	videoCodec, hwAccel, hwDevice, hwFeatures, hwUsable := h.effectiveVideoCodec(decision, streams)
	// Admission spill: a hw stream pinned to the software route (GPU pool was
	// full) must be FINGERPRINTED as software - the fingerprint describes the
	// bytes the software producer will make, and LadderClassOf(fp) is what the
	// spawn bills (CPU pool, not GPU). AV1 rides hardware only, so a spilled
	// AV1 stream carries the H.264 the software encoder will actually produce.
	if hwUsable && h.admissionPinnedClass != nil {
		if cls, ok := h.admissionPinnedClass(h.admissionPinKey(item, streams, decision)); ok && cls != apptranscoding.LadderClassHW {
			hwAccel, hwDevice, hwFeatures, hwUsable = "", "", nil, false
			if strings.EqualFold(videoCodec, "av1") {
				videoCodec = "h264"
			}
		}
	}
	srcAudio := sourceAudioCodec(streams, audioTrack)
	audioCopy := audioCopyEligible(decision,
		h.audioNormalizeGetter != nil && h.audioNormalizeGetter(r.Context()),
		len(fpProfiles), srcAudio)
	fp := apptranscoding.DecisionFingerprint{
		MediaID:    mediaID,
		SourcePath: item.File.Path,
		VideoCodec: videoCodec,
		// The SOURCE codec (excluded from Hash - derivable from MediaID): the
		// copy builder tags the fMP4 sample entry from it (-tag:v hvc1).
		SourceVideoCodec: sourceVideoCodec(streams),
		// Legacy-codec corrections (B3): idet content verdict + anamorphic SAR,
		// both hashed conditionally so progressive/square titles keep their
		// cache. No-ops on modern progressive sources.
		DeinterlaceAll: sourceDeinterlaceAll(streams),
		SourceSAR:      sourceSAR(streams),
		// The codec the segments actually carry (source codec on passthrough).
		// The builder only reads it on the transcode branch; here it keeps the
		// hash truthful per served bytes.
		AudioCodec:          servedAudioCodec(audioCopy, srcAudio, decision),
		AudioTrack:          videoAudioTrack,
		SubtitleBurnInTrack: subBurnIn,
		SubtitleBurnInCodec: sourceSubtitleCodec(streams, subBurnIn),
		// HDR->SDR tonemap is needed whenever the decision flagged it (HDR source +
		// non-HDR client, or Dolby Vision). Baked into the fingerprint so it (a)
		// enters the cache hash - tonemapped vs not are different bytes - and (b)
		// reaches the orchestrator/builder, which has no source streams to detect it.
		HDRTonemap:       slices.Contains(decision.TranscodeReasons, "hdr_tonemapping_needed"),
		AudioNormalize:   h.audioNormalizeGetter != nil && h.audioNormalizeGetter(r.Context()),
		MaxAudioChannels: decision.MaxAudioChannels,
		// Audio passthrough eligibility - see audioCopyEligible (shared with
		// buildManifestInput so the master CODECS always matches the bytes).
		AudioCopy: audioCopy,
		// User audio resync (ms) - hashed conditionally (non-zero only) and
		// threaded to the builder as a net input shift with the source-delay
		// cancel. makeDecision already forced audio transcode when non-zero.
		AudioOffsetMs:    audioOffsetMsQuery(r),
		ForceSidecarSubs: decision.SubtitleDecision == "external",
		UseFMP4:          h.useFMP4,
		HWAccel:          hwAccel,
		HWDevice:         hwDevice,
		HWFeatures:       hwFeatures,
		// GPU tone-mapping for this request (only when the HW path is actually
		// taken). Excluded from Hash() like HWAccel - a HW- vs software-tonemapped
		// segment is interchangeable in the addressable cache.
		HWTonemap: hwTonemap && hwUsable,
		Profiles:  fpProfiles,
	}
	// Tell the admission shaper which session this pin shaped: while that
	// session holds a limiter slot, the pinned ceiling must not move (see
	// VideoAdmissionShaper - the cache-hash invariant, live edition).
	if h.admissionBind != nil {
		h.admissionBind(h.admissionPinKey(item, streams, decision), fp.Hash())
	}
	return fp, nil
}

func (h *hlsVODServe) makeDecision(ctx context.Context, streams []*domainlibrary.MediaStream, container, sourcePath string, r *http.Request) domaintranscoding.PlaybackDecision {
	claims := HTTPClaimsFromContext(r.Context())
	prefs := loadProfilePrefs(ctx, h.profilePrefs, claims.ProfileID)
	// Inject the selected subtitle track so the decision only burns (and so only
	// denies the copy rung) when the user actually plays this bitmap subtitle -
	// not whenever the file merely carries one. Mirrors the subtitleTrack param
	// the fingerprint (buildFingerprint) and the burn-in filter already consume.
	prefs.SubtitleBurnInTrack = parseIntQuery(r, "subtitleTrack", -1)
	// Inject the audio track the request will actually serve, so the audio
	// copy-vs-transcode decision is made on THAT track (the default may be DTS
	// while the user plays an AAC track, or vice-versa). Mirrors the audioTrack
	// resolution the fingerprint and the ffmpeg -map already use.
	prefs.AudioTrack = h.resolveAudioTrack(r, streams)
	hdrCapable := r.URL.Query().Get("hdr") == "1"
	client := parseClientCapabilities(r, hdrCapable)
	d := apptranscoding.MakePlaybackDecisionWithPrefs(ctx, streams, container, client, 0, prefs, sourcePath)
	if d == nil {
		return domaintranscoding.PlaybackDecision{}
	}
	// User audio resync: shifting audio requires re-encoding it from a
	// time-shifted input, so a non-zero offset forces the audio decision to
	// transcode (a copied track cannot be shifted). The offset itself rides
	// the fingerprint (cache fork) and every baked URL.
	if audioOffsetMsQuery(r) != 0 && d.AudioDecision != "transcode" {
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
	// live EVENT playlist - measured in prod ("a beta tester", 2026-07-07): the
	// keyframe backfill indexed the file 22 min into a TRANSCODE playback,
	// the same playlist URL switched to the copy plan, and the source-h264
	// segments under the transcode init rendered as green garbage on the
	// Chromecast's hardware decoder. Pinning "transcode" always holds (a
	// re-encode is always serveable). Pinning "copy" cannot be forced - if
	// the fresh decision no longer says copy the plan is structurally gone
	// (keyframes wiped) and a re-encode is the only serveable output - so a
	// broken copy pin flips WITH a warning instead of 500ing the session.
	switch r.URL.Query().Get("vd") {
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
			slog.Warn("hls-vod: pinned copy decision no longer serveable, session flips to transcode mid-play (player may need a restart)",
				slog.String("path", r.URL.Path),
				slog.String("reasons", strings.Join(d.TranscodeReasons, ",")))
		}
	}
	// Audio-side pin, same contract as vd: "transcode" always holds; a broken
	// "copy" pin (loudnorm toggled on, codec no longer passthrough-eligible)
	// flips WITH a warning - the session's audio codec changes mid-play and
	// the player may need a restart, but a re-encode is always serveable.
	switch r.URL.Query().Get("ad") {
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
			slog.Warn("hls-vod: pinned audio copy decision no longer serveable, session flips to transcoded audio mid-play",
				slog.String("path", r.URL.Path),
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
	servedAsRawCopy := d.VideoDecision == "copy" && h.copyProducer != nil && sourceCopyVideoCodec(streams) != ""
	if !servedAsRawCopy &&
		!slices.Contains(d.TranscodeReasons, "hdr_tonemapping_needed") &&
		sourceIsHDR(streams) {
		d.TranscodeReasons = append(d.TranscodeReasons, "hdr_tonemapping_needed")
		d.VideoDecision = "transcode"
		if d.VideoCodec == "" {
			d.VideoCodec = "h264"
		}
	}
	return *d
}
