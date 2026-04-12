package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/gravity-zero/evey/internal/domain/shared"
)

type ServerConfigRepository struct {
	db *sql.DB
}

func NewServerConfigRepository(db *sql.DB) *ServerConfigRepository {
	return &ServerConfigRepository{db: db}
}

func (r *ServerConfigRepository) Get(ctx context.Context, key string) (*shared.ServerConfigEntry, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	e := &shared.ServerConfigEntry{}
	err := r.db.QueryRowContext(ctx,
		`SELECT key, value, updated_at FROM server_config WHERE key = $1`, key,
	).Scan(&e.Key, &e.Value, &e.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, err
	}
	return e, nil
}

func (r *ServerConfigRepository) Set(ctx context.Context, key, value string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO server_config (key, value, updated_at) VALUES ($1, $2, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		key, value,
	)
	return err
}

func (r *ServerConfigRepository) Delete(ctx context.Context, key string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `DELETE FROM server_config WHERE key = $1`, key)
	return err
}

func (r *ServerConfigRepository) List(ctx context.Context) ([]*shared.ServerConfigEntry, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx, `SELECT key, value, updated_at FROM server_config ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*shared.ServerConfigEntry
	for rows.Next() {
		e := &shared.ServerConfigEntry{}
		if err := rows.Scan(&e.Key, &e.Value, &e.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

var _ shared.ServerConfigRepository = (*ServerConfigRepository)(nil)
