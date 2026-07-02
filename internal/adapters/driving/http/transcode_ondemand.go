package httpapi

import (
	"log/slog"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
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

// tryServePlanMaster serves the whole presentation from the on-demand packager
// when the request is copy-eligible and the source packages verbatim. Returns
// true when it has written the response (served or a hard error) so serveMaster
// stops; false to fall through to the ffmpeg master (feature off, not
// copy-eligible, or the plan could not be built — a clean, logged fallback).
func (h *hlsVODServe) tryServePlanMaster(w http.ResponseWriter, r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, span trace.Span) bool {
	if !h.planHLSEligible(r, item, streams) {
		return false
	}
	plan, err := h.buildPlan(r, item)
	if err != nil {
		// The source is not packageable verbatim after all (a codec CMAF can't
		// carry, a Matroska without a Cues seek index). Fall back to ffmpeg.
		slog.Warn("hls-vod: on-demand plan build failed, falling back to ffmpeg",
			slog.String("media_id", mediaID), slog.Any("err", err))
		return false
	}
	h.servePlanResourceData(w, r, plan, "master.m3u8", span)
	return true
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

// planHLSEligible reports whether this request should be served by the
// on-demand packager instead of the ffmpeg copy/transcode pipeline. It is the
// intersection of: the feature wired on, the video decided as copy (reusing
// isCopyRequest — same codec/level/HDR/disk gates as today's copy rung), the
// audio decided as copy with no loudnorm (the packager copies audio verbatim),
// and EVERY audio track being CMAF-carriable — the packager demuxes them all
// into renditions, so one track it can't carry would strand a client that
// selects it. Anything less falls back to ffmpeg.
func (h *hlsVODServe) planHLSEligible(r *http.Request, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) bool {
	if h.onDemand == nil || item == nil || item.File == nil || item.File.Path == "" {
		return false
	}
	decision := h.makeDecision(r.Context(), streams, containerOf(item), r)
	if !h.isCopyRequest(decision, streams, sourceBytesOf(item)) {
		return false
	}
	if decision.AudioDecision != "copy" {
		return false
	}
	if h.audioNormalizeGetter != nil && h.audioNormalizeGetter(r.Context()) {
		return false // loudnorm requires a re-encode — ffmpeg path
	}
	// Every audio track must be passthrough-carriable (the packager copies all
	// of them into renditions; a non-carriable one has no re-encode fallback).
	hasAudio := false
	for _, s := range streams {
		if s == nil || s.StreamType != "audio" {
			continue
		}
		hasAudio = true
		if !audioCopyCodecs[strings.ToLower(s.Codec)] {
			return false
		}
	}
	_ = hasAudio // audio-only-video (no audio track) is fine too: PlanHLS packages video-only
	return true
}
