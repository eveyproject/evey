package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedUser(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	userRepo := postgres.NewUserRepository(db)
	require.NoError(t, userRepo.Create(context.Background(), newTestUser(id)))
}

func newRefreshToken(id, userID, hash string) *user.RefreshToken {
	return &user.RefreshToken{
		ID:         id,
		UserID:     shared.UserID(userID),
		TokenHash:  hash,
		DeviceName: "Firefox on Linux",
		ClientIP:   strPtr("192.168.1.10"),
		ExpiresAt:  time.Now().UTC().Add(24 * time.Hour).Truncate(time.Millisecond),
		CreatedAt:  time.Now().UTC().Truncate(time.Millisecond),
	}
}

func strPtr(s string) *string { return &s }

func TestRefreshTokenRepo_Create(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedUser(t, db, "rt-user-1")
	repo := postgres.NewRefreshTokenRepository(db)
	ctx := context.Background()

	tok := newRefreshToken("rt1", "rt-user-1", "hash_create")
	require.NoError(t, repo.Create(ctx, tok))

	got, err := repo.GetByTokenHash(ctx, "hash_create")
	require.NoError(t, err)
	assert.Equal(t, tok.ID, got.ID)
	assert.Equal(t, tok.UserID, got.UserID)
	assert.Equal(t, tok.TokenHash, got.TokenHash)
	assert.Equal(t, tok.DeviceName, got.DeviceName)
}

func TestRefreshTokenRepo_Create_DuplicateHash(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedUser(t, db, "rt-user-dup")
	repo := postgres.NewRefreshTokenRepository(db)
	ctx := context.Background()

	tok1 := newRefreshToken("rt-dup1", "rt-user-dup", "hash_dup")
	require.NoError(t, repo.Create(ctx, tok1))

	tok2 := newRefreshToken("rt-dup2", "rt-user-dup", "hash_dup")
	err := repo.Create(ctx, tok2)
	assert.Error(t, err, "duplicate token_hash should violate unique constraint")
}

func TestRefreshTokenRepo_GetByTokenHash(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedUser(t, db, "rt-user-2")
	repo := postgres.NewRefreshTokenRepository(db)
	ctx := context.Background()

	tok := newRefreshToken("rt2", "rt-user-2", "hash_get")
	require.NoError(t, repo.Create(ctx, tok))

	got, err := repo.GetByTokenHash(ctx, "hash_get")
	require.NoError(t, err)
	assert.Equal(t, "rt2", got.ID)
	assert.Equal(t, "rt-user-2", got.UserID)
	assert.Equal(t, "hash_get", got.TokenHash)
	assert.Equal(t, "Firefox on Linux", got.DeviceName)
	assert.NotNil(t, got.ClientIP)
	assert.Equal(t, "192.168.1.10", *got.ClientIP)
	assert.WithinDuration(t, tok.ExpiresAt, got.ExpiresAt, time.Second)
	assert.WithinDuration(t, tok.CreatedAt, got.CreatedAt, time.Second)
}

func TestRefreshTokenRepo_GetByTokenHash_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewRefreshTokenRepository(db)
	ctx := context.Background()

	_, err := repo.GetByTokenHash(ctx, "nonexistent_hash")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestRefreshTokenRepo_DeleteByID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedUser(t, db, "rt-user-3")
	repo := postgres.NewRefreshTokenRepository(db)
	ctx := context.Background()

	tok := newRefreshToken("rt3", "rt-user-3", "hash_delid")
	require.NoError(t, repo.Create(ctx, tok))

	require.NoError(t, repo.DeleteByID(ctx, "rt3"))

	_, err := repo.GetByTokenHash(ctx, "hash_delid")
	assert.ErrorIs(t, err, sql.ErrNoRows, "token should be gone after DeleteByID")
}

func TestRefreshTokenRepo_DeleteByUserID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedUser(t, db, "rt-user-4")
	seedUser(t, db, "rt-user-4b")
	repo := postgres.NewRefreshTokenRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, newRefreshToken("rt4a", "rt-user-4", "hash_u4_a")))
	require.NoError(t, repo.Create(ctx, newRefreshToken("rt4b", "rt-user-4", "hash_u4_b")))
	require.NoError(t, repo.Create(ctx, newRefreshToken("rt4c", "rt-user-4b", "hash_u4b_a")))

	require.NoError(t, repo.DeleteByUserID(ctx, "rt-user-4"))

	_, err := repo.GetByTokenHash(ctx, "hash_u4_a")
	require.ErrorIs(t, err, sql.ErrNoRows, "first token of user-4 should be deleted")

	_, err = repo.GetByTokenHash(ctx, "hash_u4_b")
	require.ErrorIs(t, err, sql.ErrNoRows, "second token of user-4 should be deleted")

	got, err := repo.GetByTokenHash(ctx, "hash_u4b_a")
	require.NoError(t, err, "token of different user should survive")
	assert.Equal(t, "rt-user-4b", got.UserID)
}

func TestRefreshTokenRepo_DeleteExpired(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedUser(t, db, "rt-user-5")
	repo := postgres.NewRefreshTokenRepository(db)
	ctx := context.Background()

	expired := newRefreshToken("rt5-exp", "rt-user-5", "hash_expired")
	expired.ExpiresAt = time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Millisecond)
	require.NoError(t, repo.Create(ctx, expired))

	valid := newRefreshToken("rt5-val", "rt-user-5", "hash_valid")
	valid.ExpiresAt = time.Now().UTC().Add(24 * time.Hour).Truncate(time.Millisecond)
	require.NoError(t, repo.Create(ctx, valid))

	require.NoError(t, repo.DeleteExpired(ctx))

	_, err := repo.GetByTokenHash(ctx, "hash_expired")
	require.ErrorIs(t, err, sql.ErrNoRows, "expired token should be deleted")

	got, err := repo.GetByTokenHash(ctx, "hash_valid")
	require.NoError(t, err, "valid token should survive")
	assert.Equal(t, "rt5-val", got.ID)
}

func TestRefreshTokenRepo_NilClientIP(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedUser(t, db, "rt-user-6")
	repo := postgres.NewRefreshTokenRepository(db)
	ctx := context.Background()

	tok := newRefreshToken("rt6", "rt-user-6", "hash_nilip")
	tok.ClientIP = nil
	require.NoError(t, repo.Create(ctx, tok))

	got, err := repo.GetByTokenHash(ctx, "hash_nilip")
	require.NoError(t, err)
	assert.Nil(t, got.ClientIP)
}
