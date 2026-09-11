package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/pkg/procfork"

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
	// Count, not "list one row and read the total off it". The music and photo
	// list queries carry a COUNT(*) OVER() window, so asking them for a single
	// row still read and sorted the whole library - and this runs once per
	// library on a stats page that loops over all of them.
	case domainlibrary.Music:
		if c.music == nil {
			return 0, nil
		}
		return c.music.CountTracks(ctx, libraryID)
	case domainlibrary.Photo:
		if c.photo == nil {
			return 0, nil
		}
		return c.photo.CountPhotos(ctx, libraryID)
	default:
		if c.lib == nil {
			return 0, nil
		}
		lid := libraryID
		return c.lib.CountMediaItems(ctx, domainlibrary.ListMediaItemsFilter{LibraryID: &lid})
	}
}

func chaptersHandler(libSvc ChaptersGetter, baseURL string, libraryAccess LibraryAccessChecker, mediaGetter MediaItemGetter, parental ParentalChecker) http.HandlerFunc {
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
			slog.WarnContext(r.Context(), "json encode failed", slog.Any("err", err))
		}
	}
}

// LibraryItemCounter counts items per library, dispatching to the right backend
// (media_items for movie/tvshow, music_tracks for music, photos for photo).
type LibraryItemCounter interface {
	CountByLibrary(ctx context.Context, libraryID shared.LibraryID, libType string) (int, error)
}

func libraryStatsHandler(libSvc LibraryLister, counter LibraryItemCounter) http.HandlerFunc {
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
					slog.WarnContext(ctx, "library-stats count failed", slog.String("lib", string(lib.ID)), slog.Any("err", cerr))
				}
			}
			stats = append(stats, libStat{ID: string(lib.ID), LastScannedAt: ts, ItemCount: count})
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"stats": stats}); err != nil {
			slog.WarnContext(ctx, "json encode failed", slog.Any("err", err))
		}
	}
}

// --- playbackInfoHandler ---

type playbackServe struct {
	libSvc        MediaStreamsGetter
	libraryAccess LibraryAccessChecker
	profilePrefs  ProfilePrefsGetter
	parental      ParentalChecker
}

func playbackInfoHandler(libSvc MediaStreamsGetter, libraryAccess LibraryAccessChecker, profilePrefs ProfilePrefsGetter, parental ParentalChecker) http.HandlerFunc {
	h := &playbackServe{libSvc: libSvc, libraryAccess: libraryAccess, profilePrefs: profilePrefs, parental: parental}
	return h.serveHTTP
}

func (h *playbackServe) serveHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.transcode.PlaybackInfo")
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
	// Gate burn on the selected subtitle (if the client signalled one) rather than
	// on existence, so PlaybackInfo doesn't report "transcode" for a title that
	// merely carries a bitmap track the user isn't playing.
	prefs.SubtitleBurnInTrack = parseIntQuery(r, "subtitleTrack", -1)
	containerFormat := ""
	if item.File != nil {
		containerFormat = item.File.ContainerFormat
	}
	sourcePath := ""
	if item.File != nil {
		sourcePath = item.File.Path
	}
	decision := apptranscoding.MakePlaybackDecisionWithPrefs(r.Context(), streams, containerFormat, client, userMaxBitrate, prefs, sourcePath)
	if apptranscoding.TranscodeDisallowed(decision, prefs) {
		span.SetStatus(codes.Error, "transcoding disallowed by prefs")
		http.Error(w, `{"message":"transcoding disabled by user preferences"}`, http.StatusUnprocessableEntity)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(toPlaybackInfoDTO(decision)); err != nil {
		slog.WarnContext(r.Context(), "json encode failed", slog.Any("err", err))
	}
}

// playbackInfoDTO is the explicit HTTP contract for GET /media/{id}/playback-info.
// It mirrors the wire shape the endpoint has always sent but decouples it from
// the domain PlaybackDecision, so a refactor of the domain struct can't silently
// reshape the public response (the boundary is now owned by the adapter).
type playbackInfoDTO struct {
	PlayMethod       string   `json:"play_method"`
	VideoDecision    string   `json:"video_decision"`
	AudioDecision    string   `json:"audio_decision"`
	SubtitleDecision string   `json:"subtitle_decision"`
	TranscodeReasons []string `json:"transcode_reasons"`
	VideoCodec       string   `json:"video_codec,omitempty"`
	AudioCodec       string   `json:"audio_codec,omitempty"`
	Container        string   `json:"container,omitempty"`
	VideoBitrate     int64    `json:"video_bitrate,omitempty"`
	AudioBitrate     int64    `json:"audio_bitrate,omitempty"`
	MaxWidth         int      `json:"max_width,omitempty"`
	MaxHeight        int      `json:"max_height,omitempty"`
	MaxAudioChannels int      `json:"max_audio_channels,omitempty"`
}

func toPlaybackInfoDTO(d *domaintranscoding.PlaybackDecision) playbackInfoDTO {
	return playbackInfoDTO{
		PlayMethod:       d.PlayMethod,
		VideoDecision:    d.VideoDecision,
		AudioDecision:    d.AudioDecision,
		SubtitleDecision: d.SubtitleDecision,
		TranscodeReasons: d.TranscodeReasons,
		VideoCodec:       d.VideoCodec,
		AudioCodec:       d.AudioCodec,
		Container:        d.Container,
		VideoBitrate:     d.VideoBitrate,
		AudioBitrate:     d.AudioBitrate,
		MaxWidth:         d.MaxWidth,
		MaxHeight:        d.MaxHeight,
		MaxAudioChannels: d.MaxAudioChannels,
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
	if hdrQuerySupported(r) {
		client.SupportsHDR = true
	}
	return client
}

// --- transcodeHandler ---

type transcodeServe struct {
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
	clientSupportsHDR := hdrQuerySupported(r)
	client := parseClientCapabilities(r, clientSupportsHDR)
	analysis := h.analyzeStreams(r.Context(), mediaID, params.remuxOnly, client, params.subtitleTrack, params.audioTrack)
	params.audioTrack = clampAudioTrack(params.audioTrack, analysis.streams)

	slog.Debug("transcode: decision", slog.String("media_id", mediaID), slog.Bool("needsVideoTranscode", analysis.needsVideoTranscode), slog.Bool("needsAudioTranscode", analysis.needsAudioTranscode), slog.Bool("remuxOnly", params.remuxOnly), slog.Bool("needsToneMap", analysis.needsToneMap))

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	args := h.buildFFmpegArgs(item.File.Path, params, analysis)
	slog.Debug("transcode: ffmpeg", slog.String("media_id", mediaID), slog.String("args", strings.Join(args, " ")))
	// A viewer is waiting on this stream: default priority, production caps.
	cmd, finish, err := procfork.Command(ctx, procfork.Options{Class: procfork.Production, Site: "http.transcode", NoNice: true}, "ffmpeg", args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "transcode refused")
		slog.WarnContext(r.Context(), "transcode refused", slog.String("media_id", mediaID), slog.Any("err", err))
		http.Error(w, `{"message":"server busy, retry later"}`, http.StatusServiceUnavailable)
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "ffmpeg stdout pipe failed")
		slog.ErrorContext(r.Context(), "ffmpeg stdout pipe", slog.Any("err", err))
		http.Error(w, "transcode failed", http.StatusInternalServerError)
		return
	}
	if err := cmd.Start(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "ffmpeg start failed")
		slog.ErrorContext(r.Context(), "ffmpeg start", slog.Any("err", err))
		http.Error(w, "transcode failed", http.StatusInternalServerError)
		return
	}
	// Disable write deadline - transcode streams last minutes/hours, global WriteTimeout would cut them.
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
				slog.ErrorContext(ctx, "panic recovered in transcode ffmpeg cancel", "error", r, "stack", string(debug.Stack()))
			}
		}()
		<-r.Context().Done()
		cancel()
	}()
	copied, copyErr := io.Copy(w, stdout)
	if copyErr != nil && ctx.Err() == nil {
		slog.WarnContext(r.Context(), "transcode copy", slog.Any("err", copyErr))
	}
	if err := finish(cmd.Wait(), nil); err != nil && ctx.Err() == nil {
		slog.WarnContext(r.Context(), "ffmpeg transcode", slog.String("media_id", mediaID), slog.Any("err", err))
		// ffmpeg failed before emitting any bytes: the 200 status line hasn't
		// been committed yet (io.Copy wrote nothing), so we can still send a 500.
		// Otherwise the player gets a bare 200 with an empty body and never
		// retries.
		if copied == 0 {
			http.Error(w, "transcode failed", http.StatusInternalServerError)
		}
	}
}

// ProfileAudioPrefsGetter extends ProfilePrefsGetter with the profile-level
// audio language fields that live outside the JSONB preferences map.
// Implemented by ProfilePrefsAdapter when wired with a full profile getter.
type ProfileAudioPrefsGetter interface {
	GetProfileAudioPrefs(ctx context.Context, profileID string) (audioLanguages []string, locale string, err error)
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
	tp := apptranscoding.ParseTranscodingPreferences(raw)
	// If the getter also provides audio language fields (profile.audio_languages
	// and profile.locale), inject them so the handler can resolve the default
	// audio track without a second round-trip.
	if ag, ok := getter.(ProfileAudioPrefsGetter); ok {
		langs, locale, aerr := ag.GetProfileAudioPrefs(ctx, profileID)
		if aerr == nil {
			tp.AudioLanguages = langs
			tp.Locale = locale
		} else {
			slog.Debug("load profile audio prefs failed", slog.String("profile_id", profileID), slog.Any("err", aerr))
		}
	}
	return tp
}
