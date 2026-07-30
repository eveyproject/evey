package user

import (
	"context"
	"testing"
	"time"
)

// The in-memory login-attempt limiter had NO test file at all, and mutation
// testing surfaced six survivors inside it. It is the component that slows a
// brute-force down, so every one of its boundaries decides either "an attacker
// gets more tries than intended" or "a legitimate user stays locked out longer
// than intended".

func TestLoginAttemptLimiter_LocksOnTheNthFailure(t *testing.T) {
	l := newLoginAttemptLimiter(3, time.Hour)
	ctx := context.Background()

	for i := 1; i < 3; i++ {
		if err := l.Record(ctx, "alice", 3, time.Hour); err != nil {
			t.Fatalf("Record: %v", err)
		}
		if l.isLocked("alice") {
			t.Fatalf("locked after %d failures, the threshold is 3", i)
		}
	}

	// The third failure is the one that locks: the guard is `>=`, so an
	// off-by-one here either hands out a free attempt or locks one early.
	if err := l.Record(ctx, "alice", 3, time.Hour); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if !l.isLocked("alice") {
		t.Fatal("the third failure must lock the account")
	}
}

func TestLoginAttemptLimiter_UnknownAndResetUsersAreNotLocked(t *testing.T) {
	l := newLoginAttemptLimiter(2, time.Hour)
	ctx := context.Background()

	if l.isLocked("nobody") {
		t.Error("a username never seen must not be locked")
	}

	_ = l.Record(ctx, "bob", 2, time.Hour)
	_ = l.Record(ctx, "bob", 2, time.Hour)
	if !l.isLocked("bob") {
		t.Fatal("bob should be locked")
	}
	l.reset("bob")
	if l.isLocked("bob") {
		t.Error("a successful login must clear the lock")
	}
}

// A failure older than the window starts a FRESH count rather than adding to a
// stale one - otherwise two failures a month apart would eventually lock an
// account that was never actually under attack.
func TestLoginAttemptLimiter_StaleFailuresDoNotAccumulate(t *testing.T) {
	l := newLoginAttemptLimiter(2, time.Hour)
	ctx := context.Background()

	// One failure, then age it past the window by rewriting lastSeen - the
	// limiter reads the wall clock, so this is how we travel in time.
	_ = l.Record(ctx, "carol", 2, time.Hour)
	l.mu.Lock()
	l.attempts["carol"].lastSeen = time.Now().Add(-2 * time.Hour)
	l.mu.Unlock()

	// The next failure must be counted as the FIRST of a new window.
	_ = l.Record(ctx, "carol", 2, time.Hour)
	if l.isLocked("carol") {
		t.Fatal("a failure older than the window must not count towards the lock")
	}
}

// The lock itself expires: once the window has passed since it was set, the
// account is usable again and the entry is dropped.
func TestLoginAttemptLimiter_LockExpiresAfterTheWindow(t *testing.T) {
	l := newLoginAttemptLimiter(1, time.Hour)
	ctx := context.Background()

	_ = l.Record(ctx, "dave", 1, time.Hour)
	if !l.isLocked("dave") {
		t.Fatal("one failure with maxFails=1 must lock")
	}

	l.mu.Lock()
	l.attempts["dave"].lockedAt = time.Now().Add(-2 * time.Hour)
	l.mu.Unlock()

	if l.isLocked("dave") {
		t.Error("a lock older than the window must have expired")
	}
	l.mu.Lock()
	_, still := l.attempts["dave"]
	l.mu.Unlock()
	if still {
		t.Error("an expired lock must also drop its entry, not just report unlocked")
	}
}

// A lock still inside its window stays a lock - the mirror case, so that
// "expired" cannot be implemented as "always expired".
func TestLoginAttemptLimiter_FreshLockStillHolds(t *testing.T) {
	l := newLoginAttemptLimiter(1, time.Hour)
	_ = l.Record(context.Background(), "erin", 1, time.Hour)

	l.mu.Lock()
	l.attempts["erin"].lockedAt = time.Now().Add(-time.Minute)
	l.mu.Unlock()

	if !l.isLocked("erin") {
		t.Error("a lock set a minute ago must still hold for an hour-long window")
	}
}

// Eviction must drop entries the window has made irrelevant, both the
// still-counting ones and the locked ones, so a burst of distinct usernames
// cannot pin the map at its cap forever.
func TestLoginAttemptLimiter_EvictionDropsExpiredEntries(t *testing.T) {
	l := newLoginAttemptLimiter(5, time.Hour)
	now := time.Now()

	l.mu.Lock()
	l.attempts["stale-counting"] = &attemptEntry{count: 1, lastSeen: now.Add(-2 * time.Hour)}
	l.attempts["stale-locked"] = &attemptEntry{count: 9, lastSeen: now.Add(-2 * time.Hour), lockedAt: now.Add(-2 * time.Hour)}
	l.attempts["fresh"] = &attemptEntry{count: 1, lastSeen: now}
	l.attempts["locked-recently"] = &attemptEntry{count: 9, lastSeen: now, lockedAt: now}
	l.evictLocked(now)
	_, staleCounting := l.attempts["stale-counting"]
	_, staleLocked := l.attempts["stale-locked"]
	_, fresh := l.attempts["fresh"]
	_, lockedRecently := l.attempts["locked-recently"]
	l.mu.Unlock()

	if staleCounting {
		t.Error("an entry whose last failure predates the window must be evicted")
	}
	if staleLocked {
		t.Error("a lock older than the window must be evicted")
	}
	if !fresh {
		t.Error("a fresh entry must survive eviction")
	}
	if !lockedRecently {
		t.Error("a live lock must survive eviction - evicting it would free the attacker")
	}
}
