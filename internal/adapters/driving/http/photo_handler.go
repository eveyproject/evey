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

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	appphoto "github.com/evey-project/evey/internal/app/photo"
	domainphoto "github.com/evey-project/evey/internal/domain/photo"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/exifstrip"
	"github.com/evey-project/evey/internal/pkg/tracing"
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
	ctx, span := tracing.StartSpan(r.Context(), "http.photo.Serve")
	defer span.End()
	r = r.WithContext(ctx)
	claims := HTTPClaimsFromContext(r.Context())
	photoID := r.PathValue("id")
	span.SetAttributes(attribute.String("photo_id", photoID))
	p, err := h.photoSvc.GetPhoto(r.Context(), photoID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "get photo failed")
		slog.Error("serve photo get", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if p == nil {
		span.SetStatus(otelcodes.Error, "photo not found")
		http.NotFound(w, r)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, p.LibraryID, h.libChecker) {
		span.SetStatus(otelcodes.Error, "library access denied")
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
	var ext, contentType string
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
