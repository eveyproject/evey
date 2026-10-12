package postgres_test

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/feed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeedAdminConfigRepo(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewFeedAdminConfigRepository(db)
	ctx := context.Background()

	// Migration seeds 3 default configs
	all, err := repo.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	// Update one
	require.NoError(t, repo.Upsert(ctx, &feed.AdminConfig{
		FeedType:     feed.TypeRecentlyAdded,
		Enabled:      false,
		DisplayOrder: 99,
		MaxItems:     5,
	}))

	all, err = repo.GetAll(ctx)
	require.NoError(t, err)
	var ra *feed.AdminConfig
	for _, c := range all {
		if c.FeedType == feed.TypeRecentlyAdded {
			ra = c
		}
	}
	require.NotNil(t, ra)
	assert.False(t, ra.Enabled)
	assert.Equal(t, 5, ra.MaxItems)
}

func TestFeedProfileConfigRepo(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewFeedProfileConfigRepository(db)
	ctx := context.Background()

	maxItems := 10
	enabled := false
	cfg := &feed.ProfileConfig{
		ProfileID: "prof-1",
		FeedType:  feed.TypeContinueWatching,
		Enabled:   &enabled,
		MaxItems:  &maxItems,
	}

	require.NoError(t, repo.Upsert(ctx, cfg))

	results, err := repo.GetByProfile(ctx, "prof-1")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, feed.TypeContinueWatching, results[0].FeedType)

	require.NoError(t, repo.Delete(ctx, "prof-1", feed.TypeContinueWatching))
	results, _ = repo.GetByProfile(ctx, "prof-1")
	assert.Empty(t, results)
}
