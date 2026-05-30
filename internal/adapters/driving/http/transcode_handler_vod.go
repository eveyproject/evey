package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	appqoe "github.com/evey-project/evey/internal/app/qoe"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/cmcd"
	eveymetrics "github.com/evey-project/evey/internal/pkg/metrics"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// hlsVODServe wires the new pipeline: a static, content-addressable HLS VOD
// manifest (no live-edge snap, no drift), an on-disk segment cache shared
// across sessions with the same DecisionHash, and an orchestrator that spawns
// ffmpeg rafales on cache miss. Replaces hlsMasterServe / hlsSegmentHandler
// once the runtime validation step lands.
type hlsVODServe struct {
	tokens               TokenVerifier
	libSvc               MediaStreamsGetter
	libraryAccess        LibraryAccessChecker
	parental             ParentalChecker
	profilePrefs         ProfilePrefsGetter
	audioNormalizeGetter func(ctx context.Context) bool
	userMaxStreams       UserMaxStreamsGetter

	manifestGen  *apptranscoding.ManifestGenerator
	cache        *apptranscoding.SegmentCache
	orchestrator *apptranscoding.SegmentOrchestrator

	// qoeSink ingests CMCD telemetry from segment requests. Nil when QoE is
	// disabled (tests, dev without postgres). Record is non-blocking.
	qoeSink appqoe.EventSink

	// complexityRepo reads the per-title classification used to pick the
	// ABR ladder. Nil-safe — profilesForDecision falls back to
	// LadderFilmNormal when the repo isn't wired (dev w/o postgres) or
	// the row hasn't been classified yet.
	complexityRepo domaintranscoding.ComplexityRepo

	// useFMP4 controls the segment container. fMP4 is the project default —
	// segments are ~30% smaller and the codecs string in the master playlist
	// matches what hls.js expects on modern Chromium/Firefox.
	useFMP4 bool

	// segmentWaitTimeout caps how long the segment handler will block waiting
	// for ffmpeg to land a segment. Larger than expected encode time but
	// smaller than the user's patience for a 504. Default 15s.
	segmentWaitTimeout time.Duration
}

// vodSegmentNameRE parses the on-the-wire segment file name and returns the
// (variantIdx, segmentIdx, isInit, ext) tuple. Strict: any other shape 404s.
var vodSegmentNameRE = regexp.MustCompile(`^stream_(\d+)(?:_(\d{3})|_init)\.(ts|m4s|mp4)$`)

// vodVariantNameRE matches the sub-playlist file name.
var vodVariantNameRE = regexp.MustCompile(`^stream_(\d+)\.m3u8$`)

// serveHTTP is the single dispatch entry point for `/hls/{file}`. It splits
// master vs sub-playlist vs segment by file name and delegates.
func (h *hlsVODServe) serveHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.transcode.HLSVOD")
	defer span.End()
	r = r.WithContext(ctx)

	mediaID := r.PathValue("id")
	file := r.PathValue("file")
	span.SetAttributes(attribute.String("media.id", mediaID), attribute.String("hls.file", file))

	// commonAuth resolves the media item (and its streams) on its way through
	// the access checks. Passing them down to the per-file handlers prevents
	// each downstream function from re-fetching — one master + one variant +
	// one segment request used to fan out to 9 DB hits, now down to 3.
	item, streams, ok := h.commonAuth(w, r, mediaID, span)
	if !ok {
		return
	}

	switch {
	case file == "master.m3u8":
		h.serveMaster(w, r, mediaID, item, streams, span)
	case vodVariantNameRE.MatchString(file):
		h.serveVariant(w, r, mediaID, file, item, streams, span)
	case vodSegmentNameRE.MatchString(file):
		h.serveSegment(w, r, mediaID, file, item, streams, span)
	default:
		span.SetStatus(codes.Error, "unknown hls path")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
	}
}

// commonAuth runs auth + access + parental and resolves the media item +
// streams. Returns (item, streams, true) to continue, (nil, nil, false) after
// writing an error response to abort. Resolving the media item here lets the
// per-file handlers avoid re-fetching downstream.
func (h *hlsVODServe) commonAuth(w http.ResponseWriter, r *http.Request, mediaID string, span trace.Span) (*domainlibrary.MediaItem, []*domainlibrary.MediaStream, bool) {
	if st, ok := StreamTokenFromContext(r.Context()); ok {
		if st.MediaItemID != shared.MediaItemID(mediaID) {
			span.SetStatus(codes.Error, "stream token media mismatch")
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return nil, nil, false
		}
	}
	claims := HTTPClaimsFromContext(r.Context())
	item, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
	if err != nil || item.File == nil || item.File.Path == "" {
		if err != nil {
			span.RecordError(err)
		}
		span.SetStatus(codes.Error, "media item not found")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return nil, nil, false
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
		span.SetStatus(codes.Error, "library access denied")
		return nil, nil, false
	}
	if !checkParental(r.Context(), w, claims, item, h.parental) {
		span.SetStatus(codes.Error, "parental restricted")
		return nil, nil, false
	}
	streams, err := h.libSvc.GetMediaStreams(r.Context(), mediaID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "streams resolve failed")
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return nil, nil, false
	}
	return item, streams, true
}

func (h *hlsVODServe) serveMaster(w http.ResponseWriter, r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, span trace.Span) {
	// Catches the startup CMCD payload (?CMCD=su,sid=...,ot=m) — the first
	// thing every player sends. Useful for "time to first frame" admin views.
	h.ingestCMCD(r, mediaID, 0, 0, false, true, false)

	in, err := h.buildManifestInput(r, mediaID, item, streams)
	if err != nil {
		h.writeManifestErr(w, span, err, mediaID)
		return
	}

	out, err := h.manifestGen.Master(in)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "manifest master")
		http.Error(w, `{"message":"manifest error"}`, http.StatusInternalServerError)
		return
	}

	setCORSHeaders(w, r, "")
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(out))

	slog.Info("hls-vod: master served",
		slog.String("media_id", mediaID),
		slog.Int("profiles", len(in.Profiles)),
		slog.Int("segments_per_variant", in.SegmentCount()),
	)
}

func (h *hlsVODServe) serveVariant(w http.ResponseWriter, r *http.Request, mediaID, file string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, span trace.Span) {
	m := vodVariantNameRE.FindStringSubmatch(file)
	profileIdx, _ := strconv.Atoi(m[1])

	// Catches CMCD on variant playlist fetches (typical when the player ABR
	// switches renditions). Helps the dashboard count switch frequency.
	h.ingestCMCD(r, mediaID, profileIdx, 0, false, false, true)

	in, err := h.buildManifestInput(r, mediaID, item, streams)
	if err != nil {
		h.writeManifestErr(w, span, err, mediaID)
		return
	}
	if profileIdx < 0 || profileIdx >= len(in.Profiles) {
		span.SetStatus(codes.Error, "variant idx out of range")
		http.Error(w, `{"message":"variant not found"}`, http.StatusNotFound)
		return
	}

	out, err := h.manifestGen.Variant(in, profileIdx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "manifest variant")
		http.Error(w, `{"message":"manifest error"}`, http.StatusInternalServerError)
		return
	}

	setCORSHeaders(w, r, "")
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(out))
}

func (h *hlsVODServe) serveSegment(w http.ResponseWriter, r *http.Request, mediaID, file string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, span trace.Span) {
	m := vodSegmentNameRE.FindStringSubmatch(file)
	variantIdx, _ := strconv.Atoi(m[1])
	isInit := m[2] == ""
	var segmentIdx int
	if !isInit {
		segmentIdx, _ = strconv.Atoi(m[2])
	}

	// CMCD ingest (CTA-5004). Parsed and shipped to the QoE sink before we
	// do anything that can 5xx — we want telemetry even on failure paths.
	// Sink is non-blocking; an absent payload is the no-op common case.
	h.ingestCMCD(r, mediaID, variantIdx, segmentIdx, isInit, false, false)
	// Phase 3 feedback loop: forward the CMCD signal to the orchestrator so
	// it can wake up a paused ffmpeg on starvation and stretch the prefetch
	// window when the client has a fat buffer. NEVER changes which variant
	// is served — that's the player's call.
	h.notifyOrchestratorHint(r, mediaID, segmentIdx)

	fp, err := h.buildFingerprint(r, mediaID, item, streams)
	if err != nil {
		h.writeManifestErr(w, span, err, mediaID)
		return
	}

	// Ensure ffmpeg is producing the right rafale. Async — we then block on
	// SegmentCache.Wait below for the specific file. Init segments piggyback
	// on the first rafale (they are produced alongside segment 0 by fMP4 hls
	// muxer).
	if !isInit {
		if err := h.orchestrator.EnsureSegmentRange(r.Context(), fp, segmentIdx); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "ensure segment range")
			slog.Warn("hls-vod: ensure failed",
				slog.String("media_id", mediaID),
				slog.Int("variant", variantIdx),
				slog.Int("seg", segmentIdx),
				slog.Any("err", err))
			http.Error(w, `{"message":"transcode unavailable"}`, http.StatusServiceUnavailable)
			return
		}
	} else {
		// Init segments are emitted alongside the first rafale; ensure a
		// rafale exists by asking for segment 0.
		if err := h.orchestrator.EnsureSegmentRange(r.Context(), fp, 0); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "ensure init range")
			http.Error(w, `{"message":"transcode unavailable"}`, http.StatusServiceUnavailable)
			return
		}
	}

	key := apptranscoding.SegmentKey{
		DecisionHash: fp.Hash(),
		VariantIdx:   variantIdx,
		SegmentIdx:   segmentIdx,
		IsInit:       isInit,
	}

	waitCtx, cancel := context.WithTimeout(r.Context(), h.segmentWaitTimeout)
	defer cancel()
	if err := h.cache.Wait(waitCtx, key, fp.UseFMP4, h.segmentWaitTimeout); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "segment wait timeout")
		slog.Warn("hls-vod: segment timeout",
			slog.String("media_id", mediaID),
			slog.Int("variant", variantIdx),
			slog.Int("seg", segmentIdx),
			slog.Bool("init", isInit),
			slog.Any("err", err))
		http.Error(w, `{"message":"segment not yet ready, retry"}`, http.StatusServiceUnavailable)
		return
	}

	path := h.cache.Path(key, fp.UseFMP4)
	if _, statErr := os.Stat(path); statErr != nil {
		span.RecordError(statErr)
		span.SetStatus(codes.Error, "segment stat")
		http.Error(w, `{"message":"segment missing"}`, http.StatusNotFound)
		return
	}

	setCORSHeaders(w, r, "")
	switch filepath.Ext(path) {
	case ".ts":
		w.Header().Set("Content-Type", "video/mp2t")
	case ".m4s", ".mp4":
		w.Header().Set("Content-Type", "video/mp4")
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, path)
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

	decision := h.makeDecision(r.Context(), streams, item.File.ContainerFormat, r)
	profiles := h.profilesForDecision(r.Context(), item, streams, decision)

	audioTrack := parseIntQuery(r, "audioTrack", 0)
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
		ChosenVideoCodec:    decision.VideoCodec,
		ChosenAudioCodec:    decision.AudioCodec,
		ResumeOffsetSec:     resumeSec,
	}, nil
}

// buildFingerprint reproduces the DecisionFingerprint from a segment-time
// request. Must match exactly what was produced at manifest time, or the
// cache lookup misses on every segment. Hence shared helpers.
func (h *hlsVODServe) buildFingerprint(r *http.Request, mediaID string, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) (apptranscoding.DecisionFingerprint, error) {
	if item == nil || item.File == nil {
		return apptranscoding.DecisionFingerprint{}, errors.New("buildFingerprint: nil item")
	}
	decision := h.makeDecision(r.Context(), streams, item.File.ContainerFormat, r)
	profiles := h.profilesForDecision(r.Context(), item, streams, decision)

	audioTrack := parseIntQuery(r, "audioTrack", 0)
	subBurnIn := parseIntQuery(r, "subtitleTrack", -1)

	fpProfiles := make([]apptranscoding.ProfileFingerprint, len(profiles))
	for i, p := range profiles {
		fpProfiles[i] = apptranscoding.ProfileFingerprint{
			Name:         p.Name,
			Width:        p.Width,
			Height:       p.Height,
			VideoBitrate: p.VideoBitrate,
			AudioBitrate: p.AudioBitrate,
		}
	}

	return apptranscoding.DecisionFingerprint{
		MediaID:             mediaID,
		SourcePath:          item.File.Path,
		VideoCodec:          decision.VideoCodec,
		AudioCodec:          decision.AudioCodec,
		AudioTrack:          audioTrack,
		SubtitleBurnInTrack: subBurnIn,
		HDRTonemap:          false, // wired in Phase 3
		AudioNormalize:      h.audioNormalizeGetter != nil && h.audioNormalizeGetter(r.Context()),
		MaxAudioChannels:    decision.MaxAudioChannels,
		ForceSidecarSubs:    decision.SubtitleDecision == "external",
		UseFMP4:             h.useFMP4,
		Profiles:            fpProfiles,
	}, nil
}

func (h *hlsVODServe) makeDecision(ctx context.Context, streams []*domainlibrary.MediaStream, container string, r *http.Request) domaintranscoding.PlaybackDecision {
	claims := HTTPClaimsFromContext(r.Context())
	prefs := loadProfilePrefs(ctx, h.profilePrefs, claims.ProfileID)
	hdr := r.URL.Query().Get("hdr") == "1"
	client := parseClientCapabilities(r, hdr)
	d := apptranscoding.MakePlaybackDecisionWithPrefs(ctx, streams, container, client, 0, prefs)
	if d == nil {
		return domaintranscoding.PlaybackDecision{}
	}
	return *d
}

// profilesForDecision picks the ABR ladder for a given (decision, mediaItem,
// streams) triple. Reads the per-title complexity class persisted by the
// background ComplexityWorker; falls back to LadderFilmNormal when the
// item hasn't been classified yet (safe: matches pre-refactor behaviour).
// Caps the ladder to the source resolution so we never advertise a 4K rung
// for a 1080p source.
func (h *hlsVODServe) profilesForDecision(ctx context.Context, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, _ domaintranscoding.PlaybackDecision) []domaintranscoding.HLSProfile {
	class := domaintranscoding.ClassFilmNormal
	if h.complexityRepo != nil && item != nil {
		if entry, found, err := h.complexityRepo.Get(ctx, string(item.ID)); err == nil && found && entry.Class.IsValid() {
			class = entry.Class
		}
	}
	ladder := domaintranscoding.LadderFor(class)
	// Source resolution comes from the first video stream's dimensions.
	// Streams may be missing (degenerate file) — CapLadderToSource handles
	// zero values as "unknown, don't cap". Width/Height are *int on the
	// MediaStream domain type so a missing probe doesn't conflate "unknown"
	// with "zero pixels".
	var srcW, srcH int
	for _, s := range streams {
		if s != nil && s.StreamType == "video" {
			if s.Width != nil {
				srcW = *s.Width
			}
			if s.Height != nil {
				srcH = *s.Height
			}
			break
		}
	}
	return domaintranscoding.CapLadderToSource(ladder, srcW, srcH)
}

// durationMissingError is the specific 503 we return when a media file's
// duration isn't yet probed. The frontend retries on 503; a background re-probe
// is triggered by the library scheduler in the normal course.
type durationMissingError struct{ mediaID string }

func (e *durationMissingError) Error() string {
	return fmt.Sprintf("durationMissingError: media %s has no duration_ms", e.mediaID)
}

func (h *hlsVODServe) writeManifestErr(w http.ResponseWriter, span trace.Span, err error, mediaID string) {
	var dme *durationMissingError
	if errors.As(err, &dme) {
		span.SetStatus(codes.Error, "duration missing")
		slog.Warn("hls-vod: duration missing",
			slog.String("media_id", mediaID))
		w.Header().Set("Retry-After", "5")
		http.Error(w, `{"message":"media duration not yet probed, retry"}`, http.StatusServiceUnavailable)
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, "buildManifestInput")
	http.Error(w, `{"message":"manifest error"}`, http.StatusInternalServerError)
}

// MediaSourceResolver implements apptranscoding.SourceResolver on top of
// MediaStreamsGetter. The orchestrator calls it when it needs the file path
// and streams to feed the ffmpeg adapter; the handler doesn't go through it
// (it already has the resolved media item to also build the manifest).
type MediaSourceResolver struct {
	LibSvc MediaStreamsGetter
}

func (r *MediaSourceResolver) ResolveSource(ctx context.Context, fp apptranscoding.DecisionFingerprint) (string, []apptranscoding.SourceStream, error) {
	if r == nil || r.LibSvc == nil {
		return "", nil, errors.New("MediaSourceResolver: LibSvc nil")
	}
	item, err := r.LibSvc.GetMediaItem(ctx, fp.MediaID)
	if err != nil {
		return "", nil, fmt.Errorf("ResolveSource: %w", err)
	}
	if item.File == nil || item.File.Path == "" {
		return "", nil, errors.New("ResolveSource: media has no file")
	}
	// Phase 1: streams not needed at orchestrator level — the ffmpeg adapter's
	// defaults cover the video+audio path. Phase 3 will plumb real streams for
	// burn-in subtitle / HDR tonemap branches.
	return item.File.Path, nil, nil
}

// newHLSVODServe binds the handler to the orchestrator + cache that wire.go
// has already built. Returns nil if either prerequisite is missing so the
// caller skips route registration cleanly.
func newHLSVODServe(s *HTTPServer, libSvc MediaStreamsGetter, libAccess LibraryAccessChecker, parental ParentalChecker) *hlsVODServe {
	_ = libAccess // wired through commonAuth via s.LibraryAccessRepo
	if s == nil || s.HLSVODOrch == nil || s.HLSVODCache == nil {
		return nil
	}
	cache := s.HLSVODCache
	orch := s.HLSVODOrch
	return &hlsVODServe{
		tokens:               s.TokenGen,
		libSvc:               libSvc,
		libraryAccess:        libAccess,
		parental:             parental,
		profilePrefs:         s.ProfilePrefs,
		audioNormalizeGetter: s.AudioNormalizeGetter,
		userMaxStreams:       s.UserMaxStreams,
		manifestGen:          apptranscoding.NewManifestGenerator(),
		cache:                cache,
		orchestrator:         orch,
		qoeSink:              s.QoESink,
		complexityRepo:       s.ComplexityRepo,
		// fMP4 is now the project default — matches the ffmpeg adapter
		// (needsFMP4Segments returns true unconditionally) and Apple's
		// post-WWDC-2024 stance on CMAF. Per-rendition behaviour stays
		// uniform across all codecs (H.264 / HEVC / AV1 / VP9).
		useFMP4: true,
		// 25 s covers a worst-case ffmpeg cold start at -ss far into the
		// media (the slow probe + initial decode + 1st keyframe + first
		// segment encode chain has been measured at 11-14 s on the dev
		// box). hls.js retries 503 automatically so a tighter cap would
		// just produce visible buffering.
		segmentWaitTimeout: 25 * time.Second,
	}
}

// ---- small parsing helpers (kept private to this file) ----

func parseIntQuery(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func splitCSVQuery(r *http.Request, key string) []string {
	if v := r.URL.Query().Get(key); v != "" {
		return strings.Split(v, ",")
	}
	return nil
}

// servePrewarmNext handles POST /api/v1/media/{id}/prewarm-next. The {id}
// is the *current* episode. The handler resolves the next episode in the
// series and spawns a ffmpeg rafale targeting its first segment so a
// later autoplay (or next-ep button) hits a warm cache and starts in
// under a second. Safe to fire multiple times — EnsureSegmentRange is
// idempotent.
//
// Returns 200 + {next_media_item_id} on success, 204 when the current
// episode is the series finale, 4xx on bad input. Never propagates a
// transcode error to the player; warm-up failure is a UX nicety, not a
// hard playback path.
func (h *hlsVODServe) servePrewarmNext(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.transcode.PrewarmNext")
	defer span.End()
	r = r.WithContext(ctx)

	currentID := r.PathValue("id")
	if currentID == "" {
		http.Error(w, `{"message":"missing media id"}`, http.StatusBadRequest)
		return
	}

	claims := HTTPClaimsFromContext(r.Context())
	curItem, err := h.libSvc.GetMediaItem(r.Context(), currentID)
	if err != nil || curItem == nil {
		http.Error(w, `{"message":"current media not found"}`, http.StatusNotFound)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, curItem.LibraryID, h.libraryAccess) {
		return
	}
	if curItem.Type != "episode" {
		// Only series episodes have a "next ep". For movies, no-op.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	nextItem, err := h.libSvc.NextEpisodeOf(r.Context(), currentID)
	if err != nil {
		span.RecordError(err)
		slog.Warn("prewarm: next episode lookup failed", slog.String("current_id", currentID), slog.Any("err", err))
		http.Error(w, `{"message":"prewarm lookup failed"}`, http.StatusInternalServerError)
		return
	}
	if nextItem == nil || nextItem.File == nil || nextItem.File.Path == "" {
		// Series finale or next ep file missing — nothing to warm.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Re-check access on the NEXT item independently. The current item's
	// access doesn't imply the next one — a series can straddle libraries,
	// and a kid profile may have access to the current ep but not the next
	// (different rating / blocked tags). Without these two checks an
	// attacker could pivot from an accessible ep to trigger a transcode
	// rafale on any media they shouldn't see.
	if !checkLibraryAccess(r.Context(), w, claims, nextItem.LibraryID, h.libraryAccess) {
		return
	}
	if !checkParental(r.Context(), w, claims, nextItem, h.parental) {
		return
	}

	streams, err := h.libSvc.GetMediaStreams(r.Context(), string(nextItem.ID))
	if err != nil {
		span.RecordError(err)
		slog.Warn("prewarm: streams lookup failed", slog.String("next_id", string(nextItem.ID)), slog.Any("err", err))
		http.Error(w, `{"message":"prewarm streams lookup failed"}`, http.StatusInternalServerError)
		return
	}

	// Build the same fingerprint the player would generate when it starts
	// playing this next episode: capabilities forwarded via query params,
	// per-title ladder via profilesForDecision, defaults safe.
	decision := h.makeDecision(r.Context(), streams, nextItem.File.ContainerFormat, r)
	profiles := h.profilesForDecision(r.Context(), nextItem, streams, decision)
	fpProfiles := make([]apptranscoding.ProfileFingerprint, len(profiles))
	for i, p := range profiles {
		fpProfiles[i] = apptranscoding.ProfileFingerprint{
			Name: p.Name, Width: p.Width, Height: p.Height,
			VideoBitrate: p.VideoBitrate, AudioBitrate: p.AudioBitrate,
		}
	}
	fp := apptranscoding.DecisionFingerprint{
		MediaID:             string(nextItem.ID),
		SourcePath:          nextItem.File.Path,
		Profiles:            fpProfiles,
		AudioTrack:          parseIntQuery(r, "audioTrack", 0),
		SubtitleBurnInTrack: parseIntQuery(r, "subtitleTrack", -1),
		ForceSidecarSubs:    false,
		VideoCodec:          decision.VideoCodec,
		AudioCodec:          decision.AudioCodec,
		MaxAudioChannels:    decision.MaxAudioChannels,
		HDRTonemap:          false,
		AudioNormalize:      h.audioNormalizeGetter != nil && h.audioNormalizeGetter(r.Context()),
		UseFMP4:             h.useFMP4,
	}

	if err := h.orchestrator.EnsureSegmentRange(r.Context(), fp, 0); err != nil {
		// Don't fail the response — the player will reach a cold cache
		// on switch, just no faster than before.
		slog.Warn("prewarm: orchestrator EnsureSegmentRange failed",
			slog.String("next_id", string(nextItem.ID)), slog.Any("err", err))
	} else {
		slog.Info("prewarm: rafale spawned for next episode",
			slog.String("current_id", currentID),
			slog.String("next_id", string(nextItem.ID)),
			slog.String("next_title", nextItem.Title))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{"next_media_item_id":%q}`+"\n", string(nextItem.ID))
}

// ingestCMCD parses CMCD (CTA-5004) from the request and forwards the event
// to the QoE sink. No-op when CMCD is absent or the sink isn't wired (dev /
// tests). Resolves user_id from the request context — anonymous plays
// (shared link, no token) end up with an empty user_id, which is fine.
func (h *hlsVODServe) ingestCMCD(r *http.Request, mediaID string, variantIdx, segmentIdx int, isInit, isManifest, isVariantPL bool) {
	if h.qoeSink == nil {
		return
	}
	payload, ok := cmcd.FromRequest(r)
	if !ok {
		return
	}
	eveymetrics.QoEIngestEvents.Inc()

	var userID string
	if claims := HTTPClaimsFromContext(r.Context()); claims != nil {
		userID = claims.UserID
	}

	// TODO: remove after CMCD stabilization — Info-level so we see ingest
	// activity in prod logs during Phase 1 rollout. Downgrade to Debug
	// (or drop entirely) once the SQL table and dashboard are confirmed
	// working.
	slog.Info("hls-vod: CMCD ingest",
		slog.String("media_id", mediaID),
		slog.String("user_id", userID),
		slog.Int("variant", variantIdx),
		slog.Int("seg", segmentIdx),
		slog.Bool("init", isInit),
		slog.Bool("manifest", isManifest),
		slog.Bool("variant_pl", isVariantPL),
		slog.String("sid", payload.SessionID),
		slog.Int("buffer_ms", payload.BufferLengthMs),
		slog.Int("encoded_kbps", payload.EncodedKbps),
		slog.Int("measured_kbps", payload.MeasuredKbps),
		slog.Bool("starved", payload.BufferStarved),
		slog.Bool("startup", payload.Startup))

	h.qoeSink.Record(appqoe.Event{
		UserID:      userID,
		MediaItemID: mediaID,
		VariantIdx:  variantIdx,
		SegmentIdx:  segmentIdx,
		IsInit:      isInit,
		IsManifest:  isManifest,
		IsVariantPL: isVariantPL,
		DeviceClass: appqoe.DeviceClass(r),
		RemoteIP:    appqoe.RemoteIP(r),
		Payload:     payload,
	})
}

// notifyOrchestratorHint forwards CMCD telemetry (buffer length, starvation
// flag, current segment cursor) to the orchestrator's NotifyClientHint so
// the throttle / prefetch policy can react in real time. Read-only on the
// HTTP path — never blocks, never errors. No-op when CMCD is absent.
//
// NEVER touches which variant is served — the player owns rendition
// selection. Phase 3 feedback loop only adjusts encoder pacing.
func (h *hlsVODServe) notifyOrchestratorHint(r *http.Request, mediaID string, segmentIdx int) {
	if h.orchestrator == nil {
		return
	}
	payload, ok := cmcd.FromRequest(r)
	if !ok {
		return
	}
	// Bare-minimum signals: buffer length (ms) and starvation flag drive
	// the policy. Object type "v"/"av" matters most (video pipeline),
	// audio-only fetches can still bump clientHighWater forward harmlessly.
	h.orchestrator.NotifyClientHint(mediaID, payload.BufferLengthMs, payload.BufferStarved, segmentIdx)
}
