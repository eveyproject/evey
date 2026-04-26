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
}
