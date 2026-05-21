package library

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// reindexBatchSize bounds how many media items are loaded from Postgres and
// pushed to the search index per round-trip. 500 mirrors the metadata
// bulk-refresh pager.
const reindexBatchSize = 500

// ReindexSearchIndex rebuilds the search index straight from Postgres, with
// NO metadata-provider calls (no TMDB). It pages through media items and
// pushes each page via IndexMediaItemsBatch. libraryID == "" reindexes every
// library. onProgress (optional) is invoked after each batch with (done,
// total). Returns the number of items indexed.
//
// This is the canonical recovery path after a search-index volume loss, a
// Meilisearch version/format change, an embedded↔external migration, or a
// Postgres restore (the DB backup does not carry the search index). It is
// shared verbatim by the admin endpoint (QueryService.ReindexSearch) and the
// `evey admin reindex-search` CLI so both behave identically.
func ReindexSearchIndex(
	ctx context.Context,
	items library.MediaItemQueryRepo,
	search library.SearchIndex,
	libraryID string,
	onProgress func(done, total int),
) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.ReindexSearchIndex")
	defer span.End()

	filter := library.ListMediaItemsFilter{}
	if libraryID != "" {
		libID := shared.LibraryID(libraryID)
		filter.LibraryID = &libID
	}

	total, err := items.Count(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("count media items: %w", err)
	}

	indexed := 0
	for offset := 0; ; offset += reindexBatchSize {
		filter.Limit = reindexBatchSize
		filter.Offset = offset
		batch, err := items.List(ctx, filter)
		if err != nil {
			return indexed, fmt.Errorf("list media items at offset %d: %w", offset, err)
		}
		if len(batch) == 0 {
			break
		}
		if err := search.IndexMediaItemsBatch(ctx, batch); err != nil {
			return indexed, fmt.Errorf("index batch at offset %d: %w", offset, err)
		}
		indexed += len(batch)
		if onProgress != nil {
			onProgress(indexed, total)
		}
		if len(batch) < reindexBatchSize {
			break
		}
	}

	slog.Info("search reindex complete", slog.Int("indexed", indexed), slog.String("library_id", libraryID))
	return indexed, nil
}
