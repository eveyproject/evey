package postgres

import (
	"context"
	"database/sql"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type PluginRuntimeRepository struct {
	db *sql.DB
}

func NewPluginRuntimeRepository(db *sql.DB) *PluginRuntimeRepository {
	return &PluginRuntimeRepository{db: db}
}

func (r *PluginRuntimeRepository) Create(ctx context.Context, rt *plugin.PluginRuntime) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin_runtime.Create")
	defer span.End()
	span.SetAttributes(attribute.String("plugin_runtime.id", rt.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO plugin_runtimes (id, language, version, status, notes, tested_at)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		rt.ID, rt.Language, rt.Version, rt.Status, rt.Notes, rt.TestedAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *PluginRuntimeRepository) GetByLanguageAndVersion(ctx context.Context, language, version string) (*plugin.PluginRuntime, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin_runtime.GetByLanguageAndVersion")
	defer span.End()
	span.SetAttributes(attribute.String("plugin_runtime.language", language), attribute.String("plugin_runtime.version", version))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rt := &plugin.PluginRuntime{}
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, language, version, status, notes, tested_at
		 FROM plugin_runtimes WHERE language = $1 AND version = $2`, language, version,
	).Scan(&rt.ID, &rt.Language, &rt.Version, &rt.Status, &rt.Notes, &rt.TestedAt)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return rt, nil
}

func (r *PluginRuntimeRepository) ListByLanguage(ctx context.Context, language string) ([]*plugin.PluginRuntime, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin_runtime.ListByLanguage")
	defer span.End()
	span.SetAttributes(attribute.String("plugin_runtime.language", language))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, language, version, status, notes, tested_at
		 FROM plugin_runtimes WHERE language = $1 ORDER BY version LIMIT 100`, language)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	result, err := r.scanRuntimeRows(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *PluginRuntimeRepository) List(ctx context.Context) ([]*plugin.PluginRuntime, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin_runtime.List")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, language, version, status, notes, tested_at
		 FROM plugin_runtimes ORDER BY language, version LIMIT 1000`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	result, err := r.scanRuntimeRows(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *PluginRuntimeRepository) Delete(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin_runtime.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("plugin_runtime.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM plugin_runtimes WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
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
