package search

import (
	"context"
	"log/slog"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type ResilientSearch struct {
	primary  library.SearchIndex
	fallback library.SearchIndex
}

func NewResilient(primary, fallback library.SearchIndex) *ResilientSearch {
	return &ResilientSearch{primary: primary, fallback: fallback}
}

func (r *ResilientSearch) IndexMediaItem(ctx context.Context, item *library.MediaItem, castNames []string) error {
	ctx, span := tracing.StartSpan(ctx, "search.resilient.IndexMediaItem")
	defer span.End()

	if r.primary == nil {
		return nil
	}
	if err := r.primary.IndexMediaItem(ctx, item, castNames); err != nil {
		slog.Warn("search index: primary failed, skipping", slog.Any("err", err))
	}
	return nil
}

func (r *ResilientSearch) IndexMediaItemsBatch(ctx context.Context, items []*library.MediaItem) error {
	ctx, span := tracing.StartSpan(ctx, "search.resilient.IndexMediaItemsBatch")
	defer span.End()

	if r.primary == nil {
		return nil
	}
	if err := r.primary.IndexMediaItemsBatch(ctx, items); err != nil {
		slog.Warn("search index batch: primary failed, skipping", slog.Any("err", err))
	}
	return nil
}

func (r *ResilientSearch) RemoveMediaItem(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "search.resilient.RemoveMediaItem")
	defer span.End()

	if r.primary == nil {
		return nil
	}
	if err := r.primary.RemoveMediaItem(ctx, id); err != nil {
		slog.Warn("search remove: primary failed, skipping", slog.Any("err", err))
	}
	return nil
}

func (r *ResilientSearch) Clear(ctx context.Context) error {
	ctx, span := tracing.StartSpan(ctx, "search.resilient.Clear")
	defer span.End()

	if r.primary == nil {
		return nil
	}
	// Unlike the other writes, surface the error: the reconcile clears then
	// reindexes and must NOT proceed to reindex (and log success) if the wipe
	// silently failed — that would leave the stale docs in place.
	return r.primary.Clear(ctx)
}

func (r *ResilientSearch) Primary() library.SearchIndex { return r.primary }

func (r *ResilientSearch) Search(ctx context.Context, query string, filters map[string]string, sort []string, limit int64) ([]library.SearchHit, int64, error) {
	ctx, span := tracing.StartSpan(ctx, "search.resilient.Search")
	defer span.End()

	if r.primary != nil {
		hits, total, err := r.primary.Search(ctx, query, filters, sort, limit)
		if err == nil {
			return hits, total, nil
		}
		slog.Warn("search: primary failed, falling back to SQL", slog.Any("err", err))
	}
	return r.fallback.Search(ctx, query, filters, sort, limit)
}
