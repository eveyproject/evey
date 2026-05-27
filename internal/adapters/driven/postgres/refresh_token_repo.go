package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type RefreshTokenRepository struct {
	db *sql.DB
}

func NewRefreshTokenRepository(db *sql.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

func (r *RefreshTokenRepository) Create(ctx context.Context, t *user.RefreshToken) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.refresh_token.Create")
	defer span.End()
	span.SetAttributes(attribute.String("refresh_token.id", t.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var profileID any
	if t.ProfileID != nil {
		profileID = string(*t.ProfileID)
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, device_name, client_ip, expires_at, created_at, profile_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		t.ID, t.UserID, t.TokenHash, t.DeviceName, t.ClientIP, t.ExpiresAt, t.CreatedAt, profileID,
	)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			conflictErr := fmt.Errorf("%w: %s", shared.ErrConflict, pqErr.Message)
			span.RecordError(conflictErr)
			span.SetStatus(codes.Error, conflictErr.Error())
			return conflictErr
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *RefreshTokenRepository) GetByTokenHash(ctx context.Context, hash string) (*user.RefreshToken, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.refresh_token.GetByTokenHash")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	t := &user.RefreshToken{}
	var profileID sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, token_hash, device_name, client_ip, used, expires_at, created_at, profile_id
		 FROM refresh_tokens WHERE token_hash = $1`, hash,
	).Scan(&t.ID, &t.UserID, &t.TokenHash, &t.DeviceName, &t.ClientIP, &t.Used, &t.ExpiresAt, &t.CreatedAt, &profileID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	if profileID.Valid {
		pid := shared.ProfileID(profileID.String)
		t.ProfileID = &pid
	}
	return t, nil
}

func (r *RefreshTokenRepository) MarkUsedIfNotUsed(ctx context.Context, id string) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.refresh_token.MarkUsedIfNotUsed")
	defer span.End()
	span.SetAttributes(attribute.String("refresh_token.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := r.db.ExecContext(ctx, `UPDATE refresh_tokens SET used = TRUE WHERE id = $1 AND used = FALSE`, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, err
	}
	return n > 0, nil
}

func (r *RefreshTokenRepository) DeleteByID(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.refresh_token.DeleteByID")
	defer span.End()
	span.SetAttributes(attribute.String("refresh_token.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *RefreshTokenRepository) DeleteByUserID(ctx context.Context, userID shared.UserID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.refresh_token.DeleteByUserID")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", string(userID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, string(userID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *RefreshTokenRepository) DeleteExpired(ctx context.Context) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.refresh_token.DeleteExpired")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE expires_at < NOW()`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

var _ user.RefreshTokenRepository = (*RefreshTokenRepository)(nil)
