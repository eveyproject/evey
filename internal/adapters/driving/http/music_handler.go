package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	appmusic "github.com/gravity-zero/evey/internal/app/music"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

type musicHandler struct {
	svc        *appmusic.Service
	tokenGen   *auth.JWTGenerator
	libChecker LibraryAccessChecker
	libGetter  libraryPathsGetter
}

func registerMusicRoutes(mux *http.ServeMux, svc *appmusic.Service, tokenGen *auth.JWTGenerator, libChecker LibraryAccessChecker, libGetter libraryPathsGetter) {
	h := &musicHandler{svc: svc, tokenGen: tokenGen, libChecker: libChecker, libGetter: libGetter}
	mux.HandleFunc("GET /api/v1/music/tracks", h.listTracks)
	mux.HandleFunc("GET /api/v1/music/tracks/{id}/art", h.getTrackArt)
	mux.HandleFunc("GET /api/v1/music/tracks/{id}/stream", h.streamTrack)
	mux.HandleFunc("GET /api/v1/music/albums", h.listAlbums)
	mux.HandleFunc("GET /api/v1/music/artists", h.listArtists)
	mux.HandleFunc("GET /api/v1/music/search", h.searchTracks)
	mux.HandleFunc("DELETE /api/v1/music/tracks/{id}", h.deleteTrack)
	mux.HandleFunc("POST /api/v1/music/scan", requireAdmin(tokenGen, h.scanDirectory))
}

func (h *musicHandler) authenticateJSON(r *http.Request, w http.ResponseWriter) (*domainuser.Claims, bool) {
	token := extractBearerToken(r)
	if token == "" {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return nil, false
	}
	claims, err := h.tokenGen.ValidateAccessToken(token)
	if err != nil {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return nil, false
	}
	return claims, true
}

func (h *musicHandler) authenticatePlain(r *http.Request, w http.ResponseWriter) (*domainuser.Claims, bool) {
	token := extractBearerToken(r)
	if token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil, false
	}
	claims, err := h.tokenGen.ValidateAccessToken(token)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil, false
	}
	return claims, true
}

func (h *musicHandler) listTracks(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.authenticateJSON(r, w)
	if !ok {
		return
	}
	libraryID := r.URL.Query().Get("library_id")
	if !checkLibraryAccess(r.Context(), w, claims, libraryID, h.libChecker) {
		return
	}
	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, parseErr := strconv.Atoi(l); parseErr == nil && n > 0 {
			limit = n
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if n, parseErr := strconv.Atoi(o); parseErr == nil && n >= 0 {
			offset = n
		}
	}
	search := r.URL.Query().Get("search")
	tracks, total, err := h.svc.ListTracks(r.Context(), libraryID, limit, offset, search)
	if err != nil {
		slog.Error("music list tracks", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"tracks": tracks,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	}); err != nil {
		slog.Error("encode music tracks", slog.Any("err", err))
	}
}

func (h *musicHandler) getTrackArt(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.authenticatePlain(r, w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	track, err := h.svc.GetTrack(r.Context(), id)
	if err != nil || track == nil {
		http.NotFound(w, r)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, track.LibraryID, h.libChecker) {
		return
	}
	data, mimeType, err := h.svc.GetTrackArt(r.Context(), id)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		slog.Error("music get art", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if _, err := w.Write(data); err != nil {
		slog.Error("music art write", slog.Any("err", err))
	}
}

func (h *musicHandler) streamTrack(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.authenticatePlain(r, w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	track, err := h.svc.GetTrack(r.Context(), id)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		slog.Error("music get track", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if track == nil {
		http.NotFound(w, r)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, track.LibraryID, h.libChecker) {
		return
	}
	filePath, mimeType, err := h.svc.StreamTrack(r.Context(), id)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		slog.Error("music stream", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	f, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	if mimeType != "" {
		w.Header().Set("Content-Type", mimeType)
	}
	// Disable write deadline for streaming — global WriteTimeout would cut large files.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		slog.Warn("set write deadline", slog.Any("err", err))
	}
	http.ServeContent(w, r, filePath, time.Time{}, f)
}

func (h *musicHandler) listAlbums(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.authenticateJSON(r, w)
	if !ok {
		return
	}
	libraryID := r.URL.Query().Get("library_id")
	if !checkLibraryAccess(r.Context(), w, claims, libraryID, h.libChecker) {
		return
	}
	albums, err := h.svc.GetAlbums(r.Context(), libraryID)
	if err != nil {
		slog.Error("music list albums", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"albums": albums}); err != nil {
		slog.Error("encode music albums", slog.Any("err", err))
	}
}

func (h *musicHandler) listArtists(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.authenticateJSON(r, w)
	if !ok {
		return
	}
	libraryID := r.URL.Query().Get("library_id")
	if !checkLibraryAccess(r.Context(), w, claims, libraryID, h.libChecker) {
		return
	}
	artists, err := h.svc.GetArtists(r.Context(), libraryID)
	if err != nil {
		slog.Error("music list artists", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"artists": artists}); err != nil {
		slog.Error("encode music artists", slog.Any("err", err))
	}
}

func (h *musicHandler) searchTracks(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.authenticateJSON(r, w)
	if !ok {
		return
	}
	libraryID := r.URL.Query().Get("library_id")
	if !checkLibraryAccess(r.Context(), w, claims, libraryID, h.libChecker) {
		return
	}
	q := r.URL.Query().Get("q")
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, parseErr := strconv.Atoi(l); parseErr == nil && n > 0 {
			limit = n
		}
	}
	tracks, err := h.svc.Search(r.Context(), q, limit)
	if err != nil {
		slog.Error("music search", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"tracks": tracks}); err != nil {
		slog.Error("encode music search", slog.Any("err", err))
	}
}

func (h *musicHandler) deleteTrack(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.authenticateJSON(r, w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	track, err := h.svc.GetTrack(r.Context(), id)
	if err != nil {
		slog.Error("music delete: get track", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if track == nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if !claims.IsAdmin && !checkLibraryAccess(r.Context(), w, claims, track.LibraryID, h.libChecker) {
		return
	}
	if err := h.svc.DeleteTrack(r.Context(), id); err != nil {
		slog.Error("music delete track", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *musicHandler) scanDirectory(w http.ResponseWriter, r *http.Request) {
	claims, err := h.tokenGen.ValidateAccessToken(extractBearerToken(r))
	if err != nil {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	var body struct {
		LibraryID string `json:"library_id"`
		Dir       string `json:"dir"`
	}
	if decErr := json.NewDecoder(r.Body).Decode(&body); decErr != nil {
		http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
		return
	}
	if body.LibraryID == "" || body.Dir == "" {
		http.Error(w, `{"message":"library_id and dir required"}`, http.StatusBadRequest)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, body.LibraryID, h.libChecker) {
		return
	}
	body.Dir = filepath.Clean(body.Dir)
	if !filepath.IsAbs(body.Dir) || strings.Contains(body.Dir, "..") {
		http.Error(w, `{"message":"invalid dir"}`, http.StatusBadRequest)
		return
	}
	lib, err := h.libGetter.GetLibrary(r.Context(), body.LibraryID)
	if err != nil || lib == nil {
		http.Error(w, `{"message":"library not found"}`, http.StatusNotFound)
		return
	}
	allowed := false
	for _, p := range lib.Paths {
		if isWithinDir(p, body.Dir) || filepath.Clean(body.Dir) == filepath.Clean(p) {
			allowed = true
			break
		}
	}
	if !allowed {
		http.Error(w, `{"message":"dir not within library paths"}`, http.StatusBadRequest)
		return
	}
	count, err := h.svc.ScanDirectory(r.Context(), body.LibraryID, body.Dir)
	if err != nil {
		slog.Error("music scan", slog.Any("err", err))
		http.Error(w, `{"message":"scan error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"scanned": count}); err != nil {
		slog.Error("encode music scan", slog.Any("err", err))
	}
}
