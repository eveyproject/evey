package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

const drainBatchSize = 500

// maxSearchWriteAttempts is how many drains a document the engine refuses gets
// before it is dead-lettered, so it stops being replayed every minute.
const maxSearchWriteAttempts = 5

// searchWriteFailureRecorder is the outbox side of a refused document: it
// counts the attempt and dead-letters the row past maxAttempts.
type searchWriteFailureRecorder interface {
	RecordFailure(ctx context.Context, mediaItemID string, maxAttempts int) (deadLettered bool, err error)
}

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
// loss (empty engine vs full DB - restore, version change, corruption), where no
// writes are in flight to replay.
func DrainSearchWriteQueue(
	ctx context.Context,
	queue library.PendingSearchWriteQueue,
	items library.MediaItemQueryRepo,
	search library.SearchIndex,
	translations reindexTranslationReader,
	cast reindexCastReader,
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

		// Collect the upsert IDs from this page and re-read them from the DB
		// with a SINGLE GetByIDs call, then index the found ones with a SINGLE
		// IndexMediaItemsBatch call - instead of one GetByID + one
		// IndexMediaItemsBatch([1]) per item, which was thousands of round trips
		// on a large backlog after an outage. queue.Delete has no batch
		// equivalent, so it stays per-item below.
		var upsertIDs []shared.MediaItemID
		for _, p := range pending {
			if p.Op != library.SearchOpDelete {
				upsertIDs = append(upsertIDs, shared.MediaItemID(p.MediaItemID))
			}
		}
		found := make(map[shared.MediaItemID]*library.MediaItem, len(upsertIDs))
		refused := map[string]bool{}
		var recorder searchWriteFailureRecorder
		if len(upsertIDs) > 0 {
			dbItems, gerr := items.GetByIDs(ctx, upsertIDs)
			if gerr != nil {
				return drained, fmt.Errorf("drain: get pending items: %w", gerr)
			}
			for _, item := range dbItems {
				found[item.ID] = item
			}
			if len(dbItems) > 0 {
				attachTranslations(ctx, translations, dbItems)
				attachCast(ctx, cast, dbItems)
				if ierr := search.IndexMediaItemsBatch(ctx, dbItems); ierr != nil {
					// The engine indexed the page except the documents it refused:
					// count those instead of replaying the page forever behind them.
					var rej interface{ RejectedIDs() []string }
					rec, canRecord := queue.(searchWriteFailureRecorder)
					if !errors.As(ierr, &rej) || !canRecord {
						// Engine still down / transient - stop before deleting ANY op from
						// this page (including delete-ops below), unlike the previous
						// per-item stop. Every op here is idempotent (upsert/delete), so
						// replaying the whole page again on the next tick is safe.
						return drained, fmt.Errorf("drain: index batch: %w", ierr)
					}
					recorder = rec
					for _, id := range rej.RejectedIDs() {
						refused[id] = true
					}
				}
			}
		}

		for _, p := range pending {
			if ctx.Err() != nil {
				return drained, ctx.Err()
			}
			if refused[p.MediaItemID] {
				dead, rerr := recorder.RecordFailure(ctx, p.MediaItemID, maxSearchWriteAttempts)
				if rerr != nil {
					return drained, fmt.Errorf("drain: record refused %s: %w", p.MediaItemID, rerr)
				}
				if dead {
					slog.WarnContext(ctx, "search write dead-lettered: the engine keeps refusing the document",
						slog.String("media_item_id", p.MediaItemID), slog.Int("attempts", maxSearchWriteAttempts))
				}
				continue
			}
			var opErr error
			switch {
			case p.Op == library.SearchOpDelete:
				opErr = search.RemoveMediaItem(ctx, p.MediaItemID)
			case found[shared.MediaItemID(p.MediaItemID)] == nil:
				// Deleted between enqueue and drain - ensure the index has no stale
				// doc, then drop the pending op.
				opErr = search.RemoveMediaItem(ctx, p.MediaItemID)
			default:
				// Already indexed in the batch call above.
			}
			if opErr != nil {
				// Engine still down / transient - stop; the rest stays queued for the
				// next tick (returning the error surfaces it, unlike a swallowed write).
				return drained, fmt.Errorf("drain: replay %s %s: %w", p.Op, p.MediaItemID, opErr)
			}
			if err := queue.Delete(ctx, p.MediaItemID); err != nil {
				return drained, fmt.Errorf("drain: delete drained %s: %w", p.MediaItemID, err)
			}
			drained++
		}
		// A refused row stays listed until dead-lettered: one attempt per run.
		if len(pending) < drainBatchSize || len(refused) > 0 {
			break
		}
	}
	if drained > 0 {
		slog.InfoContext(ctx, "search write-queue drained", slog.Int("count", drained))
	}
	return drained, nil
}
