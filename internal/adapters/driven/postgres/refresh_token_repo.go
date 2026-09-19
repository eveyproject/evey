package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

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
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
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
	return scanRefreshToken(txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, user_id, token_hash, device_name, client_ip, used, used_at, expires_at, created_at, profile_id, successor_id
		 FROM refresh_tokens WHERE token_hash = $1`, hash), span)
}

func (r *RefreshTokenRepository) GetByID(ctx context.Context, id string) (*user.RefreshToken, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.refresh_token.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("refresh_token.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return scanRefreshToken(txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, user_id, token_hash, device_name, client_ip, used, used_at, expires_at, created_at, profile_id, successor_id
		 FROM refresh_tokens WHERE id = $1`, id), span)
}

func scanRefreshToken(row *sql.Row, span trace.Span) (*user.RefreshToken, error) {
	t := &user.RefreshToken{}
	var profileID, successorID sql.NullString
	var usedAt sql.NullTime
	err := row.Scan(&t.ID, &t.UserID, &t.TokenHash, &t.DeviceName, &t.ClientIP, &t.Used, &usedAt, &t.ExpiresAt, &t.CreatedAt, &profileID, &successorID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	if profileID.Valid {
		pid := shared.ProfileID(profileID.String)
		t.ProfileID = &pid
	}
	if usedAt.Valid {
		ua := usedAt.Time
		t.UsedAt = &ua
	}
	if successorID.Valid {
		sid := successorID.String
		t.SuccessorID = &sid
	}
	return t, nil
}

func (r *RefreshTokenRepository) SetSuccessor(ctx context.Context, id, successorID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.refresh_token.SetSuccessor")
	defer span.End()
	span.SetAttributes(attribute.String("refresh_token.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `UPDATE refresh_tokens SET successor_id = $2 WHERE id = $1`, id, successorID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *RefreshTokenRepository) MarkUsedIfNotUsed(ctx context.Context, id string) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.refresh_token.MarkUsedIfNotUsed")
	defer span.End()
	span.SetAttributes(attribute.String("refresh_token.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `UPDATE refresh_tokens SET used = TRUE, used_at = NOW() WHERE id = $1 AND used = FALSE`, id)
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
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM refresh_tokens WHERE id = $1`, id)
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
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, string(userID))
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
	// Rotated rows are retained for the app-layer rotation grace (a multi-tab
	// race loser must find them to be recognised); sweep them once the grace
	// has LONG passed - 10 min is safely above the 30 s window.
	//
	// EXCEPT the ones carrying a successor: those are the breadcrumbs that let
	// a client whose rotation answer was lost (app killed, TV crashed at boot)
	// prove it is not a replay. It may not come back for days, so they live
	// until the token's own expiry like any other row.
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM refresh_tokens WHERE expires_at < NOW() OR (used AND successor_id IS NULL AND used_at < NOW() - INTERVAL '10 minutes')`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

var _ user.RefreshTokenRepository = (*RefreshTokenRepository)(nil)
