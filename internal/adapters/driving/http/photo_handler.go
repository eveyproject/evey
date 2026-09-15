package httpapi

import (
	"bytes"
	"context"
	domainuser "github.com/evey-project/evey/internal/domain/user"
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
	"github.com/evey-project/evey/internal/pkg/exifstrip"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// photoAllowedWidths caps photo variant generation to a fixed set so the
// on-disk cache stays bounded, mirroring artwork_handler.go's allowedWidths
// (see its comment for the regression this pattern guards against: a
// caller-chosen width that isn't snapped writes one file per distinct value
// requested - here that meant up to 8192 variants for a single photo, since
// nothing capped how many distinct widths a user could ask for). 400 is
// included because it's the only width the web and mobile clients actually
// request for photo thumbnails today; the rest give room for higher-density
// variants without letting a caller pick an arbitrary width.
var photoAllowedWidths = []int{200, 400, 800, 1600}

// snapPhotoWidth parses raw (the ?w= query value) and snaps it to the
// nearest entry in photoAllowedWidths. ok is false when raw is empty,
// unparseable, or <= 0 - identical to the pre-fix contract, which treated
// those cases as "no width requested" rather than an error.
func snapPhotoWidth(raw string) (width int, ok bool) {
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 0, false
	}
	best := photoAllowedWidths[0]
	bestDiff := v - best
	if bestDiff < 0 {
		bestDiff = -bestDiff
	}
	for _, allowed := range photoAllowedWidths[1:] {
		diff := v - allowed
		if diff < 0 {
			diff = -diff
		}
		if diff < bestDiff {
			bestDiff = diff
			best = allowed
		}
	}
	return best, true
}

// PhotoTranscoder resizes/reformats photos into the cache dir.
type PhotoTranscoder interface {
	Transcode(ctx context.Context, srcPath, dstPath string, width int, format string) error
}

type photoHandler struct {
	photoSvc   *appphoto.Service
	libChecker LibraryAccessChecker
	transcoder PhotoTranscoder
	cacheDir   string
	libLister  LibraryLister // may be nil; used for path confinement (fail-open)
	// Charged only when a request actually forks ffmpeg (cache miss); nil in
	// tests means pass-through.
	transcodeRL *ratelimit.Limiter
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
		slog.ErrorContext(r.Context(), "serve photo get", slog.Any("err", err))
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
		slog.WarnContext(r.Context(), "set write deadline", slog.Any("err", dlErr))
	}
	// Defense-in-depth: a poisoned photos.file_path must not become an arbitrary-file
	// read. Mirror media/remux handlers: fail-open when roots are unknown.
	if h.libLister != nil && !pathWithinRoots(mediaRoots(r.Context(), h.libLister), p.FilePath) {
		slog.WarnContext(r.Context(), "photo serve: file path outside configured library roots",
			slog.String("photo_id", photoID), slog.String("path", p.FilePath))
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
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
		logInternal(r.Context(), "photo: read file", err)
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
		v, ok := snapPhotoWidth(wStr)
		if !ok {
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
		slog.WarnContext(r.Context(), "photo cache mkdir", slog.Any("err", err))
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

	// Only a cache MISS forks ffmpeg, and only that deserves the snapshot
	// ceiling. Gating the whole route charged cache hits too, so one television
	// opening a photo grid (about twenty thumbnails at once) emptied the burst
	// and the rest of the grid came back empty - measured on device 2026-08-03.
	if h.transcodeRL != nil {
		ip := ratelimit.ClientIP(r)
		if !h.transcodeRL.Allow(ip) {
			slog.WarnContext(r.Context(), "photo transcode rate limited",
				slog.String("client_ip", ip), slog.String("id", p.ID))
			w.Header().Set("Retry-After", strconv.Itoa(h.transcodeRL.RetryAfter(ip)))
			http.Error(w, `{"message":"too many requests"}`, http.StatusTooManyRequests)
			return true
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := h.transcoder.Transcode(ctx, p.FilePath, dstPath, width, fmtStr); err != nil {
		slog.WarnContext(r.Context(), "photo transcode", slog.String("id", p.ID), slog.Any("err", err))
		_ = os.Remove(dstPath)
		return false
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, dstPath)
	return true
}

// registerPhotoRoutes wires the photo serve route. rl (nil = pass-through,
// e.g. tests) rate-limits the same handler the snapshot/subtitle routes
// already gate: an uncached ?w=/&fmt= request forks ffmpeg per request, the
// same cost as those routes, so it must not ride the generous global limiter.
func registerPhotoRoutes(mux *http.ServeMux, photoSvc *appphoto.Service, tokenGen domainuser.TokenGenerator, libChecker LibraryAccessChecker, libLister LibraryLister, _ context.Context, transcoder PhotoTranscoder, cacheDir string, rl *ratelimit.Limiter) {
	authMw := httpAuthMiddleware(tokenGen)
	h := &photoHandler{
		photoSvc:    photoSvc,
		libChecker:  libChecker,
		transcoder:  transcoder,
		cacheDir:    cacheDir,
		libLister:   libLister,
		transcodeRL: rl,
	}
	mux.HandleFunc("GET /api/v1/photos/{id}/serve", authMw(h.servePhoto))
}
