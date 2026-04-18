package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	applibrary "github.com/evey-project/evey/internal/app/library"
	appmusic "github.com/evey-project/evey/internal/app/music"
	appphoto "github.com/evey-project/evey/internal/app/photo"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// libraryItemCounter implements LibraryItemCounter by dispatching to the right
// service depending on library type (movie/tvshow -> media_items, music -> tracks, photo -> photos).
type libraryItemCounter struct {
	lib   *applibrary.QueryService
	music *appmusic.Service
	photo *appphoto.Service
}

// NewLibraryItemCounter wires the composite counter.
func NewLibraryItemCounter(lib *applibrary.QueryService, music *appmusic.Service, photo *appphoto.Service) LibraryItemCounter {
	return &libraryItemCounter{lib: lib, music: music, photo: photo}
}

func (c *libraryItemCounter) CountByLibrary(ctx context.Context, libraryID shared.LibraryID, libType string) (int, error) {
	switch domainlibrary.LibraryType(libType) {
	case domainlibrary.Music:
		if c.music == nil {
			return 0, nil
		}
		_, total, err := c.music.ListTracks(ctx, libraryID, 1, 0, "")
		return total, err
	case domainlibrary.Photo:
		if c.photo == nil {
			return 0, nil
		}
		_, total, err := c.photo.ListPhotos(ctx, libraryID, 1, 0)
		return total, err
	default:
		if c.lib == nil {
			return 0, nil
		}
		lid := libraryID
		return c.lib.CountMediaItems(ctx, domainlibrary.ListMediaItemsFilter{LibraryID: &lid})
	}
}

func chaptersHandler(tokenGen TokenVerifier, libSvc ChaptersGetter, baseURL string, libraryAccess LibraryAccessChecker, mediaGetter MediaItemGetter) http.HandlerFunc {
	type chapterJSON struct {
		Title       string `json:"title"`
		StartTimeMs int64  `json:"startTimeMs"`
		EndTimeMs   int64  `json:"endTimeMs"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		claims := HTTPClaimsFromContext(r.Context())

		if st, ok := StreamTokenFromContext(r.Context()); ok {
			if st.MediaItemID != shared.MediaItemID(mediaID) {
				http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
				return
			}
		}
		item, err := mediaGetter.GetMediaItem(r.Context(), mediaID)
		if err != nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			return
		}
		chapters, err := libSvc.GetChapters(r.Context(), mediaID)
		if err != nil {
			http.Error(w, `{"message":"internal"}`, http.StatusInternalServerError)
			return
		}
		out := make([]chapterJSON, 0, len(chapters))
		for _, ch := range chapters {
			out = append(out, chapterJSON{
				Title:       ch.Title,
				StartTimeMs: ch.StartTimeMs,
				EndTimeMs:   ch.EndTimeMs,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		setCORSHeaders(w, r, baseURL)
		if err := json.NewEncoder(w).Encode(map[string]any{"chapters": out}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}
}

// LibraryItemCounter counts items per library, dispatching to the right backend
// (media_items for movie/tvshow, music_tracks for music, photos for photo).
type LibraryItemCounter interface {
	CountByLibrary(ctx context.Context, libraryID shared.LibraryID, libType string) (int, error)
}

func libraryStatsHandler(tokenGen TokenVerifier, libSvc LibraryLister, counter LibraryItemCounter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		libs, err := libSvc.ListLibraries(r.Context())
		if err != nil {
			http.Error(w, `{"message":"internal"}`, http.StatusInternalServerError)
			return
		}
		type libStat struct {
			ID            string  `json:"id"`
			LastScannedAt *string `json:"lastScannedAt"`
			ItemCount     int     `json:"itemCount"`
		}
		stats := make([]libStat, 0, len(libs))
		for _, lib := range libs {
			var ts *string
			if lib.LastScannedAt != nil {
				s := lib.LastScannedAt.UTC().Format(time.RFC3339)
				ts = &s
			}
			count := 0
			if counter != nil {
				if n, cerr := counter.CountByLibrary(r.Context(), lib.ID, string(lib.Type)); cerr == nil {
					count = n
				} else {
					slog.Warn("library-stats count failed", slog.String("lib", string(lib.ID)), slog.Any("err", cerr))
				}
			}
			stats = append(stats, libStat{ID: string(lib.ID), LastScannedAt: ts, ItemCount: count})
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"stats": stats}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}
}

// --- playbackInfoHandler ---

type playbackServe struct {
	tokens        TokenVerifier
	libSvc        MediaStreamsGetter
	libraryAccess LibraryAccessChecker
	profilePrefs  ProfilePrefsGetter
}

func playbackInfoHandler(tokenGen TokenVerifier, libSvc MediaStreamsGetter, libraryAccess LibraryAccessChecker, profilePrefs ProfilePrefsGetter) http.HandlerFunc {
	h := &playbackServe{tokens: tokenGen, libSvc: libSvc, libraryAccess: libraryAccess, profilePrefs: profilePrefs}
	return h.serveHTTP
}

func (h *playbackServe) serveHTTP(w http.ResponseWriter, r *http.Request) {
	mediaID := r.PathValue("id")
	claims := HTTPClaimsFromContext(r.Context())

	item, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
	if err != nil || item.File == nil || item.File.Path == "" {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
		return
	}

	streams, err := h.libSvc.GetMediaStreams(r.Context(), mediaID)
	if err != nil {
		http.Error(w, `{"message":"internal"}`, http.StatusInternalServerError)
		return
	}

	client := h.parseClientProfile(r)

	var userMaxBitrate int
	if client.MaxBitrate > 0 {
		userMaxBitrate = int(client.MaxBitrate)
		client.MaxBitrate = 0
	}

	prefs := loadProfilePrefs(r.Context(), h.profilePrefs, claims.ProfileID)
	containerFormat := ""
	if item.File != nil {
		containerFormat = item.File.ContainerFormat
	}
	decision := apptranscoding.MakePlaybackDecisionWithPrefs(streams, containerFormat, client, userMaxBitrate, prefs)
	if apptranscoding.TranscodeDisallowed(decision, prefs) {
		http.Error(w, `{"message":"transcoding disabled by user preferences"}`, http.StatusUnprocessableEntity)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(decision); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *playbackServe) parseClientProfile(r *http.Request) domaintranscoding.ClientProfile {
	client := domaintranscoding.DefaultWebProfile
	if v := r.URL.Query().Get("maxWidth"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			client.MaxWidth = n
		}
	}
	if v := r.URL.Query().Get("maxHeight"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			client.MaxHeight = n
		}
	}
	if v := r.URL.Query().Get("maxBitrate"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			client.MaxBitrate = n
		}
	}
	if v := r.URL.Query().Get("videoCodecs"); v != "" {
		client.VideoCodecs = strings.Split(v, ",")
	}
	if v := r.URL.Query().Get("audioCodecs"); v != "" {
		client.AudioCodecs = strings.Split(v, ",")
	}
	if v := r.URL.Query().Get("containers"); v != "" {
		client.Containers = strings.Split(v, ",")
	}
	if r.URL.Query().Get("hdr") == "true" {
		client.SupportsHDR = true
	}
	return client
}

// --- hlsMasterHandler ---

type hlsMasterServe struct {
	tokens              TokenVerifier
	libSvc              MediaStreamsGetter
	hlsManager          *apptranscoding.HLSManager
	libraryAccess       LibraryAccessChecker
	userMaxStreams       UserMaxStreamsGetter
	audioNormalizeGetter func(ctx context.Context) bool
	bitrateGetter       UserBitrateGetter
	profilePrefs        ProfilePrefsGetter
}

func hlsMasterHandler(deps hlsMasterServe) http.HandlerFunc {
	h := deps
	return h.serveHTTP
}

func (h *hlsMasterServe) serveHTTP(w http.ResponseWriter, r *http.Request) {
	mediaID := r.PathValue("id")
	claims := HTTPClaimsFromContext(r.Context())
	userID := claims.UserID

	if st, ok := StreamTokenFromContext(r.Context()); ok {
		if st.MediaItemID != shared.MediaItemID(mediaID) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	item, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
	if err != nil || item.File == nil || item.File.Path == "" {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
		return
	}

	maxStreams := 0
	if h.userMaxStreams != nil {
		if ms := h.userMaxStreams.GetMaxStreams(r.Context(), userID); ms != nil {
			maxStreams = *ms
		}
	}

	sourceWidth, sourceHeight, mediaStreams := h.resolveSourceDimensions(r.Context(), mediaID)
	subtitleTrackIndex, subtitleCodec := h.resolveSubtitle(r, mediaStreams)

	audioTrackIndex := 0
	if at := r.URL.Query().Get("audioTrack"); at != "" {
		if n, parseErr := strconv.Atoi(at); parseErr == nil && n >= 0 {
			audioTrackIndex = n
		}
	}

	hdrSupported := r.URL.Query().Get("hdr") == "1"
	audioNormalize := h.audioNormalizeGetter != nil && h.audioNormalizeGetter(r.Context())

	// Compute playback decision to determine video/audio copy vs transcode
	client := parseClientCapabilities(r, hdrSupported)
	prefs := loadProfilePrefs(r.Context(), h.profilePrefs, claims.ProfileID)
	containerFormat := ""
	if item.File != nil {
		containerFormat = item.File.ContainerFormat
	}
	decision := apptranscoding.MakePlaybackDecisionWithPrefs(mediaStreams, containerFormat, client, 0, prefs)
	if apptranscoding.TranscodeDisallowed(decision, prefs) {
		http.Error(w, `{"message":"transcoding disabled by user preferences"}`, http.StatusUnprocessableEntity)
		return
	}
	videoDecision := decision.VideoDecision
	audioDecision := decision.AudioDecision

	var startSec float64
	if ss := r.URL.Query().Get("startSec"); ss != "" {
		if f, parseErr := strconv.ParseFloat(ss, 64); parseErr == nil && f > 0 {
			startSec = f
		}
	}

	// Wire user transcoding prefs from the decision (preferred codec /
	// audio channels / subtitle mode) into the HLS pipeline so the ffmpeg
	// adapter actually emits hevc / opus / -ac N / sidecar-subs when asked.
	overrides := apptranscoding.WithOverrides(apptranscoding.HLSOverrides{
		VideoCodec:       decision.VideoCodec,
		AudioCodec:       decision.AudioCodec,
		MaxAudioChannels: decision.MaxAudioChannels,
		ForceSidecarSubs: decision.SubtitleDecision == "external",
	})
	var session *apptranscoding.HLSSession
	if startSec > 0 {
		session, err = h.hlsManager.RestartFromPosition(userID, mediaID, item.File.Path, sourceWidth, sourceHeight, maxStreams, audioTrackIndex, subtitleTrackIndex, subtitleCodec, mediaStreams, hdrSupported, audioNormalize, startSec, videoDecision, audioDecision, overrides)
	} else {
		session, err = h.hlsManager.GetOrCreate(userID, mediaID, item.File.Path, sourceWidth, sourceHeight, maxStreams, audioTrackIndex, subtitleTrackIndex, subtitleCodec, mediaStreams, hdrSupported, audioNormalize, 0, videoDecision, audioDecision, overrides)
	}
	if err != nil {
		slog.Error("hls: get or create session", slog.Any("err", err))
		http.Error(w, `{"message":"transcode busy"}`, http.StatusServiceUnavailable)
		return
	}
	if err := session.WaitReady(r.Context()); err != nil {
		slog.Error("hls: wait ready", slog.Any("err", err))
		http.Error(w, `{"message":"transcode failed"}`, http.StatusInternalServerError)
		return
	}

	if session.StartSec > 0 {
		w.Header().Set("X-Start-Sec", fmt.Sprintf("%.3f", session.StartSec))
	}
	h.serveMaster(w, r, session.OutputDir, userID)
}

func (h *hlsMasterServe) serveMaster(w http.ResponseWriter, r *http.Request, outputDir, userID string) {
	setCORSHeaders(w, r, "")
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")

	masterPath := filepath.Join(outputDir, "master.m3u8")
	data, readErr := os.ReadFile(masterPath)
	if readErr != nil {
		slog.Error("hls: read master playlist", slog.Any("err", readErr))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}

	content := string(data)

	var maxBps int64
	if h.bitrateGetter != nil {
		if limitKbps, bitrateErr := h.bitrateGetter.GetUserBitrateLimit(r.Context(), userID); bitrateErr == nil && limitKbps > 0 {
			maxBps = int64(limitKbps) * 1000
		}
	}
	if maxBps > 0 {
		content = filterMasterPlaylist(content, maxBps)
	}

	// Inject auth token into segment URLs so native players (ExoPlayer, AVPlayer)
	// can fetch segments without needing custom header injection
	if tok := RawTokenFromContext(r.Context()); tok != "" {
		content = injectTokenIntoPlaylist(content, tok)
	}

	_, _ = w.Write([]byte(content))
}

// injectTokenIntoPlaylist appends ?token= to relative segment/playlist URIs
// in HLS playlists so native players can auth without custom headers.
// Handles URIs that already carry a query string, CRLF line endings, and
// both http:// and https:// absolute URLs.
func injectTokenIntoPlaylist(content, token string) string {
	var result strings.Builder
	for _, line := range strings.Split(content, "\n") {
		// Preserve any trailing \r so CRLF-strict clients (some TVs) are happy.
		var suffix string
		if strings.HasSuffix(line, "\r") {
			line = strings.TrimSuffix(line, "\r")
			suffix = "\r"
		}
		trimmed := strings.TrimSpace(line)
		// Rewrite only relative URIs (not comments, not absolute URLs).
		if trimmed != "" &&
			!strings.HasPrefix(trimmed, "#") &&
			!strings.HasPrefix(trimmed, "http://") &&
			!strings.HasPrefix(trimmed, "https://") {
			sep := "?"
			if strings.Contains(trimmed, "?") {
				sep = "&"
			}
			line = trimmed + sep + "token=" + token
		}
		result.WriteString(line)
		result.WriteString(suffix)
		result.WriteByte('\n')
	}
	return result.String()
}

func hlsSegmentHandler(tokenGen TokenVerifier, libSvc MediaItemGetter, hlsManager *apptranscoding.HLSManager, libraryAccess LibraryAccessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		file := r.PathValue("file")
		claims := HTTPClaimsFromContext(r.Context())
		sessionUserID := claims.UserID

		if st, ok := StreamTokenFromContext(r.Context()); ok {
			if st.MediaItemID != shared.MediaItemID(mediaID) {
				http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
				return
			}
		} else {
			item, err := libSvc.GetMediaItem(r.Context(), mediaID)
			if err != nil {
				http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
				return
			}
			if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
				return
			}
		}

		// Segments MUST be tied to a live session. The disk-only fallback was
		// a cross-user leak vector: after a server restart, segments cached by
		// User A (with forced audio/subtitle tracks reflecting A's preferences)
		// could be read by any User B with library access. Return 404 instead;
		// clients re-fetch master.m3u8 which re-creates the session with the
		// caller's own decisions.
		session, ok := hlsManager.GetSession(mediaID)
		if !ok {
			http.Error(w, `{"message":"session expired, refresh playlist"}`, http.StatusNotFound)
			return
		}
		if sessionUserID != string(session.UserID) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		if waitErr := session.WaitReady(r.Context()); waitErr != nil {
			http.Error(w, `{"message":"session not ready"}`, http.StatusServiceUnavailable)
			return
		}
		outputDir := session.OutputDir

		filePath, err := safeJoin(outputDir, filepath.Base(file))
		if err != nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if _, err := os.Stat(filePath); err != nil {
			http.Error(w, `{"message":"segment not found"}`, http.StatusNotFound)
			return
		}

		if s, ok := hlsManager.GetSession(mediaID); ok {
			s.MarkAccess()
		}

		setCORSHeaders(w, r, "")

		switch {
		case strings.HasSuffix(file, ".m3u8"):
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.Header().Set("Cache-Control", "no-cache")
			// Inject token into sub-playlist so native players can fetch .ts segments
			if tok := RawTokenFromContext(r.Context()); tok != "" {
				data, readErr := os.ReadFile(filePath)
				if readErr != nil {
					http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
					return
				}
				_, _ = w.Write([]byte(injectTokenIntoPlaylist(string(data), tok)))
				return
			}
			http.ServeFile(w, r, filePath)
		case strings.HasSuffix(file, ".ts"):
			w.Header().Set("Content-Type", "video/mp2t")
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, filePath)
		case strings.HasSuffix(file, ".m4s"), strings.HasSuffix(file, ".mp4"):
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, filePath)
		default:
			http.ServeFile(w, r, filePath)
		}
	}
}

// --- transcodeHandler ---

type transcodeServe struct {
	tokens        TokenVerifier
	libSvc        MediaStreamsGetter
	baseURL       string
	libraryAccess LibraryAccessChecker
	throttle      *apptranscoding.TranscodeThrottle
	userMaxStreams UserMaxStreamsGetter
}

func transcodeHandler(deps transcodeServe) http.HandlerFunc {
	h := deps
	return h.serveHTTP
}

func (h *transcodeServe) serveHTTP(w http.ResponseWriter, r *http.Request) {
	mediaID := r.PathValue("id")
	claims := HTTPClaimsFromContext(r.Context())
	userID := claims.UserID

	if st, ok := StreamTokenFromContext(r.Context()); ok {
		if st.MediaItemID != shared.MediaItemID(mediaID) {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
	}
	item, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
	if err != nil || item.File == nil || item.File.Path == "" {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
		return
	}
	maxStreams := 0
	if h.userMaxStreams != nil {
		if ms := h.userMaxStreams.GetMaxStreams(r.Context(), userID); ms != nil {
			maxStreams = *ms
		}
	}
	if !h.throttle.Acquire(userID, maxStreams) {
		http.Error(w, `{"message":"max concurrent streams reached"}`, http.StatusTooManyRequests)
		return
	}
	defer h.throttle.Release(userID)

	params := h.parseParams(r)
	clientSupportsHDR := r.URL.Query().Get("hdr") == "true"
	client := parseClientCapabilities(r, clientSupportsHDR)
	analysis := h.analyzeStreams(r.Context(), mediaID, params.remuxOnly, client, params.subtitleTrack)

	slog.Debug("transcode: decision", slog.String("media_id", mediaID), slog.Bool("needsVideoTranscode", analysis.needsVideoTranscode), slog.Bool("needsAudioTranscode", analysis.needsAudioTranscode), slog.Bool("remuxOnly", params.remuxOnly), slog.Bool("needsToneMap", analysis.needsToneMap))

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	args := h.buildFFmpegArgs(item.File.Path, params, analysis)
	slog.Debug("transcode: ffmpeg", slog.String("media_id", mediaID), slog.String("args", strings.Join(args, " ")))
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		slog.Error("ffmpeg stdout pipe", slog.Any("err", err))
		http.Error(w, "transcode failed", http.StatusInternalServerError)
		return
	}
	if err := cmd.Start(); err != nil {
		slog.Error("ffmpeg start", slog.Any("err", err))
		http.Error(w, "transcode failed", http.StatusInternalServerError)
		return
	}
	// Disable write deadline — transcode streams last minutes/hours, global WriteTimeout would cut them.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		slog.Debug("set write deadline unsupported", slog.Any("err", err))
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Accept-Ranges", "none")
	setCORSHeaders(w, r, h.baseURL)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered in transcode ffmpeg cancel", "error", r, "stack", debug.Stack())
			}
		}()
		<-r.Context().Done()
		cancel()
	}()
	if _, err := io.Copy(w, stdout); err != nil && ctx.Err() == nil {
		slog.Warn("transcode copy", slog.Any("err", err))
	}
	if err := cmd.Wait(); err != nil && ctx.Err() == nil {
		slog.Warn("ffmpeg transcode", slog.String("media_id", mediaID), slog.Any("err", err))
	}
}

// loadProfilePrefs fetches transcoding prefs for the profile and falls back to
// permissive defaults on any error or when no getter is configured.
func loadProfilePrefs(ctx context.Context, getter ProfilePrefsGetter, profileID string) apptranscoding.TranscodingPreferences {
	if getter == nil || profileID == "" {
		return apptranscoding.DefaultTranscodingPreferences()
	}
	raw, err := getter.GetProfilePreferences(ctx, profileID)
	if err != nil {
		slog.Debug("load profile prefs failed, using defaults", slog.String("profile_id", profileID), slog.Any("err", err))
		return apptranscoding.DefaultTranscodingPreferences()
	}
	return apptranscoding.ParseTranscodingPreferences(raw)
}

