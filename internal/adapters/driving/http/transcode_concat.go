package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
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
// STARTING episode's id. The master stamps the chain it served into every
// sub-resource URI (concatChainParam: its length and a digest of its sources),
// and a sub-resource replays exactly that chain. It used to re-decide the
// chain from its own request, which carries none of the client's capabilities:
// a stereo device's master left out a 5.1 episode its playlists then served.

const (
	concatPrefix = "cc_"
	// concatMaxEpisodes bounds the chain walk (NextEpisodeOf + per-episode
	// eligibility, each a few DB/probe round-trips) so a pathological season
	// doesn't make one request pay for the whole library.
	concatMaxEpisodes = 20
	// concatChainParam carries the chain the master served: "<n>.<digest>".
	concatChainParam = "cc"
)

// concatChainStamp names a chain by its length and a digest of its sources.
func concatChainStamp(chain []*domainlibrary.MediaItem) string {
	h := sha256.New()
	for _, ep := range chain {
		h.Write([]byte(ep.File.Path))
		h.Write([]byte{0})
	}
	return strconv.Itoa(len(chain)) + "." + hex.EncodeToString(h.Sum(nil))[:16]
}

// concatManifestSuffix is planManifestSuffix plus the chain stamp.
func concatManifestSuffix(r *http.Request, stamp string) string {
	return planManifestSuffix(r, "") + "&" + concatChainParam + "=" + stamp
}

// stampedConcatChain replays the chain a master stamped: the same number of
// episodes from start, still in its season and still allowed to the profile.
// The capability probes are not run again - the request of a sub-resource
// carries none of the client's capabilities, the master already judged them.
// ok is false when the chain no longer is the one stamped (an episode added,
// removed or replaced in between), which the caller answers 410.
func (h *hlsVODServe) stampedConcatChain(r *http.Request, start *domainlibrary.MediaItem, stamp string) (chain []*domainlibrary.MediaItem, ok bool) {
	nStr, _, found := strings.Cut(stamp, ".")
	n, err := strconv.Atoi(nStr)
	if !found || err != nil || n < 2 || n > concatMaxEpisodes {
		return nil, false
	}
	claims := HTTPClaimsFromContext(r.Context())
	chain = []*domainlibrary.MediaItem{start}
	for cur := start; len(chain) < n; {
		next, err := h.libSvc.NextEpisodeOf(r.Context(), string(cur.ID))
		if err != nil || next == nil || next.ParentID != start.ParentID || next.File == nil || next.File.Path == "" {
			return nil, false
		}
		if h.parental != nil && h.parental.BlockedByParental(r.Context(), claims.ProfileID, next) {
			return nil, false
		}
		chain = append(chain, next)
		cur = next
	}
	return chain, concatChainStamp(chain) == stamp
}

// serveConcatMaster resolves the episode chain starting at item (itself plus
// every following same-season episode, individually copy-eligible and not
// parental-blocked, up to concatMaxEpisodes) and serves it as one gapless HLS
// session. Returns false - the caller falls through to the normal
// per-episode master - when concat isn't requested/wired, encryption is on
// (v1 has no encrypted concat), fewer than two episodes qualify, or the plan
// build fails (an incompatible episode mkvgo's own probe caught). rp is the
// plan the master already resolved for item: the first link is not resolved twice.
func (h *hlsVODServe) serveConcatMaster(w http.ResponseWriter, r *http.Request, mediaID string, item *domainlibrary.MediaItem, rp *requestPlan, span trace.Span) bool {
	if h.onDemandConcat == nil || h.onDemandEncrypt || !r.URL.Query().Has("gapless") {
		return false
	}
	if !h.episodeConcatEligible(r, item, rp) {
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
	h.playbackActivity.Touch(mediaID)
	h.serveConcatResourceData(w, r, plan, "master.m3u8", concatChainStamp(chain), span)
	return true
}

// episodeConcatEligible reports whether one episode, given its resolved plan,
// qualifies as a concat chain link: video+audio copy-eligible (concat v1 has no
// hybrid/ffmpeg-audio mode, so both must be copy, unlike the standalone
// on-demand path) and not parental-blocked for the requesting profile.
func (h *hlsVODServe) episodeConcatEligible(r *http.Request, item *domainlibrary.MediaItem, rp *requestPlan) bool {
	if item == nil || item.File == nil || item.File.Path == "" || rp == nil {
		return false
	}
	claims := HTTPClaimsFromContext(r.Context())
	if h.parental != nil && h.parental.BlockedByParental(r.Context(), claims.ProfileID, item) {
		return false
	}
	// Every episode of the chain must be the packager's whole presentation
	// (video and every audio track carried verbatim): a projection of its plan.
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
		if err != nil {
			break
		}
		nPlan, err := h.probePlan(r, itemID(next), next, nStreams)
		if err != nil || !h.episodeConcatEligible(r, next, nPlan) {
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
	if isIFramePlaylist(name) {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	var chain []*domainlibrary.MediaItem
	stamp := r.URL.Query().Get(concatChainParam)
	if stamp != "" {
		var ok bool
		if chain, ok = h.stampedConcatChain(r, item, stamp); !ok {
			servePlanSourceGone(w, r, mediaID, name, span)
			return
		}
	} else {
		// A URI from a master served before the chain was stamped.
		chain = h.resolveEpisodeChain(r, item)
		stamp = concatChainStamp(chain)
	}
	if len(chain) < 2 {
		span.SetStatus(codes.Error, "concat chain no longer resolvable")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	// A viewer on a gapless session is watching as much as one on a single
	// episode: the background work that yields to playback must see it.
	h.playbackActivity.Touch(mediaID)
	plan, err := h.buildConcatPlan(r, chain)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "concat plan build")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	h.serveConcatResourceData(w, r, plan, name, stamp, span)
}

// serveConcatResourceData writes one concat plan resource. Same cache-header
// split as servePlanResourceData: playlists no-cache, their bare URIs stamped
// with the packager version, the request's query token and the chain;
// media/init segments immutable (byte-identical across builds).
//
// A part's media segment streams from its source through the plan's Open
// (mkvgo 0.51), as a single episode's does; playlists stay on Resource.
func (h *hlsVODServe) serveConcatResourceData(w http.ResponseWriter, r *http.Request, plan apptranscoding.ConcatPlan, name, stamp string, span trace.Span) {
	if answerPlanResourceByStat(w, r, plan, name, "mkvgo-concat") {
		return
	}
	data, contentType, streamed, err := fetchPlanResource(r.Context(), plan, name, r.Method == http.MethodHead)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "concat resource")
		writePlanResourceError(w, err)
		return
	}
	if streamed != nil {
		servePlanResourceStreamed(w, r, name, streamed, "mkvgo-concat", span)
		return
	}
	setCORSHeaders(w, r, "")
	// Same observability marker as the single-episode rung, distinguished so
	// an operator can tell a gapless session's traffic apart at a glance.
	w.Header().Set("X-Evey-Served-By", "mkvgo-concat")
	w.Header().Set("Content-Type", contentType)
	if strings.HasSuffix(name, ".m3u8") {
		w.Header().Set("Cache-Control", "no-cache")
		data = []byte(rebakeManifestURIs(string(data), concatManifestSuffix(r, stamp)))
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	_, _ = w.Write(data)
}
