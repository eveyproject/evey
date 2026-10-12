package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/pkg/ratelimit"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

func (h *adminHandler) handleRefreshMetadata(w http.ResponseWriter, r *http.Request, mediaItemID string) {
	ctx, span := tracing.StartSpan(r.Context(), "http.admin.RefreshMetadata")
	defer span.End()
	r = r.WithContext(ctx)
	span.SetAttributes(attribute.String("media_item_id", mediaItemID))
	if !h.heavyRL.Allow(ratelimit.ClientIP(r)) {
		span.SetStatus(otelcodes.Error, "rate limited")
		http.Error(w, `{"message":"too many requests"}`, http.StatusTooManyRequests)
		return
	}
	var body struct {
		TmdbID    string `json:"tmdb_id"`
		ImdbID    string `json:"imdb_id"`
		MediaType string `json:"media_type"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB cap on JSON body
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "bad request")
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
		span.RecordError(refreshErr)
		span.SetStatus(otelcodes.Error, "refresh metadata failed")
		slog.ErrorContext(r.Context(), "refresh metadata", slog.Any("err", refreshErr))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *adminHandler) handleResetMetadata(w http.ResponseWriter, r *http.Request, mediaItemID string) {
	ctx, span := tracing.StartSpan(r.Context(), "http.admin.ResetMetadata")
	defer span.End()
	r = r.WithContext(ctx)
	span.SetAttributes(attribute.String("media_item_id", mediaItemID))
	if !h.heavyRL.Allow(ratelimit.ClientIP(r)) {
		span.SetStatus(otelcodes.Error, "rate limited")
		http.Error(w, `{"message":"too many requests"}`, http.StatusTooManyRequests)
		return
	}
	if err := h.metaCmd.ResetItemMetadata(r.Context(), mediaItemID); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "reset metadata failed")
		slog.ErrorContext(r.Context(), "reset metadata", slog.Any("err", err))
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	h.audit(r, "reset_metadata", "media_item", &mediaItemID, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (h *adminHandler) handleSearchMetadata(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.admin.SearchMetadata")
	defer span.End()
	r = r.WithContext(ctx)
	query := r.URL.Query().Get("query")
	mediaType := r.URL.Query().Get("type")
	span.SetAttributes(attribute.String("media_type", mediaType))
	if query == "" || mediaType == "" {
		span.SetStatus(otelcodes.Error, "query and type required")
		http.Error(w, "query and type required", http.StatusBadRequest)
		return
	}
	results, err := h.metaQuery.SearchMetadata(r.Context(), query, mediaType)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "search metadata failed")
		slog.ErrorContext(r.Context(), "search metadata", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	type searchResult struct {
		TmdbID    string `json:"tmdb_id,omitempty"`
		ImdbID    string `json:"imdb_id,omitempty"`
		Provider  string `json:"provider"`
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
		// SearchMetadata fans out across TMDB + OMDB + TVMaze + iTunes +
		// IMDb. Non-TMDB providers don't carry a tmdb_id but do expose
		// imdb_id; the admin refresh-metadata endpoint accepts either,
		// so surface both to the Identify UI for a richer pick list.
		if tmdbID, ok := r.ProviderIDs["tmdb"]; ok {
			sr.TmdbID = tmdbID
		}
		if imdbID, ok := r.ProviderIDs["imdb"]; ok {
			sr.ImdbID = imdbID
		}
		switch {
		case sr.TmdbID != "":
			sr.Provider = "tmdb"
		case sr.ImdbID != "":
			sr.Provider = "imdb"
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
		slog.WarnContext(r.Context(), "json encode failed", slog.Any("err", err))
	}
}

func (h *adminHandler) handleRefreshAll(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.admin.RefreshAllMetadata")
	defer span.End()
	r = r.WithContext(ctx)
	if !h.heavyRL.Allow(ratelimit.ClientIP(r)) {
		span.SetStatus(otelcodes.Error, "rate limited")
		http.Error(w, `{"message":"too many requests"}`, http.StatusTooManyRequests)
		return
	}
	libraryID := r.URL.Query().Get("library_id")
	if libraryID != "" {
		span.SetAttributes(attribute.String("library_id", libraryID))
	}
	h.audit(r, "refresh_all_metadata", "metadata", nil, nil)
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				// No ctx: this goroutine is detached and outlives the request,
				// which is why the work below runs on h.serverCtx. r.Context()
				// is already cancelled by the time a panic lands here.
				slog.Error("recovered panic in bulk metadata refresh", slog.Any("error", rec))
			}
		}()
		if err := h.metaCmd.AutoRefreshAllItems(h.serverCtx, libraryID); err != nil {
			// h.serverCtx, not r.Context(): same detached goroutine as above.
			slog.ErrorContext(h.serverCtx, "bulk metadata refresh", slog.Any("err", err))
		}
	}()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(map[string]string{"message": "refresh started"}); err != nil {
		slog.WarnContext(r.Context(), "json encode failed", slog.Any("err", err))
	}
}
