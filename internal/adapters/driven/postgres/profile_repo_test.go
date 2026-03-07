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

func newTestProfile(id, userID string) *user.Profile {
	return &user.Profile{
		ID:          id,
		UserID:      userID,
		Name:        "Profile " + id,
		Preferences: map[string]string{"lang": "en"},
		IsDefault:   false,
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
}


func TestProfileRepo_Create_GetByID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	ctx := context.Background()

	u := newTestUser("pu1")
	require.NoError(t, userRepo.Create(ctx, u))

	p := newTestProfile("p1", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))

	got, err := profileRepo.GetByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, p.Name, got.Name)
	assert.Equal(t, p.UserID, got.UserID)
	assert.Equal(t, p.Preferences, got.Preferences)
}

func TestProfileRepo_GetByUserID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	ctx := context.Background()

	u := newTestUser("pu2")
	require.NoError(t, userRepo.Create(ctx, u))

	require.NoError(t, profileRepo.Create(ctx, newTestProfile("p2a", u.ID)))
	require.NoError(t, profileRepo.Create(ctx, newTestProfile("p2b", u.ID)))

	profiles, err := profileRepo.GetByUserID(ctx, u.ID)
	require.NoError(t, err)
	assert.Len(t, profiles, 2)
}

func TestProfileRepo_Update(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	ctx := context.Background()

	u := newTestUser("pu3")
	require.NoError(t, userRepo.Create(ctx, u))

	p := newTestProfile("p3", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))

	p.Name = "Updated Profile"
	p.IsDefault = true
	require.NoError(t, profileRepo.Update(ctx, p))

	got, err := profileRepo.GetByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Profile", got.Name)
	assert.True(t, got.IsDefault)
}

func TestProfileRepo_LanguagePreferences(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	ctx := context.Background()

	u := newTestUser("pu5")
	require.NoError(t, userRepo.Create(ctx, u))

	p := newTestProfile("p5", u.ID)
	p.AudioLanguages = []string{"fr", "en"}
	p.SubtitleLanguages = []string{"fr"}
	require.NoError(t, profileRepo.Create(ctx, p))

	got, err := profileRepo.GetByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"fr", "en"}, got.AudioLanguages)
	assert.Equal(t, []string{"fr"}, got.SubtitleLanguages)

	got.AudioLanguages = []string{"de", "fr", "en"}
	require.NoError(t, profileRepo.Update(ctx, got))

	updated, err := profileRepo.GetByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"de", "fr", "en"}, updated.AudioLanguages)
}

func TestProfileRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	ctx := context.Background()

	u := newTestUser("pu4")
	require.NoError(t, userRepo.Create(ctx, u))

	p := newTestProfile("p4", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	require.NoError(t, profileRepo.Delete(ctx, p.ID))

	_, err := profileRepo.GetByID(ctx, p.ID)
	assert.Error(t, err)
}
