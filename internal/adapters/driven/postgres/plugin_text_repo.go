package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// PluginTextRepository keeps the metadata plugins' texts
// (media_item_plugin_texts) and reads them through the administrator's
// per-plugin field choice (plugin_metadata_settings).
type PluginTextRepository struct {
	db *sql.DB
}

func NewPluginTextRepository(db *sql.DB) *PluginTextRepository {
	return &PluginTextRepository{db: db}
}

// ReplaceForItem is library.PluginTextRepository: one transaction drops what
// the plugin gave for the item before and writes what it gives now.
func (r *PluginTextRepository) ReplaceForItem(ctx context.Context, mediaItemID, pluginID string, texts []library.PluginText) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin_text.ReplaceForItem")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID), attribute.String("plugin.id", pluginID), attribute.Int("text.count", len(texts)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	fail := func(err error) error {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM media_item_plugin_texts WHERE media_item_id = $1 AND plugin_id = $2`, mediaItemID, pluginID); err != nil {
		return fail(err)
	}
	for _, t := range texts {
		if t.Empty() || t.Locale == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO media_item_plugin_texts (media_item_id, plugin_id, locale, title, overview, tagline, fetched_at)
			VALUES ($1, $2, $3, $4, $5, $6, now())
			ON CONFLICT (media_item_id, plugin_id, locale)
			DO UPDATE SET title = EXCLUDED.title, overview = EXCLUDED.overview, tagline = EXCLUDED.tagline, fetched_at = now()
		`, mediaItemID, pluginID, t.Locale, t.Title, t.Overview, t.Tagline); err != nil {
			return fail(fmt.Errorf("locale %s: %w", t.Locale, err))
		}
	}
	if err := tx.Commit(); err != nil {
		return fail(err)
	}
	return nil
}

// DisplayedForLocale is library.PluginTextRepository: the field filter runs in
// the query, so a field the administrator did not allow never leaves the
// database.
func (r *PluginTextRepository) DisplayedForLocale(ctx context.Context, mediaItemIDs []string, locale string) (map[string][]library.PluginText, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin_text.DisplayedForLocale")
	defer span.End()
	span.SetAttributes(attribute.Int("media_item.count", len(mediaItemIDs)), attribute.String("locale", locale))
	out := make(map[string][]library.PluginText)
	if len(mediaItemIDs) == 0 || locale == "" {
		return out, nil
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT t.media_item_id, t.plugin_id, t.locale,
		       CASE WHEN 'title' = ANY(f.fields) THEN t.title ELSE '' END,
		       CASE WHEN 'overview' = ANY(f.fields) THEN t.overview ELSE '' END,
		       CASE WHEN 'tagline' = ANY(f.fields) THEN t.tagline ELSE '' END
		FROM media_item_plugin_texts t
		JOIN plugins p ON p.id = t.plugin_id AND p.enabled
		CROSS JOIN LATERAL (
			SELECT COALESCE((SELECT s.fields FROM plugin_metadata_settings s WHERE s.plugin_id = t.plugin_id), $3::text[]) AS fields
		) f
		WHERE t.media_item_id = ANY($1) AND t.locale = $2
		ORDER BY t.media_item_id, t.plugin_id
	`, pq.Array(mediaItemIDs), locale, pq.Array(plugin.DefaultMetadataFields))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var t library.PluginText
		if err := rows.Scan(&t.MediaItemID, &t.PluginID, &t.Locale, &t.Title, &t.Overview, &t.Tagline); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		if t.Empty() {
			continue
		}
		out[t.MediaItemID] = append(out[t.MediaItemID], t)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return out, nil
}
