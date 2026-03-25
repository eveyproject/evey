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

func TestPlaySessionStatsRepo_GetUserStats_Empty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPlaySessionStatsReader(db)
	ctx := context.Background()

	stats, err := repo.GetUserStats(ctx, "nonexistent-profile")
	require.NoError(t, err)
	assert.Equal(t, int64(0), stats.TotalWatchTimeMs)
	assert.Equal(t, 0, stats.TotalPlayCount)
}

func TestPlaySessionStatsRepo_GetUserStats_WithData(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	statsRepo := postgres.NewPlaySessionStatsReader(db)
	ctx := context.Background()

	u := newTestUser("pss-u1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := &user.Profile{ID: "pss-p1", UserID: u.ID, Name: "Default", Preferences: map[string]string{}, IsDefault: true, CreatedAt: time.Now().UTC()}
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("pss-lib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("pss-mi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	session := newTestPlaySession("pss-ps1", p.ID, item.ID)
	dur := int64(60000)
	session.DurationMs = &dur
	require.NoError(t, sessionRepo.Create(ctx, session))

	stats, err := statsRepo.GetUserStats(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.TotalPlayCount)
	assert.Equal(t, int64(60000), stats.TotalWatchTimeMs)
}

func TestPlaySessionStatsRepo_GetServerStats(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPlaySessionStatsReader(db)
	ctx := context.Background()

	stats, err := repo.GetServerStats(ctx)
	require.NoError(t, err)
	assert.NotNil(t, stats)
	assert.GreaterOrEqual(t, stats.TotalUsers, 0)
}

func TestPlaySessionStatsRepo_GetMostPopular_Empty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewPlaySessionStatsReader(db)
	ctx := context.Background()

	result, err := repo.GetMostPopular(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, result)
}
