package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	domainmeta "github.com/gravity-zero/evey/internal/domain/metadata"
	"github.com/gravity-zero/evey/internal/pkg/urlvalidator"
)

func (h *mediaServe) handleArtwork(w http.ResponseWriter, r *http.Request) {
	mediaID := r.PathValue("id")
	artType := r.PathValue("artType")
	artAuthHeader := r.Header.Get("Authorization")
	if len(artAuthHeader) < 8 {
		if t := r.URL.Query().Get("token"); t != "" {
			artAuthHeader = "Bearer " + t
		}
	}
	if len(artAuthHeader) < 8 || artAuthHeader[:7] != "Bearer " {
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	artClaims, err := h.tokens.ValidateAccessToken(artAuthHeader[7:])
	if err != nil {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}
	artItem, err := h.libSvc.GetMediaItem(r.Context(), mediaID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !checkLibraryAccess(r.Context(), w, artClaims, artItem.LibraryID, h.libraryAccess) {
		return
	}

	records := h.findArtworkRecords(r.Context(), mediaID, artType, artItem.ParentID)
	if records == nil {
		http.NotFound(w, r)
		return
	}

	for _, rec := range records {
		if artType == "" || rec.Type == artType {
			if rec.LocalPath != nil && *rec.LocalPath != "" {
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

func selectArtworkHandler(tokens TokenVerifier, artworkRepo ArtworkRepo, libSvc MediaItemGetter, libraryAccess LibraryAccessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		claims, err := tokens.ValidateAccessToken(authHeader[7:])
		if err != nil {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		if !claims.IsAdmin {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
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
