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

// transcode_concat.go - gapless multi-episode HLS (mkvgo's mp4.PlanConcat,
// apptranscoding.ConcatPackager). Opt-in per request (?gapless=1 on the
// master URL, the frontend's "lecture enchaînée" toggle): the episode chain
// starting at the requested one is packaged as ONE continuous HLS session
// (EXT-X-DISCONTINUITY at each episode boundary, no player reload). Any
// failure - too short a chain, an incompatible episode, encryption on, the
// feature off - falls through to the normal per-episode master unchanged;
// this is purely additive.
//
// Wire: sub-resources are prefixed concatPrefix so serveHTTP routes them here
// instead of the single-episode on-demand path; the URL still keys on the
// STARTING episode's id (the chain itself is re-resolved, cache-hit on the
// packager, from that one id - no episode list travels in the URL).

const (
	concatPrefix = "cc_"
	// concatMaxEpisodes bounds the chain walk (NextEpisodeOf + per-episode
	// eligibility, each a few DB/probe round-trips) so a pathological season
	// doesn't make one request pay for the whole library.
	concatMaxEpisodes = 20
)

// serveConcatMaster resolves the episode chain starting at item (itself plus
// every following same-season episode, individually copy-eligible and not
// parental-blocked, up to concatMaxEpisodes) and serves it as one gapless HLS
// session. Returns false - the caller falls through to the normal
// per-episode master - when concat isn't requested/wired, encryption is on
// (v1 has no encrypted concat), fewer than two episodes qualify, or the plan
// build fails (an incompatible episode mkvgo's own probe caught).
func (h *hlsVODServe) serveConcatMaster(w http.ResponseWriter, r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, span trace.Span) bool {
	if h.onDemandConcat == nil || h.onDemandEncrypt || !r.URL.Query().Has("gapless") {
		return false
	}
	if !h.episodeConcatEligible(r, item, streams) {
		return false
	}

	chain := h.resolveEpisodeChain(r, item)
	if len(chain) < 2 {
		return false
	}

	plan, err := h.buildConcatPlan(r, chain)
	if err != nil {
		slog.WarnContext(r.Context(), "hls vod: concat plan build failed, falling back to per-episode playback",
			slog.String("media_id", mediaID), slog.Int("chain_len", len(chain)), slog.Any("err", err))
		return false
	}
	h.serveConcatResourceData(w, r, plan, "master.m3u8", span)
	return true
}

// episodeConcatEligible reports whether one already-fetched episode qualifies
// as a concat chain link: video+audio copy-eligible (concat v1 has no
// hybrid/ffmpeg-audio mode, so both must be copy, unlike the standalone
// on-demand path) and not parental-blocked for the requesting profile.
func (h *hlsVODServe) episodeConcatEligible(r *http.Request, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) bool {
	if item == nil || item.File == nil || item.File.Path == "" {
		return false
	}
	claims := HTTPClaimsFromContext(r.Context())
	if h.parental != nil && h.parental.BlockedByParental(r.Context(), claims.ProfileID, item) {
		return false
	}
	// Every episode of the chain must be the packager's whole presentation
	// (video and every audio track carried verbatim): a projection of its plan.
	rp, err := h.probePlan(r, itemID(item), item, streams)
	if err != nil {
		return false
	}
	d := rp.Plan.Delivery
	return d.Video == apptranscoding.EngineMkvgo && d.Audio == apptranscoding.EngineMkvgo && !d.AudioTrimmed
}

// resolveEpisodeChain walks NextEpisodeOf from start, keeping only episodes
// still in the same season (same ParentID - NextEpisodeOf itself crosses into
// the next season at a finale, which v1 deliberately does not chain across)
// and individually concat-eligible. Stops at the first episode that is
// neither, rather than failing the whole chain: a mid-season edit or a
// genuinely incompatible episode should not cost the episodes already
// gathered their gapless session.
func (h *hlsVODServe) resolveEpisodeChain(r *http.Request, start *domainlibrary.MediaItem) []*domainlibrary.MediaItem {
	chain := []*domainlibrary.MediaItem{start}
	cur := start
	for len(chain) < concatMaxEpisodes {
		next, err := h.libSvc.NextEpisodeOf(r.Context(), string(cur.ID))
		if err != nil || next == nil || next.ParentID != start.ParentID {
			break
		}
		nStreams, err := h.libSvc.GetMediaStreams(r.Context(), string(next.ID))
		if err != nil || !h.episodeConcatEligible(r, next, nStreams) {
			break
		}
		chain = append(chain, next)
		cur = next
	}
	return chain
}

// buildConcatPlan resolves the concat plan for an episode chain - cached by
// the packager on the chain's source paths, so a master request and its
// sub-resource requests (which re-resolve the same chain) hit the cache.
func (h *hlsVODServe) buildConcatPlan(r *http.Request, chain []*domainlibrary.MediaItem) (apptranscoding.ConcatPlan, error) {
	sources := make([]string, len(chain))
	for i, ep := range chain {
		sources[i] = ep.File.Path
	}
	opts := apptranscoding.ConcatOptions{SegmentMs: onDemandSegmentMs, URLPrefix: concatPrefix}
	return h.onDemandConcat.Plan(r.Context(), sources, opts)
}

// serveConcatResource handles a routed `cc_<name>` sub-resource request:
// re-resolves the same chain from the starting episode alone (cache hit on
// the packager) and serves the named resource.
func (h *hlsVODServe) serveConcatResource(w http.ResponseWriter, r *http.Request, mediaID, file string, item *domainlibrary.MediaItem, span trace.Span) {
	if h.onDemandConcat == nil {
		span.SetStatus(codes.Error, "concat packaging not wired")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	name := strings.TrimPrefix(file, concatPrefix)
	chain := h.resolveEpisodeChain(r, item)
	if len(chain) < 2 {
		span.SetStatus(codes.Error, "concat chain no longer resolvable")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	plan, err := h.buildConcatPlan(r, chain)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "concat plan build")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	h.serveConcatResourceData(w, r, plan, name, span)
}

// serveConcatResourceData writes one concat plan resource. Same cache-header
// split as servePlanResourceData: playlists no-cache, media/init segments
// immutable (byte-identical across builds).
func (h *hlsVODServe) serveConcatResourceData(w http.ResponseWriter, r *http.Request, plan apptranscoding.ConcatPlan, name string, span trace.Span) {
	data, contentType, err := plan.Resource(r.Context(), name)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "concat resource")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	setCORSHeaders(w, r, "")
	// Same observability marker as the single-episode rung, distinguished so
	// an operator can tell a gapless session's traffic apart at a glance.
	w.Header().Set("X-Evey-Served-By", "mkvgo-concat")
	w.Header().Set("Content-Type", contentType)
	if strings.HasSuffix(name, ".m3u8") {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	_, _ = w.Write(data)
}
