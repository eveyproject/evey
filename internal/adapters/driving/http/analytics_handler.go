package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func registerAnalyticsRoutes(mux *http.ServeMux, activityQuery *appactivity.QueryService, tokenGen *auth.JWTGenerator) {
	mux.HandleFunc("GET /api/v1/admin/analytics", requireAdmin(tokenGen, func(w http.ResponseWriter, r *http.Request) {
		data, err := activityQuery.GetAnalytics(r.Context())
		if err != nil {
			slog.Error("get analytics", slog.Any("err", err))
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}

		type topMediaItem struct {
			MediaItemID          string `json:"media_item_id"`
			Title                string `json:"title"`
			PlayCount            int    `json:"play_count"`
			TotalDurationSeconds int64  `json:"total_duration_seconds"`
		}
		type recentActivityEntry struct {
			UserID     string    `json:"user_id"`
			Username   string    `json:"username"`
			MediaTitle string    `json:"media_title"`
			PlayedAt   time.Time `json:"played_at"`
		}
		type response struct {
			TopMedia           []topMediaItem        `json:"top_media"`
			RecentActivity     []recentActivityEntry `json:"recent_activity"`
			TotalPlaysToday    int                   `json:"total_plays_today"`
			TotalPlaysThisWeek int                   `json:"total_plays_this_week"`
			ActiveUsersToday   int                   `json:"active_users_today"`
		}

		topMedia := make([]topMediaItem, 0, len(data.TopMedia))
		for _, item := range data.TopMedia {
			topMedia = append(topMedia, topMediaItem{
				MediaItemID:          item.MediaItemID,
				Title:                item.Title,
				PlayCount:            item.PlayCount,
				TotalDurationSeconds: item.TotalDurationSeconds,
			})
		}

		recentActivity := make([]recentActivityEntry, 0, len(data.RecentActivity))
		for _, entry := range data.RecentActivity {
			recentActivity = append(recentActivity, recentActivityEntry{
				UserID:     entry.UserID,
				Username:   entry.Username,
				MediaTitle: entry.MediaTitle,
				PlayedAt:   entry.PlayedAt,
			})
		}

		resp := response{
			TopMedia:           topMedia,
			RecentActivity:     recentActivity,
			TotalPlaysToday:    data.TotalPlaysToday,
			TotalPlaysThisWeek: data.TotalPlaysThisWeek,
			ActiveUsersToday:   data.ActiveUsersToday,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Warn("json encode failed", slog.Any("err", err))
		}
	}))
}
