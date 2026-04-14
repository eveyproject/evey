package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"

	domainmeta "github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/pkg/urlvalidator"
)

func (h *mediaServe) handleArtwork(w http.ResponseWriter, r *http.Request) {
	mediaID := r.PathValue("id")
	artType := r.PathValue("artType")
	claims := HTTPClaimsFromContext(r.Context())

	if st, ok := StreamTokenFromContext(r.Context()); ok {
		if st.MediaItemID != mediaID {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
	}

	artItem, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// Artwork is public: skip library access check when no auth claims.
	if claims != nil && !checkLibraryAccess(r.Context(), w, claims, artItem.LibraryID, h.libraryAccess) {
		return
	}

	records := h.findArtworkRecords(r.Context(), mediaID, artType, artItem.ParentID)
	if records == nil {
		http.NotFound(w, r)
		return
	}

	wantsWebP := strings.Contains(r.Header.Get("Accept"), "image/webp")
	w.Header().Set("Vary", "Accept")

	for _, rec := range records {
		if artType == "" || rec.Type == artType {
			if rec.LocalPath != nil && *rec.LocalPath != "" {
				if wantsWebP {
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
		http.NotFound(w, r)
		return
	}
	if err := urlvalidator.ValidateRedirectURL(artURL); err != nil {
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
		claims := HTTPClaimsFromContext(r.Context())
		mediaID := r.PathValue("id")
		artworkID := r.PathValue("artworkId")
		item, err := libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			return
		}
		records, err := artworkRepo.GetByMediaItem(r.Context(), mediaID)
		if err != nil || len(records) == 0 {
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
