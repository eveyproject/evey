package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
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
		// Chapters are the library's metadata: a block-ordered file plays
		// through the packager and keeps them.
		chapters, err := libSvc.GetChapters(r.Context(), mediaID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "get chapters failed")
			logInternal(r.Context(), "chapters: get", err)
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
			logInternal(r.Context(), "transcode: list libraries", err)
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
	decider       apptranscoding.PlaybackDecider
	libSvc        MediaStreamsGetter
	libraryAccess LibraryAccessChecker
	profilePrefs  ProfilePrefsGetter
	parental      ParentalChecker
}

func playbackInfoHandler(decider apptranscoding.PlaybackDecider, libSvc MediaStreamsGetter, libraryAccess LibraryAccessChecker, profilePrefs ProfilePrefsGetter, parental ParentalChecker) http.HandlerFunc {
	if decider == nil {
		decider = apptranscoding.UnwiredDecider()
	}
	h := &playbackServe{decider: decider, libSvc: libSvc, libraryAccess: libraryAccess, profilePrefs: profilePrefs, parental: parental}
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
	if !checkPlayable(w, item) {
		span.SetStatus(codes.Error, "file needs remux")
		return
	}

	streams, err := h.libSvc.GetMediaStreams(r.Context(), mediaID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get media streams failed")
		logInternal(r.Context(), "transcode: get media streams", err)
		http.Error(w, `{"message":"internal"}`, http.StatusInternalServerError)
		return
	}

	client := parseClientCapabilities(r, hdrQuerySupported(r))

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
	// The plan's decision (Decide: the core plus the request-time pins), the
	// same one the master this info announces will be served from.
	decided := h.decider.Decide(r.Context(), apptranscoding.PlaybackRequest{
		MediaID:             mediaID,
		SourcePath:          item.File.Path,
		Container:           item.File.ContainerFormat,
		Streams:             streams,
		Client:              client,
		Prefs:               prefs,
		MaxBitrateKbps:      userMaxBitrate,
		AudioTrack:          prefs.AudioTrack,
		SubtitleBurnInTrack: prefs.SubtitleBurnInTrack,
		WasmAudio:           prefs.WasmAudio,
		SourceBytes:         sourceBytesOf(item),
		SourceBitrateKbps:   item.File.BitrateBps() / 1000,
		SourceNeedsRemux:    item.File.NeedsRemux(),
	})
	decision := &decided
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
