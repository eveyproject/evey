package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testMaxFails = 5
	testWindow   = 15 * time.Minute
)

func TestLoginAttemptRepo_IsLocked_NoRow(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewLoginAttemptRepository(db)
	locked, err := repo.IsLocked(context.Background(), "nobody")
	require.NoError(t, err)
	assert.False(t, locked)
}

func TestLoginAttemptRepo_RecordWithoutLock(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewLoginAttemptRepository(db)
	ctx := context.Background()

	// 4 failures - below maxFails=5, account must not be locked.
	for i := 0; i < 4; i++ {
		require.NoError(t, repo.Record(ctx, "alice", testMaxFails, testWindow))
	}

	locked, err := repo.IsLocked(ctx, "alice")
	require.NoError(t, err)
	assert.False(t, locked, "not locked - threshold not yet reached")
}

func TestLoginAttemptRepo_RecordLocksAtMaxFails(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewLoginAttemptRepository(db)
	ctx := context.Background()

	// Reach maxFails=5.
	for i := 0; i < testMaxFails; i++ {
		require.NoError(t, repo.Record(ctx, "bob", testMaxFails, testWindow))
	}

	locked, err := repo.IsLocked(ctx, "bob")
	require.NoError(t, err)
	assert.True(t, locked, "should be locked after reaching maxFails")
}

func TestLoginAttemptRepo_IsLocked_ExpiredLock(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewLoginAttemptRepository(db)
	ctx := context.Background()

	// Lock with a very short window (1ms), then sleep past it.
	for i := 0; i < 1; i++ {
		require.NoError(t, repo.Record(ctx, "carol", 1, time.Millisecond))
	}
	time.Sleep(5 * time.Millisecond)

	locked, err := repo.IsLocked(ctx, "carol")
	require.NoError(t, err)
	assert.False(t, locked, "expired lock should not be reported as locked")
}

func TestLoginAttemptRepo_Reset(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewLoginAttemptRepository(db)
	ctx := context.Background()

	for i := 0; i < testMaxFails; i++ {
		require.NoError(t, repo.Record(ctx, "dave", testMaxFails, testWindow))
	}

	locked, err := repo.IsLocked(ctx, "dave")
	require.NoError(t, err)
	require.True(t, locked)

	require.NoError(t, repo.Reset(ctx, "dave"))

	locked, err = repo.IsLocked(ctx, "dave")
	require.NoError(t, err)
	assert.False(t, locked, "should not be locked after reset")
}

func TestLoginAttemptRepo_Reset_Idempotent(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewLoginAttemptRepository(db)
	ctx := context.Background()

	// Reset on a non-existent row must not error.
	require.NoError(t, repo.Reset(ctx, "ghost"))
}

// TestLoginAttemptRepo_LockExactlyAtMaxFails verifies lock is set at exactly
// maxFails attempts and not a single attempt before that.
func TestLoginAttemptRepo_LockExactlyAtMaxFails(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewLoginAttemptRepository(db)
	ctx := context.Background()

	for i := 1; i < testMaxFails; i++ {
		require.NoError(t, repo.Record(ctx, "eve", testMaxFails, testWindow))
		locked, err := repo.IsLocked(ctx, "eve")
		require.NoError(t, err)
		assert.Falsef(t, locked, "must not be locked after %d/%d attempts", i, testMaxFails)
	}

	require.NoError(t, repo.Record(ctx, "eve", testMaxFails, testWindow))
	locked, err := repo.IsLocked(ctx, "eve")
	require.NoError(t, err)
	assert.True(t, locked, "must be locked exactly at maxFails")
}

// TestLoginAttemptRepo_WindowExpiredResetsCount verifies that after the window
// expires the counter resets to 1 and no lock is applied.
func TestLoginAttemptRepo_WindowExpiredResetsCount(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewLoginAttemptRepository(db)
	ctx := context.Background()

	// Build up attempts below the lock threshold.
	for range testMaxFails - 1 {
		require.NoError(t, repo.Record(ctx, "frank", testMaxFails, testWindow))
	}

	// Simulate window expiry by backdating updated_at past the window.
	_, err := db.ExecContext(ctx,
		`UPDATE login_attempts SET updated_at = NOW() - interval '1 hour' WHERE username = $1`,
		"frank",
	)
	require.NoError(t, err)

	// One more Record: window expired → count resets to 1 (below maxFails=5), no lock.
	require.NoError(t, repo.Record(ctx, "frank", testMaxFails, testWindow))

	locked, err := repo.IsLocked(ctx, "frank")
	require.NoError(t, err)
	assert.False(t, locked, "count must reset to 1 after window expiry; no lock expected")
}

// TestLoginAttemptRepo_ExistingLockNotOverwritten verifies that a subsequent
// Record after the lock is already set does not move locked_until.
func TestLoginAttemptRepo_ExistingLockNotOverwritten(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewLoginAttemptRepository(db)
	ctx := context.Background()

	for range testMaxFails {
		require.NoError(t, repo.Record(ctx, "grace", testMaxFails, testWindow))
	}

	var lockedUntilBefore time.Time
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT locked_until FROM login_attempts WHERE username = $1`, "grace",
	).Scan(&lockedUntilBefore))

	// Additional Record must not change the existing locked_until.
	require.NoError(t, repo.Record(ctx, "grace", testMaxFails, testWindow))

	var lockedUntilAfter time.Time
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT locked_until FROM login_attempts WHERE username = $1`, "grace",
	).Scan(&lockedUntilAfter))

	assert.True(t, lockedUntilBefore.Equal(lockedUntilAfter),
		"locked_until must not change on subsequent Record: before=%v after=%v",
		lockedUntilBefore, lockedUntilAfter)
}

// TestLoginAttemptRepo_ImmediateLockAtMaxFailsOne verifies that when maxFails=1
// the very first Record locks the account (INSERT path edge case).
func TestLoginAttemptRepo_ImmediateLockAtMaxFailsOne(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewLoginAttemptRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Record(ctx, "heidi", 1, testWindow))

	locked, err := repo.IsLocked(ctx, "heidi")
	require.NoError(t, err)
	assert.True(t, locked, "maxFails=1: first attempt must lock immediately (INSERT path)")
}

// PurgeStale forgets settled rows only: an outstanding lock is kept whatever
// its age, because dropping it would hand the attacker a fresh budget.
func TestLoginAttemptRepo_PurgeStale_KeepsOutstandingLocks(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	repo := postgres.NewLoginAttemptRepository(db)
	ctx := context.Background()

	// "settled": a couple of failures, no lock.
	require.NoError(t, repo.Record(ctx, "settled", testMaxFails, testWindow))
	// "locked": at the threshold, lock outstanding for an hour.
	for range testMaxFails {
		require.NoError(t, repo.Record(ctx, "locked", testMaxFails, time.Hour))
	}
	// Age both rows so the stale predicate sees them as old.
	_, err := db.ExecContext(ctx, `UPDATE login_attempts SET updated_at = NOW() - interval '2 days'`)
	require.NoError(t, err)

	n, err := repo.PurgeStale(ctx, 24*time.Hour)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n, "only the settled row goes")

	locked, err := repo.IsLocked(ctx, "locked")
	require.NoError(t, err)
	assert.True(t, locked, "the outstanding lock survives the purge")
	locked, err = repo.IsLocked(ctx, "settled")
	require.NoError(t, err)
	assert.False(t, locked)

	// A row younger than the horizon is kept even when settled.
	require.NoError(t, repo.Record(ctx, "fresh", testMaxFails, testWindow))
	n, err = repo.PurgeStale(ctx, 24*time.Hour)
	require.NoError(t, err)
	assert.Zero(t, n)
}

// An expired lock must not disarm the counter: a key that never logs in
// successfully (an attacker's IP, the per-user cap) is never Reset, so it has
// to lock again once its old lock has run out.
func TestLoginAttemptRepo_RelocksAfterExpiredLock(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewLoginAttemptRepository(db)
	ctx := context.Background()
	for range testMaxFails {
		require.NoError(t, repo.Record(ctx, "ivan", testMaxFails, testWindow))
	}
	_, err := db.ExecContext(ctx,
		`UPDATE login_attempts SET locked_until = NOW() - interval '1 minute', updated_at = NOW() - interval '1 hour' WHERE username = $1`,
		"ivan")
	require.NoError(t, err)

	for range testMaxFails {
		require.NoError(t, repo.Record(ctx, "ivan", testMaxFails, testWindow))
	}
	locked, err := repo.IsLocked(ctx, "ivan")
	require.NoError(t, err)
	assert.True(t, locked, "a fresh run of maxFails failures after an expired lock must lock again")
}
