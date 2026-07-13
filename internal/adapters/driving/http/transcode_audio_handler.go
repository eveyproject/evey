package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
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
	EnsureAudioInit(ctx context.Context, fp apptranscoding.AudioFingerprint) error
}

// audioServe groups the shared dependencies for the three audio-rendition
// HTTP handlers (playlist / init / segment). Constructed once and shared.
type audioServe struct {
	cfg           *config.Config
	libSvc        MediaStreamsGetter
	libraryAccess LibraryAccessChecker
	parental      ParentalChecker
	manifestGen   *apptranscoding.ManifestGenerator
	cache         *apptranscoding.SegmentCache
	audioOrch     AudioSegmentEnsurer

	// audioNormalizeGetter mirrors the video pipeline knob.
	audioNormalizeGetter func(ctx context.Context) bool

	// segmentWaitTimeout caps how long serveSegmentFile blocks waiting for
	// an audio segment to materialise on disk.
	segmentWaitTimeout time.Duration
}

// audioRenditionEnabled reports whether the audio-rendition feature flag is on.
// When off, the serve paths (playlist/init/segment) return 404 immediately -
// the master manifest never advertises EXT-X-MEDIA entries in that case, so
// well-behaved players won't hit them. Fail-closed for misbehaving or probing
// clients.
func (h *audioServe) audioRenditionEnabled() bool {
	return h.cfg != nil && h.cfg.Transcoding.AudioRenditionEnabled
}

// servePlaylistFile is the auth-and-render path for the audio playlist,
// called from the central hlsVODServe dispatcher (regex-routed via
// /hls/{file}) - the on-the-wire URL the manifest emits resolves relative to
// /api/v1/media/{id}/hls/master.m3u8, so the dispatcher is the actual
// production code path.
func (h *audioServe) servePlaylistFile(w http.ResponseWriter, r *http.Request, mediaID string, trackIdx int) {
	if !h.audioRenditionEnabled() {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
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
		MediaID:     mediaID,
		DurationSec: float64(item.File.Duration.Milliseconds()) / 1000.0,
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
		// Same re-bake rule as buildManifestInput: hdr is a decision input.
		HDRCapable: r.URL.Query().Get("hdr") == "1",
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

func (h *audioServe) serveInitFile(w http.ResponseWriter, r *http.Request, mediaID string, trackIdx int) {
	if !h.audioRenditionEnabled() {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
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

	// The init describes the codec, not a position: it is served off whatever
	// session is live. Asking for segment 0 here (as this once did) makes every
	// deep resume mute - the session a resume spawns starts at the viewer's
	// position and never writes segment 0.
	waitCtx, cancel := context.WithTimeout(r.Context(), h.segmentWaitTimeout)
	defer cancel()
	if err := h.audioOrch.EnsureAudioInit(waitCtx, fp); err != nil {
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

func (h *audioServe) serveSegmentFile(w http.ResponseWriter, r *http.Request, mediaID string, trackIdx, segIdx int) {
	if !h.audioRenditionEnabled() {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
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

// buildAudioFingerprint constructs the AudioFingerprint for an HTTP request.
// AudioCodec is "aac" (safe on every client) and AudioBitrate carries the
// STEREO BASE - BuildFFmpegAudioOnlyArgs scales it to the channel count with the
// same rule as the muxed ladder (~64 kbps/channel beyond stereo, 5.1 -> 384k).
//
// MaxAudioChannels comes from the requested SOURCE track: ffmpeg reads 0 as
// `-ac 2`, so leaving it unset would downmix a 5.1 AC-3 track to stereo. The
// muxed path this rung replaces preserves the layout, and the hybrid rung is
// meant to serve the source's audio, not a quietly degraded one. Both fields are
// hashed, so a channel change forks the cache instead of aliasing.
//
// TODO(audio-rendition): wire MakePlaybackDecisionWithPrefs result for the
// audio-only path so passthrough is chosen when the source codec is AC-3 /
// E-AC-3 and the client advertises support.
func (h *audioServe) buildAudioFingerprint(r *http.Request, mediaID string, item *domainlibrary.MediaItem, trackIdx int) apptranscoding.AudioFingerprint {
	return apptranscoding.AudioFingerprint{
		MediaID:          mediaID,
		SourcePath:       item.File.Path,
		SourceTrackIdx:   trackIdx,
		AudioCodec:       "aac",
		AudioBitrate:     domaintranscoding.AudioRenditionBaseKbps, // scaled to the channel count at arg-build
		AudioNormalize:   h.audioNormalizeGetter != nil && h.audioNormalizeGetter(r.Context()),
		MaxAudioChannels: h.sourceAudioChannels(r.Context(), mediaID, trackIdx),
	}
}

// maxRenditionChannels is the highest channel count a browser will decode as AAC
// (5.1). A 7.1 source is folded down to it rather than shipped un-decodable.
const maxRenditionChannels = 6

// sourceAudioChannels returns the channel count of the trackIdx-th AUDIO stream
// (the `ffmpeg -map 0:a:N` numbering buildAudioRenditions advertises). Returns 0
// - ffmpeg's stereo default, the previous behaviour - when the streams can't be
// read or the source never reported a channel count, so an unknown layout is
// never guessed.
func (h *audioServe) sourceAudioChannels(ctx context.Context, mediaID string, trackIdx int) int {
	if h.libSvc == nil {
		return 0
	}
	streams, err := h.libSvc.GetMediaStreams(ctx, mediaID)
	if err != nil {
		slog.Warn("audio rendition: stream lookup failed, defaulting to stereo",
			slog.String("media_id", mediaID), slog.Any("err", err))
		return 0
	}
	audioIdx := 0
	for _, s := range streams {
		if s == nil || s.StreamType != domainlibrary.Audio {
			continue
		}
		if audioIdx == trackIdx {
			if s.Channels == nil || *s.Channels <= 0 {
				return 0
			}
			// Cap at 5.1: no browser decodes AAC 7.1 in MSE (Chrome tops out at
			// 5.1). Emitting `-ac 8` for a Dolby Atmos 7.1 source made the
			// SourceBuffer refuse the stream and the title never started - the
			// opposite failure to the stereo downmix this derivation replaced.
			if *s.Channels > maxRenditionChannels {
				return maxRenditionChannels
			}
			return *s.Channels
		}
		audioIdx++
	}
	return 0
}

// buildAudioRenditions converts the raw media streams into the
// AudioRendition slice the ManifestGenerator expects. Only audio streams are
// included; the SourceTrackIdx is the index AMONG AUDIO streams (not the
// overall ffmpeg stream index) - this matches `ffmpeg -map 0:a:N` semantics
// so the audio orchestrator hands ffmpeg the right specifier. Name falls
// back to the language code (upper-cased) when the source track has no
// title; empty Language is kept empty so the master playlist omits the
// LANGUAGE attribute entirely (some strict players reject LANGUAGE="").
//
// The source-flagged default track (IsDefault) is bubbled to index 0 so
// EXT-X-MEDIA emits DEFAULT=YES/AUTOSELECT=YES on it. Falls back to source
// order when no track is flagged default.
func buildAudioRenditions(streams []*domainlibrary.MediaStream) []apptranscoding.AudioRendition {
	var out []apptranscoding.AudioRendition
	audioIdx := 0
	defaultIdx := -1
	for _, s := range streams {
		if s == nil || s.StreamType != domainlibrary.Audio {
			continue
		}
		name := s.Title
		if name == "" {
			name = strings.ToUpper(s.Language)
		}
		out = append(out, apptranscoding.AudioRendition{
			SourceTrackIdx: audioIdx,
			Name:           name,
			LanguageCode:   s.Language,
		})
		if defaultIdx < 0 && s.IsDefault {
			defaultIdx = len(out) - 1
		}
		audioIdx++
	}
	if defaultIdx > 0 {
		out[0], out[defaultIdx] = out[defaultIdx], out[0]
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
