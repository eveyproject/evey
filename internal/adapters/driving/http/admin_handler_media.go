package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

func (h *adminHandler) handleMedia(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/media/"), "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	mediaItemID := parts[0]
	action := parts[1]

	switch {
	case action == "refresh-metadata" && r.Method == http.MethodPost:
		h.handleRefreshMetadata(w, r, mediaItemID)
	case action == "reset-metadata" && r.Method == http.MethodPost:
		h.handleResetMetadata(w, r, mediaItemID)
	case action == "search-metadata" && r.Method == http.MethodGet:
		h.handleSearchMetadata(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *adminHandler) handleRefreshMetadata(w http.ResponseWriter, r *http.Request, mediaItemID string) {
	if !h.heavyRL.Allow(ratelimit.ClientIP(r)) {
		http.Error(w, `{"message":"too many requests"}`, http.StatusTooManyRequests)
		return
	}
	var body struct {
		TmdbID    string `json:"tmdb_id"`
		ImdbID    string `json:"imdb_id"`
		MediaType string `json:"media_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var refreshErr error
	if body.ImdbID != "" {
		refreshErr = h.metaCmd.RefreshItemMetadataByIMDB(r.Context(), mediaItemID, body.ImdbID, body.MediaType)
	} else {
		refreshErr = h.metaCmd.RefreshItemMetadata(r.Context(), mediaItemID, body.TmdbID, body.MediaType)
	}
	if refreshErr != nil {
		slog.Error("refresh metadata", slog.Any("err", refreshErr))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *adminHandler) handleResetMetadata(w http.ResponseWriter, r *http.Request, mediaItemID string) {
	if !h.heavyRL.Allow(ratelimit.ClientIP(r)) {
		http.Error(w, `{"message":"too many requests"}`, http.StatusTooManyRequests)
		return
	}
	if err := h.metaCmd.ResetItemMetadata(r.Context(), mediaItemID); err != nil {
		slog.Error("reset metadata", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	h.audit(r, "reset_metadata", "media_item", &mediaItemID, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (h *adminHandler) handleSearchMetadata(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	mediaType := r.URL.Query().Get("type")
	if query == "" || mediaType == "" {
		http.Error(w, "query and type required", http.StatusBadRequest)
		return
	}
	results, err := h.metaQuery.SearchMetadata(r.Context(), query, mediaType)
	if err != nil {
		slog.Error("search metadata", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	type searchResult struct {
		TmdbID    string `json:"tmdb_id"`
		Title     string `json:"title"`
		Year      *int   `json:"year,omitempty"`
		Overview  string `json:"overview"`
		PosterURL string `json:"poster_url,omitempty"`
	}
	out := make([]searchResult, 0, len(results))
	for _, r := range results {
		sr := searchResult{
			Title:    r.Title,
			Year:     r.Year,
			Overview: r.Overview,
		}
		if tmdbID, ok := r.ProviderIDs["tmdb"]; ok {
			sr.TmdbID = tmdbID
		}
		for _, art := range r.Artwork {
			if art.Type == "poster" {
				sr.PosterURL = art.URL
				break
			}
		}
		out = append(out, sr)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *adminHandler) handleRefreshAll(w http.ResponseWriter, r *http.Request) {
	if !h.heavyRL.Allow(ratelimit.ClientIP(r)) {
		http.Error(w, `{"message":"too many requests"}`, http.StatusTooManyRequests)
		return
	}
	h.audit(r, "refresh_all_metadata", "metadata", nil, nil)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("recovered panic in bulk metadata refresh", slog.Any("error", r))
			}
		}()
		if err := h.metaCmd.AutoRefreshAllItems(h.serverCtx); err != nil {
			slog.Error("bulk metadata refresh", slog.Any("err", err))
		}
	}()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(map[string]string{"message": "refresh started"}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}
