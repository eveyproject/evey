package postgres

import (
	"context"
	"database/sql"
	"errors"

	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// PluginDataRepository persists a plugin's own key/value store (table
// plugin_data). The scope is chosen by (userID, profileID), checked in this
// order:
//   - profileID set   -> per-profile (row keyed by (plugin, profile, key);
//     user_id stays NULL since a profile has one owner)
//   - userID set      -> per-user (shared across the user's profiles)
//   - both empty      -> plugin-global (user_id NULL, profile_id NULL)
//
// Each scope has its own partial unique index, so upserts target the matching
// one via a WHERE-qualified ON CONFLICT.
type PluginDataRepository struct {
	db *sql.DB
}

func NewPluginDataRepository(db *sql.DB) *PluginDataRepository {
	return &PluginDataRepository{db: db}
}

func (r *PluginDataRepository) Get(ctx context.Context, pluginID, userID, profileID, key string) (*plugin.PluginDataEntry, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin_data.Get")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	e := &plugin.PluginDataEntry{PluginID: pluginID, UserID: userID, ProfileID: profileID, Key: key}
	var err error
	switch {
	case profileID != "":
		err = txFromCtx(ctx, r.db).QueryRowContext(ctx,
			`SELECT value, updated_at FROM plugin_data WHERE plugin_id=$1 AND profile_id=$2 AND key=$3`,
			pluginID, profileID, key).Scan(&e.Value, &e.UpdatedAt)
	case userID != "":
		err = txFromCtx(ctx, r.db).QueryRowContext(ctx,
			`SELECT value, updated_at FROM plugin_data WHERE plugin_id=$1 AND user_id=$2 AND profile_id IS NULL AND key=$3`,
			pluginID, userID, key).Scan(&e.Value, &e.UpdatedAt)
	default:
		err = txFromCtx(ctx, r.db).QueryRowContext(ctx,
			`SELECT value, updated_at FROM plugin_data WHERE plugin_id=$1 AND user_id IS NULL AND profile_id IS NULL AND key=$2`,
			pluginID, key).Scan(&e.Value, &e.UpdatedAt)
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

func (r *PluginDataRepository) Set(ctx context.Context, pluginID, userID, profileID, key, value string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin_data.Set")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	var err error
	switch {
	case profileID != "":
		_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
			`INSERT INTO plugin_data (plugin_id, profile_id, key, value, updated_at) VALUES ($1, $2, $3, $4, NOW())
			 ON CONFLICT (plugin_id, profile_id, key) WHERE profile_id IS NOT NULL
			 DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
			pluginID, profileID, key, value)
	case userID != "":
		_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
			`INSERT INTO plugin_data (plugin_id, user_id, key, value, updated_at) VALUES ($1, $2, $3, $4, NOW())
			 ON CONFLICT (plugin_id, user_id, key) WHERE user_id IS NOT NULL AND profile_id IS NULL
			 DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
			pluginID, userID, key, value)
	default:
		_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
			`INSERT INTO plugin_data (plugin_id, user_id, key, value, updated_at) VALUES ($1, NULL, $2, $3, NOW())
			 ON CONFLICT (plugin_id, key) WHERE user_id IS NULL AND profile_id IS NULL
			 DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
			pluginID, key, value)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *PluginDataRepository) Delete(ctx context.Context, pluginID, userID, profileID, key string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin_data.Delete")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	var err error
	switch {
	case profileID != "":
		_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
			`DELETE FROM plugin_data WHERE plugin_id=$1 AND profile_id=$2 AND key=$3`, pluginID, profileID, key)
	case userID != "":
		_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
			`DELETE FROM plugin_data WHERE plugin_id=$1 AND user_id=$2 AND profile_id IS NULL AND key=$3`, pluginID, userID, key)
	default:
		_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
			`DELETE FROM plugin_data WHERE plugin_id=$1 AND user_id IS NULL AND profile_id IS NULL AND key=$2`, pluginID, key)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *PluginDataRepository) List(ctx context.Context, pluginID, userID, profileID string) ([]*plugin.PluginDataEntry, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin_data.List")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	var rows *sql.Rows
	var err error
	switch {
	case profileID != "":
		rows, err = txFromCtx(ctx, r.db).QueryContext(ctx,
			`SELECT key, value, updated_at FROM plugin_data WHERE plugin_id=$1 AND profile_id=$2 ORDER BY key`,
			pluginID, profileID)
	case userID != "":
		rows, err = txFromCtx(ctx, r.db).QueryContext(ctx,
			`SELECT key, value, updated_at FROM plugin_data WHERE plugin_id=$1 AND user_id=$2 AND profile_id IS NULL ORDER BY key`,
			pluginID, userID)
	default:
		rows, err = txFromCtx(ctx, r.db).QueryContext(ctx,
			`SELECT key, value, updated_at FROM plugin_data WHERE plugin_id=$1 AND user_id IS NULL AND profile_id IS NULL ORDER BY key`,
			pluginID)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var result []*plugin.PluginDataEntry
	for rows.Next() {
		e := &plugin.PluginDataEntry{PluginID: pluginID, UserID: userID, ProfileID: profileID}
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

func (r *PluginDataRepository) Count(ctx context.Context, pluginID, userID, profileID string) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin_data.Count")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	var n int
	var err error
	switch {
	case profileID != "":
		err = txFromCtx(ctx, r.db).QueryRowContext(ctx,
			`SELECT COUNT(*) FROM plugin_data WHERE plugin_id=$1 AND profile_id=$2`,
			pluginID, profileID).Scan(&n)
	case userID != "":
		err = txFromCtx(ctx, r.db).QueryRowContext(ctx,
			`SELECT COUNT(*) FROM plugin_data WHERE plugin_id=$1 AND user_id=$2 AND profile_id IS NULL`,
			pluginID, userID).Scan(&n)
	default:
		err = txFromCtx(ctx, r.db).QueryRowContext(ctx,
			`SELECT COUNT(*) FROM plugin_data WHERE plugin_id=$1 AND user_id IS NULL AND profile_id IS NULL`,
			pluginID).Scan(&n)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	return n, nil
}

var _ plugin.PluginDataRepository = (*PluginDataRepository)(nil)
