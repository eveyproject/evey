package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestResetToken(id, userID, hash string, expiresAt time.Time, used bool) *user.PasswordResetToken {
	return &user.PasswordResetToken{
		ID:        id,
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: expiresAt.UTC().Truncate(time.Millisecond),
		Used:      used,
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
}

func TestPasswordResetTokenRepo_RoundtripSuccess(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	repo := postgres.NewPasswordResetTokenRepository(db)
	ctx := context.Background()

	u := newTestUser("prt-u1")
	require.NoError(t, userRepo.Create(ctx, u))

	tok := newTestResetToken("prt1", u.ID, "hash-valid", time.Now().Add(time.Hour), false)
	require.NoError(t, repo.Create(ctx, tok))

	got, err := repo.GetByTokenHash(ctx, "hash-valid")
	require.NoError(t, err)
	assert.Equal(t, tok.ID, got.ID)
	assert.Equal(t, tok.UserID, got.UserID)
	assert.False(t, got.Used)
}

func TestPasswordResetTokenRepo_ExpiredReturnsNotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	repo := postgres.NewPasswordResetTokenRepository(db)
	ctx := context.Background()

	u := newTestUser("prt-u2")
	require.NoError(t, userRepo.Create(ctx, u))

	tok := newTestResetToken("prt2", u.ID, "hash-expired", time.Now().Add(-time.Hour), false)
	require.NoError(t, repo.Create(ctx, tok))

	_, err := repo.GetByTokenHash(ctx, "hash-expired")
	assert.True(t, errors.Is(err, shared.ErrNotFound))
}

func TestPasswordResetTokenRepo_UsedReturnsNotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	repo := postgres.NewPasswordResetTokenRepository(db)
	ctx := context.Background()

	u := newTestUser("prt-u3")
	require.NoError(t, userRepo.Create(ctx, u))

	tok := newTestResetToken("prt3", u.ID, "hash-used", time.Now().Add(time.Hour), false)
	require.NoError(t, repo.Create(ctx, tok))
	require.NoError(t, repo.MarkUsed(ctx, tok.ID))

	_, err := repo.GetByTokenHash(ctx, "hash-used")
	assert.True(t, errors.Is(err, shared.ErrNotFound))
}

func TestPasswordResetTokenRepo_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPasswordResetTokenRepository(db)
	ctx := context.Background()

	_, err := repo.GetByTokenHash(ctx, "does-not-exist")
	assert.True(t, errors.Is(err, shared.ErrNotFound))
}
