package feed_test

import (
	"context"
	"testing"

	appfeed "github.com/eveyproject/evey/internal/app/feed"
	"github.com/eveyproject/evey/internal/domain/feed"
	"github.com/eveyproject/evey/internal/domain/feed/fakes"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/plugin"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
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

func TestCommandService_UpdateAdminConfig_InvalidReturnsError(t *testing.T) {
	repo := fakes.NewFakeConfigCommandRepo()
	svc := appfeed.NewCommandService(repo, fakes.NewFakeConfigQueryRepo())

	cfg := &feed.AdminConfig{FeedType: "", Enabled: true, MaxItems: 10}
	err := svc.UpdateAdminConfig(context.Background(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid admin feed config")
}

func TestCommandService_UpdateProfileConfig_InvalidReturnsError(t *testing.T) {
	repo := fakes.NewFakeConfigCommandRepo()
	svc := appfeed.NewCommandService(repo, fakes.NewFakeConfigQueryRepo())

	cfg := &feed.ProfileConfig{ProfileID: "", FeedType: feed.TypeRecentlyAdded}
	err := svc.UpdateProfileConfig(context.Background(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid profile feed config")
}

func TestCommandService_BatchUpdateAdminConfigs(t *testing.T) {
	repo := fakes.NewFakeConfigCommandRepo()
	svc := appfeed.NewCommandService(repo, fakes.NewFakeConfigQueryRepo())

	cfgs := []*feed.AdminConfig{
		{FeedType: feed.TypeRecentlyAdded, Enabled: true, DisplayOrder: 0, MaxItems: 20},
		{FeedType: feed.TypeRecommended, Enabled: true, DisplayOrder: 1, MaxItems: 15},
	}
	require.NoError(t, svc.BatchUpdateAdminConfigs(context.Background(), cfgs))
	assert.Len(t, repo.AdminConfigs, 2)
}

func TestCommandService_BatchUpdateAdminConfigs_InvalidReturnsError(t *testing.T) {
	repo := fakes.NewFakeConfigCommandRepo()
	svc := appfeed.NewCommandService(repo, fakes.NewFakeConfigQueryRepo())

	cfgs := []*feed.AdminConfig{
		{FeedType: feed.TypeRecentlyAdded, Enabled: true, MaxItems: 20},
		{FeedType: "", Enabled: true, MaxItems: 10},
	}
	err := svc.BatchUpdateAdminConfigs(context.Background(), cfgs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid admin feed config")
}

func TestCommandService_EnsureLibraryFeedConfig_New(t *testing.T) {
	cmdRepo := fakes.NewFakeConfigCommandRepo()
	queryRepo := fakes.NewFakeConfigQueryRepo()
	svc := appfeed.NewCommandService(cmdRepo, queryRepo)

	queryRepo.AdminConfigs = []*feed.AdminConfig{
		{FeedType: feed.TypeRecentlyAdded, Enabled: true, DisplayOrder: 1, MaxItems: 20},
	}

	require.NoError(t, svc.EnsureLibraryFeedConfig(context.Background(), "lib-1", "Movies"))
	created := cmdRepo.AdminConfigs[feed.LibraryType("lib-1")]
	require.NotNil(t, created)
	assert.True(t, created.Enabled)
	assert.Equal(t, 2, created.DisplayOrder)
	assert.Equal(t, 20, created.MaxItems)
}

func TestCommandService_EnsureLibraryFeedConfig_AlreadyExists(t *testing.T) {
	cmdRepo := fakes.NewFakeConfigCommandRepo()
	queryRepo := fakes.NewFakeConfigQueryRepo()
	svc := appfeed.NewCommandService(cmdRepo, queryRepo)

	ft := feed.LibraryType("lib-1")
	queryRepo.AdminConfigs = []*feed.AdminConfig{
		{FeedType: ft, Enabled: true, DisplayOrder: 0, MaxItems: 20},
	}

	require.NoError(t, svc.EnsureLibraryFeedConfig(context.Background(), "lib-1", "Movies"))
	assert.Empty(t, cmdRepo.AdminConfigs)
}

func TestCommandService_DeleteLibraryFeedConfig(t *testing.T) {
	cmdRepo := fakes.NewFakeConfigCommandRepo()
	svc := appfeed.NewCommandService(cmdRepo, fakes.NewFakeConfigQueryRepo())

	ft := feed.LibraryType("lib-1")
	cmdRepo.AdminConfigs[ft] = &feed.AdminConfig{FeedType: ft, Enabled: true}

	require.NoError(t, svc.DeleteLibraryFeedConfig(context.Background(), "lib-1"))
	assert.Empty(t, cmdRepo.AdminConfigs)
}

func TestCommandService_WithEventBus_LibraryCreated(t *testing.T) {
	cmdRepo := fakes.NewFakeConfigCommandRepo()
	queryRepo := fakes.NewFakeConfigQueryRepo()
	bus := sharedfakes.NewFakeEventBus()
	svc := appfeed.NewCommandService(cmdRepo, queryRepo)
	appfeed.CommandWithEventBus(bus)(svc)

	require.NoError(t, bus.Publish(context.Background(), library.LibraryCreated{
		LibraryID: "lib-new",
		Name:      "TV Shows",
	}))

	ft := feed.LibraryType("lib-new")
	created := cmdRepo.AdminConfigs[ft]
	require.NotNil(t, created)
	assert.True(t, created.Enabled)
}

func TestCommandService_WithEventBus_LibraryDeleted(t *testing.T) {
	cmdRepo := fakes.NewFakeConfigCommandRepo()
	queryRepo := fakes.NewFakeConfigQueryRepo()
	bus := sharedfakes.NewFakeEventBus()
	svc := appfeed.NewCommandService(cmdRepo, queryRepo)
	appfeed.CommandWithEventBus(bus)(svc)

	ft := feed.LibraryType("lib-del")
	cmdRepo.AdminConfigs[ft] = &feed.AdminConfig{FeedType: ft, Enabled: true}

	require.NoError(t, bus.Publish(context.Background(), library.LibraryDeleted{
		LibraryID: "lib-del",
	}))

	assert.Empty(t, cmdRepo.AdminConfigs)
}

// Every reinstall of a widget used to leave its old plugin:<id> row in each
// profile's home config: nothing listened to the uninstall. The homes then
// rendered those ghosts (80 dp of nothing above the weather widget on mobile).
func TestCommandService_WithEventBus_PluginUninstalled(t *testing.T) {
	cmdRepo := fakes.NewFakeConfigCommandRepo()
	queryRepo := fakes.NewFakeConfigQueryRepo()
	bus := sharedfakes.NewFakeEventBus()
	svc := appfeed.NewCommandService(cmdRepo, queryRepo)
	appfeed.CommandWithEventBus(bus)(svc)

	gone := feed.PluginType("plg-gone")
	alive := feed.PluginType("plg-alive")
	cmdRepo.AdminConfigs[gone] = &feed.AdminConfig{FeedType: gone, Enabled: true}
	cmdRepo.ProfileConfigs["prf-1"] = map[feed.Type]*feed.ProfileConfig{
		gone:  {ProfileID: "prf-1", FeedType: gone},
		alive: {ProfileID: "prf-1", FeedType: alive},
	}
	cmdRepo.ProfileConfigs["prf-2"] = map[feed.Type]*feed.ProfileConfig{
		gone: {ProfileID: "prf-2", FeedType: gone},
	}

	require.NoError(t, bus.Publish(context.Background(), plugin.PluginUninstalled{PluginID: "plg-gone"}))

	assert.Empty(t, cmdRepo.AdminConfigs)
	assert.NotContains(t, cmdRepo.ProfileConfigs["prf-1"], gone, "the row is gone from every profile")
	assert.Contains(t, cmdRepo.ProfileConfigs["prf-1"], alive, "another plugin's row is untouched")
	assert.NotContains(t, cmdRepo.ProfileConfigs, "prf-2", "a disabled ghost goes too")
}

func TestCommandService_WithEventBus_WrongEventTypeIgnored(t *testing.T) {
	cmdRepo := fakes.NewFakeConfigCommandRepo()
	queryRepo := fakes.NewFakeConfigQueryRepo()
	bus := sharedfakes.NewFakeEventBus()
	svc := appfeed.NewCommandService(cmdRepo, queryRepo)
	appfeed.CommandWithEventBus(bus)(svc)

	_ = bus.Publish(context.Background(), library.LibraryDeleted{LibraryID: "x"})
	// no panic = success; deleted handler runs on "library.deleted" not "library.created"
	assert.Empty(t, cmdRepo.AdminConfigs)
}
