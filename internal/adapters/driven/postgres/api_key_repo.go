package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type APIKeyRepository struct {
	db *sql.DB
}

func NewAPIKeyRepository(db *sql.DB) *APIKeyRepository {
	return &APIKeyRepository{db: db}
}

func (r *APIKeyRepository) Create(ctx context.Context, key *user.APIKeyModel) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.api_key.Create")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	permJSON, err := json.Marshal(key.Permissions)
	if err != nil {
		return err
	}
	_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO api_keys (id, user_id, key_hash, name, permissions, last_used_at, expires_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		key.ID, key.UserID, key.KeyHash, key.Name, permJSON, key.LastUsedAt, key.ExpiresAt, key.CreatedAt,
	)
	return err
}

func (r *APIKeyRepository) GetByKeyHash(ctx context.Context, hash string) (*user.APIKeyModel, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.api_key.GetByKeyHash")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	key := &user.APIKeyModel{}
	var permJSON []byte
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, user_id, key_hash, name, permissions, last_used_at, expires_at, created_at
		 FROM api_keys WHERE key_hash = $1`, hash,
	).Scan(&key.ID, &key.UserID, &key.KeyHash, &key.Name, &permJSON, &key.LastUsedAt, &key.ExpiresAt, &key.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(permJSON, &key.Permissions); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "permissions"), slog.Any("err", err))
	}
	return key, nil
}

func (r *APIKeyRepository) ListByUserID(ctx context.Context, userID shared.UserID) ([]*user.APIKeyModel, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.api_key.ListByUserID")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, user_id, key_hash, name, permissions, last_used_at, expires_at, created_at
		 FROM api_keys WHERE user_id = $1 ORDER BY created_at LIMIT 100`, string(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*user.APIKeyModel
	for rows.Next() {
		key := &user.APIKeyModel{}
		var permJSON []byte
		if err := rows.Scan(&key.ID, &key.UserID, &key.KeyHash, &key.Name, &permJSON, &key.LastUsedAt, &key.ExpiresAt, &key.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(permJSON, &key.Permissions); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "permissions"), slog.Any("err", err))
		}
		result = append(result, key)
	}
	return result, rows.Err()
}

func (r *APIKeyRepository) Delete(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.api_key.Delete")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM api_keys WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *APIKeyRepository) UpdateLastUsed(ctx context.Context, id string, t time.Time) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.api_key.UpdateLastUsed")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE api_keys SET last_used_at = $1 WHERE id = $2`, t, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return shared.ErrNotFound
	}
	return nil
}

var _ user.APIKeyRepository = (*APIKeyRepository)(nil)
