package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	appsubtitle "github.com/evey-project/evey/internal/app/subtitle"
	"github.com/evey-project/evey/internal/domain/shared"
	domainsubtitle "github.com/evey-project/evey/internal/domain/subtitle"
	domainuser "github.com/evey-project/evey/internal/domain/user"
)

// subtitleService is the subset of appsubtitle.Service used by HTTP routes.
type subtitleService interface {
	Search(ctx context.Context, mediaItemID, language string) ([]domainsubtitle.SearchResult, error)
	Download(ctx context.Context, mediaItemID string, fileID int, language, format string) (*domainsubtitle.SubtitleDownload, error)
	List(ctx context.Context, mediaItemID string) ([]*domainsubtitle.SubtitleDownload, error)
	Delete(ctx context.Context, subtitleID string) error
}

func subtitleHandler(tokenGen TokenVerifier, libSvc MediaStreamsGetter, baseURL string, libraryAccess LibraryAccessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		trackStr := r.PathValue("track")
		claims := HTTPClaimsFromContext(r.Context())

		if st, ok := StreamTokenFromContext(r.Context()); ok {
			if st.MediaItemID != shared.MediaItemID(mediaID) {
				http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
				return
			}
		}
		trackIndex, _ := strconv.Atoi(trackStr)
		item, err := libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil || item.File == nil || item.File.Path == "" {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			return
		}
		filePath := item.File.Path

		if streams, serr := libSvc.GetMediaStreams(r.Context(), mediaID); serr == nil {
			subIdx := 0
			for _, s := range streams {
				if s.StreamType != "subtitle" {
					continue
				}
				if subIdx == trackIndex && s.IsExternal && s.ExternalPath != nil {
					serveExternalSubtitle(w, r, *s.ExternalPath, filepath.Dir(filePath), baseURL)
					return
				}
				subIdx++
			}
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
					slog.Error("panic recovered in subtitle ffmpeg cancel", "error", r, "stack", debug.Stack())
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
				slog.Error("panic recovered in external subtitle ffmpeg cancel", "error", r, "stack", debug.Stack())
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
	mediaID := r.PathValue("id")
	if _, ok := h.requireMediaAccess(w, r, mediaID); !ok {
		return
	}
	language := r.URL.Query().Get("language")
	results, err := h.subtitleSvc.Search(r.Context(), mediaID, language)
	if err != nil {
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
	mediaID := r.PathValue("id")
	if _, ok := h.requireMediaAccess(w, r, mediaID); !ok {
		return
	}
	var body struct {
		FileID   int    `json:"fileId"`
		Language string `json:"language"`
		Format   string `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
		return
	}
	if body.FileID == 0 {
		http.Error(w, `{"message":"fileId required"}`, http.StatusBadRequest)
		return
	}
	sub, err := h.subtitleSvc.Download(r.Context(), mediaID, body.FileID, body.Language, body.Format)
	if err != nil {
		slog.Error("subtitle download", slog.Any("err", err))
		http.Error(w, `{"message":"download failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"id":           sub.ID,
		"mediaItemId":  sub.MediaItemID,
		"language":     sub.Language,
		"format":       sub.Format,
		"filePath":     sub.FilePath,
		"source":       sub.Source,
		"downloadedAt": sub.DownloadedAt.UTC().Format(time.RFC3339),
	}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *subtitleRoutes) list(w http.ResponseWriter, r *http.Request) {
	mediaID := r.PathValue("id")
	if _, ok := h.requireMediaAccess(w, r, mediaID); !ok {
		return
	}
	subs, err := h.subtitleSvc.List(r.Context(), mediaID)
	if err != nil {
		slog.Error("subtitle list", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	type subJSON struct {
		ID           string `json:"id"`
		MediaItemID  string `json:"mediaItemId"`
		Language     string `json:"language"`
		Format       string `json:"format"`
		FilePath     string `json:"filePath"`
		Source       string `json:"source"`
		DownloadedAt string `json:"downloadedAt"`
	}
	out := make([]subJSON, 0, len(subs))
	for _, s := range subs {
		out = append(out, subJSON{
			ID:           s.ID,
			MediaItemID:  string(s.MediaItemID),
			Language:     s.Language,
			Format:       s.Format,
			FilePath:     s.FilePath,
			Source:       s.Source,
			DownloadedAt: s.DownloadedAt.UTC().Format(time.RFC3339),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"subtitles": out}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *subtitleRoutes) delete(w http.ResponseWriter, r *http.Request) {
	mediaID := r.PathValue("id")
	subtitleID := r.PathValue("subtitleId")
	if _, ok := h.requireMediaAccess(w, r, mediaID); !ok {
		return
	}
	subs, err := h.subtitleSvc.List(r.Context(), mediaID)
	if err != nil {
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
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if err := h.subtitleSvc.Delete(r.Context(), subtitleID); err != nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
