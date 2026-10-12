package grpcapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appconfig "github.com/eveyproject/evey/internal/app/config"
	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	plugindomain "github.com/eveyproject/evey/internal/domain/plugin"
	pluginfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// The admin config route must hide secret fields exactly like the public one
// does (Plugin.PublicConfig): an admin response is still a response that can be
// logged, proxied or read over a shoulder.
func newSecretConfigHandler(t *testing.T) (*adminPluginHandler, *pluginfakes.FakePluginRepository) {
	t.Helper()
	events := sharedfakes.NewFakeEventBus()
	repo := pluginfakes.NewFakePluginRepository()
	rtRepo := pluginfakes.NewFakePluginRuntimeRepository()
	runner := pluginfakes.NewFakePluginRunner()
	parser := pluginfakes.NewFakeManifestParser()
	h := &adminPluginHandler{
		pluginRuntime: appplugin.NewRuntimeService(repo, rtRepo, runner, parser, events),
		pluginQuery:   appplugin.NewQueryService(repo, rtRepo),
		enabled:       true,
	}
	require.NoError(t, repo.Create(context.Background(), &plugindomain.Plugin{
		ID: "plg-sec", Name: "sec", Version: "1.0.0",
		Kind: plugindomain.KindPlugin, Language: "go", Entrypoint: "./p",
		Config: map[string]any{"api_key": "real-key", "region": "eu", "empty_secret": ""},
		ConfigSchema: map[string]any{
			"api_key":      map[string]any{"type": "string", "secret": true},
			"region":       map[string]any{"type": "string"},
			"empty_secret": map[string]any{"type": "password"},
		},
	}))
	return h, repo
}

func TestAdminGetPluginConfig_MasksSecretFields(t *testing.T) {
	h, _ := newSecretConfigHandler(t)
	resp, err := h.GetPluginConfig(context.Background(), &eveyv1.GetPluginConfigRequest{PluginId: "plg-sec"})
	require.NoError(t, err)
	assert.Equal(t, appconfig.SecretMask, resp.Values["api_key"])
	assert.Equal(t, "eu", resp.Values["region"])
	// An unset secret stays empty: masking it would claim a credential exists.
	assert.Empty(t, resp.Values["empty_secret"])
}

func TestAdminUpdatePluginConfig_MaskEchoKeepsStoredSecret(t *testing.T) {
	h, repo := newSecretConfigHandler(t)
	ctx := context.Background()
	resp, err := h.UpdatePluginConfig(ctx, &eveyv1.UpdatePluginConfigRequest{
		PluginId: "plg-sec",
		Values:   map[string]string{"api_key": appconfig.SecretMask, "region": "us"},
	})
	require.NoError(t, err)
	assert.Equal(t, appconfig.SecretMask, resp.Values["api_key"])
	assert.Equal(t, "us", resp.Values["region"])

	p, err := repo.GetByID(ctx, "plg-sec")
	require.NoError(t, err)
	assert.Equal(t, "real-key", p.Config["api_key"])
	assert.Equal(t, "us", p.Config["region"])
}

func TestAdminUpdatePluginConfig_NewSecretValueIsStored(t *testing.T) {
	h, repo := newSecretConfigHandler(t)
	ctx := context.Background()
	resp, err := h.UpdatePluginConfig(ctx, &eveyv1.UpdatePluginConfigRequest{
		PluginId: "plg-sec",
		Values:   map[string]string{"api_key": "new-key", "region": "eu"},
	})
	require.NoError(t, err)
	assert.Equal(t, appconfig.SecretMask, resp.Values["api_key"])

	p, err := repo.GetByID(ctx, "plg-sec")
	require.NoError(t, err)
	assert.Equal(t, "new-key", p.Config["api_key"])
}
