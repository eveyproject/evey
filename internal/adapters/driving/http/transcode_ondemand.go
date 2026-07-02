package httpapi

import (
	"log/slog"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// transcode_ondemand.go — the on-demand HLS/DASH copy rung served straight from
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
// a cache hit — the URL carries no decision state.

const (
	// onDemandPrefix decorates every sub-resource URI the plan's manifests
	// reference. serveHTTP routes `od_*` to the plan; Resource() gets the
	// canonical (unprefixed) name back.
	onDemandPrefix = "od_"
	// onDemandSegmentMs is the target segment duration for the on-demand plan.
	// Must be stable: master and sub-requests build the same plan (cached on
	// srcPath+segMs), so a drift here would desync the playlist from segments.
	onDemandSegmentMs = int64(6000)
)

// tryServePlanMaster serves the presentation from the on-demand packager when
// the request's VIDEO is copy-eligible and the source packages verbatim. Two
// modes, both stateless video from mp4.PlanHLS:
//
//   - audio also copy (every track CMAF-carriable, no loudnorm): the whole
//     presentation is the packager's master — video + demuxed audio + subs.
//   - audio must be re-encoded (DTS/TrueHD, loudnorm, a non-carriable track):
//     a HYBRID master — the video variant points at the packager (od_playlist),
//     the audio group stays Evey's ffmpeg AAC renditions. Requires the
//     audio-rendition pipeline wired + enabled; otherwise fall through.
//
// Returns true when it has written the response (served or a hard error) so
// serveMaster stops; false to fall through to the ffmpeg master (feature off,
// video not copy-eligible, plan unbuildable, or hybrid audio unavailable — a
// clean, logged fallback).
func (h *hlsVODServe) tryServePlanMaster(w http.ResponseWriter, r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, span trace.Span) bool {
	if !h.planHLSVideoEligible(r, item, streams) {
		return false
	}
	// The source must package verbatim (a codec CMAF can't carry, or a
	// Matroska without a Cues seek index → ffmpeg). Building here also warms
	// the plan cache the sub-resource requests hit.
	plan, err := h.buildPlan(r, item)
	if err != nil {
		slog.Warn("hls-vod: on-demand plan build failed, falling back to ffmpeg",
			slog.String("media_id", mediaID), slog.Any("err", err))
		return false
	}

	if h.audioAllCopyEligible(r, item, streams) {
		// Full packager presentation (video + audio + subtitles).
		h.servePlanResourceData(w, r, plan, "master.m3u8", span)
		return true
	}

	// Hybrid: video from the packager, audio re-encoded by Evey's rendition
	// pipeline. Only possible when that pipeline is wired and enabled.
	if !h.audioRenditionEnabled || h.audio == nil {
		return false
	}
	master, err := h.buildHybridMaster(r, mediaID, item, streams)
	if err != nil {
		slog.Warn("hls-vod: hybrid on-demand master failed, falling back to ffmpeg",
			slog.String("media_id", mediaID), slog.Any("err", err))
		return false
	}
	setCORSHeaders(w, r, "")
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(master))
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
	// Estimate the copy rung's peak bitrate for BANDWIDTH: the probed file
	// bitrate (kbps) if known, else derived from size/duration. Advisory only
	// (single variant, no ABR switching), so a rough value is fine.
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
		AudioBitrate: 128,
		Copy:         true,
	}}
	in.VideoPlaylistURI = onDemandPrefix + "playlist.m3u8"
	return h.manifestGen.Master(in)
}

// serveOnDemandResource handles a routed `od_<name>` sub-resource request (a
// playlist, init, segment, subtitle) or the top-level `manifest.mpd`. The plan
// is rebuilt (cache hit) from the source alone; access was already enforced by
// commonAuth. Unknown/unpackageable → 404.
func (h *hlsVODServe) serveOnDemandResource(w http.ResponseWriter, r *http.Request, mediaID, file string, item *domainlibrary.MediaItem, span trace.Span) {
	if h.onDemand == nil {
		span.SetStatus(codes.Error, "on-demand packaging not wired")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	name := strings.TrimPrefix(file, onDemandPrefix) // "od_seg1.m4s" → "seg1.m4s"; "manifest.mpd" unchanged
	plan, err := h.buildPlan(r, item)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "on-demand plan build")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	h.servePlanResourceData(w, r, plan, name, span)
}

// servePlanResourceData writes one plan resource (name is canonical). Playlists
// and manifests are marked no-cache (a re-scan can change them); media segments
// and init segments are immutable (byte-identical across builds), so they get a
// long immutable cache lifetime.
func (h *hlsVODServe) servePlanResourceData(w http.ResponseWriter, r *http.Request, plan apptranscoding.OnDemandPlan, name string, span trace.Span) {
	data, contentType, err := plan.Resource(r.Context(), name)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "on-demand resource")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	setCORSHeaders(w, r, "")
	w.Header().Set("Content-Type", contentType)
	if strings.HasSuffix(name, ".m3u8") || strings.HasSuffix(name, ".mpd") {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	_, _ = w.Write(data)
}

// buildPlan resolves the plan for this request's media file with the fixed
// on-demand options (segment duration + URL prefix). Cheap on a cache hit.
func (h *hlsVODServe) buildPlan(r *http.Request, item *domainlibrary.MediaItem) (apptranscoding.OnDemandPlan, error) {
	return h.onDemand.Plan(r.Context(), item.File.Path, apptranscoding.OnDemandOptions{
		SegmentMs: onDemandSegmentMs,
		URLPrefix: onDemandPrefix,
	})
}

// planHLSVideoEligible reports whether this request's VIDEO can be served by
// the on-demand packager: the feature is wired on and the video was decided as
// copy (reusing isCopyRequest — same codec/level/HDR/disk gates as today's copy
// rung). Audio eligibility is decided separately (audioAllCopyEligible) because
// the packager can serve video-only and let Evey re-encode the audio.
func (h *hlsVODServe) planHLSVideoEligible(r *http.Request, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) bool {
	if h.onDemand == nil || item == nil || item.File == nil || item.File.Path == "" {
		return false
	}
	decision := h.makeDecision(r.Context(), streams, containerOf(item), r)
	return h.isCopyRequest(decision, streams, sourceBytesOf(item))
}

// audioAllCopyEligible reports whether every audio track can be served verbatim
// by the packager: the decision is copy, no loudnorm, and EVERY audio track is
// CMAF-carriable — the packager demuxes them all into renditions, so one track
// it can't carry would strand a client that selects it. False routes to the
// hybrid master (packager video + Evey AAC audio renditions).
func (h *hlsVODServe) audioAllCopyEligible(r *http.Request, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) bool {
	decision := h.makeDecision(r.Context(), streams, containerOf(item), r)
	if decision.AudioDecision != "copy" {
		return false
	}
	if h.audioNormalizeGetter != nil && h.audioNormalizeGetter(r.Context()) {
		return false // loudnorm requires a re-encode — hybrid path
	}
	for _, s := range streams {
		if s == nil || s.StreamType != "audio" {
			continue
		}
		if !audioCopyCodecs[strings.ToLower(s.Codec)] {
			return false
		}
	}
	return true // no audio track is fine too: PlanHLS packages video-only
}
