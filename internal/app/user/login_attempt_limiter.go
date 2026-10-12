package user

import (
	"context"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/user"
)

const loginLimiterMaxEntries = 10000

// loginAttemptLimiter tracks failed login attempts per username. It is the
// default in-memory implementation of user.LoginAttemptRepository; wire injects
// a persistent one (postgres) via WithLoginAttemptRepository when lockout state
// must survive restarts.
type loginAttemptLimiter struct {
	mu       sync.Mutex
	attempts map[string]*attemptEntry
	maxFails int
	maxSize  int
	window   time.Duration
}

type attemptEntry struct {
	count    int
	lockedAt time.Time
	lastSeen time.Time
}

func newLoginAttemptLimiter(maxFails int, window time.Duration) *loginAttemptLimiter {
	return &loginAttemptLimiter{
		attempts: make(map[string]*attemptEntry),
		maxFails: maxFails,
		maxSize:  loginLimiterMaxEntries,
		window:   window,
	}
}

// evictLocked removes expired entries, then oldest if still over cap. Caller holds mu.
func (l *loginAttemptLimiter) evictLocked(now time.Time) {
	// pass 1: remove expired
	for k, e := range l.attempts {
		if !e.lockedAt.IsZero() && now.Sub(e.lockedAt) > l.window {
			delete(l.attempts, k)
		} else if e.lockedAt.IsZero() && now.Sub(e.lastSeen) > l.window {
			delete(l.attempts, k)
		}
	}
	if len(l.attempts) < l.maxSize {
		return
	}
	// pass 2: evict oldest entries until under cap
	for len(l.attempts) >= l.maxSize {
		oldestKey := ""
		var oldestTime time.Time
		for k, e := range l.attempts {
			if oldestKey == "" || e.lastSeen.Before(oldestTime) {
				oldestKey = k
				oldestTime = e.lastSeen
			}
		}
		if oldestKey == "" {
			break
		}
		delete(l.attempts, oldestKey)
	}
}

func (l *loginAttemptLimiter) isLocked(username string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.attempts[username]
	if !ok {
		return false
	}
	if e.lockedAt.IsZero() {
		return false
	}
	if time.Since(e.lockedAt) < l.window {
		return true
	}
	delete(l.attempts, username)
	return false
}

func (l *loginAttemptLimiter) reset(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, username)
}

// IsLocked implements user.LoginAttemptRepository.
func (l *loginAttemptLimiter) IsLocked(_ context.Context, username string) (bool, error) {
	return l.isLocked(username), nil
}

// Record implements user.LoginAttemptRepository.
// maxFails and window override the limiter's own values when provided.
func (l *loginAttemptLimiter) Record(_ context.Context, username string, maxFails int, window time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()

	if len(l.attempts) >= l.maxSize {
		l.evictLocked(now)
	}

	e, ok := l.attempts[username]
	if !ok {
		e = &attemptEntry{}
		l.attempts[username] = e
	} else if now.Sub(e.lastSeen) > window {
		e = &attemptEntry{}
		l.attempts[username] = e
	}
	e.count++
	e.lastSeen = now
	if e.count >= maxFails {
		e.lockedAt = now
	}
	return nil
}

// Reset implements user.LoginAttemptRepository.
func (l *loginAttemptLimiter) Reset(_ context.Context, username string) error {
	l.reset(username)
	return nil
}

var _ user.LoginAttemptRepository = (*loginAttemptLimiter)(nil)

// PurgeStale drops settled entries. The in-memory limiter already evicts on
// every write (expired first, then oldest over the cap), so this is a no-op
// kept for interface parity with the Postgres repository - which has no such
// self-eviction and needs the sweep.
func (l *loginAttemptLimiter) PurgeStale(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}
