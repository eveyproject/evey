package feed_test

import (
	"context"
	"testing"

	appfeed "github.com/gravity-zero/evey/internal/app/feed"
	"github.com/gravity-zero/evey/internal/domain/feed"
	"github.com/gravity-zero/evey/internal/domain/feed/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandService_UpdateAdminConfig(t *testing.T) {
	repo := fakes.NewFakeConfigCommandRepo()
	svc := appfeed.NewCommandService(repo, fakes.NewFakeConfigQueryRepo())

	cfg := &feed.AdminConfig{
		FeedType:     feed.TypeRecentlyAdded,
		Enabled:      true,
		DisplayOrder: 1,
		MaxItems:     20,
	}
	require.NoError(t, svc.UpdateAdminConfig(context.Background(), cfg))
	assert.Equal(t, cfg, repo.AdminConfigs[feed.TypeRecentlyAdded])
}

func TestCommandService_UpdateProfileConfig(t *testing.T) {
	repo := fakes.NewFakeConfigCommandRepo()
	svc := appfeed.NewCommandService(repo, fakes.NewFakeConfigQueryRepo())

	enabled := false
	cfg := &feed.ProfileConfig{
		ProfileID: "p1",
		FeedType:  feed.TypeContinueWatching,
		Enabled:   &enabled,
	}
	require.NoError(t, svc.UpdateProfileConfig(context.Background(), cfg))
	assert.Equal(t, cfg, repo.ProfileConfigs["p1"][feed.TypeContinueWatching])
}

func TestCommandService_ResetProfileConfig(t *testing.T) {
	repo := fakes.NewFakeConfigCommandRepo()
	svc := appfeed.NewCommandService(repo, fakes.NewFakeConfigQueryRepo())

	enabled := true
	cfg := &feed.ProfileConfig{
		ProfileID: "p1",
		FeedType:  feed.TypeRecommended,
		Enabled:   &enabled,
	}
	require.NoError(t, svc.UpdateProfileConfig(context.Background(), cfg))
	require.NoError(t, svc.ResetProfileConfig(context.Background(), "p1", feed.TypeRecommended))
	assert.Empty(t, repo.ProfileConfigs["p1"])
}
