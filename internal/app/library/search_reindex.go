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
// docs (multi-language search). Optional - a nil reader skips the enrichment;
// the per-item enricher path then backfills them on the next re-enrich.
type reindexTranslationReader interface {
	GetByMediaItems(ctx context.Context, mediaItemIDs []string) (map[string][]*library.MediaItemTranslation, error)
}

// reindexCastReader batch-fetches the people names linked to a page of media
// items, so the reindex can keep the searchable "cast" attribute populated.
// Optional - a nil reader skips the enrichment; actor search then works again
// only after the next per-item enrich.
type reindexCastReader interface {
	CastNamesByMediaItems(ctx context.Context, mediaItemIDs []string) (map[string][]string, error)
}

// attachCast populates each item's CastNames from one batched read, so
// ToDocument keeps the searchable cast attribute across rebuilds. Same error
// posture as attachTranslations: log and index without cast rather than
// failing the recovery path.
func attachCast(ctx context.Context, cr reindexCastReader, batch []*library.MediaItem) {
	if cr == nil || len(batch) == 0 {
		return
	}
	ids := make([]string, 0, len(batch))
	for _, it := range batch {
		ids = append(ids, string(it.ID))
	}
	byID, err := cr.CastNamesByMediaItems(ctx, ids)
	if err != nil {
		slog.WarnContext(ctx, "reindex: fetch cast names (indexing without them)", slog.Any("err", err))
		return
	}
	for _, it := range batch {
		it.CastNames = byID[string(it.ID)]
	}
}

// attachTranslations populates each item's Translations from one batched read,
// so ToDocument can emit their translated_titles. A read error is logged and
// the page is indexed WITHOUT translations rather than failing the whole
// reindex - a recovery path must not abort on the translation table.
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
		slog.WarnContext(ctx, "reindex: fetch translations (indexing without them)", slog.Any("err", err))
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
	cast reindexCastReader,
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
		attachCast(ctx, cast, batch)
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

	slog.InfoContext(ctx, "search reindex complete", slog.Int("indexed", indexed), slog.String("library_id", libraryID))
	return indexed, nil
}

// searchDocCounter reports how many documents the search engine currently holds.
// Satisfied by the Meilisearch adapter; lets ReconcileSearchIndex compare the
// live index size to the DB without widening the SearchIndex port.
type searchDocCounter interface {
	DocumentCount(ctx context.Context) (int64, error)
}

const reconcilePageSize = 500

// ReconcileSearchIndex heals search-index drift. The resilient wrapper swallows a
// write when the engine is down, so an item created/edited during an outage stays
// out of the index until something rebuilds it. This compares the engine's
// document count to the DB item count and rebuilds when they diverge - run it at
// boot AND periodically so a mid-session outage self-heals without a restart.
//
// LIMITATION (count-based, by design): it cannot see drift that nets to zero -
// e.g. an equal number of items added AND deleted during one outage leaves both
// counts matched while a stale deleted doc lingers in the index. Prod hit this
// twice at scale (2026-07-04 and 2026-08-07, ~half the catalog unfindable), so
// the boot path now runs a full upsert ReindexSearchIndex before this reconcile;
// between boots the blind spot remains and is accepted for the tick cadence.
//
// Returns (0, nil) and skips when the engine is UNREACHABLE (DocumentCount
// errors): a periodic caller must not scan the DB against a down engine, it just
// retries next tick. `search` MUST be the primary adapter, not the error-
// swallowing resilient wrapper, so a still-down engine surfaces its error from
// the rebuild instead of silently "succeeding" and clearing the drift. Uses
// keyset (AfterID) pagination to avoid O(n²) OFFSET scans on large libraries.
func ReconcileSearchIndex(
	ctx context.Context,
	items library.MediaItemQueryRepo,
	search library.SearchIndex,
	docCounter searchDocCounter,
	translations reindexTranslationReader,
	cast reindexCastReader,
) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.ReconcileSearchIndex")
	defer span.End()

	docCount, err := docCounter.DocumentCount(ctx)
	if err != nil {
		return 0, nil // engine unreachable - can't compare; retry on the next call
	}
	dbCount, err := items.Count(ctx, library.ListMediaItemsFilter{})
	if err != nil {
		return 0, fmt.Errorf("reconcile: count media items: %w", err)
	}
	// Equal counts = in sync, nothing to do.
	if docCount == int64(dbCount) {
		return 0, nil
	}
	// When the index has MORE docs than the DB, a delete was dropped during an
	// outage and left a stale doc. An upsert reindex can only add/overwrite, never
	// shrink, so we WIPE the index first and then rebuild it from the DB. This is
	// not a CPU loop: after the wipe+reindex the counts match, so the next tick is
	// in sync and does nothing. (The trade-off is a brief empty-search window while
	// the rebuild runs - acceptable for a 15-min reconcile, and far better than
	// stale deleted items lingering in search indefinitely.)
	if docCount > int64(dbCount) {
		slog.InfoContext(ctx, "search reconcile: index larger than DB (stale docs from a dropped delete), wiping then rebuilding",
			slog.Int64("index_docs", docCount), slog.Int("db_items", dbCount))
		if err := search.Clear(ctx); err != nil {
			return 0, fmt.Errorf("reconcile: clear oversized index: %w", err)
		}
	} else {
		slog.InfoContext(ctx, "search reconcile: index behind the DB, rebuilding",
			slog.Int64("index_docs", docCount), slog.Int("db_items", dbCount))
	}

	indexed := 0
	afterID := ""
	for {
		if ctx.Err() != nil {
			return indexed, ctx.Err()
		}
		batch, err := items.List(ctx, library.ListMediaItemsFilter{Limit: reconcilePageSize, AfterID: afterID, KeysetByID: true})
		if err != nil {
			return indexed, fmt.Errorf("reconcile: list media items after %q: %w", afterID, err)
		}
		if len(batch) == 0 {
			break
		}
		attachTranslations(ctx, translations, batch)
		attachCast(ctx, cast, batch)
		if err := search.IndexMediaItemsBatch(ctx, batch); err != nil {
			return indexed, fmt.Errorf("reconcile: index batch after %q: %w", afterID, err)
		}
		indexed += len(batch)
		afterID = string(batch[len(batch)-1].ID)
		if len(batch) < reconcilePageSize {
			break
		}
	}
	slog.InfoContext(ctx, "search reconcile complete", slog.Int("indexed", indexed))
	return indexed, nil
}
