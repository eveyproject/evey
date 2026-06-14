package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type MetadataOverrideRepository struct {
	db *sql.DB
}

func NewMetadataOverrideRepository(db *sql.DB) *MetadataOverrideRepository {
	return &MetadataOverrideRepository{db: db}
}

func (r *MetadataOverrideRepository) Set(ctx context.Context, o *metadata.MetadataOverride) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.metadata_override.Set")
	defer span.End()
	span.SetAttributes(attribute.String("metadata_override.id", o.ID), attribute.String("media_item.id", string(o.MediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	valueJSON, err := json.Marshal(o.CustomValue)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO metadata_overrides (id, media_item_id, field_name, custom_value, edited_by, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (media_item_id, field_name) DO UPDATE SET
		   custom_value = EXCLUDED.custom_value,
		   edited_by = EXCLUDED.edited_by`,
		o.ID, o.MediaItemID, o.FieldName, valueJSON, o.EditedBy, o.CreatedAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *MetadataOverrideRepository) Get(ctx context.Context, id string) (*metadata.MetadataOverride, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.metadata_override.Get")
	defer span.End()
	span.SetAttributes(attribute.String("metadata_override.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	o := &metadata.MetadataOverride{}
	var valueJSON []byte
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, media_item_id, field_name, custom_value, edited_by, created_at
		 FROM metadata_overrides WHERE id = $1`, id,
	).Scan(&o.ID, &o.MediaItemID, &o.FieldName, &valueJSON, &o.EditedBy, &o.CreatedAt)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	if err := json.Unmarshal(valueJSON, &o.CustomValue); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "custom_value"), slog.Any("err", err))
	}
	return o, nil
}

func (r *MetadataOverrideRepository) GetByMediaItem(ctx context.Context, mediaItemID string) ([]*metadata.MetadataOverride, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.metadata_override.GetByMediaItem")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, media_item_id, field_name, custom_value, edited_by, created_at
		 FROM metadata_overrides WHERE media_item_id = $1 ORDER BY field_name`, mediaItemID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var result []*metadata.MetadataOverride
	for rows.Next() {
		o := &metadata.MetadataOverride{}
		var valueJSON []byte
		if err := rows.Scan(&o.ID, &o.MediaItemID, &o.FieldName, &valueJSON, &o.EditedBy, &o.CreatedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		if err := json.Unmarshal(valueJSON, &o.CustomValue); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "custom_value"), slog.Any("err", err))
		}
		result = append(result, o)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *MetadataOverrideRepository) ListOverriddenMediaItemIDs(ctx context.Context, mediaItemIDs []string) (map[string]bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.metadata_override.ListOverriddenMediaItemIDs")
	defer span.End()
	span.SetAttributes(attribute.Int("media_item.count", len(mediaItemIDs)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if len(mediaItemIDs) == 0 {
		return make(map[string]bool), nil
	}
	placeholders := make([]string, len(mediaItemIDs))
	args := make([]any, len(mediaItemIDs))
	for i, id := range mediaItemIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	query := fmt.Sprintf(
		`SELECT DISTINCT media_item_id FROM metadata_overrides WHERE media_item_id IN (%s)`,
		strings.Join(placeholders, ","),
	)
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result[id] = true
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *MetadataOverrideRepository) Delete(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.metadata_override.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("metadata_override.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM metadata_overrides WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *MetadataOverrideRepository) DeleteByMediaItemID(ctx context.Context, mediaItemID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.metadata_override.DeleteByMediaItemID")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM metadata_overrides WHERE media_item_id = $1`, mediaItemID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

var _ metadata.MetadataOverrideRepository = (*MetadataOverrideRepository)(nil)
