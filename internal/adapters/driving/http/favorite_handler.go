package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	appfavorite "github.com/gravity-zero/evey/internal/app/favorite"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func registerFavoriteRoutes(mux *http.ServeMux, favSvc *appfavorite.Service, libQuery *applibrary.QueryService, tokenGen *auth.JWTGenerator) {
	extractClaims := func(r *http.Request) (*domainuser.Claims, bool) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			return nil, false
		}
		claims, err := tokenGen.ValidateAccessToken(h[7:])
		if err != nil {
			return nil, false
		}
		return claims, true
	}

	mux.HandleFunc("POST /api/v1/favorites/{media_id}", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := extractClaims(r)
		if !ok {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		mediaID := r.PathValue("media_id")
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
		claims, ok := extractClaims(r)
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
		for i, f := range favs {
			items[i] = favItem{MediaItemID: f.MediaItemID, CreatedAt: f.CreatedAt.UTC().Format(time.RFC3339)}
		}
		var wg sync.WaitGroup
		ctx := r.Context()
		for i, f := range favs {
			wg.Add(1)
			go func(idx int, mediaID string) {
				defer wg.Done()
				mi, ferr := libQuery.GetMediaItem(ctx, mediaID)
				if ferr != nil || mi == nil {
					return
				}
				items[idx].Title = mi.Title
				items[idx].Type = mi.Type
				items[idx].Year = mi.Year
				items[idx].CommunityRating = mi.CommunityRating
			}(i, f.MediaItemID)
		}
		wg.Wait()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"items": items}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	})

	mux.HandleFunc("GET /api/v1/favorites/{media_id}", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := extractClaims(r)
		if !ok {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		mediaID := r.PathValue("media_id")
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
