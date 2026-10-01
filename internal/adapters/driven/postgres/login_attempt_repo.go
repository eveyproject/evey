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
//
// The counter increment and the lock decision are performed in a single atomic
// UPSERT, eliminating the TOCTOU race of the previous two-statement approach.
func (r *LoginAttemptRepository) Record(ctx context.Context, username string, maxFails int, window time.Duration) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.login_attempt.Record")
	defer span.End()
	span.SetAttributes(attribute.String("login_attempt.username", username))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	db := txFromCtx(ctx, r.db)
	windowSecs := window.Seconds()

	// Single atomic upsert: increments attempt_count (resetting when the window
	// has expired) and sets locked_until in the same statement.
	//
	// Parameters: $1=username  $2=windowSecs  $3=maxFails
	//
	// INSERT path (first attempt): locked_until set immediately when maxFails<=1.
	// UPDATE path (conflict): locked_until is set when the new count reaches
	// maxFails and no lock is ACTIVE: an active lock is never moved, an expired
	// one is replaced (a never-Reset key must be able to lock again).
	_, err := db.ExecContext(ctx, `
INSERT INTO login_attempts (username, attempt_count, locked_until, updated_at)
VALUES (
    $1,
    1,
    CASE WHEN 1 >= $3 THEN NOW() + ($2 * interval '1 second') ELSE NULL END,
    NOW()
)
ON CONFLICT (username) DO UPDATE
    SET attempt_count = CASE
            WHEN NOW() - login_attempts.updated_at > ($2 * interval '1 second')
            THEN 1
            ELSE login_attempts.attempt_count + 1
        END,
        locked_until = CASE
            WHEN (
                CASE
                    WHEN NOW() - login_attempts.updated_at > ($2 * interval '1 second')
                    THEN 1
                    ELSE login_attempts.attempt_count + 1
                END
            ) >= $3 AND (login_attempts.locked_until IS NULL OR login_attempts.locked_until <= NOW())
            THEN NOW() + ($2 * interval '1 second')
            ELSE login_attempts.locked_until
          END,
        updated_at = NOW()`,
		username, windowSecs, maxFails,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
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

// PurgeStale removes settled rows: the window has expired long ago AND no lock
// is outstanding. A locked row is kept whatever its age - dropping it would
// hand the attacker a fresh budget, which is the one thing this table exists to
// deny.
func (r *LoginAttemptRepository) PurgeStale(ctx context.Context, staleFor time.Duration) (int64, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.login_attempt.PurgeStale")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `
DELETE FROM login_attempts
WHERE updated_at < NOW() - ($1 * interval '1 second')
  AND (locked_until IS NULL OR locked_until < NOW())`, staleFor.Seconds())
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
