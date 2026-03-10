package postgres

import (
	"context"
	"database/sql"

	"github.com/gravity-zero/evey/internal/domain/user"
)

type RefreshTokenRepository struct {
	db *sql.DB
}

func NewRefreshTokenRepository(db *sql.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

func (r *RefreshTokenRepository) Create(ctx context.Context, t *user.RefreshToken) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, device_name, client_ip, expires_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		t.ID, t.UserID, t.TokenHash, t.DeviceName, t.ClientIP, t.ExpiresAt, t.CreatedAt,
	)
	return err
}

func (r *RefreshTokenRepository) GetByTokenHash(ctx context.Context, hash string) (*user.RefreshToken, error) {
	t := &user.RefreshToken{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, token_hash, device_name, client_ip, used, expires_at, created_at
		 FROM refresh_tokens WHERE token_hash = $1`, hash,
	).Scan(&t.ID, &t.UserID, &t.TokenHash, &t.DeviceName, &t.ClientIP, &t.Used, &t.ExpiresAt, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (r *RefreshTokenRepository) MarkUsed(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE refresh_tokens SET used = TRUE WHERE id = $1`, id)
	return err
}

func (r *RefreshTokenRepository) DeleteByID(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE id = $1`, id)
	return err
}

func (r *RefreshTokenRepository) DeleteByUserID(ctx context.Context, userID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, userID)
	return err
}

func (r *RefreshTokenRepository) DeleteExpired(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE expires_at < NOW()`)
	return err
}
