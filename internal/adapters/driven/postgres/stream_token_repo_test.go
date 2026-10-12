package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamTokenRepo_RoundTripAndRevocations(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	seedUser(t, db, "usr-st-1")
	repo := postgres.NewStreamTokenRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	native := shared.StreamToken{UserID: "usr-st-1", ProfileID: "prf-1", MediaItemID: "m1", Scope: shared.ScopeStream, TTL: 12 * time.Hour, ExpiresAt: now.Add(12 * time.Hour)}
	guest := shared.StreamToken{MediaItemID: "m1", Scope: shared.ScopeSyncplayGuest, RoomID: "room-1", TTL: time.Hour, ExpiresAt: now.Add(time.Hour)}
	require.NoError(t, repo.SaveStreamToken(ctx, "h-native", native, now))
	require.NoError(t, repo.SaveStreamToken(ctx, "h-guest", guest, now))

	got, issued, ok, err := repo.LoadStreamToken(ctx, "h-native")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, native.UserID, got.UserID)
	assert.Equal(t, native.ProfileID, got.ProfileID)
	assert.Equal(t, native.MediaItemID, got.MediaItemID)
	assert.Equal(t, native.TTL, got.TTL)
	assert.True(t, native.ExpiresAt.Equal(got.ExpiresAt))
	assert.True(t, now.Equal(issued))

	g, _, ok, err := repo.LoadStreamToken(ctx, "h-guest")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Empty(t, g.UserID, "a guest has no account")
	assert.Equal(t, "room-1", g.RoomID)

	// Extensions may land out of order: never shortened.
	require.NoError(t, repo.ExtendStreamToken(ctx, "h-native", now.Add(13*time.Hour)))
	require.NoError(t, repo.ExtendStreamToken(ctx, "h-native", now.Add(time.Hour)))
	got, _, _, _ = repo.LoadStreamToken(ctx, "h-native")
	assert.True(t, now.Add(13*time.Hour).Equal(got.ExpiresAt))

	require.NoError(t, repo.DeleteStreamTokensByRoom(ctx, "room-1"))
	_, _, ok, _ = repo.LoadStreamToken(ctx, "h-guest")
	assert.False(t, ok)
	require.NoError(t, repo.DeleteStreamTokensByUser(ctx, "usr-st-1"))
	_, _, ok, _ = repo.LoadStreamToken(ctx, "h-native")
	assert.False(t, ok)

	_, _, ok, err = repo.LoadStreamToken(ctx, "unknown")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestStreamTokenRepo_PurgesExpiredAndFollowsUserDeletion(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	seedUser(t, db, "usr-st-2")
	repo := postgres.NewStreamTokenRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	require.NoError(t, repo.SaveStreamToken(ctx, "h-old", shared.StreamToken{UserID: "usr-st-2", MediaItemID: "m1", Scope: shared.ScopeStream, TTL: time.Minute, ExpiresAt: now.Add(-time.Minute)}, now.Add(-2*time.Minute)))
	require.NoError(t, repo.SaveStreamToken(ctx, "h-live", shared.StreamToken{UserID: "usr-st-2", MediaItemID: "m1", Scope: shared.ScopeStream, TTL: time.Hour, ExpiresAt: now.Add(time.Hour)}, now))
	require.NoError(t, repo.DeleteExpiredStreamTokens(ctx, now))
	_, _, ok, _ := repo.LoadStreamToken(ctx, "h-old")
	assert.False(t, ok)
	_, _, ok, _ = repo.LoadStreamToken(ctx, "h-live")
	assert.True(t, ok)

	// A deleted account takes its tokens with it.
	_, err := db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, "usr-st-2")
	require.NoError(t, err)
	_, _, ok, _ = repo.LoadStreamToken(ctx, "h-live")
	assert.False(t, ok)
}
