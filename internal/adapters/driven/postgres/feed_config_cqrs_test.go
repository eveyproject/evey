package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/feed"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeedConfigCQRS_UpsertAdminConfig_GetAdminConfig(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	cmdRepo := postgres.NewFeedConfigCommandRepo(db)
	queryRepo := postgres.NewFeedConfigQueryRepo(db)
	ctx := context.Background()

	cfg := &feed.AdminConfig{
		FeedType:     feed.TypeMostPopular,
		Enabled:      true,
		DisplayOrder: 5,
		MaxItems:     15,
		Params:       map[string]any{"days": 30},
	}
	require.NoError(t, cmdRepo.UpsertAdminConfig(ctx, cfg))

	configs, err := queryRepo.GetAdminConfig(ctx)
	require.NoError(t, err)
	var found bool
	for _, c := range configs {
		if c.FeedType == feed.TypeMostPopular {
			found = true
			assert.True(t, c.Enabled)
			assert.Equal(t, 15, c.MaxItems)
		}
	}
	assert.True(t, found)
}

func TestFeedConfigCQRS_UpsertProfileConfig_GetProfileConfig(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	cmdRepo := postgres.NewFeedConfigCommandRepo(db)
	queryRepo := postgres.NewFeedConfigQueryRepo(db)
	ctx := context.Background()

	u := newTestUser("fc-u1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := &user.Profile{ID: "fc-p1", UserID: shared.UserID(u.ID), Name: "Default", Preferences: map[string]string{}, IsDefault: true, CreatedAt: time.Now().UTC()}
	require.NoError(t, profileRepo.Create(ctx, p))

	enabled := true
	cfg := &feed.ProfileConfig{
		ProfileID:    p.ID,
		FeedType:     feed.TypeRecentlyAdded,
		Enabled:      &enabled,
		DisplayOrder: intPtr(0),
		MaxItems:     intPtr(10),
		Params:       map[string]any{},
	}
	require.NoError(t, cmdRepo.UpsertProfileConfig(ctx, cfg))

	configs, err := queryRepo.GetProfileConfig(ctx, string(p.ID))
	require.NoError(t, err)
	assert.Len(t, configs, 1)
	assert.Equal(t, feed.TypeRecentlyAdded, configs[0].FeedType)
}

func TestFeedConfigCQRS_BatchUpsertAdminConfig(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	cmdRepo := postgres.NewFeedConfigCommandRepo(db)
	queryRepo := postgres.NewFeedConfigQueryRepo(db)
	ctx := context.Background()

	cfgs := []*feed.AdminConfig{
		{FeedType: feed.TypeResumePlayback, Enabled: true, DisplayOrder: 10, MaxItems: 5, Params: map[string]any{}},
		{FeedType: feed.TypeNextEpisode, Enabled: true, DisplayOrder: 11, MaxItems: 5, Params: map[string]any{}},
	}
	require.NoError(t, cmdRepo.BatchUpsertAdminConfig(ctx, cfgs))

	all, err := queryRepo.GetAdminConfig(ctx)
	require.NoError(t, err)
	foundTypes := map[feed.Type]bool{}
	for _, c := range all {
		foundTypes[c.FeedType] = true
	}
	assert.True(t, foundTypes[feed.TypeResumePlayback])
	assert.True(t, foundTypes[feed.TypeNextEpisode])
}

func TestFeedConfigCQRS_DeleteProfileConfig(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	cmdRepo := postgres.NewFeedConfigCommandRepo(db)
	queryRepo := postgres.NewFeedConfigQueryRepo(db)
	ctx := context.Background()

	u := newTestUser("fc-u2")
	require.NoError(t, userRepo.Create(ctx, u))
	p := &user.Profile{ID: "fc-p2", UserID: shared.UserID(u.ID), Name: "Default", Preferences: map[string]string{}, IsDefault: true, CreatedAt: time.Now().UTC()}
	require.NoError(t, profileRepo.Create(ctx, p))

	enabled := true
	cfg := &feed.ProfileConfig{
		ProfileID: p.ID,
		FeedType:  feed.TypeRecentlyAdded,
		Enabled:   &enabled,
		Params:    map[string]any{},
	}
	require.NoError(t, cmdRepo.UpsertProfileConfig(ctx, cfg))
	require.NoError(t, cmdRepo.DeleteProfileConfig(ctx, string(p.ID), feed.TypeRecentlyAdded))

	configs, err := queryRepo.GetProfileConfig(ctx, string(p.ID))
	require.NoError(t, err)
	assert.Empty(t, configs)
}

func TestFeedConfigCQRS_DeleteAdminConfigByType(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	cmdRepo := postgres.NewFeedConfigCommandRepo(db)
	queryRepo := postgres.NewFeedConfigQueryRepo(db)
	ctx := context.Background()

	cfg := &feed.AdminConfig{
		FeedType:     feed.TypeCollections,
		Enabled:      true,
		DisplayOrder: 20,
		MaxItems:     10,
		Params:       map[string]any{},
	}
	require.NoError(t, cmdRepo.UpsertAdminConfig(ctx, cfg))
	require.NoError(t, cmdRepo.DeleteAdminConfigByType(ctx, feed.TypeCollections))

	all, err := queryRepo.GetAdminConfig(ctx)
	require.NoError(t, err)
	for _, c := range all {
		assert.NotEqual(t, feed.TypeCollections, c.FeedType)
	}
}
