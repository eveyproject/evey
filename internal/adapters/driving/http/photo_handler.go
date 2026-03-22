package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	appphoto "github.com/gravity-zero/evey/internal/app/photo"
	domainphoto "github.com/gravity-zero/evey/internal/domain/photo"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/exifstrip"
)

// photoResponse is the API DTO for a photo, omitting GPS coordinates.
type photoResponse struct {
	ID          string     `json:"ID"`
	LibraryID   string     `json:"LibraryID"`
	Title       string     `json:"Title"`
	FilePath    string     `json:"FilePath"`
	FileSize    int64      `json:"FileSize"`
	MimeType    string     `json:"MimeType"`
	Width       int        `json:"Width"`
	Height      int        `json:"Height"`
	TakenAt     *time.Time `json:"TakenAt"`
	CameraModel string     `json:"CameraModel"`
	Album       string     `json:"Album"`
	CreatedAt   time.Time  `json:"CreatedAt"`
	UpdatedAt   time.Time  `json:"UpdatedAt"`
}

func toPhotoResponse(p domainphoto.Photo) photoResponse {
	return photoResponse{
		ID:          p.ID,
		LibraryID:   p.LibraryID,
		Title:       p.Title,
		FilePath:    p.FilePath,
		FileSize:    p.FileSize,
		MimeType:    p.MimeType,
		Width:       p.Width,
		Height:      p.Height,
		TakenAt:     p.TakenAt,
		CameraModel: p.CameraModel,
		Album:       p.Album,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

type photoHandler struct {
	photoSvc   *appphoto.Service
	tokenGen   *auth.JWTGenerator
	libChecker LibraryAccessChecker
	libGetter  libraryPathsGetter
	serverCtx  context.Context
}

func (h *photoHandler) extractClaims(r *http.Request) (*domainuser.Claims, bool) {
	hdr := r.Header.Get("Authorization")
	if len(hdr) < 8 || hdr[:7] != "Bearer " {
		return nil, false
	}
	claims, err := h.tokenGen.ValidateAccessToken(hdr[7:])
	if err != nil {
		return nil, false
	}
	return claims, true
}

func (h *photoHandler) getAlbums(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.extractClaims(r)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	libraryID := r.URL.Query().Get("library_id")
	if libraryID == "" {
		http.Error(w, `{"message":"library_id required"}`, http.StatusBadRequest)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, libraryID, h.libChecker) {
		return
	}
	albums, err := h.photoSvc.GetAlbums(r.Context(), libraryID)
	if err != nil {
		slog.Error("get albums", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"albums": albums}); err != nil {
		slog.Error("encode photo albums", slog.Any("err", err))
	}
}

func (h *photoHandler) searchPhotos(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.extractClaims(r)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	libraryID := r.URL.Query().Get("library_id")
	q := r.URL.Query().Get("q")
	if libraryID == "" || q == "" {
		http.Error(w, `{"message":"library_id and q required"}`, http.StatusBadRequest)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, libraryID, h.libChecker) {
		return
	}
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	photos, err := h.photoSvc.Search(r.Context(), libraryID, q, limit)
	if err != nil {
		slog.Error("search photos", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	dtos := make([]photoResponse, len(photos))
	for i, p := range photos {
		dtos[i] = toPhotoResponse(p)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"photos": dtos}); err != nil {
		slog.Error("encode photo search", slog.Any("err", err))
	}
}

func (h *photoHandler) servePhoto(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.extractClaims(r)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	photoID := r.PathValue("id")
	p, err := h.photoSvc.GetPhoto(r.Context(), photoID)
	if err != nil {
		slog.Error("serve photo get", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if p == nil {
		http.NotFound(w, r)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, p.LibraryID, h.libChecker) {
		return
	}
	w.Header().Set("Content-Type", p.MimeType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		slog.Warn("set write deadline", slog.Any("err", err))
	}
	if p.MimeType != "image/jpeg" {
		f, err := os.Open(p.FilePath)
		if err != nil {
			http.Error(w, `{"message":"file not found"}`, http.StatusNotFound)
			return
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil {
			http.Error(w, `{"message":"file not found"}`, http.StatusNotFound)
			return
		}
		http.ServeContent(w, r, filepath.Base(p.FilePath), fi.ModTime(), f)
		return
	}
	data, err := os.ReadFile(p.FilePath)
	if err != nil {
		http.Error(w, `{"message":"file not found"}`, http.StatusNotFound)
		return
	}
	if stripped := exifstrip.StripJPEGAppExif(data); stripped != nil {
		data = stripped
	}
	http.ServeContent(w, r, filepath.Base(p.FilePath), time.Time{}, bytes.NewReader(data))
}

func (h *photoHandler) getPhoto(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.extractClaims(r)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	p, err := h.photoSvc.GetPhoto(r.Context(), r.PathValue("id"))
	if err != nil {
		slog.Error("get photo", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if p == nil {
		http.NotFound(w, r)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, p.LibraryID, h.libChecker) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(toPhotoResponse(*p)); err != nil {
		slog.Error("encode photo", slog.Any("err", err))
	}
}

func (h *photoHandler) listPhotos(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.extractClaims(r)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	libraryID := r.URL.Query().Get("library_id")
	if libraryID == "" {
		http.Error(w, `{"message":"library_id required"}`, http.StatusBadRequest)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, libraryID, h.libChecker) {
		return
	}
	limit, offset := 50, 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}
	photos, total, err := h.photoSvc.ListPhotos(r.Context(), libraryID, limit, offset)
	if err != nil {
		slog.Error("list photos", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	dtos := make([]photoResponse, len(photos))
	for i, p := range photos {
		dtos[i] = toPhotoResponse(p)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"photos": dtos, "total": total}); err != nil {
		slog.Error("encode photo list", slog.Any("err", err))
	}
}

func (h *photoHandler) deletePhoto(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.extractClaims(r)
	if !ok {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	photoID := r.PathValue("id")
	p, err := h.photoSvc.GetPhoto(r.Context(), photoID)
	if err != nil {
		slog.Error("photo delete: get photo", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if p == nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if !claims.IsAdmin && !checkLibraryAccess(r.Context(), w, claims, p.LibraryID, h.libChecker) {
		return
	}
	if err := h.photoSvc.DeletePhoto(r.Context(), photoID); err != nil {
		slog.Error("photo delete", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *photoHandler) scanPhotos(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.extractClaims(r)
	if !ok || !claims.IsAdmin {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	var body struct {
		LibraryID string `json:"library_id"`
		Dir       string `json:"dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.LibraryID == "" || body.Dir == "" {
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
	go func() {
		n, err := h.photoSvc.ScanDirectory(h.serverCtx, body.LibraryID, body.Dir)
		if err != nil {
			slog.Error("photo scan", slog.Any("err", err))
		} else {
			slog.Info("photo scan complete", slog.Int("count", n), slog.String("dir", body.Dir))
		}
	}()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(map[string]string{"message": "scan started"}); err != nil {
		slog.Error("encode photo scan", slog.Any("err", err))
	}
}

func registerPhotoRoutes(mux *http.ServeMux, photoSvc *appphoto.Service, tokenGen *auth.JWTGenerator, libChecker LibraryAccessChecker, libGetter libraryPathsGetter, serverCtx context.Context) {
	h := &photoHandler{
		photoSvc:   photoSvc,
		tokenGen:   tokenGen,
		libChecker: libChecker,
		libGetter:  libGetter,
		serverCtx:  serverCtx,
	}
	mux.HandleFunc("GET /api/v1/photos/albums", h.getAlbums)
	mux.HandleFunc("GET /api/v1/photos/search", h.searchPhotos)
	mux.HandleFunc("GET /api/v1/photos/{id}/serve", h.servePhoto)
	mux.HandleFunc("GET /api/v1/photos/{id}", h.getPhoto)
	mux.HandleFunc("GET /api/v1/photos", h.listPhotos)
	mux.HandleFunc("DELETE /api/v1/photos/{id}", h.deletePhoto)
	mux.HandleFunc("POST /api/v1/photos/scan", h.scanPhotos)
}
