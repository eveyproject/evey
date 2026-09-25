package postgres_test

import (
	"context"
	"database/sql"
	"sync"
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

func TestRefreshTokenRepo_ProfileID_RoundTrip(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedUser(t, db, "rt-prof-user")
	require.NoError(t, postgres.NewProfileRepository(db).Create(context.Background(), newTestProfile("rt-prof-1", "rt-prof-user")))
	repo := postgres.NewRefreshTokenRepository(db)
	ctx := context.Background()

	bound := newRefreshToken("rtp1", "rt-prof-user", "hash_bound")
	pid := shared.ProfileID("rt-prof-1")
	bound.ProfileID = &pid
	require.NoError(t, repo.Create(ctx, bound))
	got, err := repo.GetByTokenHash(ctx, "hash_bound")
	require.NoError(t, err)
	require.NotNil(t, got.ProfileID, "bound profile_id must persist")
	assert.Equal(t, shared.ProfileID("rt-prof-1"), *got.ProfileID)

	legacy := newRefreshToken("rtp2", "rt-prof-user", "hash_unbound")
	require.NoError(t, repo.Create(ctx, legacy))
	got2, err := repo.GetByTokenHash(ctx, "hash_unbound")
	require.NoError(t, err)
	assert.Nil(t, got2.ProfileID, "unbound token reads back nil")
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
	assert.Equal(t, shared.UserID("rt-user-2"), got.UserID)
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
	assert.Equal(t, shared.UserID("rt-user-4b"), got.UserID)
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

// The breadcrumb survives the sweep. A rotated row is normally swept ten
// minutes later, but one carrying a successor is the only proof a client
// whose rotation answer was lost is not a replay - and that client may not
// come back for days.
func TestRefreshTokenRepo_SuccessorSurvivesTheSweepAndRoundTrips(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	seedUser(t, db, "rt-user-succ")
	repo := postgres.NewRefreshTokenRepository(db)
	ctx := context.Background()

	old := newRefreshToken("rt-succ-old", "rt-user-succ", "hash_rotated")
	require.NoError(t, repo.Create(ctx, old))
	next := newRefreshToken("rt-succ-new", "rt-user-succ", "hash_successor")
	require.NoError(t, repo.Create(ctx, next))

	claimed, err := repo.MarkUsedIfNotUsed(ctx, old.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, repo.SetSuccessor(ctx, old.ID, next.ID))

	// Age the rotation well past the ten-minute sweep window.
	_, err = db.ExecContext(ctx, `UPDATE refresh_tokens SET used_at = NOW() - INTERVAL '3 hours' WHERE id = $1`, old.ID)
	require.NoError(t, err)
	require.NoError(t, repo.DeleteExpired(ctx))

	got, err := repo.GetByID(ctx, old.ID)
	require.NoError(t, err, "a rotated row carrying a successor must outlive the sweep")
	require.NotNil(t, got.SuccessorID)
	assert.Equal(t, next.ID, *got.SuccessorID)

	// One without a successor is still swept, as before.
	bare := newRefreshToken("rt-succ-bare", "rt-user-succ", "hash_bare")
	require.NoError(t, repo.Create(ctx, bare))
	_, err = repo.MarkUsedIfNotUsed(ctx, bare.ID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE refresh_tokens SET used_at = NOW() - INTERVAL '3 hours' WHERE id = $1`, bare.ID)
	require.NoError(t, err)
	require.NoError(t, repo.DeleteExpired(ctx))
	_, err = repo.GetByID(ctx, bare.ID)
	require.ErrorIs(t, err, sql.ErrNoRows, "a rotated row with no successor is still swept")

	// The breadcrumb itself is not kept for ever: past the retention it goes
	// (prod 2026-09: one row per 15-minute rotation kept for 180 days).
	_, err = db.ExecContext(ctx, `UPDATE refresh_tokens SET used_at = NOW() - INTERVAL '31 days' WHERE id = $1`, old.ID)
	require.NoError(t, err)
	require.NoError(t, repo.DeleteExpired(ctx))
	_, err = repo.GetByID(ctx, old.ID)
	require.ErrorIs(t, err, sql.ErrNoRows, "a breadcrumb older than the retention is swept")
	_, err = repo.GetByID(ctx, next.ID)
	require.NoError(t, err, "the live successor is untouched")
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

// Single-use rotation: the first consumer of a refresh token wins, every
// concurrent or later attempt on the same id loses, and the row records
// when it was spent. This is the database-side guard that makes a replayed
// refresh token worthless.
func TestRefreshTokenRepo_MarkUsedIfNotUsed_ExactlyOnceUnderContention(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	seedUser(t, db, "rt-once")
	repo := postgres.NewRefreshTokenRepository(db)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, newRefreshToken("rt-once-1", "rt-once", "hash_once")))

	const racers = 8
	wins := make(chan bool, racers)
	var wg sync.WaitGroup
	for range racers {
		wg.Go(func() {
			ok, err := repo.MarkUsedIfNotUsed(ctx, "rt-once-1")
			require.NoError(t, err)
			wins <- ok
		})
	}
	wg.Wait()
	close(wins)
	won := 0
	for ok := range wins {
		if ok {
			won++
		}
	}
	assert.Equal(t, 1, won, "exactly one consumer may spend the token")

	got, err := repo.GetByTokenHash(ctx, "hash_once")
	require.NoError(t, err)
	require.NotNil(t, got.UsedAt, "the spend is timestamped")
	ok, err := repo.MarkUsedIfNotUsed(ctx, "rt-once-1")
	require.NoError(t, err)
	assert.False(t, ok, "a spent token stays spent")
	ok, err = repo.MarkUsedIfNotUsed(ctx, "rt-ghost")
	require.NoError(t, err)
	assert.False(t, ok, "an unknown id is not a success")
}
