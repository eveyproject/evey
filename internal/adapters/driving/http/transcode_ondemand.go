package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"

	domainplayback "github.com/eveyproject/evey/internal/domain/playback"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
)

// transcode_ondemand.go - the on-demand HLS/DASH copy rung served straight from
// mkvgo's mp4.PlanHLS (apptranscoding.OnDemandPackager). When a request is
// video+audio copy-eligible AND the source packages verbatim, serveMaster
// hands the whole presentation to the packager instead of the paced
// `ffmpeg -c copy` + fMP4-rebase producer: every playlist, init, segment and
// the DASH manifest is built on request, byte-identical, stateless, with no
// on-disk segments and no session bookkeeping. Anything the packager can't
// carry (a codec, a Cues-less Matroska, or the whole feature flagged off)
// falls through to the unchanged ffmpeg pipeline.
//
// Wire: the master (master.m3u8 / manifest.mpd) references its sub-resources
// through Options.URLPrefix = onDemandPrefix, so their on-the-wire names never
// collide with the transcode pipeline's (stream_N_*, audio_N_*) and serveHTTP
// routes them by that prefix to servePlanResource. The plan is keyed on the
// source alone (not the client), so it is cached once and every sub-request is
// a cache hit - the URL carries no decision state.

const (
	// onDemandPrefix decorates every sub-resource URI the plan's manifests
	// reference. serveHTTP routes `od_*` to the plan; Resource() gets the
	// canonical (unprefixed) name back.
	onDemandPrefix = "od_"
	// onDemandSegmentMs is the target segment duration for the on-demand plan.
	// Must be stable: master and sub-requests build the same plan (cached on
	// srcPath+segMs), so a drift here would desync the playlist from segments.
	onDemandSegmentMs = int64(6000)
	// maxRotationKeys bounds the AES-128 key pool for a single presentation, so a
	// pathological rotate-every-1 setting can't derive keys without limit. Past
	// the cap mkvgo cycles keys (still per-period, just reused at the tail).
	maxRotationKeys = 512
)

// servePackagerMaster EXECUTES a plan whose video the packager carries
// (plan.Delivery.Video == mkvgo): the whole presentation from mp4.PlanHLS
// when the audio is the packager's too, the HYBRID master (packager video +
// Evey's ffmpeg AAC renditions) when the plan put the audio on ffmpeg. It
// decides nothing: the engines are the plan's.
//
// Returns served=true once it has written the response. served=false with a
// fallback reason is a RUNTIME outcome the plan could not know (the source
// does not package: a codec CMAF cannot carry, a Cues-less Matroska): the
// caller serves the ffmpeg path for the same plan and records the fallback,
// counted and logged, never re-decided.
func (h *hlsVODServe) servePackagerMaster(w http.ResponseWriter, r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, rp *requestPlan, span trace.Span) (served bool, fallback string) {
	if h.onDemand == nil {
		return false, fallbackPackagerUnwired // the plan named an engine this handler lacks: an invariant, not a decision
	}
	// The source must package verbatim. Building here also warms the plan
	// cache the sub-resource requests hit.
	plan, err := h.buildPlan(r, mediaID, item, streams, rp)
	if err != nil {
		slog.WarnContext(r.Context(), "hls-vod: on-demand plan build failed, the ffmpeg path serves this plan",
			slog.String("media_id", mediaID), slog.Any("err", err))
		return false, fallbackPackagerUnbuildable
	}

	if rp.Plan.Delivery.Audio == apptranscoding.EngineMkvgo {
		// Full packager presentation (video + audio + subtitles). When the plan
		// trimmed the audio to the SELECTED track, buildPlan packaged that one
		// track, so what we serve holds exactly the tracks the packager carries.
		h.recordServing(r, mediaID, rp, domainplayback.ServingPath{VideoDecision: "copy", VideoEngine: "mkvgo", AudioDecision: "copy", AudioEngine: "mkvgo"}, "")
		h.servePlanResourceData(w, r, mediaID, plan, "master.m3u8", span)
		return true, ""
	}

	// Hybrid: video from the packager, audio re-encoded by Evey's rendition
	// pipeline - the plan chose it knowing the pipeline is wired.
	if h.audio == nil {
		return false, fallbackHybridUnwired
	}
	master, err := h.buildHybridMaster(r, mediaID, item, streams, rp)
	if err != nil {
		slog.WarnContext(r.Context(), "hls-vod: hybrid on-demand master failed, the ffmpeg path serves this plan",
			slog.String("media_id", mediaID), slog.Any("err", err))
		return false, fallbackHybridMasterFailed
	}
	master = stampPlanSourceURIs(master, plan.SourceID())
	setCORSHeaders(w, r, "")
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(master))
	// Counterpart of the ffmpeg path's "master served": one line per playback
	// start on the hybrid rung too, so the serving path is readable straight
	// from the logs (the copy fallback rung logs its own outcome just before).
	slog.Info("hls-vod: hybrid on-demand master served",
		slog.String("media_id", mediaID))
	// The hybrid's audio rides Evey's rendition pipeline, which COPIES a
	// carriable track instead of re-encoding it - so "hybrid" does not mean
	// "audio transcoded", and saying so unconditionally is the same lie the
	// ffmpeg master told (see audioServedAsCopy).
	ap := domainplayback.ServingPath{VideoDecision: "copy", VideoEngine: "mkvgo", AudioDecision: "transcode", AudioEngine: "ffmpeg"}
	if h.audioServedAsCopy(r, item, streams, rp.FP.AudioCopy, true, rp.AudioTrack) {
		ap.AudioDecision = "copy"
	}
	h.recordServing(r, mediaID, rp, ap, "")
	return true, ""
}

// Fallback reasons: why the master was NOT served the way the plan decided.
// Runtime outcomes, observable (evey_hlsvod_delivery_fallbacks_total, a WARN)
// and never a new decision - the plan stays as resolved, the ffmpeg path
// serves it.
const (
	fallbackPackagerUnbuildable = "packager_unbuildable"
	fallbackPackagerUnwired     = "packager_unwired"
	fallbackHybridUnwired       = "hybrid_unwired"
	fallbackHybridMasterFailed  = "hybrid_master_failed"
	fallbackWasmMasterFailed    = "wasm_master_failed"
)

// planSubtitleRenditions is the packager's segmented-WebVTT renditions for
// this file, mapped to the picker's ordinals; nil when the file cannot be
// planned (the extraction route still serves the tracks). rp is the caller's
// resolved plan: a master resolves once and hands it down.
func (h *hlsVODServe) planSubtitleRenditions(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, rp *requestPlan) []apptranscoding.SubtitleRendition {
	if h.onDemand == nil || item == nil || item.File == nil || item.File.Path == "" {
		return nil
	}
	plan, err := h.buildPlan(r, mediaID, item, streams, rp)
	if err != nil {
		return nil
	}
	return planSubtitleRenditionsFor(r.Context(), plan, streams)
}

// buildHybridMaster renders the hybrid master: a single video-COPY variant
// whose playlist is the packager's (od_playlist.m3u8) tied to Evey's ffmpeg
// AAC audio group. Reuses buildManifestInputWith so the audio renditions,
// token, and query suffix match exactly what the audio handlers reconstruct,
// off the plan the master already resolved; serving binds this token to that
// plan's hash (resolvePlan's side of probePlan).
func (h *hlsVODServe) buildHybridMaster(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, rp *requestPlan) (string, error) {
	h.reclaimSupersededHash(r, mediaID, rp)
	in, err := h.buildManifestInputWith(r, mediaID, item, streams, rp)
	if err != nil {
		return "", err
	}
	dims := sourceDimsOf(streams)
	// Estimate the copy rung's peak bitrate for BANDWIDTH. Advisory only
	// (single variant, no ABR switching), so a rough value is fine.
	videoKbps := copyPeakKbpsOf(item)
	in.Profiles = []domaintranscoding.HLSProfile{{
		Name:         "od-copy",
		Width:        dims.Width,
		Height:       dims.Height,
		VideoBitrate: videoKbps,
		AudioBitrate: domaintranscoding.AudioRenditionBaseKbps,
		Copy:         true,
	}}
	in.VideoPlaylistURI = onDemandPrefix + "playlist.m3u8"
	in.SubtitleRenditions = h.planSubtitleRenditions(r, mediaID, item, streams, rp)
	if fb := h.copyFallbackVariant(r, mediaID, item, streams, videoKbps+domaintranscoding.AudioRenditionBaseKbps); fb != nil {
		in.FallbackVariants = []apptranscoding.FallbackVariant{*fb}
	}
	return h.manifestGen.Master(in)
}

// copyFallbackVariant resolves the low transcoded rung the hybrid master
// advertises next to its copy variant, or nil to keep the master single-variant
// (feature off, plan unresolvable, or the rung would not buy a real drop). The
// copy path has no rung below the source bitrate, so a throughput collapse
// freezes the picture for as long as the collapse lasts (measured on a cast
// session, 2026-07-31); one low rung gives the player's own ABR an escape.
//
// The rung is the LOWEST profile of this same request's natural TRANSCODE plan
// (the vd=transcode pin, everything else identical), so its URI resolves
// through the ordinary ffmpeg ladder with its own sealed fingerprint: no new
// decision inputs, no ladder shaping of this path's own, and the stale-fp
// machinery covers it like any transcode stream. Nothing runs until a player
// actually fetches that rung's segments - the admission gates then apply.
//
// Best-effort by design: this only ever ADDS a rung to a master that is
// already serveable, so any resolution failure downgrades to today's
// single-variant master, never to an error.
func (h *hlsVODServe) copyFallbackVariant(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, copyPeakKbps int64) *apptranscoding.FallbackVariant {
	if !h.copyFallbackRung {
		return nil
	}
	// Never for a cast receiver. CAF's ABR does not dive onto an in-master rung
	// (Bouygtel4K 2026-08-03: variant pinned through 9 minutes of freeze - the
	// receiver's abr=floor reload is its rescue path), and the mux/demux
	// asymmetry the rung creates (demuxed copy variant next to a muxed rung)
	// stalls the Chromecast HD's MPL before a single media fetch (2026-08-05:
	// master fetched, then zero playlist/segment requests and no ERROR event,
	// while the same TV played a muxed transcode ladder the same afternoon).
	if st, ok := StreamTokenFromContext(r.Context()); ok && st != nil && st.Cast {
		slog.InfoContext(r.Context(), "hls-vod: copy fallback rung skipped for cast receiver, master stays single-variant",
			slog.String("media_id", mediaID))
		return nil
	}
	rT := r.Clone(r.Context())
	q := rT.URL.Query()
	q.Set("vd", "transcode")
	// The rung keeps the copy rung's codec family (vc= pin, honoured only if
	// the client decodes it): the player's emergency drop must be a plain rung
	// switch, not an H.264->HEVC decoder swap under starvation - the resolver
	// otherwise upgrades an HEVC-capable client's transcode to a GPU HEVC.
	if c := domaintranscoding.CanonicalVideoCodec(apptranscoding.SourceVideoCodec(streams)); c == "h264" || c == "hevc" {
		q.Set("vc", c)
	}
	rT.URL.RawQuery = q.Encode()
	// probePlan, not resolvePlan: this resolution exists to DECIDE whether to
	// advertise the rung and is dropped on the three paths below. Binding it
	// would kill the encoder of whatever hash the player is really on.
	rpT, err := h.probePlan(rT, mediaID, item, streams)
	if err != nil || rpT.IsCopy || len(rpT.Profiles) == 0 {
		// One line per outcome, always: the rung's absence must be readable in
		// the logs, not inferred from a master someone has to fetch and diff.
		slog.WarnContext(r.Context(), "hls-vod: copy fallback rung unresolvable, master stays single-variant",
			slog.String("media_id", mediaID), slog.Any("err", err))
		return nil
	}
	rung := rpT.Profiles[0]
	// Only when the drop is real: a rung at half the copy's peak or less. On a
	// source already lean enough that the lowest transcode saves nothing, an
	// extra variant is pure switching risk for zero relief.
	if (rung.VideoBitrate+rung.AudioBitrate)*2 > copyPeakKbps {
		slog.InfoContext(r.Context(), "hls-vod: copy fallback rung skipped, source already lean",
			slog.String("media_id", mediaID),
			slog.Int64("rung_kbps", rung.VideoBitrate+rung.AudioBitrate),
			slog.Int64("copy_peak_kbps", copyPeakKbps))
		return nil
	}
	inT, err := h.buildManifestInputWith(rT, mediaID, item, streams, rpT)
	if err != nil {
		slog.WarnContext(r.Context(), "hls-vod: copy fallback rung unresolvable, master stays single-variant",
			slog.String("media_id", mediaID), slog.Any("err", err))
		return nil
	}
	// ABR compatibility gate. The fallback rung is an AUTO-SWITCH variant next
	// to the copy rung: the player drops onto it on its own under starvation.
	// That drop must not swap the decoder. The vc= pin above keeps the codec
	// FAMILY aligned only when the source is h264/hevc AND the client decodes
	// it; it does nothing for an AV1 source (no pin), for a box that cannot
	// encode the source family (the resolver falls back to H.264), or for an
	// HDR source whose copy rung is 10-bit PQ/HLG while the low transcode is
	// tonemapped SDR 8-bit. In all those the fallback lands in a DIFFERENT ABR
	// group than the copy rung, and auto-switching to it froze ExoPlayer
	// (a series episode). When the groups differ we OMIT the rung: the master
	// stays single-variant and the client reaches a lower quality only through
	// an explicit source handoff (frontend), never a transparent ABR switch.
	copyGroup := apptranscoding.NewABRGroup(apptranscoding.SourceVideoCodec(streams), sourceCopyVideoRange(streams))
	fbGroup := apptranscoding.NewABRGroup(inT.ChosenVideoCodec, inT.TranscodeVideoRange)
	if !copyGroup.SameAs(fbGroup) {
		slog.InfoContext(r.Context(), "hls-vod: copy fallback rung skipped, incompatible ABR group (would swap the decoder on an auto-drop; use an explicit source handoff)",
			slog.String("media_id", mediaID),
			slog.String("copy_codec", copyGroup.CodecFamily), slog.Bool("copy_hdr", copyGroup.HDR),
			slog.String("fallback_codec", fbGroup.CodecFamily), slog.Bool("fallback_hdr", fbGroup.HDR))
		return nil
	}
	// The fp here is the KEY log field: a later "software: session started
	// hash=<fp>" (or "hls: ffmpeg start") on this same hash is the proof the
	// player actually DROPPED to the rung - advertised vs engaged, one grep.
	slog.InfoContext(r.Context(), "hls-vod: copy master advertises a fallback rung",
		slog.String("media_id", mediaID),
		slog.String("rung", rung.Name),
		slog.Int64("rung_kbps", rung.VideoBitrate+rung.AudioBitrate),
		slog.Int64("copy_peak_kbps", copyPeakKbps),
		slog.String("fp", rpT.FPHash))
	return &apptranscoding.FallbackVariant{
		Profile:    rung,
		URI:        "stream_0.m3u8" + apptranscoding.QuerySuffixFor(inT),
		VideoCodec: inT.ChosenVideoCodec,
		VideoRange: inT.TranscodeVideoRange,
		AudioCodec: inT.ChosenAudioCodec,
		VideoOnly:  len(inT.AudioRenditions) > 0 && !inT.ExternalAudio,
	}
}

// floorRequested reports the survival mode our cast receiver's starvation
// rescue asks for (?abr=floor) - a plan input (PlaybackRequest.ABRFloor).
func floorRequested(r *http.Request) bool {
	return r != nil && r.URL.Query().Get("abr") == "floor"
}

// maxKbpsRequested is the receiver's own throughput measurement (its network
// stack included), 0 when absent or garbage - a plan input (PlaybackRequest.LineKbps).
func maxKbpsRequested(r *http.Request) int64 {
	n, err := strconv.ParseInt(r.URL.Query().Get("maxKbps"), 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// copyPeakKbpsOf estimates the copy rung's peak bitrate: the probed file
// bitrate if known, else derived from size/duration, else a 5000 kbps guess.
func copyPeakKbpsOf(item *domainlibrary.MediaItem) int64 {
	kbps := item.File.Bitrate / 1000
	if kbps <= 0 {
		if sec := item.File.Duration.Seconds(); sec > 0 && item.File.Size > 0 {
			kbps = int64(float64(item.File.Size) * 8 / sec / 1000)
		}
	}
	if kbps <= 0 {
		kbps = 5000
	}
	return kbps
}

const (
	bitrateTestDefaultBytes = 3_000_000
	bitrateTestMaxBytes     = 8_000_000
)

// serveBitrateTest streams opaque bytes so the cast receiver can time the
// throughput of ITS OWN network stack before choosing what to ask for
// (?maxKbps=) - the receiver-side BitrateTest every adaptive service runs.
// Access is already enforced by the dispatcher's commonAuth.
func (h *hlsVODServe) serveBitrateTest(w http.ResponseWriter, r *http.Request) {
	size := int64(bitrateTestDefaultBytes)
	if s := r.URL.Query().Get("size"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
			size = min(n, int64(bitrateTestMaxBytes))
		}
	}
	setCORSHeaders(w, r, "")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	buf := make([]byte, 64*1024)
	for size > 0 {
		n := min(int64(len(buf)), size)
		if _, err := w.Write(buf[:n]); err != nil {
			return
		}
		size -= n
	}
}

// wasmAudioRequested reports whether the client asked for the browser wasm-audio
// path (?wasmAudio=1). Such a client decodes the AC-3/E-AC-3 audio itself
// (ac3go wasm) from a rendition it fetches out of band, so the master it gets
// must be VIDEO-ONLY - MSE cannot decode the ec-3 an in-master group would carry.
func wasmAudioRequested(r *http.Request) bool {
	return r != nil && r.URL.Query().Get("wasmAudio") == "1"
}

// wasmAudioEffective is THE answer to "is the wasm-audio path actually in play
// for this request" - what the client asked for, minus what the server refuses.
//
// Every decision must read THIS, never wasmAudioRequested alone. The master
// handler and the segment fingerprint have to agree: when they disagreed on
// 2026-07-29 the master advertised muxed audio over audio-free segments and the
// Chromecast answered 104 then 905. The codec condition was made shared then;
// the cast switch broke the agreement again the same evening, the same way,
// within an hour of shipping. One function, read everywhere, is the only shape
// that stops this recurring.
func wasmAudioEffective(r *http.Request, castEnabled bool) bool {
	if !wasmAudioRequested(r) {
		return false
	}
	// A cast receiver is identified by its stream token, the only identity it
	// has. Browsers are never gated by the cast switch.
	if st, ok := StreamTokenFromContext(r.Context()); ok && st != nil && st.Cast && !castEnabled {
		return false
	}
	return true
}

// Shared with the playback decision so both name the same sessions.
const wasmAudioMaxChannels = apptranscoding.WasmAudioMaxChannels

// serveWasmAudioMaster serves the VIDEO-ONLY master for a browser wasm-audio
// client (?wasmAudio=1) whose SELECTED audio track is AC-3/E-AC-3. That client
// can't put ec-3 through MSE but runs the ac3go wasm decoder, so it plays the
// video muted via hls.js and decodes the audio itself from a rendition it
// fetches OUT OF BAND (named by the master's #EVEY-AUDIO-URI tag). Backend
// choice, preferring the zero-ffmpeg packager and falling back so every source
// is covered:
//   - audio: mkvgo's plan rendition (od_audio{k}.m3u8) when the plan builds,
//     else the ffmpeg AC-3 copy rendition (audio_{k}.m3u8?wasmAudio=1);
//   - video: mkvgo's copy playlist (od_playlist.m3u8) when copy-eligible, else
//     the ffmpeg video-only ladder (stream_N, made audio-free by the fingerprint).
//
// Returns true once it has written a response; false to fall through to the
// normal master paths (feature not applicable, or the selected track isn't AC-3).
func (h *hlsVODServe) serveWasmAudioMaster(w http.ResponseWriter, r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, rp *requestPlan, span trace.Span) (served bool, fallback string) {
	if h.onDemand == nil {
		return false, fallbackPackagerUnwired
	}
	audioTrack := rp.AudioTrack // the plan's track, resolved once with it
	// The plan placed the audio on the client's decoder and named the video's
	// engine; the codec and channel gates that used to live here are the
	// resolver's (wasmAudioServes).
	videoCopy := rp.Plan.Delivery.Video == apptranscoding.EngineMkvgo

	// Prefer mkvgo's plan for the AC-3 audio rendition (no ffmpeg). Building it
	// also warms the cache the od_audio* sub-requests hit. A plan failure routes
	// the audio to the ffmpeg copy rendition instead - the source is still served.
	plan, planErr := h.buildPlan(r, mediaID, item, streams, rp)
	useMkvgoAudio := planErr == nil
	useMkvgoVideo := videoCopy && planErr == nil
	if videoCopy && planErr != nil {
		fallback = fallbackPackagerUnbuildable // the plan's video engine could not be built: ffmpeg carries it, said so below
	}

	// What the plan PUBLISHES about its own audio renditions, so the master can
	// name the one carrying the resolved track instead of deriving its number -
	// see transcode_ondemand_audio_rendition.go for the language mix-up that
	// derivation caused in prod.
	var rends []planAudioRendition
	if useMkvgoAudio && !h.onDemandTrackTrim {
		rends = planAudioRenditionsOf(r.Context(), plan)
	}

	master, err := h.buildWasmAudioMaster(r, mediaID, item, streams, rp, useMkvgoVideo, useMkvgoAudio, audioTrack, rends)
	if err != nil {
		slog.WarnContext(r.Context(), "hls-vod: wasm-audio master failed, the ffmpeg path serves this plan",
			slog.String("media_id", mediaID), slog.Any("err", err))
		return false, fallbackWasmMasterFailed
	}
	if planErr == nil {
		master = stampPlanSourceURIs(master, plan.SourceID())
	}
	setCORSHeaders(w, r, "")
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	servedBy := "ffmpeg"
	if useMkvgoVideo {
		servedBy = "mkvgo"
	}
	w.Header().Set("X-Evey-Served-By", servedBy)
	_, _ = w.Write([]byte(master))
	slog.Info("hls-vod: wasm-audio master served",
		slog.String("media_id", mediaID),
		slog.Int("audio_track", audioTrack),
		slog.Bool("mkvgo_video", useMkvgoVideo),
		slog.Bool("mkvgo_audio", useMkvgoAudio))
	p := domainplayback.ServingPath{VideoDecision: "copy", VideoEngine: "mkvgo", AudioDecision: "wasm", AudioEngine: "client"}
	if !useMkvgoVideo {
		p.VideoEngine = "ffmpeg"
		if !videoCopy {
			p.VideoDecision = "transcode"
			p.VideoBackend = videoBackendLabel(rp.Plan.HWAccel)
		}
	}
	h.recordServing(r, mediaID, rp, p, fallback)
	return true, ""
}

// buildWasmAudioMaster renders the VIDEO-ONLY master for the wasm-audio path
// (see serveWasmAudioMaster). No audio group rides the master - the AC-3/E-AC-3
// rendition is named by a custom #EVEY-AUDIO-URI tag the client reads and hls.js
// ignores. useMkvgoVideo points the single video variant at the packager's copy
// playlist; otherwise the ffmpeg video-only ladder is kept. useMkvgoAudio picks
// the audio rendition URI (mkvgo's od_audio{k} vs the ffmpeg audio_{k} copy).
func (h *hlsVODServe) buildWasmAudioMaster(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, rp *requestPlan, useMkvgoVideo, useMkvgoAudio bool, audioTrack int, rends []planAudioRendition) (string, error) {
	h.reclaimSupersededHash(r, mediaID, rp) // serving binds this token to the plan's hash
	in, err := h.buildManifestInputWith(r, mediaID, item, streams, rp)
	if err != nil {
		return "", err
	}
	// Video-only master: drop any AAC rendition group and switch Master() to the
	// no-audio-group shape. WasmAudio marks the (ffmpeg) video segments audio-free
	// (the fingerprint reads it) and, when the audio is the ffmpeg copy rendition,
	// bakes wasmAudio=1 into its URI via the shared query suffix.
	in.AudioRenditions = nil
	in.ExternalAudio = true
	in.WasmAudio = true

	if useMkvgoVideo {
		dims := sourceDimsOf(streams)
		videoKbps := item.File.Bitrate / 1000
		if videoKbps <= 0 {
			if sec := item.File.Duration.Seconds(); sec > 0 && item.File.Size > 0 {
				videoKbps = int64(float64(item.File.Size) * 8 / sec / 1000)
			}
		}
		if videoKbps <= 0 {
			videoKbps = 5000
		}
		in.Profiles = []domaintranscoding.HLSProfile{{
			Name:         "od-copy",
			Width:        dims.Width,
			Height:       dims.Height,
			VideoBitrate: videoKbps,
			AudioBitrate: 0,
			Copy:         true,
		}}
		in.VideoPlaylistURI = onDemandPrefix + "playlist.m3u8"
	}
	// else: keep buildManifestInput's ffmpeg profiles (stream_N). ExternalAudio
	// makes the master video-only and the fingerprint makes the segments audio-free.

	if useMkvgoAudio {
		// mkvgo's own rendition under the od_ prefix. The rendition index is
		// 1-based AMONG THE AUDIO TRACKS PRESENT IN THE PLAN: with track-trim on
		// the plan keeps only the resolved track and renumbers it to audio1;
		// without trim every track survives and the k-th is audio{k+1}. Getting
		// this wrong 404s the rendition (wasm falls back to AAC).
		//
		// ?audioTrack is threaded so the sub-resource requests
		// (serveOnDemandResource -> buildPlan -> resolveAudioTrack) rebuild the
		// SAME trimmed plan/track; servePlanResourceData re-bakes this request's
		// RawQuery onto the segment URIs the playlist lists, so it propagates.
		// Harmless with trim off (buildPlan ignores it, cache key unchanged).
		idx := audioTrack + 1
		if h.onDemandTrackTrim {
			idx = 1
		}
		name := fmt.Sprintf("%saudio%d.m3u8", onDemandPrefix, idx)
		// The plan's own master is the authority on which rendition carries which
		// source track: the arithmetic above is right only when the plan kept
		// EVERY audio track, and when it did not it addressed another language
		// rather than 404ing. Falls back to the arithmetic when the two sequences
		// cannot be aligned, so an unrecognised shape is no worse than before.
		if resolved, ok := planRenditionURIFor(rends, streams, audioTrack); ok {
			if resolved != name {
				slog.InfoContext(r.Context(), "hls-vod: plan drops an audio track, naming the rendition the plan published",
					slog.String("media_id", mediaID),
					slog.Int("audio_track", audioTrack),
					slog.String("derived", name),
					slog.String("published", resolved))
			}
			name = resolved
		}
		uri := fmt.Sprintf("%s?audioTrack=%d", name, audioTrack)
		if in.StreamToken != "" {
			uri += "&token=" + url.QueryEscape(in.StreamToken)
		}
		in.ExternalAudioURI = uri
	} else {
		// ffmpeg AC-3 copy rendition: audio_{ord}.m3u8 with the full decision
		// suffix (QuerySuffixFor already carries wasmAudio=1 since WasmAudio is set)
		// so its init/segment requests resolve to the same passthrough bytes.
		in.ExternalAudioURI = fmt.Sprintf("audio_%d.m3u8%s", audioTrack, apptranscoding.QuerySuffixFor(in))
	}
	// Bi-source relay: also name the AAC rendition of the same track so the
	// client gets sound via native decode while the wasm decoder boots. The URI
	// must NOT carry wasmAudio=1 (that flips the handler to the AC-3 copy), and
	// is only advertised when the AAC rendition handlers are enabled at all.
	if h.audioRenditionEnabled {
		aacIn := in
		aacIn.WasmAudio = false
		in.ExternalAudioAACURI = fmt.Sprintf("audio_%d.m3u8%s", audioTrack, apptranscoding.QuerySuffixFor(aacIn))
	}
	in.SubtitleRenditions = h.planSubtitleRenditions(r, mediaID, item, streams, rp)
	return h.manifestGen.Master(in)
}

// serveOnDemandResource handles a routed `od_<name>` sub-resource request (a
// playlist, init, segment, subtitle) or the top-level `manifest.mpd`. The plan
// is rebuilt (cache hit) from the source alone; access was already enforced by
// commonAuth. Unknown/unpackageable → 404.
func (h *hlsVODServe) serveOnDemandResource(w http.ResponseWriter, r *http.Request, mediaID, file string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, span trace.Span) {
	if h.onDemand == nil {
		span.SetStatus(codes.Error, "on-demand packaging not wired")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	name := strings.TrimPrefix(file, onDemandPrefix) // "od_seg1.m4s" → "seg1.m4s"; "manifest.mpd" unchanged
	if isIFramePlaylist(name) {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	// The sub-resource carries the master's own query, so its plan trims the
	// presentation exactly as the master's did: same tracks, same plan cache key.
	plan, err := h.buildPlan(r, mediaID, item, streams, h.planOf(r, mediaID, item, streams))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "on-demand plan build")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	// Cast progress: an od_ segment fetched on a cast token is the receiver
	// advancing through the film (see cast_observe.go for the estimate).
	if item != nil && item.File != nil {
		observeCastFetch(r, h.castProgress, onDemandSegEndMs(name, plan.NumSegments(), item.File.Duration.Milliseconds()))
	}
	h.servePlanResourceData(w, r, mediaID, plan, name, span)
}

// isIFramePlaylist reports a request for the I-frame playlist, which the
// packagers never offer (Options.NoIFrames): 404 before any plan is built, so a
// stray request reads nothing from the media.
func isIFramePlaylist(name string) bool {
	return name == "iframe.m3u8" || strings.HasSuffix(name, "/iframe.m3u8")
}

// packagerVersion is the mkvgo version that produced these bytes, read from the
// build's dependency list. Stamped into every sub-resource URI: the packaged
// bytes are immutable for a given packager, not across packager versions.
var packagerVersion = sync.OnceValue(func() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	for _, dep := range info.Deps {
		if dep.Path == "github.com/gravity-zero/mkvgo" {
			return dep.Version
		}
	}
	return "dev"
})

// servePlanResourceData writes one plan resource (name is canonical). Playlists
// are no-cache (a re-scan can change them); segments and inits are immutable for
// a year, keyed by packager version.
func (h *hlsVODServe) servePlanResourceData(w http.ResponseWriter, r *http.Request, mediaID string, plan apptranscoding.OnDemandPlan, name string, span trace.Span) {
	// Single chokepoint for everything mkvgo serves (master, playlists, inits,
	// segments), so one touch here is the whole "is this media playing" signal
	// for the on-demand path. Touch before the write, not after: a client that
	// aborts mid-segment is still a client that is watching.
	h.playbackActivity.Touch(mediaID)

	var (
		data        []byte
		contentType string
		err         error
		streamed    apptranscoding.OnDemandResource
	)
	// A URL that names another source than the plan's is from a playlist of a
	// file since replaced: 410, the player reloads the master. A plan refused
	// mid-way (the file replaced under it) is rebuilt and the request replayed
	// once, through the same check.
	// A URL without src (a master served before the stamp) is still served;
	// temporary, to be refused in a later release.
	want := r.URL.Query().Get(planSourceParam)
	for replayed := false; ; replayed = true {
		if want != "" && want != plan.SourceID() {
			servePlanSourceGone(w, r, mediaID, name, span)
			return
		}
		data, contentType, streamed, err = fetchPlanResource(r.Context(), plan, name)
		if err == nil || replayed || h.onDemand == nil || !errors.Is(err, apptranscoding.ErrSourceChanged) {
			break
		}
		fresh, rerr := h.onDemand.Replan(r.Context(), plan)
		if rerr != nil {
			err = rerr
			break
		}
		plan = fresh
	}
	// The mkvgo path serves every byte of a copy playback and used to log none
	// of them: when a cast died at load, the server's side of it was a blank.
	// Kept deliberately sparse - see shouldLogPlanResource.
	if shouldLogPlanResource(name, err != nil) {
		cast := false
		if st, ok := StreamTokenFromContext(r.Context()); ok && st != nil {
			cast = st.Cast
		}
		if err != nil {
			slog.WarnContext(r.Context(), "hls-vod: on-demand resource refused",
				slog.String("media_id", mediaID),
				slog.String("resource", name),
				slog.Bool("cast", cast),
				slog.Any("err", err))
		} else {
			size := int64(len(data))
			if streamed != nil {
				size = streamed.Size()
			}
			slog.InfoContext(r.Context(), "hls-vod: on-demand resource served",
				slog.String("media_id", mediaID),
				slog.String("resource", name),
				slog.Bool("cast", cast),
				slog.Bool("streamed", streamed != nil),
				slog.Int64("bytes", size))
		}
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "on-demand resource")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if streamed != nil {
		servePlanResourceStreamed(w, r, name, streamed, span)
		return
	}
	setCORSHeaders(w, r, "")
	// Observable serving path: this response came straight from mkvgo's
	// on-demand packager - no ffmpeg, no encode slot. The built-in load test
	// keys its "mkvgo" scenario on this header, and an operator (or a curl)
	// can verify at a glance which machinery served any playback.
	w.Header().Set("X-Evey-Served-By", "mkvgo")
	w.Header().Set("Content-Type", contentType)
	if strings.HasSuffix(name, ".m3u8") || strings.HasSuffix(name, ".mpd") {
		w.Header().Set("Cache-Control", "no-cache")
		// mkvgo emits BARE sub-resource URIs; this bakes two things into them.
		//
		// The packager version, always: sub-resources are cached `immutable` for a
		// year, which only holds while the same packager produces them (0.22.1
		// rewrote every init). Without it, a viewer keeps the old bytes for a year
		// and the upgrade never reaches them.
		//
		// The query token, when the request carried one: a cookie-less Chromecast
		// has nothing else to authenticate sub-resources with.
		suffix := planManifestSuffix(r, plan.SourceID())
		if strings.HasSuffix(name, ".mpd") {
			data = []byte(rebakeDASHManifestURIs(string(data), suffix))
		} else {
			master := string(data)
			if name == "master.m3u8" {
				master = tagPlanMasterSubtitles(master, h.planSubtitleRenditionsOf(r, mediaID, plan))
			}
			data = []byte(rebakeManifestURIs(master, suffix))
		}
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		if strings.HasSuffix(name, ".vtt") {
			data = sanitizeSubtitleVTT(data)
		}
	}
	_, _ = w.Write(data)
}

// fetchPlanResource gets one resource off the plan. A plan that can stream
// hands a media segment over as a handle read from the source while it is
// written (transcode_ondemand_stream.go); anything it returns whole -
// playlists, inits, subtitles, a segment it had to assemble - comes back as
// bytes.
func fetchPlanResource(ctx context.Context, plan apptranscoding.OnDemandPlan, name string) ([]byte, string, apptranscoding.OnDemandResource, error) {
	opener, ok := plan.(apptranscoding.OnDemandOpener)
	if !ok || isPlanPlaylist(name) {
		data, ct, err := plan.Resource(ctx, name)
		return data, ct, nil, err
	}
	res, err := opener.Open(ctx, name)
	if err != nil {
		return nil, "", nil, err
	}
	if res.Bytes() == nil {
		return nil, "", res, nil
	}
	data, ct := res.Bytes(), res.ContentType()
	closePlanResource(ctx, name, res)
	return data, ct, nil, nil
}

// servePlanSourceGone answers a sub-resource URL stamped with a source the
// plan no longer serves.
func servePlanSourceGone(w http.ResponseWriter, r *http.Request, mediaID, name string, span trace.Span) {
	slog.InfoContext(r.Context(), "hls-vod: on-demand resource of a replaced source",
		slog.String("media_id", mediaID), slog.String("resource", name))
	span.SetStatus(codes.Error, "on-demand source replaced")
	setCORSHeaders(w, r, "")
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, `{"message":"the media file was replaced, reload the playlist"}`, http.StatusGone)
}

// planSubtitleRenditionsOf maps an already-built plan's subtitle renditions
// onto the picker's ordinals; the stream list is re-read, so a lookup failure
// only means an untagged master.
func (h *hlsVODServe) planSubtitleRenditionsOf(r *http.Request, mediaID string, plan apptranscoding.OnDemandPlan) []apptranscoding.SubtitleRendition {
	if h.libSvc == nil {
		return nil
	}
	streams, err := h.libSvc.GetMediaStreams(r.Context(), mediaID)
	if err != nil {
		return nil
	}
	return planSubtitleRenditionsFor(r.Context(), plan, streams)
}

// buildPlan resolves the plan for this request's media file with the fixed
// on-demand options (segment duration + URL prefix, plus AES-128 when the
// feature is on). Cheap on a cache hit. The AES key is deterministic per media,
// so the plan cache stays coherent across the master and its sub-requests.
func (h *hlsVODServe) buildPlan(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, rp *requestPlan) (apptranscoding.OnDemandPlan, error) {
	opts := apptranscoding.OnDemandOptions{
		SegmentMs: onDemandSegmentMs,
		URLPrefix: onDemandPrefix,
	}
	if h.onDemandEncrypt && len(h.hlsAESSecret) > 0 {
		if h.hlsKeyRotateSegments > 0 {
			// Forward secrecy: one derived key per rotation period, advertised as
			// key?i=<n> and served back by serveHLSKey. Sized to the duration so
			// the pool covers the whole video without cycling (bounded).
			n := h.rotationKeyCount(item)
			keys := make([]apptranscoding.OnDemandAESKey, n)
			for i := range keys {
				keys[i] = apptranscoding.OnDemandAESKey{
					Key: h.deriveHLSKeyN(mediaID, i),
					URI: fmt.Sprintf("key?i=%d", i), // relative to /hls/{id}/
				}
			}
			opts.AESRotateSegments = h.hlsKeyRotateSegments
			opts.AESKeys = keys
		} else {
			opts.AESKey = h.deriveHLSKey(mediaID)
			opts.AESKeyURI = "key" // relative to /hls/{id}/ - served by serveHLSKey; not URL-prefixed by mkvgo
		}
	}
	// Trim either because the operator asked for a lighter master, or because
	// trimming is what lets this request's track be carried verbatim instead of
	// re-encoded (audioTrimUnlocksCopy). The second condition is recomputed from
	// the request here rather than passed in, so the master and every one of its
	// sub-resource fetches build the SAME plan.
	// The plan decided whether the presentation is trimmed to the selected
	// audio track (plan.Delivery.AudioTrimmed: the operator's trim flag, or the
	// one track the packager can carry verbatim); the master and every
	// sub-resource build from the same plan, so they package the same tracks.
	// The plan's own track (resolved once with it, the same the ffmpeg
	// pipeline maps), so both backends agree on which track a request gets.
	// Best-effort: no streams in hand serves every audio track rather than
	// failing the plan.
	if rp != nil && rp.Plan.Delivery.AudioTrimmed {
		if len(streams) > 0 {
			idx := rp.AudioTrack
			opts.KeepAudioTrack = &idx
		} else {
			slog.WarnContext(r.Context(), "hls vod: on-demand track trim: no streams in hand, serving every track",
				slog.String("media_id", mediaID))
		}
	}
	return h.onDemand.Plan(r.Context(), item.File.Path, opts)
}

// rotationKeyCount sizes the key pool to cover the whole presentation without
// cycling (true forward secrecy: one key per rotation period), derived from the
// media duration. Bounded by maxRotationKeys; an unknown duration falls back to
// the minimum valid rotation (2 keys).
func (h *hlsVODServe) rotationKeyCount(item *domainlibrary.MediaItem) int {
	if item.File == nil {
		return 2
	}
	durMs := item.File.Duration.Milliseconds()
	if durMs <= 0 || onDemandSegmentMs <= 0 {
		return 2
	}
	nSeg := (durMs + onDemandSegmentMs - 1) / onDemandSegmentMs
	rot := int64(h.hlsKeyRotateSegments)
	periods := (nSeg + rot - 1) / rot
	// +1 margin for keyframe-alignment slack in the real cut.
	n := max(int(periods)+1, 2)
	n = min(n, maxRotationKeys)
	return n
}

// deriveHLSKey derives this media's 16-byte AES-128 key from the server secret,
// HMAC-SHA256(secret, mediaID) truncated to 128 bits. Deterministic (no
// per-media storage) and unlinkable to the secret.
func (h *hlsVODServe) deriveHLSKey(mediaID string) []byte {
	mac := hmac.New(sha256.New, h.hlsAESSecret)
	mac.Write([]byte(mediaID))
	return mac.Sum(nil)[:16]
}

// deriveHLSKeyN derives the i-th rotation key for this media, HMAC(secret,
// mediaID\x00i). The \x00 index suffix makes each period's key independent and
// distinct from the single-key derivation (deriveHLSKey has no suffix).
func (h *hlsVODServe) deriveHLSKeyN(mediaID string, i int) []byte {
	mac := hmac.New(sha256.New, h.hlsAESSecret)
	mac.Write([]byte(mediaID))
	mac.Write([]byte{0})
	mac.Write([]byte(strconv.Itoa(i)))
	return mac.Sum(nil)[:16]
}

// serveHLSKey returns this media's AES-128 key bytes. Access is already
// enforced by commonAuth (token/JWT + library + parental) upstream in serveHTTP,
// so reaching here means the caller may decrypt this media. 404 when encryption
// is off so the endpoint doesn't exist unless in use.
func (h *hlsVODServe) serveHLSKey(w http.ResponseWriter, r *http.Request, mediaID string, span trace.Span) {
	if !h.onDemandEncrypt || len(h.hlsAESSecret) == 0 {
		span.SetStatus(codes.Error, "hls key endpoint disabled")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	// A rotated presentation asks for a specific period's key via ?i=<n>; a
	// single-key one omits it. Bound the index so derivation stays finite.
	key := h.deriveHLSKey(mediaID)
	if is := r.URL.Query().Get("i"); is != "" {
		i, err := strconv.Atoi(is)
		if err != nil || i < 0 || i >= maxRotationKeys {
			span.SetStatus(codes.Error, "bad hls key index")
			http.Error(w, `{"message":"bad key index"}`, http.StatusBadRequest)
			return
		}
		key = h.deriveHLSKeyN(mediaID, i)
	}
	setCORSHeaders(w, r, "")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(key)
}

// planOf is the request's plan for a caller that builds the packager plan
// without holding one (a sub-resource, the subtitle renditions): the same
// query resolves to the same strategy, so the trim agrees with the master's.
// nil when the request cannot be resolved (the caller then packages every
// track, as before).
func (h *hlsVODServe) planOf(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) *requestPlan {
	rp, err := h.probePlan(r, mediaID, item, streams)
	if err != nil {
		return nil
	}
	return rp
}

// planSourceParam is the query parameter that carries the plan's SourceID on
// every sub-resource URI, checked by servePlanResourceData.
const planSourceParam = "src"

// planManifestSuffix is the query the packager's bare sub-resource URIs get:
// the packager version and the plan's source, always, and the request's token
// (with its query) when it came in the URL - see servePlanResourceData.
func planManifestSuffix(r *http.Request, sourceID string) string {
	suffix := "?v=" + url.QueryEscape(packagerVersion())
	if sourceID != "" {
		suffix += "&" + planSourceParam + "=" + url.QueryEscape(sourceID)
	}
	if tok := RawTokenFromContext(r.Context()); tok != "" && TokenFromQuery(r.Context()) {
		suffix += "&token=" + url.QueryEscape(tok)
		if rq := r.URL.RawQuery; rq != "" {
			suffix += "&" + rq
		}
	}
	return suffix
}

// serveHybridMPD serves the DASH manifest of a hybrid plan: the packager's
// video beside Evey's audio renditions (ManifestGenerator.HybridMPD), the
// counterpart of buildHybridMaster over the same fragments. 404 when the
// packager offers no manifest (an encrypted plan) or the two do not compose.
func (h *hlsVODServe) serveHybridMPD(w http.ResponseWriter, r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, rp *requestPlan, plan apptranscoding.OnDemandPlan, span trace.Span) {
	notOffered := func(reason string, err error) {
		span.SetStatus(codes.Error, "hybrid dash unavailable")
		slog.InfoContext(r.Context(), "hls-vod: hybrid dash manifest not offered",
			slog.String("media_id", mediaID), slog.String("reason", reason), slog.Any("err", err))
		http.Error(w, `{"message":"dash is not available for this presentation"}`, http.StatusNotFound)
	}
	if !plan.HasDASH() {
		notOffered("packager offers no manifest", nil)
		return
	}
	h.playbackActivity.Touch(mediaID)
	video, _, err := plan.Resource(r.Context(), "manifest.mpd")
	if err != nil {
		notOffered("packager manifest", err)
		return
	}
	h.reclaimSupersededHash(r, mediaID, rp)
	in, err := h.buildManifestInputWith(r, mediaID, item, streams, rp)
	if err != nil {
		h.writeManifestErr(r.Context(), w, span, err, mediaID)
		return
	}
	// The renditions' fallback bitrate, as the hybrid master declares it.
	in.Profiles = []domaintranscoding.HLSProfile{{Name: "od-copy", AudioBitrate: domaintranscoding.AudioRenditionBaseKbps, Copy: true}}
	mpd, err := h.manifestGen.HybridMPD(in, rebakeDASHManifestURIs(string(video), planManifestSuffix(r, plan.SourceID())))
	if err != nil {
		notOffered("compose", err)
		return
	}
	setCORSHeaders(w, r, "")
	w.Header().Set("Content-Type", "application/dash+xml")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(mpd))
	slog.InfoContext(r.Context(), "hls-vod: hybrid dash manifest served", slog.String("media_id", mediaID))
	ap := domainplayback.ServingPath{VideoDecision: "copy", VideoEngine: "mkvgo", AudioDecision: "transcode", AudioEngine: "ffmpeg"}
	if h.audioServedAsCopy(r, item, streams, rp.FP.AudioCopy, true, rp.AudioTrack) {
		ap.AudioDecision = "copy"
	}
	h.recordServing(r, mediaID, rp, ap, "")
}
