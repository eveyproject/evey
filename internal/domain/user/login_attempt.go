package user

import (
	"context"
	"time"
)

// LoginAttempt holds the persisted rate-limit state for a single username.
type LoginAttempt struct {
	Username     string
	AttemptCount int
	LockedUntil  *time.Time
	UpdatedAt    time.Time
}

// LoginAttemptRepository persists brute-force rate-limit state so it survives
// restarts and is shared across instances.
//
// The "window + maxFails" logic is owned by each implementation: the caller
// supplies maxFails and window so the thresholds stay configurable at the
// service layer without hardcoding them in SQL or in-memory logic.
//
// Implementors must be safe for concurrent use.
type LoginAttemptRepository interface {
	// IsLocked returns true if the username is currently locked out.
	IsLocked(ctx context.Context, username string) (bool, error)

	// Record increments the failed-attempt counter for username.
	// If the new attempt count reaches maxFails within window, the account is
	// locked for window starting from now.
	Record(ctx context.Context, username string, maxFails int, window time.Duration) error

	// Reset clears all attempt state for the username (called on success).
	Reset(ctx context.Context, username string) error

	// PurgeStale deletes rows whose rate-limit window has long expired and
	// which hold no active lock. Returns how many went.
	//
	// This table is written on the UNKNOWN-USER path too - deliberately, so a
	// missing account costs the same as a wrong password and cannot be probed
	// by timing - which means one permanent row per distinct username an
	// attacker ever tries. Without this sweep a credential-stuffing dictionary
	// is stored forever, at the operator's expense. The in-memory limiter this
	// repository replaced had both a TTL and a size cap; moving to Postgres
	// silently dropped both.
	PurgeStale(ctx context.Context, staleFor time.Duration) (int64, error)
}
