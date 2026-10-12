package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAPIKey(id, userID, hash string) *user.APIKeyModel {
	return &user.APIKeyModel{
		ID:          id,
		UserID:      shared.UserID(userID),
		KeyHash:     hash,
		Name:        "key_" + id,
		Permissions: []string{"read", "write"},
		LastUsedAt:  nil,
		ExpiresAt:   nil,
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
}

func TestAPIKeyRepo_Create_GetByKeyHash(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	keyRepo := postgres.NewAPIKeyRepository(db)
	ctx := context.Background()

	u := newTestUser("ak-u1")
	require.NoError(t, userRepo.Create(ctx, u))

	key := newTestAPIKey("ak1", string(u.ID), "hash-ak1")
	require.NoError(t, keyRepo.Create(ctx, key))

	got, err := keyRepo.GetByKeyHash(ctx, "hash-ak1")
	require.NoError(t, err)
	assert.Equal(t, key.ID, got.ID)
	assert.Equal(t, key.UserID, got.UserID)
	assert.Equal(t, key.KeyHash, got.KeyHash)
	assert.Equal(t, key.Name, got.Name)
	assert.Equal(t, key.Permissions, got.Permissions)
	assert.Nil(t, got.LastUsedAt)
	assert.Nil(t, got.ExpiresAt)
	assert.Equal(t, key.CreatedAt, got.CreatedAt)
}

func TestAPIKeyRepo_ListByUserID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	keyRepo := postgres.NewAPIKeyRepository(db)
	ctx := context.Background()

	u1 := newTestUser("ak-u2")
	u2 := newTestUser("ak-u3")
	require.NoError(t, userRepo.Create(ctx, u1))
	require.NoError(t, userRepo.Create(ctx, u2))

	k1 := newTestAPIKey("ak2", string(u1.ID), "hash-ak2")
	k1.CreatedAt = time.Now().UTC().Truncate(time.Millisecond)
	k2 := newTestAPIKey("ak3", string(u1.ID), "hash-ak3")
	k2.CreatedAt = k1.CreatedAt.Add(time.Second)
	k3 := newTestAPIKey("ak4", string(u2.ID), "hash-ak4")

	require.NoError(t, keyRepo.Create(ctx, k1))
	require.NoError(t, keyRepo.Create(ctx, k2))
	require.NoError(t, keyRepo.Create(ctx, k3))

	list1, err := keyRepo.ListByUserID(ctx, shared.UserID(u1.ID))
	require.NoError(t, err)
	require.Len(t, list1, 2)
	assert.Equal(t, k1.ID, list1[0].ID)
	assert.Equal(t, k2.ID, list1[1].ID)

	list2, err := keyRepo.ListByUserID(ctx, shared.UserID(u2.ID))
	require.NoError(t, err)
	require.Len(t, list2, 1)
	assert.Equal(t, k3.ID, list2[0].ID)
}

func TestAPIKeyRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	keyRepo := postgres.NewAPIKeyRepository(db)
	ctx := context.Background()

	u := newTestUser("ak-u4")
	require.NoError(t, userRepo.Create(ctx, u))

	key := newTestAPIKey("ak5", string(u.ID), "hash-ak5")
	require.NoError(t, keyRepo.Create(ctx, key))
	require.NoError(t, keyRepo.Delete(ctx, key.ID))

	_, err := keyRepo.GetByKeyHash(ctx, "hash-ak5")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestAPIKeyRepo_UpdateLastUsed(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	keyRepo := postgres.NewAPIKeyRepository(db)
	ctx := context.Background()

	u := newTestUser("ak-u5")
	require.NoError(t, userRepo.Create(ctx, u))

	key := newTestAPIKey("ak6", string(u.ID), "hash-ak6")
	require.NoError(t, keyRepo.Create(ctx, key))

	lastUsed := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, keyRepo.UpdateLastUsed(ctx, key.ID, lastUsed))

	got, err := keyRepo.GetByKeyHash(ctx, "hash-ak6")
	require.NoError(t, err)
	require.NotNil(t, got.LastUsedAt)
	assert.Equal(t, lastUsed, got.LastUsedAt.UTC().Truncate(time.Millisecond))
}

func TestAPIKeyRepo_GetByKeyHash_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	keyRepo := postgres.NewAPIKeyRepository(db)
	ctx := context.Background()

	_, err := keyRepo.GetByKeyHash(ctx, "nonexistent-hash")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestAPIKeyRepo_Delete_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	keyRepo := postgres.NewAPIKeyRepository(db)
	err := keyRepo.Delete(context.Background(), "missing")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestAPIKeyRepo_UpdateLastUsed_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	keyRepo := postgres.NewAPIKeyRepository(db)
	err := keyRepo.UpdateLastUsed(context.Background(), "missing", time.Now())
	assert.ErrorIs(t, err, shared.ErrNotFound)
}
