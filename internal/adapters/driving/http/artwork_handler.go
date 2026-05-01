package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	domainmeta "github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"github.com/evey-project/evey/internal/pkg/urlvalidator"
)

// artworkResizeTimeout bounds the ffmpeg call for a single poster resize.
const artworkResizeTimeout = 10 * time.Second

// allowedWidths caps variant creation to a fixed set so the cache stays bounded.
// 0 is the sentinel for "no resize".
var allowedWidths = []int{185, 342, 500, 780}

// parseRequestedWidth reads ?w= from the request and snaps it to the nearest
// allowed width. Returns 0 when the parameter is absent or unparseable.
func parseRequestedWidth(r *http.Request) int {
	raw := r.URL.Query().Get("w")
	if raw == "" {
		return 0
	}
	w, err := strconv.Atoi(raw)
	if err != nil || w <= 0 {
		return 0
	}
	best := 0
	bestDiff := 1 << 30
	for _, allowed := range allowedWidths {
		diff := w - allowed
		if diff < 0 {
			diff = -diff
		}
		if diff < bestDiff {
			bestDiff = diff
			best = allowed
		}
	}
	return best
}

// buildVariantPath derives the on-disk path for a resized variant, e.g.
// /data/artworks/abc/poster.jpg → /data/artworks/abc/poster_w400.webp
func buildVariantPath(srcPath string, width int) string {
	base := srcPath
	if idx := strings.LastIndex(srcPath, "."); idx >= 0 {
		base = srcPath[:idx]
	}
	return fmt.Sprintf("%s_w%d.webp", base, width)
}

func (h *mediaServe) handleArtwork(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.artwork.Serve")
	defer span.End()
	r = r.WithContext(ctx)
	mediaID := r.PathValue("id")
	artType := r.PathValue("artType")
	span.SetAttributes(
		attribute.String("media_item_id", mediaID),
		attribute.String("art_type", artType),
	)
	claims := HTTPClaimsFromContext(r.Context())

	if st, ok := StreamTokenFromContext(r.Context()); ok {
		if st.MediaItemID != shared.MediaItemID(mediaID) {
			span.SetStatus(otelcodes.Error, "stream token media mismatch")
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	artItem, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "media item not found")
		http.NotFound(w, r)
		return
	}
	// Artwork is public: skip library access check when no auth claims.
	if claims != nil && !checkLibraryAccess(r.Context(), w, claims, artItem.LibraryID, h.libraryAccess) {
		span.SetStatus(otelcodes.Error, "library access denied")
		return
	}

	records := h.findArtworkRecords(r.Context(), mediaID, artType, artItem.ParentID)
	if records == nil {
		span.SetStatus(otelcodes.Error, "no artwork records")
		http.NotFound(w, r)
		return
	}

	wantsWebP := strings.Contains(r.Header.Get("Accept"), "image/webp")
	w.Header().Set("Vary", "Accept")
	requestedWidth := parseRequestedWidth(r)

	for _, rec := range records {
		if artType == "" || rec.Type == artType {
			if rec.LocalPath != nil && *rec.LocalPath != "" {
				if wantsWebP {
					// When a width is requested, generate (or serve from cache) a resized variant.
					if requestedWidth > 0 && h.imgConverter != nil {
						variantPath := buildVariantPath(*rec.LocalPath, requestedWidth)
						if _, statErr := os.Stat(variantPath); statErr != nil {
							// Not cached yet — generate with 10 s timeout so a slow ffmpeg
							// doesn't block the response indefinitely.
							ctx10, cancel := context.WithTimeout(r.Context(), artworkResizeTimeout)
							defer cancel()
							if resErr := h.imgConverter.ResizeAndConvertToWebP(ctx10, *rec.LocalPath, variantPath, requestedWidth, 80); resErr != nil {
								slog.Warn("artwork resize", slog.String("src", *rec.LocalPath), slog.Int("width", requestedWidth), slog.Any("err", resErr))
								variantPath = "" // fall through to full-res WebP
							}
						}
						if variantPath != "" {
							w.Header().Set("Content-Type", "image/webp")
							serveFileWithCache(w, r, variantPath)
							return
						}
					}
					if webp := h.resolveWebP(r.Context(), rec); webp != "" {
						w.Header().Set("Content-Type", "image/webp")
						serveFileWithCache(w, r, webp)
						return
					}
				}
				serveFileWithCache(w, r, *rec.LocalPath)
				return
			}
		}
	}
	var artURL string
	for _, rec := range records {
		if artType == "" || rec.Type == artType {
			if rec.SourceURL != nil {
				artURL = *rec.SourceURL
				break
			}
		}
	}
	if artURL == "" {
		span.SetStatus(otelcodes.Error, "no artwork url")
		http.NotFound(w, r)
		return
	}
	if err := urlvalidator.ValidateRedirectURL(artURL); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "invalid redirect url")
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.Redirect(w, r, artURL, http.StatusFound)
}

func (h *mediaServe) findArtworkRecords(ctx context.Context, mediaID, artType, parentID string) []*domainmeta.ArtworkRecord {
	hasMatchingType := func(recs []*domainmeta.ArtworkRecord) bool {
		for _, rec := range recs {
			if artType == "" || rec.Type == artType {
				return true
			}
		}
		return false
	}
	lookupID := mediaID
	nextParentID := parentID
	for {
		records, err := h.artworkRepo.GetByMediaItem(ctx, lookupID)
		if err != nil {
			return nil
		}
		if hasMatchingType(records) {
			return records
		}
		if nextParentID == "" {
			return nil
		}
		lookupID = nextParentID
		parent, err := h.libSvc.GetMediaItem(ctx, lookupID)
		if err != nil {
			return nil
		}
		nextParentID = parent.ParentID
	}
}

func (h *mediaServe) resolveWebP(ctx context.Context, rec *domainmeta.ArtworkRecord) string {
	if rec.WebPPath != nil && *rec.WebPPath != "" {
		if _, err := os.Stat(*rec.WebPPath); err == nil {
			return *rec.WebPPath
		}
	}
	if h.imgConverter == nil || rec.LocalPath == nil || *rec.LocalPath == "" {
		return ""
	}
	ext := ""
	if idx := strings.LastIndex(*rec.LocalPath, "."); idx >= 0 {
		ext = (*rec.LocalPath)[idx:]
	}
	webpPath := strings.TrimSuffix(*rec.LocalPath, ext) + ".webp"
	if err := h.imgConverter.ConvertToWebP(ctx, *rec.LocalPath, webpPath, 80); err != nil {
		slog.Warn("webp convert", slog.String("src", *rec.LocalPath), slog.Any("err", err))
		return ""
	}
	rec.WebPPath = &webpPath
	if h.artworkWriter != nil {
		if err := h.artworkWriter.UpdateWebPPath(ctx, rec.ID, webpPath); err != nil {
			slog.Warn("save webp path", slog.Any("err", err))
		}
	}
	return webpPath
}

func selectArtworkHandler(tokens TokenVerifier, artworkRepo ArtworkRepo, libSvc MediaItemGetter, libraryAccess LibraryAccessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.artwork.Select")
		defer span.End()
		r = r.WithContext(ctx)
		claims := HTTPClaimsFromContext(r.Context())
		mediaID := r.PathValue("id")
		artworkID := r.PathValue("artworkId")
		span.SetAttributes(
			attribute.String("media_item_id", mediaID),
			attribute.String("artwork_id", artworkID),
		)
		item, err := libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "media item not found")
			http.NotFound(w, r)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			span.SetStatus(otelcodes.Error, "library access denied")
			return
		}
		records, err := artworkRepo.GetByMediaItem(r.Context(), mediaID)
		if err != nil || len(records) == 0 {
			if err != nil {
				span.RecordError(err)
			}
			span.SetStatus(otelcodes.Error, "no artwork records")
			http.NotFound(w, r)
			return
		}
		var selected *domainmeta.ArtworkRecord
		for _, rec := range records {
			if rec.ID == artworkID {
				selected = rec
				break
			}
		}
		if selected == nil {
			span.SetStatus(otelcodes.Error, "artwork id not found")
			http.NotFound(w, r)
			return
		}
		for _, rec := range records {
			if rec.Type == selected.Type && rec.ID != selected.ID && rec.SortOrder < selected.SortOrder {
				if err := artworkRepo.UpdateSortOrder(r.Context(), rec.ID, selected.SortOrder); err != nil {
					slog.Warn("artwork sort update", slog.Any("err", err))
				}
				break
			}
		}
		if err := artworkRepo.UpdateSortOrder(r.Context(), selected.ID, -1); err != nil {
			slog.Warn("artwork sort update", slog.Any("err", err))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
}
