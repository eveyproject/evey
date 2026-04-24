package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// filterMasterPlaylist removes #EXT-X-STREAM-INF variants exceeding maxBps (bps).
// If all variants exceed the limit, keeps the one with the lowest BANDWIDTH.
func filterMasterPlaylist(content string, maxBps int64) string {
	if maxBps <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	type variant struct {
		inf string
		uri string
		bw  int64
	}
	var header []string
	var variants []variant
	i := 0
	for i < len(lines) {
		line := lines[i]
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			bw := int64(0)
			attrs := strings.TrimPrefix(line, "#EXT-X-STREAM-INF:")
			if _, after, ok := strings.Cut(attrs, "BANDWIDTH="); ok {
				if _, err := fmt.Sscanf(after, "%d", &bw); err != nil {
					slog.Warn("parse BANDWIDTH", slog.Any("err", err))
				}
			}
			uri := ""
			if i+1 < len(lines) {
				uri = lines[i+1]
				i++
			}
			variants = append(variants, variant{inf: line, uri: uri, bw: bw})
		} else {
			header = append(header, line)
		}
		i++
	}

	var allowed []variant
	for _, v := range variants {
		if v.bw <= maxBps {
			allowed = append(allowed, v)
		}
	}
	if len(allowed) == 0 && len(variants) > 0 {
		lowest := variants[0]
		for _, v := range variants[1:] {
			if v.bw < lowest.bw {
				lowest = v
			}
		}
		allowed = []variant{lowest}
	}

	var out []string
	out = append(out, header...)
	for _, v := range allowed {
		out = append(out, v.inf, v.uri)
	}
	return strings.Join(out, "\n")
}

// --- mediaHandler: stream + download ---

type mediaServe struct {
	tokens        TokenVerifier
	libSvc        MediaItemGetter
	artworkRepo   ArtworkReader
	artworkWriter ArtworkWebPWriter
	imgConverter  ImageConverter
	baseURL       string
	libraryAccess LibraryAccessChecker
}

// ArtworkWebPWriter updates the WebP path on an artwork record.
type ArtworkWebPWriter interface {
	UpdateWebPPath(ctx context.Context, id string, webpPath string) error
}

type mediaServeOption func(*mediaServe)

func withImageConverter(conv ImageConverter, writer ArtworkWebPWriter) mediaServeOption {
	return func(h *mediaServe) {
		h.imgConverter = conv
		h.artworkWriter = writer
	}
}

func mediaHandler(tokens TokenVerifier, libSvc MediaItemGetter, artworkRepo ArtworkReader, baseURL string, libraryAccess LibraryAccessChecker, opts ...mediaServeOption) http.HandlerFunc {
	h := &mediaServe{tokens: tokens, libSvc: libSvc, artworkRepo: artworkRepo, baseURL: baseURL, libraryAccess: libraryAccess}
	for _, o := range opts {
		o(h)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		artType := r.PathValue("artType")
		if artType != "" {
			h.handleArtwork(w, r)
		} else {
			h.handleStream(w, r)
		}
	}
}

func (h *mediaServe) handleStream(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.media.HandleStream")
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
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "media item lookup failed")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, h.libraryAccess) {
		span.SetStatus(codes.Error, "library access denied")
		return
	}
	if item.File == nil || item.File.Path == "" {
		span.SetStatus(codes.Error, "media file missing")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	setCORSHeaders(w, r, h.baseURL)
	http.ServeFile(w, r, item.File.Path)
}

func downloadHandler(tokens TokenVerifier, libSvc MediaItemGetter, baseURL string, libraryAccess LibraryAccessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.media.Download")
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

		item, err := libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil || item.File == nil || item.File.Path == "" {
			if err != nil {
				span.RecordError(err)
			}
			span.SetStatus(codes.Error, "media item lookup failed")
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			span.SetStatus(codes.Error, "library access denied")
			return
		}
		serveDownload(w, r, item, baseURL)
	}
}

func serveDownload(w http.ResponseWriter, r *http.Request, item *library.MediaItem, baseURL string) {
	ctx, span := tracing.StartSpan(r.Context(), "http.media.ServeDownload")
	defer span.End()
	r = r.WithContext(ctx)
	if item.File == nil {
		span.SetStatus(codes.Error, "media file missing")
		http.Error(w, `{"message":"file not found"}`, http.StatusNotFound)
		return
	}
	fi, err := os.Stat(item.File.Path)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "stat failed")
		http.Error(w, `{"message":"file not found"}`, http.StatusNotFound)
		return
	}
	filename := sanitizeFilename(filepath.Base(item.File.Path))
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
	setCORSHeaders(w, r, baseURL)
	http.ServeFile(w, r, item.File.Path)
}
