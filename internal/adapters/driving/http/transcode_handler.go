package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/codes"

	applibrary "github.com/evey-project/evey/internal/app/library"
	appmusic "github.com/evey-project/evey/internal/app/music"
	appphoto "github.com/evey-project/evey/internal/app/photo"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/tracing"
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

func chaptersHandler(tokenGen TokenVerifier, libSvc ChaptersGetter, baseURL string, libraryAccess LibraryAccessChecker, mediaGetter MediaItemGetter, parental ParentalChecker) http.HandlerFunc {
	type chapterJSON struct {
		Title       string `json:"title"`
		StartTimeMs int64  `json:"startTimeMs"`
		EndTimeMs   int64  `json:"endTimeMs"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.transcode.Chapters")
		defer span.End()
		r = r.WithContext(ctx)
		mediaID := r.PathValue("id")
		claims := HTTPClaimsFromContext(r.Context())

		if st, ok := StreamTokenFromContext(r.Context()); ok {
			if st.MediaItemID != shared.MediaItemID(mediaID) {
				span.SetStatus(codes.Error, "stream token media mismatch")
				http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
				return
			}
		}
		item, err := mediaGetter.GetMediaItem(r.Context(), mediaID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "media item not found")
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			span.SetStatus(codes.Error, "library access denied")
			return
		}
		if !checkParental(r.Context(), w, claims, item, parental) {
			span.SetStatus(codes.Error, "parental restricted")
			return
		}
		chapters, err := libSvc.GetChapters(r.Context(), mediaID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "get chapters failed")
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
		ctx, span := tracing.StartSpan(r.Context(), "http.transcode.LibraryStats")
		defer span.End()
		r = r.WithContext(ctx)
		libs, err := libSvc.ListLibraries(r.Context())
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "list libraries failed")
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
	parental      ParentalChecker
}

func playbackInfoHandler(tokenGen TokenVerifier, libSvc MediaStreamsGetter, libraryAccess LibraryAccessChecker, profilePrefs ProfilePrefsGetter, parental ParentalChecker) http.HandlerFunc {
	h := &playbackServe{tokens: tokenGen, libSvc: libSvc, libraryAccess: libraryAccess, profilePrefs: profilePrefs, parental: parental}
	return h.serveHTTP
}

func (h *playbackServe) serveHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.transcode.PlaybackInfo")
	defer span.End()
	r = r.WithContext(ctx)
	mediaID := r.PathValue("id")
	claims := HTTPClaimsFromContext(r.Context())

	item, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
	if err != nil || item.File == nil || item.File.Path == "" {
		if err != nil {
			span.RecordError(err)
		}
		span.SetStatus(codes.Error, "media item not found")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
		span.SetStatus(codes.Error, "library access denied")
		return
	}
	if !checkParental(r.Context(), w, claims, item, h.parental) {
		span.SetStatus(codes.Error, "parental restricted")
		return
	}

	streams, err := h.libSvc.GetMediaStreams(r.Context(), mediaID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get media streams failed")
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
	decision := apptranscoding.MakePlaybackDecisionWithPrefs(r.Context(), streams, containerFormat, client, userMaxBitrate, prefs)
	if apptranscoding.TranscodeDisallowed(decision, prefs) {
		span.SetStatus(codes.Error, "transcoding disallowed by prefs")
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


// --- transcodeHandler ---

type transcodeServe struct {
	tokens         TokenVerifier
	libSvc         MediaStreamsGetter
	baseURL        string
	libraryAccess  LibraryAccessChecker
	throttle       *apptranscoding.TranscodeThrottle
	userMaxStreams UserMaxStreamsGetter
	parental       ParentalChecker
}

func transcodeHandler(deps transcodeServe) http.HandlerFunc {
	h := deps
	return h.serveHTTP
}

func (h *transcodeServe) serveHTTP(w http.ResponseWriter, r *http.Request) {
	tctx, span := tracing.StartSpan(r.Context(), "http.transcode.Stream")
	defer span.End()
	r = r.WithContext(tctx)
	mediaID := r.PathValue("id")
	claims := HTTPClaimsFromContext(r.Context())
	userID := claims.UserID

	if st, ok := StreamTokenFromContext(r.Context()); ok {
		if st.MediaItemID != shared.MediaItemID(mediaID) {
			span.SetStatus(codes.Error, "stream token media mismatch")
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
	}
	item, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
	if err != nil || item.File == nil || item.File.Path == "" {
		if err != nil {
			span.RecordError(err)
		}
		span.SetStatus(codes.Error, "media item not found")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
		span.SetStatus(codes.Error, "library access denied")
		return
	}
	if !checkParental(r.Context(), w, claims, item, h.parental) {
		span.SetStatus(codes.Error, "parental restricted")
		return
	}
	maxStreams := 0
	if h.userMaxStreams != nil {
		if ms := h.userMaxStreams.GetMaxStreams(r.Context(), userID); ms != nil {
			maxStreams = *ms
		}
	}
	if !h.throttle.Acquire(userID, maxStreams) {
		span.SetStatus(codes.Error, "max concurrent streams reached")
		http.Error(w, `{"message":"max concurrent streams reached"}`, http.StatusTooManyRequests)
		return
	}
	defer h.throttle.Release(userID)

	params := h.parseParams(r)
	clientSupportsHDR := r.URL.Query().Get("hdr") == "true"
	client := parseClientCapabilities(r, clientSupportsHDR)
	analysis := h.analyzeStreams(r.Context(), mediaID, params.remuxOnly, client, params.subtitleTrack)
	params.audioTrack = clampAudioTrack(params.audioTrack, analysis.streams)

	slog.Debug("transcode: decision", slog.String("media_id", mediaID), slog.Bool("needsVideoTranscode", analysis.needsVideoTranscode), slog.Bool("needsAudioTranscode", analysis.needsAudioTranscode), slog.Bool("remuxOnly", params.remuxOnly), slog.Bool("needsToneMap", analysis.needsToneMap))

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	args := h.buildFFmpegArgs(item.File.Path, params, analysis)
	slog.Debug("transcode: ffmpeg", slog.String("media_id", mediaID), slog.String("args", strings.Join(args, " ")))
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "ffmpeg stdout pipe failed")
		slog.Error("ffmpeg stdout pipe", slog.Any("err", err))
		http.Error(w, "transcode failed", http.StatusInternalServerError)
		return
	}
	if err := cmd.Start(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "ffmpeg start failed")
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
				slog.Error("panic recovered in transcode ffmpeg cancel", "error", r, "stack", string(debug.Stack()))
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
