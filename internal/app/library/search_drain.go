package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

const drainBatchSize = 500

// DrainSearchWriteQueue replays the durable outbox: writes the search engine
// rejected while it was down (recorded by the resilient wrapper). Each op is
// re-applied to the PRIMARY engine (so a still-down engine surfaces its error
// instead of the resilient wrapper swallowing it into an infinite loop), and the
// item is re-read from the DB (the source of truth) so a value that changed since
// it was enqueued is indexed at its current state. On the first replay failure it
// stops and leaves the rest queued for the next tick. Returns the number drained.
//
// This is the primary drift-prevention path (no writes are lost). The count-based
// ReconcileSearchIndex stays for a DIFFERENT case it handles well: a total index
// loss (empty engine vs full DB — restore, version change, corruption), where no
// writes are in flight to replay.
func DrainSearchWriteQueue(
	ctx context.Context,
	queue library.PendingSearchWriteQueue,
	items library.MediaItemQueryRepo,
	search library.SearchIndex,
	translations reindexTranslationReader,
) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.DrainSearchWriteQueue")
	defer span.End()

	drained := 0
	// Loop batch-by-batch until the queue is empty (or the caller's ctx/timeout
	// bounds the run), so a large backlog after a long outage recovers in one pass
	// instead of one 500-item batch per tick.
	for {
		pending, err := queue.List(ctx, drainBatchSize)
		if err != nil {
			return drained, fmt.Errorf("drain: list pending: %w", err)
		}
		if len(pending) == 0 {
			break
		}
		for _, p := range pending {
			if ctx.Err() != nil {
				return drained, ctx.Err()
			}
			var opErr error
			switch p.Op {
			case library.SearchOpDelete:
				opErr = search.RemoveMediaItem(ctx, p.MediaItemID)
			default: // upsert
				item, gerr := items.GetByID(ctx, shared.MediaItemID(p.MediaItemID))
				switch {
				case errors.Is(gerr, shared.ErrNotFound):
					// Deleted between enqueue and drain — ensure the index has no stale
					// doc, then drop the pending op.
					opErr = search.RemoveMediaItem(ctx, p.MediaItemID)
				case gerr != nil:
					opErr = gerr
				default:
					batch := []*library.MediaItem{item}
					attachTranslations(ctx, translations, batch)
					opErr = search.IndexMediaItemsBatch(ctx, batch)
				}
			}
			if opErr != nil {
				// Engine still down / transient — stop; the rest stays queued for the
				// next tick (returning the error surfaces it, unlike a swallowed write).
				return drained, fmt.Errorf("drain: replay %s %s: %w", p.Op, p.MediaItemID, opErr)
			}
			if err := queue.Delete(ctx, p.MediaItemID); err != nil {
				return drained, fmt.Errorf("drain: delete drained %s: %w", p.MediaItemID, err)
			}
			drained++
		}
		if len(pending) < drainBatchSize {
			break
		}
	}
	if drained > 0 {
		slog.Info("search write-queue drained", slog.Int("count", drained))
	}
	return drained, nil
}
