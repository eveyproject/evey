package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/notification"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationSettingsRepo_Upsert_GetByProfile(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	repo := postgres.NewNotificationSettingsRepository(db)
	ctx := context.Background()

	u := newTestUser("ns-u1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := &user.Profile{ID: "ns-p1", UserID: shared.UserID(u.ID), Name: "Default", Preferences: map[string]string{}, IsDefault: true, CreatedAt: time.Now().UTC()}
	require.NoError(t, profileRepo.Create(ctx, p))

	s := &notification.Settings{
		ProfileID:   p.ID,
		MovieLevel:  notification.LevelAll,
		SeriesLevel: notification.LevelRelevant,
		MusicLevel:  notification.LevelDisabled,
	}
	require.NoError(t, repo.Upsert(ctx, s))

	got, err := repo.GetByProfile(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, notification.LevelAll, got.MovieLevel)
	assert.Equal(t, notification.LevelRelevant, got.SeriesLevel)
	assert.Equal(t, notification.LevelDisabled, got.MusicLevel)
}

func TestNotificationSettingsRepo_GetByProfile_FallbackToGlobal(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewNotificationSettingsRepository(db)
	ctx := context.Background()

	got, err := repo.GetByProfile(ctx, "nonexistent-profile")
	require.NoError(t, err)
	assert.NotNil(t, got)
}

func TestNotificationSettingsRepo_GetByProfiles(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	repo := postgres.NewNotificationSettingsRepository(db)
	ctx := context.Background()

	u := newTestUser("ns-u2")
	require.NoError(t, userRepo.Create(ctx, u))
	p1 := &user.Profile{ID: "ns-p2a", UserID: shared.UserID(u.ID), Name: "P1", Preferences: map[string]string{}, IsDefault: true, CreatedAt: time.Now().UTC()}
	p2 := &user.Profile{ID: "ns-p2b", UserID: shared.UserID(u.ID), Name: "P2", Preferences: map[string]string{}, CreatedAt: time.Now().UTC()}
	require.NoError(t, profileRepo.Create(ctx, p1))
	require.NoError(t, profileRepo.Create(ctx, p2))

	require.NoError(t, repo.Upsert(ctx, &notification.Settings{
		ProfileID:   p1.ID,
		MovieLevel:  notification.LevelAll,
		SeriesLevel: notification.LevelAll,
		MusicLevel:  notification.LevelAll,
	}))

	result, err := repo.GetByProfiles(ctx, []shared.ProfileID{p1.ID, p2.ID})
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.NotNil(t, result[p1.ID])
	assert.NotNil(t, result[p2.ID])
}

func TestNotificationSettingsRepo_UpsertGlobalDefaults(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewNotificationSettingsRepository(db)
	ctx := context.Background()

	s := &notification.Settings{
		MovieLevel:  notification.LevelRelevant,
		SeriesLevel: notification.LevelAll,
		MusicLevel:  notification.LevelDisabled,
	}
	require.NoError(t, repo.UpsertGlobalDefaults(ctx, s))

	got, err := repo.GetGlobalDefaults(ctx)
	require.NoError(t, err)
	assert.Equal(t, notification.LevelRelevant, got.MovieLevel)
}
