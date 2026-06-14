package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuickConnectRepo_Create_GetByCode(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewQuickConnectRepository(db)
	ctx := context.Background()

	req := &user.QuickConnectRequest{
		ID:        "qc1",
		Code:      "ABC123",
		Secret:    "secret123",
		Status:    user.Pending,
		ExpiresAt: time.Now().Add(10 * time.Minute).UTC().Truncate(time.Millisecond),
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, req))

	got, err := repo.GetByCode(ctx, "ABC123")
	require.NoError(t, err)
	assert.Equal(t, "qc1", got.ID)
	assert.Equal(t, user.Pending, got.Status)
}

// An expired code must be treated as non-existent — otherwise the 2-minute TTL
// (a security control) isn't enforced on the check/authorize paths until the
// hourly cleanup job runs, so an expired-but-authorized code could still mint tokens.
func TestQuickConnectRepo_GetByCode_ExpiredIsNotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewQuickConnectRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &user.QuickConnectRequest{
		ID:        "qc-exp",
		Code:      "EXPIRED1",
		Secret:    "s",
		Status:    user.Pending,
		ExpiresAt: time.Now().Add(-1 * time.Minute).UTC().Truncate(time.Millisecond),
		CreatedAt: time.Now().Add(-3 * time.Minute).UTC().Truncate(time.Millisecond),
	}))

	_, err := repo.GetByCode(ctx, "EXPIRED1")
	require.ErrorIs(t, err, shared.ErrNotFound)
}

func TestQuickConnectRepo_UpdateStatus(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	repo := postgres.NewQuickConnectRepository(db)
	ctx := context.Background()

	u := newTestUser("qc-u1")
	require.NoError(t, userRepo.Create(ctx, u))

	req := &user.QuickConnectRequest{
		ID:        "qc2",
		Code:      "DEF456",
		Secret:    "secret456",
		Status:    user.Pending,
		ExpiresAt: time.Now().Add(10 * time.Minute).UTC().Truncate(time.Millisecond),
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, req))

	userID := string(u.ID)
	require.NoError(t, repo.UpdateStatus(ctx, "qc2", string(user.Authorized), &userID))

	got, err := repo.GetByCode(ctx, "DEF456")
	require.NoError(t, err)
	assert.Equal(t, user.Authorized, got.Status)
	assert.NotNil(t, got.UserID)
	assert.Equal(t, userID, string(*got.UserID))

	// A second authorization (the request is no longer pending) must report
	// not-found rather than a false success — this is the race-loser / leaked-code
	// replay guard. Without it the loser of a concurrent authorize believes the
	// device was bound to them.
	require.ErrorIs(t, repo.UpdateStatus(ctx, "qc2", string(user.Authorized), &userID), shared.ErrNotFound)
}

func TestQuickConnectRepo_DeleteExpired(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewQuickConnectRepository(db)
	ctx := context.Background()

	req := &user.QuickConnectRequest{
		ID:        "qc3",
		Code:      "GHI789",
		Secret:    "secret789",
		Status:    user.Pending,
		ExpiresAt: time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Millisecond),
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, req))
	require.NoError(t, repo.DeleteExpired(ctx))

	_, err := repo.GetByCode(ctx, "GHI789")
	assert.Error(t, err)
}
