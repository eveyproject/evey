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

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
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

// serveHTTP is the single dispatch entry point for `/hls/v2/{file}`. It splits
// master vs sub-playlist vs segment by file name and delegates.
func (h *hlsVODServe) serveHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.transcode.HLSVOD")
	defer span.End()
	r = r.WithContext(ctx)

	mediaID := r.PathValue("id")
	file := r.PathValue("file")
	span.SetAttributes(attribute.String("media.id", mediaID), attribute.String("hls.file", file))

	if !h.commonAuth(w, r, mediaID, span) {
		return
	}

	switch {
	case file == "master.m3u8":
		h.serveMaster(w, r, mediaID, span)
	case vodVariantNameRE.MatchString(file):
		h.serveVariant(w, r, mediaID, file, span)
	case vodSegmentNameRE.MatchString(file):
		h.serveSegment(w, r, mediaID, file, span)
	default:
		span.SetStatus(codes.Error, "unknown hls path")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
	}
}

// commonAuth runs the auth + access + parental checks shared by every handler.
// Returns true to continue, false (after writing an error response) to abort.
func (h *hlsVODServe) commonAuth(w http.ResponseWriter, r *http.Request, mediaID string, span trace.Span) bool {
	if st, ok := StreamTokenFromContext(r.Context()); ok {
		if st.MediaItemID != shared.MediaItemID(mediaID) {
			span.SetStatus(codes.Error, "stream token media mismatch")
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return false
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
		return false
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
		span.SetStatus(codes.Error, "library access denied")
		return false
	}
	if !checkParental(r.Context(), w, claims, item, h.parental) {
		span.SetStatus(codes.Error, "parental restricted")
		return false
	}
	return true
}

func (h *hlsVODServe) serveMaster(w http.ResponseWriter, r *http.Request, mediaID string, span trace.Span) {
	in, err := h.buildManifestInput(r, mediaID)
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

func (h *hlsVODServe) serveVariant(w http.ResponseWriter, r *http.Request, mediaID, file string, span trace.Span) {
	m := vodVariantNameRE.FindStringSubmatch(file)
	profileIdx, _ := strconv.Atoi(m[1])

	in, err := h.buildManifestInput(r, mediaID)
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

func (h *hlsVODServe) serveSegment(w http.ResponseWriter, r *http.Request, mediaID, file string, span trace.Span) {
	m := vodSegmentNameRE.FindStringSubmatch(file)
	variantIdx, _ := strconv.Atoi(m[1])
	isInit := m[2] == ""
	var segmentIdx int
	if !isInit {
		segmentIdx, _ = strconv.Atoi(m[2])
	}

	fp, err := h.buildFingerprint(r, mediaID)
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
func (h *hlsVODServe) buildManifestInput(r *http.Request, mediaID string) (apptranscoding.ManifestInput, error) {
	item, streams, err := h.resolveMedia(r.Context(), mediaID)
	if err != nil {
		return apptranscoding.ManifestInput{}, err
	}

	if item.File.Duration.Milliseconds() <= 0 {
		return apptranscoding.ManifestInput{}, &durationMissingError{mediaID: mediaID}
	}

	decision := h.makeDecision(r.Context(), streams, item.File.ContainerFormat, r)
	profiles := h.profilesForDecision(decision)

	audioTrack := parseIntQuery(r, "audioTrack", 0)
	subBurnIn := parseIntQuery(r, "subtitleTrack", -1)

	token := RawTokenFromContext(r.Context())

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
	}, nil
}

// buildFingerprint reproduces the DecisionFingerprint from a segment-time
// request. Must match exactly what was produced at manifest time, or the
// cache lookup misses on every segment. Hence shared helpers.
func (h *hlsVODServe) buildFingerprint(r *http.Request, mediaID string) (apptranscoding.DecisionFingerprint, error) {
	item, streams, err := h.resolveMedia(r.Context(), mediaID)
	if err != nil {
		return apptranscoding.DecisionFingerprint{}, err
	}
	decision := h.makeDecision(r.Context(), streams, item.File.ContainerFormat, r)
	profiles := h.profilesForDecision(decision)

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

func (h *hlsVODServe) resolveMedia(ctx context.Context, mediaID string) (*domainlibrary.MediaItem, []*domainlibrary.MediaStream, error) {
	item, err := h.libSvc.GetMediaItem(ctx, mediaID)
	if err != nil {
		return nil, nil, fmt.Errorf("resolveMedia: %w", err)
	}
	if item.File == nil || item.File.Path == "" {
		return nil, nil, errors.New("resolveMedia: media has no file")
	}
	streams, err := h.libSvc.GetMediaStreams(ctx, mediaID)
	if err != nil {
		return nil, nil, fmt.Errorf("resolveMedia: streams: %w", err)
	}
	return item, streams, nil
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

// profilesForDecision picks the ABR ladder for a given decision. Phase 1 keeps
// only the 360p + 480p rungs (the production-deployed pair, see logs); Phase 2
// will refine based on source resolution and client capabilities.
func (h *hlsVODServe) profilesForDecision(_ domaintranscoding.PlaybackDecision) []domaintranscoding.HLSProfile {
	all := domaintranscoding.DefaultHLSProfiles
	if len(all) < 2 {
		return all
	}
	return all[:2]
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

// newHLSVODServe wires the orchestration components (cache + orchestrator)
// onto the server's dependencies. Called once at route registration time;
// returns nil if any prerequisite is missing (in which case the caller skips
// route registration so the legacy /hls/ path stays the only entry point).
func newHLSVODServe(s *HTTPServer, libSvc MediaStreamsGetter, libAccess LibraryAccessChecker, parental ParentalChecker) *hlsVODServe {
	if s == nil || s.HLSTranscoder == nil || s.HLSManager == nil {
		return nil
	}
	cache, err := apptranscoding.NewSegmentCache(s.HLSManager.CacheDir() + "/vod")
	if err != nil {
		slog.Warn("hls-vod: cache init failed, skipping v2 routes", slog.Any("err", err))
		return nil
	}
	resolver := &MediaSourceResolver{LibSvc: libSvc}
	orch := apptranscoding.NewSegmentOrchestrator(
		cache,
		s.HLSTranscoder,
		resolver,
		apptranscoding.OrchestratorOptions{},
		slog.Default(),
	)
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
		// Phase 1 keeps TS to match the ffmpeg adapter's default container path
		// (which only switches to fMP4 for hevc/av1/vp9 sources). Phase 2 will
		// thread a UseFMP4 flag into HLSProcessOpts and flip this to true.
		useFMP4: false,
		segmentWaitTimeout:   15 * time.Second,
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
