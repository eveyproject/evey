package postgres

import (
	"context"
	"database/sql"

	"github.com/gravity-zero/evey/internal/domain/plugin"
)

type PluginRuntimeRepository struct {
	db *sql.DB
}

func NewPluginRuntimeRepository(db *sql.DB) *PluginRuntimeRepository {
	return &PluginRuntimeRepository{db: db}
}

func (r *PluginRuntimeRepository) Create(ctx context.Context, rt *plugin.PluginRuntime) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO plugin_runtimes (id, language, version, status, notes, tested_at)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		rt.ID, rt.Language, rt.Version, rt.Status, rt.Notes, rt.TestedAt,
	)
	return err
}

func (r *PluginRuntimeRepository) GetByLanguageAndVersion(ctx context.Context, language, version string) (*plugin.PluginRuntime, error) {
	rt := &plugin.PluginRuntime{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, language, version, status, notes, tested_at
		 FROM plugin_runtimes WHERE language = $1 AND version = $2`, language, version,
	).Scan(&rt.ID, &rt.Language, &rt.Version, &rt.Status, &rt.Notes, &rt.TestedAt)
	if err != nil {
		return nil, err
	}
	return rt, nil
}

func (r *PluginRuntimeRepository) ListByLanguage(ctx context.Context, language string) ([]*plugin.PluginRuntime, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, language, version, status, notes, tested_at
		 FROM plugin_runtimes WHERE language = $1 ORDER BY version LIMIT 100`, language)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanRuntimeRows(rows)
}

func (r *PluginRuntimeRepository) List(ctx context.Context) ([]*plugin.PluginRuntime, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, language, version, status, notes, tested_at
		 FROM plugin_runtimes ORDER BY language, version LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanRuntimeRows(rows)
}

func (r *PluginRuntimeRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM plugin_runtimes WHERE id = $1`, id)
	return err
}

func (r *PluginRuntimeRepository) scanRuntimeRows(rows *sql.Rows) ([]*plugin.PluginRuntime, error) {
	var result []*plugin.PluginRuntime
	for rows.Next() {
		rt := &plugin.PluginRuntime{}
		if err := rows.Scan(&rt.ID, &rt.Language, &rt.Version, &rt.Status, &rt.Notes, &rt.TestedAt); err != nil {
			return nil, err
		}
		result = append(result, rt)
	}
	return result, rows.Err()
}

var _ plugin.PluginRuntimeRepository = (*PluginRuntimeRepository)(nil)
