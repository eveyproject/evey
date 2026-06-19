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

// reindexTranslationReader batch-fetches every locale's translations for a page
// of media items, so the reindex can attach translated titles to the search
// docs (multi-language search). Optional — a nil reader skips the enrichment;
// the per-item enricher path then backfills them on the next re-enrich.
type reindexTranslationReader interface {
	GetByMediaItems(ctx context.Context, mediaItemIDs []string) (map[string][]*library.MediaItemTranslation, error)
}

// attachTranslations populates each item's Translations from one batched read,
// so ToDocument can emit their translated_titles. A read error is logged and
// the page is indexed WITHOUT translations rather than failing the whole
// reindex — a recovery path must not abort on the translation table.
func attachTranslations(ctx context.Context, tr reindexTranslationReader, batch []*library.MediaItem) {
	if tr == nil || len(batch) == 0 {
		return
	}
	ids := make([]string, 0, len(batch))
	for _, it := range batch {
		ids = append(ids, string(it.ID))
	}
	byID, err := tr.GetByMediaItems(ctx, ids)
	if err != nil {
		slog.Warn("reindex: fetch translations (indexing without them)", slog.Any("err", err))
		return
	}
	for _, it := range batch {
		trs := byID[string(it.ID)]
		if len(trs) == 0 {
			continue
		}
		it.Translations = make([]library.MediaItemTranslation, 0, len(trs))
		for _, t := range trs {
			it.Translations = append(it.Translations, *t)
		}
	}
}

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
	translations reindexTranslationReader,
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
		attachTranslations(ctx, translations, batch)
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
