package httpapi

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	appphoto "github.com/evey-project/evey/internal/app/photo"
	domainphoto "github.com/evey-project/evey/internal/domain/photo"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/exifstrip"
)

// PhotoTranscoder resizes/reformats photos into the cache dir.
type PhotoTranscoder interface {
	Transcode(ctx context.Context, srcPath, dstPath string, width int, format string) error
}

type photoHandler struct {
	photoSvc     *appphoto.Service
	libChecker   LibraryAccessChecker
	transcoder   PhotoTranscoder
	cacheDir     string
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
	if !checkLibraryAccess(r.Context(), w, claims, shared.LibraryID(p.LibraryID), h.libChecker) {
		return
	}
	if dlErr := http.NewResponseController(w).SetWriteDeadline(time.Time{}); dlErr != nil {
		slog.Warn("set write deadline", slog.Any("err", dlErr))
	}

	// Transcode path: only if w or fmt query params provided AND transcoder configured.
	wStr := r.URL.Query().Get("w")
	fmtStr := r.URL.Query().Get("fmt")
	if (wStr != "" || fmtStr != "") && h.transcoder != nil && h.cacheDir != "" {
		if h.serveTranscoded(w, r, p, wStr, fmtStr) {
			return
		}
		// fallthrough to default on failure
	}

	w.Header().Set("Content-Type", p.MimeType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
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

// serveTranscoded returns true if it successfully served a transcoded variant.
func (h *photoHandler) serveTranscoded(w http.ResponseWriter, r *http.Request, p *domainphoto.Photo, wStr, fmtStr string) bool {
	width := 0
	if wStr != "" {
		v, err := strconv.Atoi(wStr)
		if err != nil || v <= 0 || v > 8192 {
			return false
		}
		width = v
	}
	ext := ""
	contentType := ""
	switch fmtStr {
	case "webp":
		ext = "webp"
		contentType = "image/webp"
	case "":
		// Keep source-ish. Default to jpeg output for simplicity.
		ext = "jpg"
		contentType = "image/jpeg"
	default:
		return false
	}

	photoDir := filepath.Join(h.cacheDir, "photos")
	if err := os.MkdirAll(photoDir, 0o755); err != nil {
		slog.Warn("photo cache mkdir", slog.Any("err", err))
		return false
	}
	dstName := p.ID + "-w" + strconv.Itoa(width) + "-" + ext + "." + ext
	dstPath := filepath.Join(photoDir, dstName)

	if fi, err := os.Stat(dstPath); err == nil && fi.Size() > 0 {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFile(w, r, dstPath)
		return true
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := h.transcoder.Transcode(ctx, p.FilePath, dstPath, width, fmtStr); err != nil {
		slog.Warn("photo transcode", slog.String("id", p.ID), slog.Any("err", err))
		_ = os.Remove(dstPath)
		return false
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, dstPath)
	return true
}

func registerPhotoRoutes(mux *http.ServeMux, photoSvc *appphoto.Service, tokenGen *auth.JWTGenerator, libChecker LibraryAccessChecker, _ libraryPathsGetter, _ context.Context, transcoder PhotoTranscoder, cacheDir string) {
	authMw := httpAuthMiddleware(tokenGen)
	h := &photoHandler{
		photoSvc:   photoSvc,
		libChecker: libChecker,
		transcoder: transcoder,
		cacheDir:   cacheDir,
	}
	mux.HandleFunc("GET /api/v1/photos/{id}/serve", authMw(h.servePhoto))
}
