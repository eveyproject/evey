package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/user"
)

type PasswordResetTokenRepository struct {
	db *sql.DB
}

func NewPasswordResetTokenRepository(db *sql.DB) *PasswordResetTokenRepository {
	return &PasswordResetTokenRepository{db: db}
}

func (r *PasswordResetTokenRepository) Create(ctx context.Context, t *user.PasswordResetToken) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at, used, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		t.ID, t.UserID, t.TokenHash, t.ExpiresAt, t.Used, t.CreatedAt)
	return err
}

func (r *PasswordResetTokenRepository) GetByTokenHash(ctx context.Context, hash string) (*user.PasswordResetToken, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	t := &user.PasswordResetToken{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, token_hash, expires_at, used, created_at
		 FROM password_reset_tokens
		 WHERE token_hash = $1 AND used = FALSE AND expires_at > NOW()`, hash).
		Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.Used, &t.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, err
	}
	return t, nil
}

func (r *PasswordResetTokenRepository) MarkUsed(ctx context.Context, id string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`UPDATE password_reset_tokens SET used = TRUE WHERE id = $1`, id)
	return err
}

func (r *PasswordResetTokenRepository) DeleteByUserID(ctx context.Context, userID string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM password_reset_tokens WHERE user_id = $1`, userID)
	return err
}

func (r *PasswordResetTokenRepository) DeleteExpired(ctx context.Context) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM password_reset_tokens WHERE expires_at < NOW() OR used = TRUE`)
	return err
}
