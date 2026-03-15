package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func registerPopularRoute(mux *http.ServeMux, activityQuery *appactivity.QueryService, libQuery *applibrary.QueryService, tokenGen *auth.JWTGenerator) {
	mux.HandleFunc("GET /api/v1/popular", func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		if _, err := tokenGen.ValidateAccessToken(h[7:]); err != nil {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		limit := 20
		popular, err := activityQuery.GetMostPopular(r.Context(), limit)
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		type popItem struct {
			MediaItemID string `json:"mediaItemId"`
			Title       string `json:"title"`
			Type        string `json:"type,omitempty"`
			PlayCount   int    `json:"playCount"`
		}
		items := make([]popItem, 0, len(popular))
		results := make([]popItem, len(popular))
		for i, p := range popular {
			results[i] = popItem{MediaItemID: p.MediaItemID, Title: p.Title, PlayCount: p.PlayCount}
		}
		var wg sync.WaitGroup
		ctx := r.Context()
		for i, p := range popular {
			wg.Add(1)
			go func(idx int, mediaID string) {
				defer wg.Done()
				mi, ferr := libQuery.GetMediaItem(ctx, mediaID)
				if ferr == nil && mi != nil {
					results[idx].Type = mi.Type
				}
			}(i, p.MediaItemID)
		}
		wg.Wait()
		items = append(items, results...)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"items": items}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	})
}
