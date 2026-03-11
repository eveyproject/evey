package plugin_test

import (
	"context"
	"testing"

	appplugin "github.com/gravity-zero/evey/internal/app/plugin"
	plugindomain "github.com/gravity-zero/evey/internal/domain/plugin"
	plgfakes "github.com/gravity-zero/evey/internal/domain/plugin/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallPlugin(t *testing.T) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runtimeRepo := plgfakes.NewFakePluginRuntimeRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()

	parser.Result = &plugindomain.PluginManifest{
		Name:        "test-plugin",
		Version:     "1.0.0",
		Description: "A test plugin",
		Author:      "tester",
		Language:    "go",
	}

	svc := appplugin.NewService(pluginRepo, runtimeRepo, runner, parser, bus)
	p, err := svc.InstallPlugin(context.Background(), "/tmp/plugin.zip")
	require.NoError(t, err)
	assert.Equal(t, "test-plugin", p.Name)
	assert.Equal(t, string(plugindomain.Running), p.Status)
}
