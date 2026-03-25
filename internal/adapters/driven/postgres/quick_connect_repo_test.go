package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/gravity-zero/evey/internal/domain/user"
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
		Status:    "pending",
		ExpiresAt: time.Now().Add(10 * time.Minute).UTC().Truncate(time.Millisecond),
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, req))

	got, err := repo.GetByCode(ctx, "ABC123")
	require.NoError(t, err)
	assert.Equal(t, "qc1", got.ID)
	assert.Equal(t, "pending", got.Status)
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
		Status:    "pending",
		ExpiresAt: time.Now().Add(10 * time.Minute).UTC().Truncate(time.Millisecond),
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, req))

	userID := u.ID
	require.NoError(t, repo.UpdateStatus(ctx, "qc2", "authorized", &userID))

	got, err := repo.GetByCode(ctx, "DEF456")
	require.NoError(t, err)
	assert.Equal(t, "authorized", got.Status)
	assert.NotNil(t, got.UserID)
	assert.Equal(t, userID, *got.UserID)
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
		Status:    "pending",
		ExpiresAt: time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Millisecond),
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, req))
	require.NoError(t, repo.DeleteExpired(ctx))

	_, err := repo.GetByCode(ctx, "GHI789")
	assert.Error(t, err)
}
