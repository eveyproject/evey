package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	appcollection "github.com/evey-project/evey/internal/app/collection"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// collectionDownloadMaxItems caps how many collection members are bundled into
// a download. Mirrors the access-check ceiling used by the gRPC handler.
const collectionDownloadMaxItems = 1000

// collectionSource reads a collection and its member media items.
type collectionSource interface {
	GetByID(ctx context.Context, id string) (*library.Collection, error)
	GetItems(ctx context.Context, collectionID string, limit, offset int) ([]*library.MediaItem, int, error)
}

type collectionHandler struct {
	collSvc    collectionSource
	libSvc     MediaArchiveSource
	libChecker LibraryAccessChecker
	baseURL    string
}

func registerCollectionRoutes(mux *http.ServeMux, collSvc *appcollection.Service, libSvc MediaArchiveSource, tokenGen *auth.JWTGenerator, libChecker LibraryAccessChecker, baseURL string, rl *ratelimit.Limiter) {
	authMw := httpAuthMiddleware(tokenGen)
	h := &collectionHandler{collSvc: collSvc, libSvc: libSvc, libChecker: libChecker, baseURL: baseURL}
	// A collection ZIP can fan out to many large files, so it's a heavier
	// amplification vector than a single-file download — rate-limit it.
	mux.HandleFunc("GET /api/v1/collections/{id}/download", ratelimit.MiddlewareFunc(rl, authMw(h.downloadCollection)))
}

// downloadCollection streams a ZIP of every accessible media file in a
// collection. A collection may span libraries, so access is resolved per item
// (admins see everything); members the caller can't reach are silently
// dropped. If nothing is accessible/downloadable the response is 404 — same as
// a missing collection, so existence isn't leaked.
func (h *collectionHandler) downloadCollection(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.collection.Download")
	defer span.End()
	r = r.WithContext(ctx)
	claims := HTTPClaimsFromContext(r.Context())
	collectionID := r.PathValue("id")
	span.SetAttributes(attribute.String("collection_id", collectionID))

	col, err := h.collSvc.GetByID(r.Context(), collectionID)
	if err != nil || col == nil {
		if err != nil {
			span.RecordError(err)
		}
		span.SetStatus(otelcodes.Error, "collection not found")
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}

	items, _, err := h.collSvc.GetItems(r.Context(), collectionID, collectionDownloadMaxItems, 0)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "list collection items failed")
		slog.Error("collection download: get items", slog.String("collection_id", collectionID), slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	entries := buildMediaArchiveEntries(r.Context(), h.libSvc, items, libraryAccessFunc(r.Context(), claims, h.libChecker), mediaRoots(r.Context(), h.libSvc))
	serveMediaArchive(w, r, col.Name, entries, h.baseURL)
}
