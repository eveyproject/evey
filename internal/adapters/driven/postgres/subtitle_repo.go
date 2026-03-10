package postgres

import (
	"context"
	"database/sql"

	"github.com/gravity-zero/evey/internal/domain/subtitle"
)

type SubtitleRepository struct {
	db *sql.DB
}

func NewSubtitleRepository(db *sql.DB) *SubtitleRepository {
	return &SubtitleRepository{db: db}
}

func (r *SubtitleRepository) Save(ctx context.Context, sub *subtitle.SubtitleDownload) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO downloaded_subtitles (id, media_item_id, language, format, file_path, source, downloaded_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (id) DO UPDATE SET
		   language = EXCLUDED.language,
		   format = EXCLUDED.format,
		   file_path = EXCLUDED.file_path,
		   source = EXCLUDED.source,
		   downloaded_at = EXCLUDED.downloaded_at`,
		sub.ID, sub.MediaItemID, sub.Language, sub.Format, sub.FilePath, sub.Source, sub.DownloadedAt,
	)
	return err
}

func (r *SubtitleRepository) GetByMediaItem(ctx context.Context, mediaItemID string) ([]*subtitle.SubtitleDownload, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, media_item_id, language, format, file_path, source, downloaded_at
		 FROM downloaded_subtitles WHERE media_item_id = $1
		 ORDER BY downloaded_at DESC`,
		mediaItemID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*subtitle.SubtitleDownload
	for rows.Next() {
		s := &subtitle.SubtitleDownload{}
		if err := rows.Scan(&s.ID, &s.MediaItemID, &s.Language, &s.Format, &s.FilePath, &s.Source, &s.DownloadedAt); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *SubtitleRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM downloaded_subtitles WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return subtitle.ErrNotFound
	}
	return nil
}
