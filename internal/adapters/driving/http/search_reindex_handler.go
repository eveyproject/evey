package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// SearchReindexer rebuilds the search index from the database. Implemented by
// the library QueryService.
type SearchReindexer interface {
	ReindexSearch(ctx context.Context, libraryID string, onProgress func(done, total int)) (int, error)
}

// searchReindexHandler kicks off a full (or per-library, via ?library_id=)
// Meilisearch reindex from Postgres in the background and returns 202
// immediately - the sweep can touch thousands of items. It contacts no
// metadata providers. Admin-gated + heavy-rate-limited at registration; runs
// on serverCtx so it survives the request returning.
func searchReindexHandler(svc SearchReindexer, serverCtx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, span := tracing.StartSpan(r.Context(), "http.admin.SearchReindex")
		defer span.End()
		libraryID := r.URL.Query().Get("library_id")
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					slog.ErrorContext(serverCtx, "recovered panic in search reindex", slog.Any("error", rec))
				}
			}()
			n, err := svc.ReindexSearch(serverCtx, libraryID, nil)
			if err != nil {
				slog.ErrorContext(serverCtx, "search reindex failed", slog.String("library_id", libraryID), slog.Any("err", err))
				return
			}
			slog.Info("search reindex finished", slog.Int("indexed", n), slog.String("library_id", libraryID))
		}()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		if _, err := w.Write([]byte(`{"message":"search reindex started"}`)); err != nil {
			slog.WarnContext(r.Context(), "search reindex: write response", slog.Any("err", err))
		}
	}
}
