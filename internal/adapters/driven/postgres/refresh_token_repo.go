package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
)

type RefreshTokenRepository struct {
	db *sql.DB
}

func NewRefreshTokenRepository(db *sql.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

func (r *RefreshTokenRepository) Create(ctx context.Context, t *user.RefreshToken) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, device_name, client_ip, expires_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		t.ID, t.UserID, t.TokenHash, t.DeviceName, t.ClientIP, t.ExpiresAt, t.CreatedAt,
	)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return fmt.Errorf("%w: %s", shared.ErrConflict, pqErr.Message)
		}
	}
	return err
}

func (r *RefreshTokenRepository) GetByTokenHash(ctx context.Context, hash string) (*user.RefreshToken, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
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

func (r *RefreshTokenRepository) MarkUsedIfNotUsed(ctx context.Context, id string) (bool, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := r.db.ExecContext(ctx, `UPDATE refresh_tokens SET used = TRUE WHERE id = $1 AND used = FALSE`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *RefreshTokenRepository) DeleteByID(ctx context.Context, id string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE id = $1`, id)
	return err
}

func (r *RefreshTokenRepository) DeleteByUserID(ctx context.Context, userID shared.UserID) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, string(userID))
	return err
}

func (r *RefreshTokenRepository) DeleteExpired(ctx context.Context) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE expires_at < NOW()`)
	return err
}

var _ user.RefreshTokenRepository = (*RefreshTokenRepository)(nil)
