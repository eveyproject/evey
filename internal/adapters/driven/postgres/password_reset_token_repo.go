package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type PasswordResetTokenRepository struct {
	db *sql.DB
}

func NewPasswordResetTokenRepository(db *sql.DB) *PasswordResetTokenRepository {
	return &PasswordResetTokenRepository{db: db}
}

func (r *PasswordResetTokenRepository) Create(ctx context.Context, t *user.PasswordResetToken) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.password_reset_token.Create")
	defer span.End()
	span.SetAttributes(attribute.String("password_reset_token.id", t.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at, used, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		t.ID, t.UserID, t.TokenHash, t.ExpiresAt, t.Used, t.CreatedAt)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *PasswordResetTokenRepository) GetByTokenHash(ctx context.Context, hash string) (*user.PasswordResetToken, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.password_reset_token.GetByTokenHash")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	t := &user.PasswordResetToken{}
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, user_id, token_hash, expires_at, used, created_at
		 FROM password_reset_tokens
		 WHERE token_hash = $1 AND used = FALSE AND expires_at > NOW()`, hash).
		Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.Used, &t.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return t, nil
}

func (r *PasswordResetTokenRepository) MarkUsed(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.password_reset_token.MarkUsed")
	defer span.End()
	span.SetAttributes(attribute.String("password_reset_token.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// `AND used = FALSE` makes the claim atomic + single-use: two concurrent
	// ResetPassword calls that both validated the (not-yet-used) token race here,
	// and only one UPDATE matches a row. The loser gets rows==0 → an error, which
	// rolls back its enclosing tx (password update + MarkUsed commit together), so
	// the token can never reset the password twice.
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE password_reset_tokens SET used = TRUE WHERE id = $1 AND used = FALSE`, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if n == 0 {
		return fmt.Errorf("password reset token %s already used", id)
	}
	return nil
}

func (r *PasswordResetTokenRepository) DeleteByUserID(ctx context.Context, userID shared.UserID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.password_reset_token.DeleteByUserID")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", string(userID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM password_reset_tokens WHERE user_id = $1`, string(userID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *PasswordResetTokenRepository) DeleteExpired(ctx context.Context) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.password_reset_token.DeleteExpired")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM password_reset_tokens WHERE expires_at < NOW() OR used = TRUE`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}
