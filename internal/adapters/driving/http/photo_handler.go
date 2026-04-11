package httpapi

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	appphoto "github.com/gravity-zero/evey/internal/app/photo"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/exifstrip"
)

type photoHandler struct {
	photoSvc   *appphoto.Service
	libChecker LibraryAccessChecker
}

func (h *photoHandler) servePhoto(w http.ResponseWriter, r *http.Request) {
	claims := HTTPClaimsFromContext(r.Context())
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
	if dlErr := http.NewResponseController(w).SetWriteDeadline(time.Time{}); dlErr != nil {
		slog.Warn("set write deadline", slog.Any("err", dlErr))
	}
	if p.MimeType != "image/jpeg" {
		f, openErr := os.Open(p.FilePath)
		if openErr != nil {
			http.Error(w, `{"message":"file not found"}`, http.StatusNotFound)
			return
		}
		defer f.Close()
		fi, statErr := f.Stat()
		if statErr != nil {
			http.Error(w, `{"message":"file not found"}`, http.StatusNotFound)
			return
		}
		http.ServeContent(w, r, filepath.Base(p.FilePath), fi.ModTime(), f)
		return
	}
	f2, err := os.Open(p.FilePath)
	if err != nil {
		http.Error(w, `{"message":"file not found"}`, http.StatusNotFound)
		return
	}
	defer f2.Close()
	const maxPhotoSize = 50 << 20 // 50 MB
	data, err := io.ReadAll(io.LimitReader(f2, maxPhotoSize))
	if err != nil {
		http.Error(w, `{"message":"read error"}`, http.StatusInternalServerError)
		return
	}
	if stripped := exifstrip.StripJPEGAppExif(data); stripped != nil {
		data = stripped
	}
	http.ServeContent(w, r, filepath.Base(p.FilePath), time.Time{}, bytes.NewReader(data))
}

func registerPhotoRoutes(mux *http.ServeMux, photoSvc *appphoto.Service, tokenGen *auth.JWTGenerator, libChecker LibraryAccessChecker, _ libraryPathsGetter, _ context.Context) {
	authMw := httpAuthMiddleware(tokenGen)
	h := &photoHandler{
		photoSvc:   photoSvc,
		libChecker: libChecker,
	}
	mux.HandleFunc("GET /api/v1/photos/{id}/serve", authMw(h.servePhoto))
}
