package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPRT(id, userID, hash string, expiresAt time.Time) *user.PasswordResetToken {
	return &user.PasswordResetToken{
		ID:        id,
		UserID:    shared.UserID(userID),
		TokenHash: hash,
		ExpiresAt: expiresAt.UTC().Truncate(time.Millisecond),
		Used:      false,
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
}

func TestPasswordResetTokenRepo_CreateAndGet(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedUser(t, db, "prt-u1")
	repo := postgres.NewPasswordResetTokenRepository(db)
	ctx := context.Background()

	tok := newPRT("prt1", "prt-u1", "hash_create", time.Now().Add(1*time.Hour))
	require.NoError(t, repo.Create(ctx, tok))

	got, err := repo.GetByTokenHash(ctx, "hash_create")
	require.NoError(t, err)
	assert.Equal(t, tok.ID, got.ID)
	assert.Equal(t, tok.UserID, got.UserID)
	assert.Equal(t, tok.TokenHash, got.TokenHash)
	assert.False(t, got.Used)
	assert.WithinDuration(t, tok.ExpiresAt, got.ExpiresAt, time.Second)
	assert.WithinDuration(t, tok.CreatedAt, got.CreatedAt, time.Second)
	assert.True(t, got.IsValid())
}

func TestPasswordResetTokenRepo_GetByTokenHash_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPasswordResetTokenRepository(db)
	_, err := repo.GetByTokenHash(context.Background(), "does_not_exist")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestPasswordResetTokenRepo_GetExpiredReturnsNotFound(t *testing.T) {
	// Defense-in-depth: repo SQL filters expired tokens - caller no longer needs IsValid().
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedUser(t, db, "prt-u-exp")
	repo := postgres.NewPasswordResetTokenRepository(db)
	ctx := context.Background()

	expired := newPRT("prt-exp", "prt-u-exp", "hash_exp", time.Now().Add(-1*time.Hour))
	require.NoError(t, repo.Create(ctx, expired))

	_, err := repo.GetByTokenHash(ctx, "hash_exp")
	assert.ErrorIs(t, err, shared.ErrNotFound, "expired token must be filtered at SQL level")
}

func TestPasswordResetTokenRepo_MarkUsed_ThenGetReturnsNotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedUser(t, db, "prt-u2")
	repo := postgres.NewPasswordResetTokenRepository(db)
	ctx := context.Background()

	tok := newPRT("prt-mu", "prt-u2", "hash_mu", time.Now().Add(1*time.Hour))
	require.NoError(t, repo.Create(ctx, tok))
	require.NoError(t, repo.MarkUsed(ctx, "prt-mu"))

	_, err := repo.GetByTokenHash(ctx, "hash_mu")
	assert.ErrorIs(t, err, shared.ErrNotFound, "used token must be filtered at SQL level")
}

func TestPasswordResetTokenRepo_MarkUsed_UnknownID_ReturnsError(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPasswordResetTokenRepository(db)
	// MarkUsed is now an atomic single-use CLAIM (`WHERE used = FALSE`): nothing
	// to claim (unknown id) → 0 rows → error, so a caller's tx rolls back.
	require.Error(t, repo.MarkUsed(context.Background(), "ghost-id"))
}

func TestPasswordResetTokenRepo_MarkUsed_Twice_SecondFails(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedUser(t, db, "prt-u3")
	repo := postgres.NewPasswordResetTokenRepository(db)
	ctx := context.Background()

	tok := newPRT("prt-twice", "prt-u3", "hash_twice", time.Now().Add(1*time.Hour))
	require.NoError(t, repo.Create(ctx, tok))
	// First claim wins; the second (the concurrent-reuse race) MUST fail so the
	// token can reset the password only once.
	require.NoError(t, repo.MarkUsed(ctx, "prt-twice"))
	require.Error(t, repo.MarkUsed(ctx, "prt-twice"), "a used token cannot be claimed again")
}

func TestPasswordResetTokenRepo_DeleteByUserID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedUser(t, db, "prt-u3")
	seedUser(t, db, "prt-u3b")
	repo := postgres.NewPasswordResetTokenRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, newPRT("prt-a", "prt-u3", "hash_a", time.Now().Add(time.Hour))))
	require.NoError(t, repo.Create(ctx, newPRT("prt-b", "prt-u3", "hash_b", time.Now().Add(time.Hour))))
	require.NoError(t, repo.Create(ctx, newPRT("prt-c", "prt-u3b", "hash_c", time.Now().Add(time.Hour))))

	require.NoError(t, repo.DeleteByUserID(ctx, "prt-u3"))

	_, err := repo.GetByTokenHash(ctx, "hash_a")
	assert.ErrorIs(t, err, shared.ErrNotFound)
	_, err = repo.GetByTokenHash(ctx, "hash_b")
	assert.ErrorIs(t, err, shared.ErrNotFound)

	got, err := repo.GetByTokenHash(ctx, "hash_c")
	require.NoError(t, err)
	assert.Equal(t, "prt-u3b", string(got.UserID))
}

func TestPasswordResetTokenRepo_DeleteExpired(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedUser(t, db, "prt-u4")
	repo := postgres.NewPasswordResetTokenRepository(db)
	ctx := context.Background()

	expired := newPRT("prt-de-exp", "prt-u4", "hash_de_exp", time.Now().Add(-2*time.Hour))
	require.NoError(t, repo.Create(ctx, expired))

	valid := newPRT("prt-de-val", "prt-u4", "hash_de_val", time.Now().Add(2*time.Hour))
	require.NoError(t, repo.Create(ctx, valid))

	// Also: a used-but-not-yet-expired token - should be deleted per WHERE clause (used = TRUE).
	usedValid := newPRT("prt-de-used", "prt-u4", "hash_de_used", time.Now().Add(2*time.Hour))
	require.NoError(t, repo.Create(ctx, usedValid))
	require.NoError(t, repo.MarkUsed(ctx, "prt-de-used"))

	require.NoError(t, repo.DeleteExpired(ctx))

	_, err := repo.GetByTokenHash(ctx, "hash_de_exp")
	assert.ErrorIs(t, err, shared.ErrNotFound, "expired token should be purged")

	_, err = repo.GetByTokenHash(ctx, "hash_de_used")
	assert.ErrorIs(t, err, shared.ErrNotFound, "used token should be purged too")

	got, err := repo.GetByTokenHash(ctx, "hash_de_val")
	require.NoError(t, err, "unused non-expired token must survive")
	assert.Equal(t, "prt-de-val", got.ID)
}

func TestPasswordResetTokenRepo_CascadeOnUserDelete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedUser(t, db, "prt-u5")
	repo := postgres.NewPasswordResetTokenRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, newPRT("prt-cas", "prt-u5", "hash_cas", time.Now().Add(time.Hour))))

	_, err := db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, "prt-u5")
	require.NoError(t, err)

	_, err = repo.GetByTokenHash(ctx, "hash_cas")
	assert.ErrorIs(t, err, shared.ErrNotFound, "ON DELETE CASCADE should remove token")
}
