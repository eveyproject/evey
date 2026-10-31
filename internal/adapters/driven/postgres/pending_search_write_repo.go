package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// PendingSearchWriteRepository is the durable outbox for search-index writes the
// engine rejected while it was down (see internal/adapters/driven/search).
type PendingSearchWriteRepository struct {
	db *sql.DB
}

func NewPendingSearchWriteRepository(db *sql.DB) *PendingSearchWriteRepository {
	return &PendingSearchWriteRepository{db: db}
}

// Enqueue records a failed write, last-op-wins per item (a later op overwrites a
// pending earlier one for the same item). A new write also clears the refusal
// count and revives a dead-lettered row: the document it replays has changed.
func (r *PendingSearchWriteRepository) Enqueue(ctx context.Context, mediaItemID, op string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.pending_search_index.Enqueue")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO pending_search_index (media_item_id, op, enqueued_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT (media_item_id) DO UPDATE SET op = EXCLUDED.op, enqueued_at = now(),
		     attempts = 0, dead_lettered_at = NULL`,
		mediaItemID, op)
	return err
}

// List returns up to limit pending ops, oldest first.
func (r *PendingSearchWriteRepository) List(ctx context.Context, limit int) ([]library.PendingSearchWrite, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.pending_search_index.List")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if limit <= 0 {
		limit = 500
	}
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT media_item_id, op, enqueued_at FROM pending_search_index WHERE dead_lettered_at IS NULL ORDER BY enqueued_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []library.PendingSearchWrite
	for rows.Next() {
		var w library.PendingSearchWrite
		if err := rows.Scan(&w.MediaItemID, &w.Op, &w.EnqueuedAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// Delete removes a drained op, unless the row was re-enqueued after w was
// listed: its newer content was never confirmed, so it waits for the next drain.
func (r *PendingSearchWriteRepository) Delete(ctx context.Context, w library.PendingSearchWrite) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.pending_search_index.Delete")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM pending_search_index WHERE media_item_id = $1 AND op = $2 AND enqueued_at <= $3`,
		w.MediaItemID, w.Op, w.EnqueuedAt)
	return err
}

// RecordFailure counts one refusal of the item's document by the engine and
// dead-letters the row once maxAttempts is reached. Reports whether it did. A row
// re-enqueued after w was listed is left alone: the refused document is gone.
func (r *PendingSearchWriteRepository) RecordFailure(ctx context.Context, w library.PendingSearchWrite, maxAttempts int) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.pending_search_index.RecordFailure")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var dead bool
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`UPDATE pending_search_index
		 SET attempts = attempts + 1,
		     dead_lettered_at = CASE WHEN attempts + 1 >= $2 THEN now() END
		 WHERE media_item_id = $1 AND op = $3 AND enqueued_at <= $4
		 RETURNING dead_lettered_at IS NOT NULL`,
		w.MediaItemID, maxAttempts, w.Op, w.EnqueuedAt).Scan(&dead)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return dead, err
}
