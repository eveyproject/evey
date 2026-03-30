package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	appfavorite "github.com/gravity-zero/evey/internal/app/favorite"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func registerFavoriteRoutes(mux *http.ServeMux, favSvc *appfavorite.Service, libQuery *applibrary.QueryService, tokenGen *auth.JWTGenerator, libraryAccess LibraryAccessChecker) {
	mux.HandleFunc("POST /api/v1/favorites/{media_id}", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := extractClaims(r, tokenGen)
		if !ok {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		mediaID := r.PathValue("media_id")
		mi, err := libQuery.GetMediaItem(r.Context(), mediaID)
		if err != nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, mi.LibraryID, libraryAccess) {
			return
		}
		isFav, err := favSvc.Toggle(r.Context(), claims.ProfileID, mediaID)
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"isFavorite": isFav}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	})

	mux.HandleFunc("GET /api/v1/favorites", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := extractClaims(r, tokenGen)
		if !ok {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		limit := 50
		offset := 0
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 {
				limit = n
			}
		}
		if o := r.URL.Query().Get("offset"); o != "" {
			if n, err := strconv.Atoi(o); err == nil && n >= 0 {
				offset = n
			}
		}
		favs, err := favSvc.List(r.Context(), claims.ProfileID, limit, offset)
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		type favItem struct {
			MediaItemID     string   `json:"mediaItemId"`
			CreatedAt       string   `json:"createdAt"`
			Title           string   `json:"title,omitempty"`
			Type            string   `json:"type,omitempty"`
			Year            *int     `json:"year,omitempty"`
			CommunityRating *float64 `json:"communityRating,omitempty"`
		}
		items := make([]favItem, len(favs))
		ids := make([]string, len(favs))
		for i, f := range favs {
			items[i] = favItem{MediaItemID: f.MediaItemID, CreatedAt: f.CreatedAt.UTC().Format(time.RFC3339)}
			ids[i] = f.MediaItemID
		}
		mediaItems, err := libQuery.GetMediaItemsByIDs(r.Context(), ids)
		if err != nil {
			slog.Warn("fetch media items for favorites", slog.Any("err", err))
		}
		byID := make(map[string]*library.MediaItem, len(mediaItems))
		for _, mi := range mediaItems {
			byID[mi.ID] = mi
		}
		// filter out items from inaccessible libraries
		filtered := items[:0]
		for i, f := range favs {
			mi, ok := byID[f.MediaItemID]
			if !ok {
				continue
			}
			if !claims.IsAdmin {
				hasAccess, aErr := libraryAccess.HasAccess(r.Context(), claims.UserID, mi.LibraryID)
				if aErr != nil || !hasAccess {
					continue
				}
			}
			items[i].Title = mi.Title
			items[i].Type = mi.Type
			items[i].Year = mi.Year
			items[i].CommunityRating = mi.CommunityRating
			filtered = append(filtered, items[i])
		}
		items = filtered
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"items": items}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	})

	mux.HandleFunc("GET /api/v1/favorites/{media_id}", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := extractClaims(r, tokenGen)
		if !ok {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		mediaID := r.PathValue("media_id")
		mi, err := libQuery.GetMediaItem(r.Context(), mediaID)
		if err != nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, mi.LibraryID, libraryAccess) {
			return
		}
		isFav, err := favSvc.IsFavorite(r.Context(), claims.ProfileID, mediaID)
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"isFavorite": isFav}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	})
}
