package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// PluginMetadataSettingsRepository keeps the fields each metadata plugin may
// replace (plugin_metadata_settings).
type PluginMetadataSettingsRepository struct {
	db *sql.DB
}

func NewPluginMetadataSettingsRepository(db *sql.DB) *PluginMetadataSettingsRepository {
	return &PluginMetadataSettingsRepository{db: db}
}

func (r *PluginMetadataSettingsRepository) MetadataFields(ctx context.Context, pluginID string) ([]string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin_metadata_settings.MetadataFields")
	defer span.End()
	span.SetAttributes(attribute.String("plugin.id", pluginID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var fields []string
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx, `SELECT fields FROM plugin_metadata_settings WHERE plugin_id = $1`, pluginID).Scan(pq.Array(&fields))
	if errors.Is(err, sql.ErrNoRows) {
		return append([]string(nil), plugin.DefaultMetadataFields...), nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	if fields == nil {
		fields = []string{}
	}
	return fields, nil
}

func (r *PluginMetadataSettingsRepository) SetMetadataFields(ctx context.Context, pluginID string, fields []string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin_metadata_settings.SetMetadataFields")
	defer span.End()
	span.SetAttributes(attribute.String("plugin.id", pluginID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if fields == nil {
		fields = []string{}
	}
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `
		INSERT INTO plugin_metadata_settings (plugin_id, fields) VALUES ($1, $2)
		ON CONFLICT (plugin_id) DO UPDATE SET fields = EXCLUDED.fields
	`, pluginID, pq.Array(fields))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}
