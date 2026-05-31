package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/config"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// AudioSegmentEnsurer is the minimal interface the audio handlers need from
// AudioSegmentOrchestrator. Using an interface allows tests to inject a fake
// without spinning up a real ffmpeg process.
type AudioSegmentEnsurer interface {
	EnsureAudioSegment(ctx context.Context, fp apptranscoding.AudioFingerprint, segIdx int) error
}

// audioServe groups the shared dependencies for the three audio-rendition
// HTTP handlers (playlist / init / segment). Constructed once and shared.
type audioServe struct {
	cfg          *config.Config
	libSvc       MediaStreamsGetter
	libraryAccess LibraryAccessChecker
	parental     ParentalChecker
	manifestGen  *apptranscoding.ManifestGenerator
	cache        *apptranscoding.SegmentCache
	audioOrch    AudioSegmentEnsurer

	// audioNormalizeGetter mirrors the video pipeline knob.
	audioNormalizeGetter func(ctx context.Context) bool

	// segmentWaitTimeout caps how long audioSegmentHandler blocks waiting for
	// an audio segment to materialise on disk.
	segmentWaitTimeout time.Duration
}

// audioRenditionEnabled reports whether the audio-rendition feature flag is on.
// When off, all three handlers return 404 immediately — the master manifest
// never advertises EXT-X-MEDIA entries in that case, so well-behaved players
// won't hit these paths.  Fail-closed for misbehaving or probing clients.
func (h *audioServe) audioRenditionEnabled() bool {
	return h.cfg != nil && h.cfg.Transcoding.AudioRenditionEnabled
}

// audioPlaylistHandler renders the audio rendition playlist for source track T.
// Same auth / parental guards as the video variant playlist handler. Returns
// 404 when the feature flag is off.
func audioPlaylistHandler(h *audioServe) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.audioRenditionEnabled() {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}

		mediaID := r.PathValue("id")
		trackIdxStr := r.PathValue("trackIdx")
		trackIdx, err := strconv.Atoi(trackIdxStr)
		if err != nil || trackIdx < 0 {
			http.Error(w, `{"message":"bad track index"}`, http.StatusBadRequest)
			return
		}

		claims := HTTPClaimsFromContext(r.Context())
		item, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil || item == nil || item.File == nil || item.File.Path == "" {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
			return
		}
		if !checkParental(r.Context(), w, claims, item, h.parental) {
			return
		}

		streams, err := h.libSvc.GetMediaStreams(r.Context(), mediaID)
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}

		if item.File.Duration.Milliseconds() <= 0 {
			w.Header().Set("Retry-After", "5")
			http.Error(w, `{"message":"media duration not yet probed, retry"}`, http.StatusServiceUnavailable)
			return
		}

		audioRenditions := buildAudioRenditions(streams)
		if len(audioRenditions) == 0 {
			http.Error(w, `{"message":"no audio renditions"}`, http.StatusNotFound)
			return
		}

		token := RawTokenFromContext(r.Context())
		in := apptranscoding.ManifestInput{
			MediaID:             mediaID,
			DurationSec:         float64(item.File.Duration.Milliseconds()) / 1000.0,
			// Profiles must be non-empty for ManifestInput.Validate().
			// AudioPlaylist doesn't use the profiles list but Validate() requires
			// at least one entry. We inject a placeholder that matches what a
			// real call would produce; the manifest output only reads segments.
			Profiles:            placeholderProfiles(),
			StreamToken:         token,
			AudioTrack:          parseIntQuery(r, "audioTrack", 0),
			SubtitleBurnInTrack: parseIntQuery(r, "subtitleTrack", -1),
			VideoCodecs:         splitCSVQuery(r, "videoCodecs"),
			AudioCodecs:         splitCSVQuery(r, "audioCodecs"),
			Containers:          splitCSVQuery(r, "containers"),
			UseFMP4:             true,
			AudioRenditions:     audioRenditions,
		}

		out, err := h.manifestGen.AudioPlaylist(in, trackIdx)
		if err != nil {
			http.Error(w, `{"message":"manifest error"}`, http.StatusInternalServerError)
			return
		}

		setCORSHeaders(w, r, "")
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(out))

		slog.Info("audio rendition: playlist served",
			slog.String("media_id", mediaID),
			slog.Int("track_idx", trackIdx))
	}
}

// audioInitHandler serves the fMP4 init segment for audio track T.
// It triggers EnsureAudioSegment(ctx, fp, 0) so the ffmpeg session is live,
// then serves the init file via http.ServeFile. Returns 404 when flag is off.
func audioInitHandler(h *audioServe) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.audioRenditionEnabled() {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}

		mediaID := r.PathValue("id")
		trackIdxStr := r.PathValue("trackIdx")
		trackIdx, err := strconv.Atoi(trackIdxStr)
		if err != nil || trackIdx < 0 {
			http.Error(w, `{"message":"bad track index"}`, http.StatusBadRequest)
			return
		}

		claims := HTTPClaimsFromContext(r.Context())
		item, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil || item == nil || item.File == nil || item.File.Path == "" {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
			return
		}
		if !checkParental(r.Context(), w, claims, item, h.parental) {
			return
		}

		fp := h.buildAudioFingerprint(r, mediaID, item, trackIdx)

		// Init segment is produced alongside segment 0 by the fMP4 muxer.
		// Trigger EnsureAudioSegment(0) so the session is spawned.
		waitCtx, cancel := context.WithTimeout(r.Context(), h.segmentWaitTimeout)
		defer cancel()
		if err := h.audioOrch.EnsureAudioSegment(waitCtx, fp, 0); err != nil {
			slog.Warn("audio rendition: ensure init failed",
				slog.String("media_id", mediaID),
				slog.Int("track_idx", trackIdx),
				slog.Any("err", err))
			http.Error(w, `{"message":"audio transcode unavailable"}`, http.StatusServiceUnavailable)
			return
		}

		key := apptranscoding.SegmentKey{
			DecisionHash: fp.Hash(),
			VariantIdx:   trackIdx,
			IsInit:       true,
			Kind:         apptranscoding.KindAudio,
		}
		path := h.cache.Path(key, true)
		if _, statErr := os.Stat(path); statErr != nil {
			http.Error(w, `{"message":"audio init segment not yet ready, retry"}`, http.StatusServiceUnavailable)
			return
		}

		setCORSHeaders(w, r, "")
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, path)
	}
}

// audioSegmentHandler serves a single audio segment audio_{T}_{NNN}.m4s.
// Returns 404 when the feature flag is off.
func audioSegmentHandler(h *audioServe) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.audioRenditionEnabled() {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}

		mediaID := r.PathValue("id")
		trackIdxStr := r.PathValue("trackIdx")
		segIdxStr := r.PathValue("segIdx")

		trackIdx, err := strconv.Atoi(trackIdxStr)
		if err != nil || trackIdx < 0 {
			http.Error(w, `{"message":"bad track index"}`, http.StatusBadRequest)
			return
		}
		segIdx, err := strconv.Atoi(segIdxStr)
		if err != nil || segIdx < 0 {
			http.Error(w, `{"message":"bad segment index"}`, http.StatusBadRequest)
			return
		}

		claims := HTTPClaimsFromContext(r.Context())
		item, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil || item == nil || item.File == nil || item.File.Path == "" {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
			return
		}
		if !checkParental(r.Context(), w, claims, item, h.parental) {
			return
		}

		fp := h.buildAudioFingerprint(r, mediaID, item, trackIdx)

		waitCtx, cancel := context.WithTimeout(r.Context(), h.segmentWaitTimeout)
		defer cancel()
		if err := h.audioOrch.EnsureAudioSegment(waitCtx, fp, segIdx); err != nil {
			slog.Warn("audio rendition: ensure segment failed",
				slog.String("media_id", mediaID),
				slog.Int("track_idx", trackIdx),
				slog.Int("seg_idx", segIdx),
				slog.Any("err", err))
			http.Error(w, `{"message":"audio transcode unavailable"}`, http.StatusServiceUnavailable)
			return
		}

		key := apptranscoding.SegmentKey{
			DecisionHash: fp.Hash(),
			VariantIdx:   trackIdx,
			SegmentIdx:   segIdx,
			Kind:         apptranscoding.KindAudio,
		}
		path := h.cache.Path(key, true)
		if _, statErr := os.Stat(path); statErr != nil {
			http.Error(w, `{"message":"audio segment not yet ready, retry"}`, http.StatusServiceUnavailable)
			return
		}

		setCORSHeaders(w, r, "")
		w.Header().Set("Content-Type", "video/iso.segment")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, path)
	}
}

// buildAudioFingerprint constructs the AudioFingerprint for an HTTP request.
// AudioCodec and AudioBitrate are hardcoded to "aac" / 128 kbps for the MVP —
// the decision pipeline doesn't yet expose a per-rendition codec choice on the
// audio path. A full passthrough / transcode decision mirroring the video
// pipeline is tracked as a TODO below.
//
// TODO(audio-rendition): wire MakePlaybackDecisionWithPrefs result for the
// audio-only path so passthrough is chosen when the source codec is AC-3 /
// E-AC-3 and the client advertises support.  For now AAC 128 kbps is safe for
// all clients (hls.js / Safari / mobile).
func (h *audioServe) buildAudioFingerprint(r *http.Request, mediaID string, item *domainlibrary.MediaItem, trackIdx int) apptranscoding.AudioFingerprint {
	return apptranscoding.AudioFingerprint{
		MediaID:          mediaID,
		SourcePath:       item.File.Path,
		SourceTrackIdx:   trackIdx,
		AudioCodec:       "aac",
		AudioBitrate:     128,
		AudioNormalize:   h.audioNormalizeGetter != nil && h.audioNormalizeGetter(r.Context()),
		MaxAudioChannels: 0, // 0 = passthrough channel count
	}
}

// buildAudioRenditions converts the raw media streams into the
// AudioRendition slice the ManifestGenerator expects. Only audio streams are
// included; the SourceTrackIdx is the index among audio streams (not the
// overall ffmpeg stream index).
func buildAudioRenditions(streams []*domainlibrary.MediaStream) []apptranscoding.AudioRendition {
	var out []apptranscoding.AudioRendition
	audioIdx := 0
	for _, s := range streams {
		if s == nil || string(s.StreamType) != "audio" {
			continue
		}
		name := s.Title
		if name == "" && s.Language != "" {
			name = s.Language
		}
		if name == "" {
			name = ""
		}
		out = append(out, apptranscoding.AudioRendition{
			SourceTrackIdx: audioIdx,
			Name:           name,
			LanguageCode:   s.Language,
		})
		audioIdx++
	}
	return out
}

// placeholderProfiles returns a minimal single-profile slice that satisfies
// ManifestInput.Validate() (which requires len(Profiles) > 0). AudioPlaylist
// does not use the profile data in its output.
func placeholderProfiles() []domaintranscoding.HLSProfile {
	return []domaintranscoding.HLSProfile{
		{Name: "placeholder", Width: 1280, Height: 720, VideoBitrate: 2000, AudioBitrate: 128},
	}
}
