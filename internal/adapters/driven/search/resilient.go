package search

import (
	"context"
	"log/slog"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type ResilientSearch struct {
	primary  library.SearchIndex
	fallback library.SearchIndex
	// queue is the durable outbox: a write the engine rejects is recorded here and
	// replayed by the drain worker, instead of being silently lost (which the
	// count-based reconcile can't always heal). nil keeps the legacy swallow.
	queue library.PendingSearchWriteQueue
	// writer, when set, takes the writes instead of primary: an adapter that
	// returns once the engine accepted the write, without waiting for its
	// task. Every upsert it takes is also queued, so the drain (which waits
	// on primary) confirms it. Keeps a slow engine from holding a request,
	// a library deletion or a scan for minutes.
	writer library.SearchIndex
}

func NewResilient(primary, fallback library.SearchIndex) *ResilientSearch {
	return &ResilientSearch{primary: primary, fallback: fallback}
}

// WithQueue wires the durable outbox so failed writes are replayed later. Chainable.
func (r *ResilientSearch) WithQueue(q library.PendingSearchWriteQueue) *ResilientSearch {
	r.queue = q
	return r
}

// WithSubmitOnlyWriter routes the writes through w, which does not wait for
// the engine's task, and queues every upsert so the drain confirms it with
// primary. Needs WithQueue: without the outbox a failed task would be lost.
// Chainable.
func (r *ResilientSearch) WithSubmitOnlyWriter(w library.SearchIndex) *ResilientSearch {
	r.writer = w
	return r
}

// writeTarget is where writes go: the submit-only writer when wired (and an
// outbox backs it), else primary.
func (r *ResilientSearch) writeTarget() (library.SearchIndex, bool) {
	if r.writer != nil && r.queue != nil {
		return r.writer, true
	}
	return r.primary, false
}

// enqueue records a failed write for replay. It detaches from the caller's ctx
// cancellation (WithoutCancel) so a write failing as the request ends is still
// durably queued rather than lost.
func (r *ResilientSearch) enqueue(ctx context.Context, id, op string) {
	if r.queue == nil {
		return
	}
	if err := r.queue.Enqueue(context.WithoutCancel(ctx), id, op); err != nil {
		slog.ErrorContext(ctx, "search index: enqueue pending write failed (item may drift out of search)",
			slog.String("media_item_id", id), slog.String("op", op), slog.Any("err", err))
	}
}

func (r *ResilientSearch) IndexMediaItem(ctx context.Context, item *library.MediaItem) error {
	ctx, span := tracing.StartSpan(ctx, "search.resilient.IndexMediaItem")
	defer span.End()

	if r.primary == nil {
		return nil
	}
	w, submitOnly := r.writeTarget()
	if err := w.IndexMediaItem(ctx, item); err != nil {
		slog.WarnContext(ctx, "search index: primary failed, queuing for replay", slog.Any("err", err))
		r.enqueue(ctx, string(item.ID), library.SearchOpUpsert)
	} else if submitOnly {
		r.enqueue(ctx, string(item.ID), library.SearchOpUpsert) // the drain confirms the task
	}
	return nil
}

func (r *ResilientSearch) IndexMediaItemsBatch(ctx context.Context, items []*library.MediaItem) error {
	ctx, span := tracing.StartSpan(ctx, "search.resilient.IndexMediaItemsBatch")
	defer span.End()

	if r.primary == nil {
		return nil
	}
	w, submitOnly := r.writeTarget()
	err := w.IndexMediaItemsBatch(ctx, items)
	if err != nil {
		slog.WarnContext(ctx, "search index batch: primary failed, queuing for replay", slog.Any("err", err), slog.Int("count", len(items)))
	}
	if err != nil || submitOnly {
		for _, it := range items {
			r.enqueue(ctx, string(it.ID), library.SearchOpUpsert)
		}
	}
	return nil
}

func (r *ResilientSearch) RemoveMediaItem(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "search.resilient.RemoveMediaItem")
	defer span.End()

	if r.primary == nil {
		return nil
	}
	w, _ := r.writeTarget()
	if err := w.RemoveMediaItem(ctx, id); err != nil {
		slog.WarnContext(ctx, "search remove: primary failed, queuing for replay", slog.Any("err", err))
		r.enqueue(ctx, id, library.SearchOpDelete)
	}
	return nil
}

// RemoveLibrary purges a deleted library's documents from the primary,
// through the submit-only writer when wired (the deletion does not wait for
// the engine). Not queued on failure: the outbox is per item, and the
// reconcile wipes and rebuilds an index left larger than the database.
func (r *ResilientSearch) RemoveLibrary(ctx context.Context, libraryID string) error {
	ctx, span := tracing.StartSpan(ctx, "search.resilient.RemoveLibrary")
	defer span.End()
	w, _ := r.writeTarget()
	rm, ok := w.(interface {
		RemoveLibrary(ctx context.Context, libraryID string) error
	})
	if !ok {
		return nil
	}
	return rm.RemoveLibrary(ctx, libraryID)
}

func (r *ResilientSearch) Clear(ctx context.Context) error {
	ctx, span := tracing.StartSpan(ctx, "search.resilient.Clear")
	defer span.End()

	if r.primary == nil {
		return nil
	}
	// Unlike the other writes, surface the error: the reconcile clears then
	// reindexes and must NOT proceed to reindex (and log success) if the wipe
	// silently failed - that would leave the stale docs in place.
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
		slog.WarnContext(ctx, "search: primary failed, falling back to SQL", slog.Any("err", err))
	}
	return r.fallback.Search(ctx, query, filters, sort, limit)
}
