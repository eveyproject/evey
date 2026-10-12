package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type ChapterRepository struct {
	db *sql.DB
}

func NewChapterRepository(db *sql.DB) *ChapterRepository {
	return &ChapterRepository{db: db}
}

func (r *ChapterRepository) ReplaceBatch(ctx context.Context, mediaItemID shared.MediaItemID, chapters []*library.Chapter) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.chapter.ReplaceBatch")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)), attribute.Int("chapter.count", len(chapters)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM chapters WHERE media_item_id = $1`, string(mediaItemID)); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	if len(chapters) == 0 {
		if err := tx.Commit(); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
		return nil
	}

	var b strings.Builder
	b.WriteString(`INSERT INTO chapters (id, media_item_id, title, start_time_ms, end_time_ms, sort_order) VALUES `)
	args := make([]any, 0, len(chapters)*6)
	for i, ch := range chapters {
		if i > 0 {
			b.WriteString(",")
		}
		base := i * 6
		fmt.Fprintf(&b, "($%d,$%d,$%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4, base+5, base+6)
		args = append(args, ch.ID, ch.MediaItemID, ch.Title, ch.StartTimeMs, ch.EndTimeMs, ch.SortOrder)
	}

	if _, err := tx.ExecContext(ctx, b.String(), args...); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if err := tx.Commit(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *ChapterRepository) GetByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) ([]*library.Chapter, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.chapter.GetByMediaItemID")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, media_item_id, title, start_time_ms, end_time_ms, sort_order
		 FROM chapters WHERE media_item_id = $1 ORDER BY sort_order`, string(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var result []*library.Chapter
	for rows.Next() {
		ch := &library.Chapter{}
		if err := rows.Scan(&ch.ID, &ch.MediaItemID, &ch.Title, &ch.StartTimeMs, &ch.EndTimeMs, &ch.SortOrder); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result = append(result, ch)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

var _ library.ChapterRepository = (*ChapterRepository)(nil)
