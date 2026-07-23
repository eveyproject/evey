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
//
// The upper end matters: a request is SNAPPED to the nearest entry, so before
// 1280/1920 existed the detail-page hero asked for w=1920 and silently got the
// 780px variant stretched across a viewport up to 1920px wide - the whole
// srcSet resolved to one blurry file. Poster sizes (185/342/500) are unchanged;
// only backdrop-scale entries were missing.
var allowedWidths = []int{185, 342, 500, 780, 1280, 1920}

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

// removeVariant best-effort deletes a stale/partial resized variant so it isn't
// served next time. A failure (other than already-gone) is logged but never
// fatal - the next request simply regenerates it.
func removeVariant(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		slog.Warn("artwork variant cleanup", slog.String("path", path), slog.Any("err", err))
	}
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
	// Artwork is public: skip library access and parental checks when no auth
	// claims. checkParental itself also no-ops for nil/admin claims, but the
	// explicit claims != nil guard here keeps this route's public contract
	// obvious without depending on that internal detail.
	if claims != nil {
		if !checkLibraryAccess(r.Context(), w, claims, artItem.LibraryID, h.libraryAccess) {
			span.SetStatus(otelcodes.Error, "library access denied")
			return
		}
		if !checkParental(r.Context(), w, claims, artItem, h.parental) {
			span.SetStatus(otelcodes.Error, "parental restricted")
			return
		}
	}

	records := h.findArtworkRecords(r.Context(), mediaID, artType, artItem.ParentID)
	if records == nil {
		span.SetStatus(otelcodes.Error, "no artwork records")
		writeNoArtwork(w)
		return
	}

	wantsWebP := strings.Contains(r.Header.Get("Accept"), "image/webp")
	w.Header().Set("Vary", "Accept")
	requestedWidth := parseRequestedWidth(r)

	for _, rec := range records {
		if artType == "" || rec.Type == artType {
			// A non-empty LocalPath may still point at a file that has vanished
			// from disk (cache cleared, manual cleanup, partial fetch). Serving
			// it would 404, and the `return` below would swallow the SourceURL
			// fallback - leaving a broken image when a remote URL is available.
			// Treat a missing file as "no local copy" and fall through.
			if rec.LocalPath != nil && *rec.LocalPath != "" && localFileExists(*rec.LocalPath) {
				if wantsWebP {
					// When a width is requested, generate (or serve from cache) a resized variant.
					if requestedWidth > 0 && h.imgConverter != nil {
						variantPath := buildVariantPath(*rec.LocalPath, requestedWidth)
						// Regenerate when the variant is missing OR a 0-byte leftover
						// from a prior failed/timed-out resize. Checking only existence
						// would serve the empty file forever, and `immutable` caching
						// then freezes the broken image in clients - the cause of a
						// poster vanishing at just one srcSet width (e.g. w=500).
						if fi, statErr := os.Stat(variantPath); statErr != nil || fi.Size() == 0 {
							// Not cached yet - generate with 10 s timeout so a slow ffmpeg
							// doesn't block the response indefinitely.
							ctx10, cancel := context.WithTimeout(r.Context(), artworkResizeTimeout)
							defer cancel()
							if resErr := h.imgConverter.ResizeAndConvertToWebP(ctx10, *rec.LocalPath, variantPath, requestedWidth, 80); resErr != nil {
								slog.WarnContext(r.Context(), "artwork resize", slog.String("src", *rec.LocalPath), slog.Int("width", requestedWidth), slog.Any("err", resErr))
								removeVariant(variantPath) // drop any partial/empty output so it isn't served next time
								variantPath = ""           // fall through to full-res WebP
							}
						}
						// Never serve an empty variant, even on the generating request
						// (the converter may exit 0 yet produce nothing).
						if variantPath != "" {
							if fi, statErr := os.Stat(variantPath); statErr != nil || fi.Size() == 0 {
								removeVariant(variantPath)
								variantPath = ""
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
		writeNoArtwork(w)
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

// writeNoArtwork answers an artwork request that resolved to no usable image
// (no records at all, or a record with neither a local file nor a source URL).
//
// It returns 204 No Content rather than 404 Not Found on purpose: a media item
// with no poster yet is an expected, benign state, not an error. A 404 makes the
// browser log a red network error to the console for every posterless card in a
// grid; a 204 lets the <img> load fail silently so the client shows its own
// placeholder, with no console noise. A short, MUTABLE cache keeps a scrolling
// grid from re-requesting on every render while still recovering within minutes
// once artwork is added - the URL is often unversioned for lite feed items, so
// it must never be cached immutably.
func writeNoArtwork(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusNoContent)
}

// localFileExists reports whether path resolves to an existing file on disk.
// Used to avoid serving a stale artwork LocalPath whose file has been removed.
func localFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
	// Bound the parent walk: the real media hierarchy is at most
	// episode -> season -> series, so a small cap is generous. The guard stops
	// a self-referential or cyclic parent_id (poisoned/buggy row) from spinning
	// this loop forever.
	const maxParentDepth = 8
	for range maxParentDepth {
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
	return nil
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
		slog.WarnContext(ctx, "webp convert", slog.String("src", *rec.LocalPath), slog.Any("err", err))
		return ""
	}
	rec.WebPPath = &webpPath
	if h.artworkWriter != nil {
		if err := h.artworkWriter.UpdateWebPPath(ctx, rec.ID, webpPath); err != nil {
			slog.WarnContext(ctx, "save webp path", slog.Any("err", err))
		}
	}
	return webpPath
}

func selectArtworkHandler(artworkRepo ArtworkRepo, libSvc MediaItemGetter, libraryAccess LibraryAccessChecker) http.HandlerFunc {
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
					slog.Error("artwork sort update (demote)", slog.Any("err", err))
					http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
					return
				}
				break
			}
		}
		// This is the defining write - promoting `selected` to primary. Reporting
		// {"ok":true} when it fails made the UI show a new poster that reverted on
		// the next refresh.
		if err := artworkRepo.UpdateSortOrder(r.Context(), selected.ID, -1); err != nil {
			slog.Error("artwork sort update (promote)", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
}
