package httpapi

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	appphoto "github.com/gravity-zero/evey/internal/app/photo"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/exifstrip"
)

type photoHandler struct {
	photoSvc   *appphoto.Service
	tokenGen   *auth.JWTGenerator
	libChecker LibraryAccessChecker
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

func registerPhotoRoutes(mux *http.ServeMux, photoSvc *appphoto.Service, tokenGen *auth.JWTGenerator, libChecker LibraryAccessChecker, _ libraryPathsGetter, _ context.Context) {
	h := &photoHandler{
		photoSvc:   photoSvc,
		tokenGen:   tokenGen,
		libChecker: libChecker,
	}
	mux.HandleFunc("GET /api/v1/photos/{id}/serve", h.servePhoto)
}
