package postgres

import (
	"context"
	"database/sql"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/subtitle"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type SubtitleRepository struct {
	db *sql.DB
}

func NewSubtitleRepository(db *sql.DB) *SubtitleRepository {
	return &SubtitleRepository{db: db}
}

// Save upserts on the NATURAL key (media_item_id, file_path) - every call
// generates a fresh id, so a double-click on "download subtitle" used to
// insert two identical rows. On conflict the existing row is refreshed and
// keeps its original id; sub.ID is updated from RETURNING so the caller
// always holds the id that is actually in the table.
func (r *SubtitleRepository) Save(ctx context.Context, sub *subtitle.SubtitleDownload) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.subtitle.Save")
	defer span.End()
	span.SetAttributes(attribute.String("subtitle.id", sub.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`INSERT INTO downloaded_subtitles (id, media_item_id, language, format, file_path, source, downloaded_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (media_item_id, file_path) DO UPDATE SET
		   language = EXCLUDED.language,
		   format = EXCLUDED.format,
		   source = EXCLUDED.source,
		   downloaded_at = EXCLUDED.downloaded_at
		 RETURNING id`,
		sub.ID, sub.MediaItemID, sub.Language, sub.Format, sub.FilePath, sub.Source, sub.DownloadedAt,
	).Scan(&sub.ID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *SubtitleRepository) GetByMediaItem(ctx context.Context, mediaItemID string) ([]*subtitle.SubtitleDownload, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.subtitle.GetByMediaItem")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, media_item_id, language, format, file_path, source, downloaded_at
		 FROM downloaded_subtitles WHERE media_item_id = $1
		 ORDER BY downloaded_at DESC`,
		mediaItemID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	var result []*subtitle.SubtitleDownload
	for rows.Next() {
		s := &subtitle.SubtitleDownload{}
		if err := rows.Scan(&s.ID, &s.MediaItemID, &s.Language, &s.Format, &s.FilePath, &s.Source, &s.DownloadedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *SubtitleRepository) Delete(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.subtitle.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("subtitle.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM downloaded_subtitles WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		slog.Warn("rows affected", slog.Any("err", err))
	}
	if n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
}

var _ subtitle.Repository = (*SubtitleRepository)(nil)
