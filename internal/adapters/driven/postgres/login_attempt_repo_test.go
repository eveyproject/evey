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

	// 4 failures — below maxFails=5, account must not be locked.
	for i := 0; i < 4; i++ {
		require.NoError(t, repo.Record(ctx, "alice", testMaxFails, testWindow))
	}

	locked, err := repo.IsLocked(ctx, "alice")
	require.NoError(t, err)
	assert.False(t, locked, "not locked — threshold not yet reached")
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
