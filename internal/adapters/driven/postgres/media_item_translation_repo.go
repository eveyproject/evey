package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type MediaItemTranslationRepository struct {
	db *sql.DB
}

func NewMediaItemTranslationRepository(db *sql.DB) *MediaItemTranslationRepository {
	return &MediaItemTranslationRepository{db: db}
}

func (r *MediaItemTranslationRepository) Upsert(ctx context.Context, t *library.MediaItemTranslation) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO media_item_translations (media_item_id, locale, title, overview, tagline)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (media_item_id, locale)
		DO UPDATE SET title = EXCLUDED.title, overview = EXCLUDED.overview, tagline = EXCLUDED.tagline
	`, t.MediaItemID, t.Locale, t.Title, t.Overview, t.Tagline)
	return err
}

func (r *MediaItemTranslationRepository) GetByMediaItemAndLocale(ctx context.Context, mediaItemID, locale string) (*library.MediaItemTranslation, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT media_item_id, locale, title, overview, tagline
		FROM media_item_translations
		WHERE media_item_id = $1 AND locale = $2
	`, mediaItemID, locale)
	var t library.MediaItemTranslation
	if err := row.Scan(&t.MediaItemID, &t.Locale, &t.Title, &t.Overview, &t.Tagline); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (r *MediaItemTranslationRepository) GetByMediaItem(ctx context.Context, mediaItemID string) ([]*library.MediaItemTranslation, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT media_item_id, locale, title, overview, tagline
		FROM media_item_translations
		WHERE media_item_id = $1
	`, mediaItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*library.MediaItemTranslation
	for rows.Next() {
		var t library.MediaItemTranslation
		if err := rows.Scan(&t.MediaItemID, &t.Locale, &t.Title, &t.Overview, &t.Tagline); err != nil {
			return nil, err
		}
		result = append(result, &t)
	}
	return result, rows.Err()
}

func (r *MediaItemTranslationRepository) DeleteByMediaItem(ctx context.Context, mediaItemID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM media_item_translations WHERE media_item_id = $1`, mediaItemID)
	return err
}

var _ library.MediaItemTranslationRepository = (*MediaItemTranslationRepository)(nil)
