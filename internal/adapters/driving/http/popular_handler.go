package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func registerPopularRoute(mux *http.ServeMux, activityQuery *appactivity.QueryService, libQuery *applibrary.QueryService, tokenGen *auth.JWTGenerator, libraryAccess LibraryAccessChecker) {
	mux.HandleFunc("GET /api/v1/popular", func(w http.ResponseWriter, r *http.Request) {
		claims, ok := extractClaims(r, tokenGen)
		if !ok {
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
		ids := make([]string, len(popular))
		for i, p := range popular {
			ids[i] = p.MediaItemID
		}
		type miInfo struct {
			typ       string
			libraryID string
		}
		infoByID := make(map[string]miInfo, len(ids))
		if len(ids) > 0 {
			mis, merr := libQuery.GetMediaItemsByIDs(r.Context(), ids)
			if merr != nil {
				slog.Warn("popular: batch fetch media items", slog.Any("err", merr))
			}
			for _, mi := range mis {
				infoByID[mi.ID] = miInfo{typ: mi.Type, libraryID: mi.LibraryID}
			}
		}
		var results []popItem
		for _, p := range popular {
			info, exists := infoByID[p.MediaItemID]
			if !exists {
				continue
			}
			if !claims.IsAdmin {
				hasAccess, aErr := libraryAccess.HasAccess(r.Context(), claims.UserID, info.libraryID)
				if aErr != nil || !hasAccess {
					continue
				}
			}
			results = append(results, popItem{
				MediaItemID: p.MediaItemID,
				Title:       p.Title,
				PlayCount:   p.PlayCount,
				Type:        info.typ,
			})
		}
		if results == nil {
			results = []popItem{}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"items": results}); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	})
}
