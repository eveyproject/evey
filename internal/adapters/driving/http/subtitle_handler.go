package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	mkvsubtitle "github.com/gravity-zero/mkvgo/mkv/subtitle"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	appsubtitle "github.com/evey-project/evey/internal/app/subtitle"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/domain/shared"
	domainsubtitle "github.com/evey-project/evey/internal/domain/subtitle"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// subtitleService is the subset of appsubtitle.Service used by HTTP routes.
type subtitleService interface {
	Search(ctx context.Context, mediaItemID, language string) ([]domainsubtitle.SearchResult, error)
	Download(ctx context.Context, mediaItemID string, fileID int, language, format string) (*domainsubtitle.SubtitleDownload, error)
	List(ctx context.Context, mediaItemID string) ([]*domainsubtitle.SubtitleDownload, error)
	Delete(ctx context.Context, subtitleID string) error
}

func subtitleHandler(tokenGen TokenVerifier, libSvc MediaStreamsGetter, baseURL string, libraryAccess LibraryAccessChecker, parental ParentalChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.subtitle.Serve")
		defer span.End()
		r = r.WithContext(ctx)
		mediaID := r.PathValue("id")
		trackStr := r.PathValue("track")
		span.SetAttributes(
			attribute.String("media_item_id", mediaID),
			attribute.String("track", trackStr),
		)
		claims := HTTPClaimsFromContext(r.Context())

		if st, ok := StreamTokenFromContext(r.Context()); ok {
			if st.MediaItemID != shared.MediaItemID(mediaID) {
				span.SetStatus(otelcodes.Error, "stream token media mismatch")
				http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
				return
			}
		}
		trackIndex, err := strconv.Atoi(trackStr)
		if err != nil || trackIndex < 0 {
			span.SetStatus(otelcodes.Error, "invalid track index")
			http.Error(w, `{"message":"invalid track index"}`, http.StatusBadRequest)
			return
		}
		item, err := libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil || item.File == nil || item.File.Path == "" {
			if err != nil {
				span.RecordError(err)
			}
			span.SetStatus(otelcodes.Error, "media item not found")
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			span.SetStatus(otelcodes.Error, "library access denied")
			return
		}
		if !checkParental(r.Context(), w, claims, item, parental) {
			span.SetStatus(otelcodes.Error, "parental restricted")
			return
		}
		filePath := item.File.Path

		streams, serr := libSvc.GetMediaStreams(r.Context(), mediaID)
		if serr != nil {
			// Fail closed (and visibly): without the stream list we can't tell a
			// valid text track from a forged ?track= index or a bitmap subtitle.
			// Silently falling through used to spawn ffmpeg on the unvalidated
			// index, emitting a broken/empty stream with no error signal.
			slog.Warn("subtitle: get media streams", slog.String("media_id", mediaID), slog.Any("err", serr))
			span.SetStatus(otelcodes.Error, "media streams unavailable")
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		subIdx := 0
		matched := false
		imageSub := false
		for _, s := range streams {
			if s.StreamType != "subtitle" {
				continue
			}
			if subIdx == trackIndex {
				if s.IsExternal && s.ExternalPath != nil {
					serveExternalSubtitle(w, r, *s.ExternalPath, filepath.Dir(filePath), baseURL)
					return
				}
				matched = true
				imageSub = apptranscoding.IsImageSubtitle(s.Codec)
				break
			}
			subIdx++
		}
		// We successfully enumerated streams, so the requested track must be a
		// real embedded subtitle. An out-of-range ?track= would otherwise become
		// `-map 0:s:99` and fail the ffmpeg spawn per request (forged deep index).
		if !matched {
			span.SetStatus(otelcodes.Error, "subtitle track out of range")
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		// A bitmap subtitle (PGS/DVDSub/DVB/XSUB) can't be muxed to text WebVTT:
		// `-f webvtt` produces an empty/failed stream. Bitmap subs are delivered
		// by burn-in, not this text endpoint, so reject rather than spawn ffmpeg.
		if imageSub {
			span.SetStatus(otelcodes.Error, "image subtitle not text-convertible")
			http.Error(w, `{"message":"subtitle not available as text"}`, http.StatusUnprocessableEntity)
			return
		}

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		cmd := exec.CommandContext(ctx, "ffmpeg",
			"-i", filePath,
			"-map", fmt.Sprintf("0:s:%d", trackIndex),
			"-f", "webvtt",
			"-",
		)
		w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		setCORSHeaders(w, r, baseURL)
		cmd.Stdout = w
		if err := cmd.Start(); err != nil {
			slog.Error("ffmpeg subtitle start", slog.Any("err", err))
			return
		}
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic recovered in subtitle ffmpeg cancel", "error", r, "stack", string(debug.Stack()))
				}
			}()
			<-r.Context().Done()
			cancel()
		}()
		if err := cmd.Wait(); err != nil && ctx.Err() == nil {
			slog.Warn("ffmpeg subtitle", slog.String("media_id", mediaID), slog.Any("err", err))
		}
	}
}

func serveExternalSubtitle(w http.ResponseWriter, r *http.Request, path, baseDir, baseURL string) {
	safePath, err := safeJoin(baseDir, filepath.Base(path))
	if err != nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	ext := strings.ToLower(filepath.Ext(safePath))
	setCORSHeaders(w, r, baseURL)
	w.Header().Set("Cache-Control", "no-cache")
	if ext == ".vtt" {
		w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
		http.ServeFile(w, r, safePath)
		return
	}
	// .srt, .ass and .ssa convert in-process via mkvgo — no ffmpeg fork per
	// request. Buffer the result first so a parse error can still fall back to
	// ffmpeg without having already written a partial body.
	if ext == ".srt" || ext == ".ass" || ext == ".ssa" {
		var buf bytes.Buffer
		mkvErr := mkvsubtitle.FileToWebVTT(safePath, &buf)
		if mkvErr == nil {
			w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
			_, _ = io.Copy(w, &buf)
			return
		}
		slog.Warn("mkvgo subtitle->webvtt failed, falling back to ffmpeg",
			slog.String("path", safePath), slog.String("ext", ext), slog.Any("err", mkvErr))
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-i", safePath, "-f", "webvtt", "-")
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	cmd.Stdout = w
	if err := cmd.Start(); err != nil {
		slog.Error("ffmpeg external subtitle", slog.Any("err", err))
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered in external subtitle ffmpeg cancel", "error", r, "stack", string(debug.Stack()))
			}
		}()
		<-r.Context().Done()
		cancel()
	}()
	if err := cmd.Wait(); err != nil && ctx.Err() == nil {
		slog.Warn("ffmpeg external subtitle convert", slog.String("path", safePath), slog.Any("err", err))
	}
}

// --- registerSubtitleRoutes ---

type subtitleRoutes struct {
	subtitleSvc   subtitleService
	tokens        TokenVerifier
	mediaGetter   MediaItemGetter
	libraryAccess LibraryAccessChecker
}

func registerSubtitleRoutes(mux *http.ServeMux, subtitleSvc *appsubtitle.Service, tokenGen TokenVerifier, mediaGetter MediaItemGetter, libraryAccess LibraryAccessChecker) {
	h := &subtitleRoutes{subtitleSvc: subtitleSvc, tokens: tokenGen, mediaGetter: mediaGetter, libraryAccess: libraryAccess}
	mux.HandleFunc("GET /api/v1/media/{id}/subtitles/search", h.search)
	mux.HandleFunc("POST /api/v1/media/{id}/subtitles/download", h.download)
	mux.HandleFunc("GET /api/v1/media/{id}/subtitles", h.list)
	mux.HandleFunc("DELETE /api/v1/media/{id}/subtitles/{subtitleId}", h.delete)
}

func (h *subtitleRoutes) requireAuth(w http.ResponseWriter, r *http.Request) (*domainuser.Claims, bool) {
	tok := r.Header.Get("Authorization")
	if strings.HasPrefix(tok, "Bearer ") {
		tok = tok[7:]
	} else if t := r.URL.Query().Get("token"); t != "" {
		tok = t
	} else {
		tok = ""
	}
	if tok == "" {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return nil, false
	}
	claims, err := h.tokens.ValidateAccessToken(tok)
	if err != nil {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return nil, false
	}
	return claims, true
}

func (h *subtitleRoutes) requireMediaAccess(w http.ResponseWriter, r *http.Request, mediaID string) (*domainuser.Claims, bool) {
	claims, ok := h.requireAuth(w, r)
	if !ok {
		return nil, false
	}
	item, err := h.mediaGetter.GetMediaItem(r.Context(), mediaID)
	if err != nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return nil, false
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
		return nil, false
	}
	return claims, true
}

func (h *subtitleRoutes) search(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.subtitle.Search")
	defer span.End()
	r = r.WithContext(ctx)
	mediaID := r.PathValue("id")
	span.SetAttributes(attribute.String("media_item_id", mediaID))
	if _, ok := h.requireMediaAccess(w, r, mediaID); !ok {
		span.SetStatus(otelcodes.Error, "media access denied")
		return
	}
	language := r.URL.Query().Get("language")
	span.SetAttributes(attribute.String("language", language))
	results, err := h.subtitleSvc.Search(r.Context(), mediaID, language)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "search failed")
		slog.Error("subtitle search", slog.Any("err", err))
		http.Error(w, `{"message":"search failed"}`, http.StatusInternalServerError)
		return
	}
	type resultJSON struct {
		FileID   int     `json:"fileId"`
		FileName string  `json:"fileName"`
		Language string  `json:"language"`
		Format   string  `json:"format"`
		Rating   float64 `json:"rating"`
	}
	out := make([]resultJSON, 0, len(results))
	for _, res := range results {
		out = append(out, resultJSON{
			FileID:   res.FileID,
			FileName: res.FileName,
			Language: res.Language,
			Format:   res.Format,
			Rating:   res.Rating,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"results": out}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *subtitleRoutes) download(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.subtitle.Download")
	defer span.End()
	r = r.WithContext(ctx)
	mediaID := r.PathValue("id")
	span.SetAttributes(attribute.String("media_item_id", mediaID))
	claims, ok := h.requireMediaAccess(w, r, mediaID)
	if !ok {
		span.SetStatus(otelcodes.Error, "media access denied")
		return
	}
	var body struct {
		FileID   int    `json:"fileId"`
		Language string `json:"language"`
		Format   string `json:"format"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB cap on JSON body
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "bad request")
		http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
		return
	}
	if body.FileID == 0 {
		span.SetStatus(otelcodes.Error, "fileId required")
		http.Error(w, `{"message":"fileId required"}`, http.StatusBadRequest)
		return
	}
	sub, err := h.subtitleSvc.Download(r.Context(), mediaID, body.FileID, body.Language, body.Format)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "download failed")
		slog.Error("subtitle download", slog.Any("err", err))
		http.Error(w, `{"message":"download failed"}`, http.StatusInternalServerError)
		return
	}
	out := map[string]any{
		"id":           sub.ID,
		"mediaItemId":  sub.MediaItemID,
		"language":     sub.Language,
		"format":       sub.Format,
		"source":       sub.Source,
		"downloadedAt": sub.DownloadedAt.UTC().Format(time.RFC3339),
	}
	// filePath is a server-side filesystem path; only admins may see it.
	if claims != nil && claims.IsAdmin {
		out["filePath"] = sub.FilePath
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(out); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *subtitleRoutes) list(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.subtitle.List")
	defer span.End()
	r = r.WithContext(ctx)
	mediaID := r.PathValue("id")
	span.SetAttributes(attribute.String("media_item_id", mediaID))
	claims, ok := h.requireMediaAccess(w, r, mediaID)
	if !ok {
		span.SetStatus(otelcodes.Error, "media access denied")
		return
	}
	subs, err := h.subtitleSvc.List(r.Context(), mediaID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "list failed")
		slog.Error("subtitle list", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	type subJSON struct {
		ID           string `json:"id"`
		MediaItemID  string `json:"mediaItemId"`
		Language     string `json:"language"`
		Format       string `json:"format"`
		FilePath     string `json:"filePath,omitempty"`
		Source       string `json:"source"`
		DownloadedAt string `json:"downloadedAt"`
	}
	// filePath is a server-side filesystem path; only expose it to admins.
	isAdmin := claims != nil && claims.IsAdmin
	out := make([]subJSON, 0, len(subs))
	for _, s := range subs {
		j := subJSON{
			ID:           s.ID,
			MediaItemID:  string(s.MediaItemID),
			Language:     s.Language,
			Format:       s.Format,
			Source:       s.Source,
			DownloadedAt: s.DownloadedAt.UTC().Format(time.RFC3339),
		}
		if isAdmin {
			j.FilePath = s.FilePath
		}
		out = append(out, j)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"subtitles": out}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *subtitleRoutes) delete(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.subtitle.Delete")
	defer span.End()
	r = r.WithContext(ctx)
	mediaID := r.PathValue("id")
	subtitleID := r.PathValue("subtitleId")
	span.SetAttributes(
		attribute.String("media_item_id", mediaID),
		attribute.String("subtitle_id", subtitleID),
	)
	if _, ok := h.requireMediaAccess(w, r, mediaID); !ok {
		span.SetStatus(otelcodes.Error, "media access denied")
		return
	}
	subs, err := h.subtitleSvc.List(r.Context(), mediaID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "list failed")
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	found := false
	for _, s := range subs {
		if s.ID == subtitleID {
			found = true
			break
		}
	}
	if !found {
		span.SetStatus(otelcodes.Error, "subtitle not found")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if err := h.subtitleSvc.Delete(r.Context(), subtitleID); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "delete failed")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
