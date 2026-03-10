package postgres

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/gravity-zero/evey/internal/domain/metadata"
)

type MetadataOverrideRepository struct {
	db *sql.DB
}

func NewMetadataOverrideRepository(db *sql.DB) *MetadataOverrideRepository {
	return &MetadataOverrideRepository{db: db}
}

func (r *MetadataOverrideRepository) Set(ctx context.Context, o *metadata.MetadataOverride) error {
	valueJSON, err := json.Marshal(o.CustomValue)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO metadata_overrides (id, media_item_id, field_name, custom_value, edited_by, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (media_item_id, field_name) DO UPDATE SET
		   custom_value = EXCLUDED.custom_value,
		   edited_by = EXCLUDED.edited_by,
		   created_at = EXCLUDED.created_at`,
		o.ID, o.MediaItemID, o.FieldName, valueJSON, o.EditedBy, o.CreatedAt,
	)
	return err
}

func (r *MetadataOverrideRepository) Get(ctx context.Context, id string) (*metadata.MetadataOverride, error) {
	o := &metadata.MetadataOverride{}
	var valueJSON []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT id, media_item_id, field_name, custom_value, edited_by, created_at
		 FROM metadata_overrides WHERE id = $1`, id,
	).Scan(&o.ID, &o.MediaItemID, &o.FieldName, &valueJSON, &o.EditedBy, &o.CreatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(valueJSON, &o.CustomValue)
	return o, nil
}

func (r *MetadataOverrideRepository) GetByMediaItem(ctx context.Context, mediaItemID string) ([]*metadata.MetadataOverride, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, media_item_id, field_name, custom_value, edited_by, created_at
		 FROM metadata_overrides WHERE media_item_id = $1 ORDER BY field_name`, mediaItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*metadata.MetadataOverride
	for rows.Next() {
		o := &metadata.MetadataOverride{}
		var valueJSON []byte
		if err := rows.Scan(&o.ID, &o.MediaItemID, &o.FieldName, &valueJSON, &o.EditedBy, &o.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(valueJSON, &o.CustomValue)
		result = append(result, o)
	}
	return result, rows.Err()
}

func (r *MetadataOverrideRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM metadata_overrides WHERE id = $1`, id)
	return err
}

func (r *MetadataOverrideRepository) DeleteByMediaItemID(ctx context.Context, mediaItemID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM metadata_overrides WHERE media_item_id = $1`, mediaItemID)
	return err
}
