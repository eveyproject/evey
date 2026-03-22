package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	appfeed "github.com/gravity-zero/evey/internal/app/feed"
	"github.com/gravity-zero/evey/internal/domain/feed"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func registerFeedRoutes(mux *http.ServeMux, feedCmd *appfeed.CommandService, feedQuery *appfeed.QueryService, tokenGen *auth.JWTGenerator) {
	if feedCmd == nil {
		return
	}
	mux.HandleFunc("PUT /api/v1/admin/feeds/config", requireAdmin(tokenGen, batchUpdateFeedConfig(feedCmd)))
	mux.HandleFunc("GET /api/v1/feeds/resume-playback", feedAuthHandler(tokenGen, resumePlaybackHandler(feedQuery)))
	mux.HandleFunc("GET /api/v1/feeds/next-episode", feedAuthHandler(tokenGen, nextEpisodeHandler(feedQuery)))
}

type feedConfigItem struct {
	FeedType     string `json:"feedType"`
	Enabled      bool   `json:"enabled"`
	DisplayOrder int    `json:"displayOrder"`
	MaxItems     int    `json:"maxItems"`
}

type feedAuthFunc func(w http.ResponseWriter, r *http.Request, profileID string)

func feedAuthHandler(tokenGen *auth.JWTGenerator, next feedAuthFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		claims, err := tokenGen.ValidateAccessToken(token)
		if err != nil {
			http.Error(w, `{"message":"invalid token"}`, http.StatusUnauthorized)
			return
		}
		next(w, r, claims.ProfileID)
	}
}

func batchUpdateFeedConfig(svc *appfeed.CommandService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var items []feedConfigItem
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&items); err != nil {
			http.Error(w, `{"message":"invalid body"}`, http.StatusBadRequest)
			return
		}

		cfgs := make([]*feed.AdminConfig, 0, len(items))
		for _, it := range items {
			cfgs = append(cfgs, &feed.AdminConfig{
				FeedType:     feed.Type(it.FeedType),
				Enabled:      it.Enabled,
				DisplayOrder: it.DisplayOrder,
				MaxItems:     it.MaxItems,
			})
		}

		if err := svc.BatchUpdateAdminConfigs(r.Context(), cfgs); err != nil {
			http.Error(w, `{"message":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func resumePlaybackHandler(svc *appfeed.QueryService) feedAuthFunc {
	return func(w http.ResponseWriter, r *http.Request, profileID string) {
		limit := parseLimit(r, 20)
		items, err := svc.ResumePlayback(r.Context(), profileID, limit)
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		writeFeedResponse(w, items)
	}
}

func nextEpisodeHandler(svc *appfeed.QueryService) feedAuthFunc {
	return func(w http.ResponseWriter, r *http.Request, profileID string) {
		limit := parseLimit(r, 20)
		items, err := svc.NextEpisode(r.Context(), profileID, limit)
		if err != nil {
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		writeFeedResponse(w, items)
	}
}

func parseLimit(r *http.Request, defaultLimit int) int {
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			return n
		}
	}
	return defaultLimit
}

type feedItemJSON struct {
	MediaItemID string `json:"mediaItemId"`
	ParentID    string `json:"parentId,omitempty"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	PositionMs  int64  `json:"positionMs,omitempty"`
}

func writeFeedResponse(w http.ResponseWriter, items []*feed.FeedItem) {
	out := make([]feedItemJSON, 0, len(items))
	for _, it := range items {
		out = append(out, feedItemJSON{
			MediaItemID: it.MediaItemID,
			ParentID:    it.ParentID,
			Title:       it.Title,
			Type:        it.Type,
			PositionMs:  it.PositionMs,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"items": out}); err != nil {
		slog.Error("encode response", slog.Any("err", err))
	}
}

