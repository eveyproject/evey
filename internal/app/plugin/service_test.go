package plugin_test

import (
	"context"
	"testing"

	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	plugindomain "github.com/eveyproject/evey/internal/domain/plugin"
	plgfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestQueryService() (*appplugin.QueryService, *plgfakes.FakePluginRepository) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runtimeRepo := plgfakes.NewFakePluginRuntimeRepository()
	svc := appplugin.NewQueryService(pluginRepo, runtimeRepo)
	return svc, pluginRepo
}

// ─── Config ─────────────────────────────────────────────────────────────────

func TestGetPluginConfig(t *testing.T) {
	svc, pluginRepo := newTestQueryService()
	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-cfg", Name: "config-plugin",
		Config:       map[string]any{"theme": "dark", "limit": 10},
		ConfigSchema: map[string]any{"theme": map[string]any{"type": "select", "options": []string{"light", "dark"}}},
		Status:       plugindomain.Stopped,
	}))

	result, err := svc.GetPluginConfig(ctx, "plg-cfg")
	require.NoError(t, err)
	assert.Equal(t, "dark", result.Config["theme"])
	assert.Equal(t, 10, result.Config["limit"])
	assert.NotNil(t, result.ConfigSchema)
	assert.Equal(t, "select", result.ConfigSchema["theme"].(map[string]any)["type"])
}

func TestGetPluginConfig_NotFound(t *testing.T) {
	svc, _ := newTestQueryService()
	_, err := svc.GetPluginConfig(context.Background(), "plg-nope")
	assert.ErrorIs(t, err, appplugin.ErrPluginNotFound)
}

func TestUpdatePluginConfig(t *testing.T) {
	svc, pluginRepo, _, _ := newTestRuntimeService()
	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-ucfg", Name: "cfg-update",
		Config: map[string]any{"old": "value"},
		Status: plugindomain.Stopped,
	}))

	newCfg := map[string]any{"new": "value", "count": 42}
	err := svc.UpdatePluginConfig(ctx, "plg-ucfg", newCfg)
	require.NoError(t, err)

	p, _ := pluginRepo.GetByID(ctx, "plg-ucfg")
	assert.Equal(t, "value", p.Config["new"])
	assert.Equal(t, 42, p.Config["count"])
	assert.Nil(t, p.Config["old"])
}

func TestUpdatePluginConfig_NotFound(t *testing.T) {
	svc, _, _, _ := newTestRuntimeService()
	err := svc.UpdatePluginConfig(context.Background(), "plg-nope", map[string]any{})
	assert.ErrorIs(t, err, appplugin.ErrPluginNotFound)
}

// ─── ListPlugins ────────────────────────────────────────────────────────────

func TestListPlugins(t *testing.T) {
	svc, pluginRepo := newTestQueryService()
	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{ID: "plg-a", Name: "alpha"}))
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{ID: "plg-b", Name: "beta"}))

	list, err := svc.ListPlugins(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestListPlugins_Empty(t *testing.T) {
	svc, _ := newTestQueryService()
	list, err := svc.ListPlugins(context.Background())
	require.NoError(t, err)
	assert.Empty(t, list)
}

// ─── ListRuntimes ───────────────────────────────────────────────────────────

func TestListRuntimes(t *testing.T) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runtimeRepo := plgfakes.NewFakePluginRuntimeRepository()
	svc := appplugin.NewQueryService(pluginRepo, runtimeRepo)
	ctx := context.Background()

	require.NoError(t, runtimeRepo.Create(ctx, &plugindomain.PluginRuntime{ID: "rt-1", Language: "go", Version: "1.22"}))
	require.NoError(t, runtimeRepo.Create(ctx, &plugindomain.PluginRuntime{ID: "rt-2", Language: "nodejs", Version: "20"}))

	list, err := svc.ListRuntimes(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestListRuntimes_Empty(t *testing.T) {
	svc, _ := newTestQueryService()
	list, err := svc.ListRuntimes(context.Background())
	require.NoError(t, err)
	assert.Empty(t, list)
}
