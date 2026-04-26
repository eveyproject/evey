package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// LoginAttemptRepository persists brute-force rate-limit state in postgres.
type LoginAttemptRepository struct {
	db *sql.DB
}

// NewLoginAttemptRepository returns a new LoginAttemptRepository.
func NewLoginAttemptRepository(db *sql.DB) *LoginAttemptRepository {
	return &LoginAttemptRepository{db: db}
}

// IsLocked returns true when the username has an active lock (locked_until in the future).
func (r *LoginAttemptRepository) IsLocked(ctx context.Context, username string) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.login_attempt.IsLocked")
	defer span.End()
	span.SetAttributes(attribute.String("login_attempt.username", username))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	db := txFromCtx(ctx, r.db)
	var lockedUntil sql.NullTime
	err := db.QueryRowContext(ctx,
		`SELECT locked_until FROM login_attempts WHERE username = $1`, username).
		Scan(&lockedUntil)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, err
	}
	if !lockedUntil.Valid {
		return false, nil
	}
	return time.Now().Before(lockedUntil.Time), nil
}

// Record increments the failed-attempt counter for username. If the new count
// reaches maxFails within window the account is locked for window from now.
// The window resets when updated_at is older than window (expired window).
func (r *LoginAttemptRepository) Record(ctx context.Context, username string, maxFails int, window time.Duration) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.login_attempt.Record")
	defer span.End()
	span.SetAttributes(attribute.String("login_attempt.username", username))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	db := txFromCtx(ctx, r.db)
	windowSecs := window.Seconds()

	// Upsert: insert new row or increment counter (resetting if window expired).
	// Returns the resulting attempt_count so we can decide on locking.
	var newCount int
	err := db.QueryRowContext(ctx, `
INSERT INTO login_attempts (username, attempt_count, updated_at)
VALUES ($1, 1, NOW())
ON CONFLICT (username) DO UPDATE
    SET attempt_count = CASE
            WHEN NOW() - login_attempts.updated_at > ($2 * interval '1 second')
            THEN 1
            ELSE login_attempts.attempt_count + 1
        END,
        updated_at = NOW()
RETURNING attempt_count`,
		username, windowSecs,
	).Scan(&newCount)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	if newCount >= maxFails {
		_, err = db.ExecContext(ctx,
			`UPDATE login_attempts
			 SET locked_until = NOW() + ($1 * interval '1 second')
			 WHERE username = $2 AND locked_until IS NULL`,
			windowSecs, username,
		)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}
	return nil
}

// Reset removes all attempt state for username (called on successful login).
func (r *LoginAttemptRepository) Reset(ctx context.Context, username string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.login_attempt.Reset")
	defer span.End()
	span.SetAttributes(attribute.String("login_attempt.username", username))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	db := txFromCtx(ctx, r.db)
	_, err := db.ExecContext(ctx,
		`DELETE FROM login_attempts WHERE username = $1`, username)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

var _ user.LoginAttemptRepository = (*LoginAttemptRepository)(nil)
