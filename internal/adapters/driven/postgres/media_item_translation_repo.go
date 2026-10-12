package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type MediaItemTranslationRepository struct {
	db *sql.DB
}

func NewMediaItemTranslationRepository(db *sql.DB) *MediaItemTranslationRepository {
	return &MediaItemTranslationRepository{db: db}
}

func (r *MediaItemTranslationRepository) Upsert(ctx context.Context, t *library.MediaItemTranslation) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_item_translation.Upsert")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(t.MediaItemID)), attribute.String("locale", t.Locale))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `
		INSERT INTO media_item_translations (media_item_id, locale, title, overview, tagline)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (media_item_id, locale)
		DO UPDATE SET title = EXCLUDED.title, overview = EXCLUDED.overview, tagline = EXCLUDED.tagline
	`, t.MediaItemID, t.Locale, t.Title, t.Overview, t.Tagline)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// UpsertBatch inserts or updates a slice of translations in a single
// multi-row statement. Enrichment typically hits 5-10 locales per item,
// so batching collapses those N round-trips into one.
func (r *MediaItemTranslationRepository) UpsertBatch(ctx context.Context, translations []*library.MediaItemTranslation) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_item_translation.UpsertBatch")
	defer span.End()
	span.SetAttributes(attribute.Int("translation.count", len(translations)))
	if len(translations) == 0 {
		return nil
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	valueClauses := make([]string, len(translations))
	args := make([]any, 0, len(translations)*5)
	for i, t := range translations {
		base := i * 5
		valueClauses[i] = fmt.Sprintf("($%d,$%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4, base+5)
		args = append(args, t.MediaItemID, t.Locale, t.Title, t.Overview, t.Tagline)
	}
	q := `INSERT INTO media_item_translations (media_item_id, locale, title, overview, tagline) VALUES ` +
		strings.Join(valueClauses, ",") +
		` ON CONFLICT (media_item_id, locale) DO UPDATE SET title = EXCLUDED.title, overview = EXCLUDED.overview, tagline = EXCLUDED.tagline`
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, q, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *MediaItemTranslationRepository) GetByMediaItemAndLocale(ctx context.Context, mediaItemID, locale string) (*library.MediaItemTranslation, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_item_translation.GetByMediaItemAndLocale")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID), attribute.String("locale", locale))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	row := txFromCtx(ctx, r.db).QueryRowContext(ctx, `
		SELECT media_item_id, locale, title, overview, tagline
		FROM media_item_translations
		WHERE media_item_id = $1 AND locale = $2
	`, mediaItemID, locale)
	var t library.MediaItemTranslation
	if err := row.Scan(&t.MediaItemID, &t.Locale, &t.Title, &t.Overview, &t.Tagline); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return &t, nil
}

// GetByMediaItemsAndLocale batch-fetches one locale's translations for the
// given items in a single query, keyed by media item ID. Used by list/feed
// surfaces to localize many titles at once instead of N round-trips.
func (r *MediaItemTranslationRepository) GetByMediaItemsAndLocale(ctx context.Context, mediaItemIDs []string, locale string) (map[string]*library.MediaItemTranslation, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_item_translation.GetByMediaItemsAndLocale")
	defer span.End()
	span.SetAttributes(attribute.Int("media_item.count", len(mediaItemIDs)))
	out := make(map[string]*library.MediaItemTranslation, len(mediaItemIDs))
	if len(mediaItemIDs) == 0 || locale == "" {
		return out, nil
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT media_item_id, locale, title, overview, tagline
		FROM media_item_translations
		WHERE media_item_id = ANY($1) AND locale = $2
	`, pq.Array(mediaItemIDs), locale)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var t library.MediaItemTranslation
		if err := rows.Scan(&t.MediaItemID, &t.Locale, &t.Title, &t.Overview, &t.Tagline); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		out[string(t.MediaItemID)] = &t
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return out, nil
}

// GetByMediaItems batch-fetches ALL locales' translations for many items in one
// query, grouped by media item ID. Used by the search reindex to attach every
// translated title to a page of items without N round-trips.
func (r *MediaItemTranslationRepository) GetByMediaItems(ctx context.Context, mediaItemIDs []string) (map[string][]*library.MediaItemTranslation, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_item_translation.GetByMediaItems")
	defer span.End()
	span.SetAttributes(attribute.Int("media_item.count", len(mediaItemIDs)))
	out := make(map[string][]*library.MediaItemTranslation, len(mediaItemIDs))
	if len(mediaItemIDs) == 0 {
		return out, nil
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT media_item_id, locale, title, overview, tagline
		FROM media_item_translations
		WHERE media_item_id = ANY($1)
		ORDER BY media_item_id, locale
	`, pq.Array(mediaItemIDs))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var t library.MediaItemTranslation
		if err := rows.Scan(&t.MediaItemID, &t.Locale, &t.Title, &t.Overview, &t.Tagline); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		id := string(t.MediaItemID)
		out[id] = append(out[id], &t)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return out, nil
}

func (r *MediaItemTranslationRepository) GetByMediaItem(ctx context.Context, mediaItemID string) ([]*library.MediaItemTranslation, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_item_translation.GetByMediaItem")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT media_item_id, locale, title, overview, tagline
		FROM media_item_translations
		WHERE media_item_id = $1
		ORDER BY locale
	`, mediaItemID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	var result []*library.MediaItemTranslation
	for rows.Next() {
		var t library.MediaItemTranslation
		if err := rows.Scan(&t.MediaItemID, &t.Locale, &t.Title, &t.Overview, &t.Tagline); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result = append(result, &t)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *MediaItemTranslationRepository) DeleteByMediaItem(ctx context.Context, mediaItemID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_item_translation.DeleteByMediaItem")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM media_item_translations WHERE media_item_id = $1`, mediaItemID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

var _ library.MediaItemTranslationRepository = (*MediaItemTranslationRepository)(nil)
