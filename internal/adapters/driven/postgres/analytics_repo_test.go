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

func TestAnalyticsReader_GetAnalytics_Empty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewAnalyticsReader(db)
	ctx := context.Background()

	data, err := repo.GetAnalytics(ctx)
	require.NoError(t, err)
	assert.NotNil(t, data)
	assert.Empty(t, data.TopMedia)
	assert.Empty(t, data.RecentActivity)
	assert.Equal(t, 0, data.TotalPlaysToday)
}

func TestAnalyticsReader_GetAnalytics_WithData(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	sessionRepo := postgres.NewPlaySessionRepository(db)
	analyticsRepo := postgres.NewAnalyticsReader(db)
	ctx := context.Background()

	u := newTestUser("an-u1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := &user.Profile{ID: "an-p1", UserID: shared.UserID(u.ID), Name: "Default", Preferences: map[string]string{}, IsDefault: true, CreatedAt: time.Now().UTC()}
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("an-lib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("an-mi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	require.NoError(t, sessionRepo.Create(ctx, newTestPlaySession("an-ps1", string(p.ID), string(item.ID))))

	data, err := analyticsRepo.GetAnalytics(ctx)
	require.NoError(t, err)
	assert.NotNil(t, data)
	assert.GreaterOrEqual(t, data.TotalPlaysToday, 1)
}
