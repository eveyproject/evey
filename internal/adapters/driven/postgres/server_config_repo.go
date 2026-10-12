package postgres

import (
	"context"
	"database/sql"
	"errors"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type ServerConfigRepository struct {
	db *sql.DB
}

func NewServerConfigRepository(db *sql.DB) *ServerConfigRepository {
	return &ServerConfigRepository{db: db}
}

func (r *ServerConfigRepository) Get(ctx context.Context, key string) (*shared.ServerConfigEntry, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.server_config.Get")
	defer span.End()
	span.SetAttributes(attribute.String("server_config.key", key))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	e := &shared.ServerConfigEntry{}
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT key, value, updated_at FROM server_config WHERE key = $1`, key,
	).Scan(&e.Key, &e.Value, &e.UpdatedAt)
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

func (r *ServerConfigRepository) Set(ctx context.Context, key, value string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.server_config.Set")
	defer span.End()
	span.SetAttributes(attribute.String("server_config.key", key))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO server_config (key, value, updated_at) VALUES ($1, $2, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		key, value,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *ServerConfigRepository) Delete(ctx context.Context, key string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.server_config.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("server_config.key", key))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM server_config WHERE key = $1`, key)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *ServerConfigRepository) List(ctx context.Context) ([]*shared.ServerConfigEntry, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.server_config.List")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `SELECT key, value, updated_at FROM server_config ORDER BY key`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var result []*shared.ServerConfigEntry
	for rows.Next() {
		e := &shared.ServerConfigEntry{}
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

var _ shared.ServerConfigRepository = (*ServerConfigRepository)(nil)
