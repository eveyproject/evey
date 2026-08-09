package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
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

// tryServePlanMaster serves the presentation from the on-demand packager when
// the request's VIDEO is copy-eligible and the source packages verbatim. Two
// modes, both stateless video from mp4.PlanHLS:
//
//   - audio also copy (every track CMAF-carriable, no loudnorm): the whole
//     presentation is the packager's master - video + demuxed audio + subs.
//   - audio must be re-encoded (DTS/TrueHD, loudnorm, a non-carriable track):
//     a HYBRID master - the video variant points at the packager (od_playlist),
//     the audio group stays Evey's ffmpeg AAC renditions. Requires the
//     audio-rendition pipeline wired + enabled; otherwise fall through.
//
// Returns true when it has written the response (served or a hard error) so
// serveMaster stops; false to fall through to the ffmpeg master (feature off,
// video not copy-eligible, plan unbuildable, or hybrid audio unavailable - a
// clean, logged fallback).
func (h *hlsVODServe) tryServePlanMaster(w http.ResponseWriter, r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, span trace.Span) bool {
	if h.onDemand == nil || item == nil || item.File == nil || item.File.Path == "" {
		return false
	}
	// One decision per master request: both the video gate and the audio gate
	// read it, and makeDecision re-derives client caps + content class each call.
	decision := h.makeDecision(r.Context(), streams, containerOf(item), sourcePathOf(item), r)
	if !h.planHLSVideoEligible(decision, item, streams) {
		return false
	}
	// The source must package verbatim (a codec CMAF can't carry, or a
	// Matroska without a Cues seek index → ffmpeg). Building here also warms
	// the plan cache the sub-resource requests hit.
	plan, err := h.buildPlan(r, mediaID, item, streams)
	if err != nil {
		slog.WarnContext(r.Context(), "hls-vod: on-demand plan build failed, falling back to ffmpeg",
			slog.String("media_id", mediaID), slog.Any("err", err))
		return false
	}

	// Receiver-fitted masters, in order of information quality: a measured
	// throughput (?maxKbps=, copy-first - only bites when the measure cannot
	// carry the copy) then the survival floor (?abr=floor, the starvation
	// rescue). Both exist because the CAF player never dives onto an
	// advertised rung by itself (Bouygtel4K, 2026-08-03: variant pinned
	// through nine minutes of freeze) - the descent is ours, deterministic.
	// Unresolvable falls through to the ordinary shapes.
	if kbps := maxKbpsRequested(r); kbps > 0 {
		if master, ok := h.buildCappedMaster(r, mediaID, item, streams, kbps); ok {
			writeHLSMaster(w, r, master)
			return true
		}
	}
	if floorRequested(r) {
		if master, ok := h.buildFloorMaster(r, mediaID, item, streams); ok {
			writeHLSMaster(w, r, master)
			return true
		}
	}

	if h.audioAllCopyEligible(r.Context(), decision, streams) {
		// Full packager presentation (video + audio + subtitles).
		h.servePlanResourceData(w, r, mediaID, plan, "master.m3u8", span)
		return true
	}

	// Hybrid: video from the packager, audio re-encoded by Evey's rendition
	// pipeline. Only possible when that pipeline is wired and enabled.
	if !h.audioRenditionsFor(decision) || h.audio == nil {
		return false
	}
	master, err := h.buildHybridMaster(r, mediaID, item, streams)
	if err != nil {
		slog.WarnContext(r.Context(), "hls-vod: hybrid on-demand master failed, falling back to ffmpeg",
			slog.String("media_id", mediaID), slog.Any("err", err))
		return false
	}
	setCORSHeaders(w, r, "")
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(master))
	// Counterpart of the ffmpeg path's "master served": one line per playback
	// start on the hybrid rung too, so the serving path is readable straight
	// from the logs (the copy fallback rung logs its own outcome just before).
	slog.Info("hls-vod: hybrid on-demand master served",
		slog.String("media_id", mediaID))
	return true
}

// buildHybridMaster renders the hybrid master: a single video-COPY variant
// whose playlist is the packager's (od_playlist.m3u8) tied to Evey's ffmpeg
// AAC audio group. Reuses buildManifestInput so the audio renditions, token,
// and query suffix match exactly what the audio handlers reconstruct.
func (h *hlsVODServe) buildHybridMaster(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) (string, error) {
	in, err := h.buildManifestInput(r, mediaID, item, streams)
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
		AudioBitrate: 128,
		Copy:         true,
	}}
	in.VideoPlaylistURI = onDemandPrefix + "playlist.m3u8"
	if fb := h.copyFallbackVariant(r, mediaID, item, streams, videoKbps+128); fb != nil {
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
	rT.URL.RawQuery = q.Encode()
	rpT, err := h.resolvePlan(rT, mediaID, item, streams)
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
		AudioCodec: inT.ChosenAudioCodec,
	}
}

func writeHLSMaster(w http.ResponseWriter, r *http.Request, master string) {
	setCORSHeaders(w, r, "")
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(master))
}

// floorRequested reports the survival mode our cast receiver's starvation
// rescue asks for (?abr=floor).
func floorRequested(r *http.Request) bool {
	return r != nil && r.URL.Query().Get("abr") == "floor"
}

// maxKbpsRequested is the receiver's own throughput measurement (its network
// stack included), 0 when absent or garbage.
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

// buildFloorMaster is buildCappedMaster's survival shape: the lowest rung,
// asked for by the receiver's starvation rescue (?abr=floor).
func (h *hlsVODServe) buildFloorMaster(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) (string, bool) {
	return h.buildCappedMaster(r, mediaID, item, streams, 0)
}

// buildCappedMaster renders a MONO-VARIANT master fitted to the receiver's
// measured throughput (?maxKbps=), or to the lowest rung when maxKbps is 0
// (?abr=floor). Copy stays first-class: with a measure that carries the copy
// peak with margin (1.2x - the starvation watchdog is the net behind an
// optimistic call), this returns false and the ordinary master shapes serve.
// Mono-variant on purpose: the receiver's own ABR is never relied upon - the
// muscle proven atrophied on old cast runtimes (Bouygtel4K, 2026-08-03).
//
// The rung rides this same request's natural vd=transcode plan with its own
// fp seal, and the variant URI carries the rung's LADDER index (stream_<i>) -
// the variant request re-derives the ladder, so any other name would serve
// the wrong profile. "session started hash=<fp>" attribution keeps working.
func (h *hlsVODServe) buildCappedMaster(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, maxKbps int64) (string, bool) {
	copyPeak := copyPeakKbpsOf(item)
	if maxKbps > 0 && copyPeak*12 <= maxKbps*10 {
		return "", false // copy-first: the measure carries the copy
	}
	rT := r.Clone(r.Context())
	q := rT.URL.Query()
	q.Set("vd", "transcode")
	rT.URL.RawQuery = q.Encode()
	rpT, err := h.resolvePlan(rT, mediaID, item, streams)
	if err != nil || rpT.IsCopy || len(rpT.Profiles) == 0 {
		slog.WarnContext(r.Context(), "hls-vod: capped master unresolvable, serving the ordinary master",
			slog.String("media_id", mediaID), slog.Any("err", err))
		return "", false
	}
	idx := 0
	if maxKbps > 0 {
		for i, p := range rpT.Profiles {
			if (p.VideoBitrate+p.AudioBitrate)*12 <= maxKbps*10 {
				idx = i
			}
		}
	}
	rung := rpT.Profiles[idx]
	if maxKbps == 0 {
		if (rung.VideoBitrate+rung.AudioBitrate)*2 > copyPeak+128 {
			slog.InfoContext(r.Context(), "hls-vod: floor refused, source already lean",
				slog.String("media_id", mediaID),
				slog.Int64("rung_kbps", rung.VideoBitrate+rung.AudioBitrate),
				slog.Int64("copy_peak_kbps", copyPeak+128))
			return "", false
		}
	} else if rung.VideoBitrate+rung.AudioBitrate >= copyPeak {
		return "", false // no real drop over the copy: pointless sidegrade
	}
	inT, err := h.buildManifestInputWith(rT, mediaID, item, streams, rpT)
	if err != nil {
		slog.WarnContext(r.Context(), "hls-vod: capped master unresolvable, serving the ordinary master",
			slog.String("media_id", mediaID), slog.Any("err", err))
		return "", false
	}
	inT.Profiles = inT.Profiles[idx : idx+1]
	// Bare name only: the manifest generator appends the baked query suffix
	// itself. Pre-suffixing here doubled it (prod 2026-08-05: a second "?"
	// mid-query, inherited by every segment URI of the rescue path).
	inT.VideoPlaylistURI = fmt.Sprintf("stream_%d.m3u8", idx)
	master, err := h.manifestGen.Master(inT)
	if err != nil {
		slog.WarnContext(r.Context(), "hls-vod: capped master build failed, serving the ordinary master",
			slog.String("media_id", mediaID), slog.Any("err", err))
		return "", false
	}
	slog.InfoContext(r.Context(), "hls-vod: capped master served",
		slog.String("media_id", mediaID),
		slog.String("rung", rung.Name),
		slog.Int64("rung_kbps", rung.VideoBitrate+rung.AudioBitrate),
		slog.Int64("max_kbps", maxKbps),
		slog.String("fp", rpT.FPHash))
	return master, true
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
func (h *hlsVODServe) serveWasmAudioMaster(w http.ResponseWriter, r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, span trace.Span) bool {
	if h.onDemand == nil || !wasmAudioRequested(r) || item == nil || item.File == nil || item.File.Path == "" {
		return false
	}
	// Cast kill switch: declining here hands the TV the ordinary muxed master -
	// sound from the first second, and not a line of our wasm decoder runs on
	// the box. See the field comment for why this is off by default.
	if wasmAudioRequested(r) && !wasmAudioEffective(r, h.castWasmAudioEnabled) {
		slog.InfoContext(r.Context(), "hls-vod: wasm-audio disabled for cast receivers, serving the muxed master",
			slog.String("media_id", mediaID))
		return false
	}

	audioTrack := h.resolveAudioTrack(r, streams)
	if !apptranscoding.WasmAudioCodecSupported(sourceAudioCodec(streams, audioTrack)) {
		return false // nothing for the wasm decoder to do - use the normal paths
	}
	// Channel gate. This path hands the client the ORIGINAL Dolby track, so it
	// can neither spare a weak box the decode nor honour a downmix the viewer
	// asked for. Declining returns the client to the muxed-AAC master, which
	// does both. 7.1 killed the receiver twice on a low-end operator box (prod
	// 2026-07-30).
	userMaxChannels := h.userPrefs(r.Context()).MaxAudioChannels
	if ch := apptranscoding.SourceAudioChannels(streams, audioTrack); !apptranscoding.WasmAudioChannelsOK(ch, userMaxChannels) {
		slog.InfoContext(r.Context(), "hls-vod: wasm-audio declined, serving the muxed master instead",
			slog.String("media_id", mediaID),
			slog.Int("audio_track", audioTrack),
			slog.Int("channels", ch),
			slog.Int("max_channels", wasmAudioMaxChannels),
			slog.Int("profile_max_channels", userMaxChannels))
		return false
	}

	decision := h.makeDecision(r.Context(), streams, containerOf(item), sourcePathOf(item), r)
	videoCopy := h.planHLSVideoEligible(decision, item, streams)

	// Prefer mkvgo's plan for the AC-3 audio rendition (no ffmpeg). Building it
	// also warms the cache the od_audio* sub-requests hit. A plan failure routes
	// the audio to the ffmpeg copy rendition instead - the source is still served.
	_, planErr := h.buildPlan(r, mediaID, item, streams)
	useMkvgoAudio := planErr == nil
	useMkvgoVideo := videoCopy && planErr == nil

	master, err := h.buildWasmAudioMaster(r, mediaID, item, streams, useMkvgoVideo, useMkvgoAudio, audioTrack)
	if err != nil {
		slog.WarnContext(r.Context(), "hls-vod: wasm-audio master failed, falling back",
			slog.String("media_id", mediaID), slog.Any("err", err))
		return false
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
	return true
}

// buildWasmAudioMaster renders the VIDEO-ONLY master for the wasm-audio path
// (see serveWasmAudioMaster). No audio group rides the master - the AC-3/E-AC-3
// rendition is named by a custom #EVEY-AUDIO-URI tag the client reads and hls.js
// ignores. useMkvgoVideo points the single video variant at the packager's copy
// playlist; otherwise the ffmpeg video-only ladder is kept. useMkvgoAudio picks
// the audio rendition URI (mkvgo's od_audio{k} vs the ffmpeg audio_{k} copy).
func (h *hlsVODServe) buildWasmAudioMaster(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, useMkvgoVideo, useMkvgoAudio bool, audioTrack int) (string, error) {
	in, err := h.buildManifestInput(r, mediaID, item, streams)
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
		uri := fmt.Sprintf("%saudio%d.m3u8?audioTrack=%d", onDemandPrefix, idx, audioTrack)
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
	plan, err := h.buildPlan(r, mediaID, item, streams)
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

	data, contentType, err := plan.Resource(r.Context(), name)
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
			slog.InfoContext(r.Context(), "hls-vod: on-demand resource served",
				slog.String("media_id", mediaID),
				slog.String("resource", name),
				slog.Bool("cast", cast),
				slog.Int("bytes", len(data)))
		}
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "on-demand resource")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
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
		suffix := "?v=" + url.QueryEscape(packagerVersion())
		if tok := RawTokenFromContext(r.Context()); tok != "" && TokenFromQuery(r.Context()) {
			suffix += "&token=" + url.QueryEscape(tok)
			if rq := r.URL.RawQuery; rq != "" {
				suffix += "&" + rq
			}
		}
		if strings.HasSuffix(name, ".mpd") {
			data = []byte(rebakeDASHManifestURIs(string(data), suffix))
		} else {
			data = []byte(rebakeManifestURIs(string(data), suffix))
		}
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	_, _ = w.Write(data)
}

// buildPlan resolves the plan for this request's media file with the fixed
// on-demand options (segment duration + URL prefix, plus AES-128 when the
// feature is on). Cheap on a cache hit. The AES key is deterministic per media,
// so the plan cache stays coherent across the master and its sub-requests.
func (h *hlsVODServe) buildPlan(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) (apptranscoding.OnDemandPlan, error) {
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
	if h.onDemandTrackTrim {
		// Same resolution the ffmpeg pipeline uses for ?audioTrack=/the
		// profile's language default (resolveAudioTrack), reused here so both
		// backends agree on which track a request gets. The streams come from
		// the CALLER (commonAuth fetched them for this very request) - this
		// used to re-query Postgres per sub-resource served. Best-effort: no
		// streams in hand serves every audio track rather than failing the plan.
		if len(streams) > 0 {
			idx := h.resolveAudioTrack(r, streams)
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

// planHLSVideoEligible reports whether this request's VIDEO can be served by
// the on-demand packager: the feature is wired on and the video was decided as
// copy (reusing isCopyRequest - same codec/level/HDR/disk gates as today's copy
// rung). Audio eligibility is decided separately (audioAllCopyEligible) because
// the packager can serve video-only and let Evey re-encode the audio. The
// decision is the caller's (computed once per request).
func (h *hlsVODServe) planHLSVideoEligible(decision domaintranscoding.PlaybackDecision, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) bool {
	if h.onDemand == nil || item == nil || item.File == nil || item.File.Path == "" {
		return false
	}
	// Deliberately NOT isCopyRequest: that gate also demands a wired ffmpeg
	// copyProducer and enough free cache to hold a full copy of the source. The
	// packager is stateless - it muxes on the fly and writes nothing to the cache
	// - so neither applies. Funnelling through it sent copy-eligible titles to a
	// TRANSCODE whenever the cache was tight or the operator had turned the ffmpeg
	// copy rung off: the most expensive path, on the weak boxes those guards
	// exist to protect.
	return videoCopyEligible(decision, streams)
}

// audioAllCopyEligible reports whether every audio track can be served verbatim
// by the packager: the decision is copy, no loudnorm, and EVERY audio track is
// CMAF-carriable - the packager demuxes them all into renditions, so one track
// it can't carry would strand a client that selects it. False routes to the
// hybrid master (packager video + Evey AAC audio renditions).
func (h *hlsVODServe) audioAllCopyEligible(ctx context.Context, decision domaintranscoding.PlaybackDecision, streams []*domainlibrary.MediaStream) bool {
	if decision.AudioDecision != "copy" {
		return false
	}
	if h.audioNormalizeGetter != nil && h.audioNormalizeGetter(ctx) {
		return false // loudnorm requires a re-encode - hybrid path
	}
	for _, s := range streams {
		if s == nil || s.StreamType != "audio" {
			continue
		}
		if !apptranscoding.AudioCopyCodecAllowed(strings.ToLower(s.Codec)) {
			return false
		}
	}
	return true // no audio track is fine too: PlanHLS packages video-only
}
