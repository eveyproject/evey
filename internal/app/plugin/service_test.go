package plugin_test

import (
	"context"
	"testing"

	appplugin "github.com/gravity-zero/evey/internal/app/plugin"
	plugindomain "github.com/gravity-zero/evey/internal/domain/plugin"
	plgfakes "github.com/gravity-zero/evey/internal/domain/plugin/fakes"
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
		Config: map[string]any{"theme": "dark", "limit": 10},
		Status: string(plugindomain.Stopped),
	}))

	cfg, err := svc.GetPluginConfig(ctx, "plg-cfg")
	require.NoError(t, err)
	assert.Equal(t, "dark", cfg["theme"])
	assert.Equal(t, 10, cfg["limit"])
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
		Status: string(plugindomain.Stopped),
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
