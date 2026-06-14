package postgres

import (
	"context"
	"database/sql"
	"errors"

	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// PluginDataRepository persists a plugin's own key/value store (table
// plugin_data). userID == "" addresses the plugin-global scope (stored as SQL
// NULL); a non-empty userID addresses per-user data. The two scopes have
// separate partial unique indexes, so upserts target the matching one.
type PluginDataRepository struct {
	db *sql.DB
}

func NewPluginDataRepository(db *sql.DB) *PluginDataRepository {
	return &PluginDataRepository{db: db}
}

func (r *PluginDataRepository) Get(ctx context.Context, pluginID, userID, key string) (*plugin.PluginDataEntry, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin_data.Get")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	e := &plugin.PluginDataEntry{PluginID: pluginID, UserID: userID, Key: key}
	var err error
	if userID == "" {
		err = txFromCtx(ctx, r.db).QueryRowContext(ctx,
			`SELECT value, updated_at FROM plugin_data WHERE plugin_id=$1 AND user_id IS NULL AND key=$2`,
			pluginID, key).Scan(&e.Value, &e.UpdatedAt)
	} else {
		err = txFromCtx(ctx, r.db).QueryRowContext(ctx,
			`SELECT value, updated_at FROM plugin_data WHERE plugin_id=$1 AND user_id=$2 AND key=$3`,
			pluginID, userID, key).Scan(&e.Value, &e.UpdatedAt)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return e, nil
}

func (r *PluginDataRepository) Set(ctx context.Context, pluginID, userID, key, value string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin_data.Set")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	var err error
	if userID == "" {
		_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
			`INSERT INTO plugin_data (plugin_id, user_id, key, value, updated_at) VALUES ($1, NULL, $2, $3, NOW())
			 ON CONFLICT (plugin_id, key) WHERE user_id IS NULL
			 DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
			pluginID, key, value)
	} else {
		_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
			`INSERT INTO plugin_data (plugin_id, user_id, key, value, updated_at) VALUES ($1, $2, $3, $4, NOW())
			 ON CONFLICT (plugin_id, user_id, key) WHERE user_id IS NOT NULL
			 DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
			pluginID, userID, key, value)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *PluginDataRepository) Delete(ctx context.Context, pluginID, userID, key string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin_data.Delete")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	var err error
	if userID == "" {
		_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
			`DELETE FROM plugin_data WHERE plugin_id=$1 AND user_id IS NULL AND key=$2`, pluginID, key)
	} else {
		_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
			`DELETE FROM plugin_data WHERE plugin_id=$1 AND user_id=$2 AND key=$3`, pluginID, userID, key)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *PluginDataRepository) List(ctx context.Context, pluginID, userID string) ([]*plugin.PluginDataEntry, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin_data.List")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	var rows *sql.Rows
	var err error
	if userID == "" {
		rows, err = txFromCtx(ctx, r.db).QueryContext(ctx,
			`SELECT key, value, updated_at FROM plugin_data WHERE plugin_id=$1 AND user_id IS NULL ORDER BY key`,
			pluginID)
	} else {
		rows, err = txFromCtx(ctx, r.db).QueryContext(ctx,
			`SELECT key, value, updated_at FROM plugin_data WHERE plugin_id=$1 AND user_id=$2 ORDER BY key`,
			pluginID, userID)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var result []*plugin.PluginDataEntry
	for rows.Next() {
		e := &plugin.PluginDataEntry{PluginID: pluginID, UserID: userID}
		if err := rows.Scan(&e.Key, &e.Value, &e.UpdatedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result = append(result, e)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

var _ plugin.PluginDataRepository = (*PluginDataRepository)(nil)
