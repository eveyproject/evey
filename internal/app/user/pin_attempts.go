package user

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/user"
)

// ErrPINLocked: too many wrong PINs for this profile, try again later.
var ErrPINLocked = errors.New("too many wrong PIN attempts")

type pinTier struct {
	prefix   string
	maxFails int
	window   time.Duration
}

// pinTiers make the lockout grow: a few wrong PINs cost a minute, a guesser who
// keeps going after each lockout hits the quarter-hour then the day tier. A
// right PIN clears every tier.
var pinTiers = []pinTier{
	{prefix: "pin1:", maxFails: 5, window: time.Minute},
	{prefix: "pin2:", maxFails: 10, window: 15 * time.Minute},
	{prefix: "pin3:", maxFails: 20, window: 24 * time.Hour},
}

// PINAttempts counts wrong parental PINs per profile with a growing lockout.
type PINAttempts struct {
	tiers  []pinTier
	stores []user.LoginAttemptRepository
}

// NewPINAttempts keeps the counters in repo (persistent across restarts), or
// in memory when repo is nil.
func NewPINAttempts(repo user.LoginAttemptRepository) *PINAttempts {
	a := &PINAttempts{tiers: append([]pinTier(nil), pinTiers...)}
	for _, t := range a.tiers {
		var s user.LoginAttemptRepository = repo
		if s == nil {
			// The in-memory limiter locks for its own window, so one per tier.
			s = newLoginAttemptLimiter(t.maxFails, t.window)
		}
		a.stores = append(a.stores, s)
	}
	return a
}

var (
	sharedPINMu sync.Mutex
	sharedPIN   *PINAttempts
)

// SharedPINAttempts is the process-wide counter both the HTTP parental routes
// and the gRPC profile RPCs check, so a guesser cannot double the budget by
// alternating transports. In memory unless SetSharedPINAttempts ran first.
func SharedPINAttempts() *PINAttempts {
	sharedPINMu.Lock()
	defer sharedPINMu.Unlock()
	if sharedPIN == nil {
		sharedPIN = NewPINAttempts(nil)
	}
	return sharedPIN
}

// SetSharedPINAttempts installs the process-wide counter, e.g. one persisted
// with NewPINAttempts(loginAttemptRepo). Call it at composition, before the
// user service and the HTTP routes are built: they capture the instance.
func SetSharedPINAttempts(a *PINAttempts) {
	sharedPINMu.Lock()
	defer sharedPINMu.Unlock()
	sharedPIN = a
}

// Locked reports whether profileID must not try a PIN now. Fails closed.
func (a *PINAttempts) Locked(ctx context.Context, profileID string) bool {
	for i, t := range a.tiers {
		locked, err := a.stores[i].IsLocked(ctx, t.prefix+profileID)
		if err != nil {
			slog.WarnContext(ctx, "pin attempts: lock check failed", slog.Any("err", err))
			return true
		}
		if locked {
			return true
		}
	}
	return false
}

// Failed records a wrong PIN for profileID.
func (a *PINAttempts) Failed(ctx context.Context, profileID string) {
	for i, t := range a.tiers {
		if err := a.stores[i].Record(ctx, t.prefix+profileID, t.maxFails, t.window); err != nil {
			slog.WarnContext(ctx, "pin attempts: record failed", slog.Any("err", err))
		}
	}
}

// Succeeded clears profileID's counters after a right PIN.
func (a *PINAttempts) Succeeded(ctx context.Context, profileID string) {
	for i, t := range a.tiers {
		if err := a.stores[i].Reset(ctx, t.prefix+profileID); err != nil {
			slog.WarnContext(ctx, "pin attempts: reset failed", slog.Any("err", err))
		}
	}
}
