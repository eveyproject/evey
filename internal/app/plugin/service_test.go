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

func newTestService() (*appplugin.Service, *plgfakes.FakePluginRepository, *plgfakes.FakePluginRunner, *plgfakes.FakeManifestParser, *plgfakes.FakePluginInstaller, *plgfakes.FakePluginAnalyzer) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runtimeRepo := plgfakes.NewFakePluginRuntimeRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	installer := plgfakes.NewFakePluginInstaller()
	analyzer := plgfakes.NewFakePluginAnalyzer()

	svc := appplugin.NewService(pluginRepo, runtimeRepo, runner, parser, bus).
		WithInstaller(installer).
		WithAnalyzer(analyzer).
		WithPluginDir("/tmp/test-plugins")

	return svc, pluginRepo, runner, parser, installer, analyzer
}

func TestInstallPlugin(t *testing.T) {
	svc, _, _, parser, _, _ := newTestService()

	parser.Result = &plugindomain.PluginManifest{
		Name:       "test-plugin",
		Version:    "1.0.0",
		Description: "A test plugin",
		Author:     "tester",
		Language:   "go",
		Entrypoint: "./plugin",
	}

	p, err := svc.InstallPlugin(context.Background(), "/tmp/plugin.zip")
	require.NoError(t, err)
	assert.Equal(t, "test-plugin", p.Name)
	assert.Equal(t, string(plugindomain.Initializing), p.Status)
	assert.Equal(t, "./plugin", p.Entrypoint)
}

func TestInstallPluginExtractsZip(t *testing.T) {
	svc, _, _, parser, installer, _ := newTestService()

	parser.Result = &plugindomain.PluginManifest{
		Name:       "test-plugin",
		Version:    "1.0.0",
		Language:   "go",
		Entrypoint: "./plugin",
	}

	p, err := svc.InstallPlugin(context.Background(), "/tmp/plugin.zip")
	require.NoError(t, err)
	assert.NotEmpty(t, p.InstallDir)
	assert.Contains(t, p.InstallDir, "/tmp/test-plugins/")

	// Verify extraction was called
	destDir, ok := installer.Extracted["/tmp/plugin.zip"]
	require.True(t, ok)
	assert.Equal(t, p.InstallDir, destDir)
}

func TestInstallPluginBlocksUnsafe(t *testing.T) {
	svc, _, _, parser, _, analyzer := newTestService()

	parser.Result = &plugindomain.PluginManifest{
		Name:       "unsafe-plugin",
		Version:    "1.0.0",
		Language:   "go",
		Entrypoint: "./plugin",
	}
	analyzer.Report = &plugindomain.AnalysisReport{
		Safe:   false,
		Issues: []string{"contains os/exec"},
	}

	_, err := svc.InstallPlugin(context.Background(), "/tmp/plugin.zip")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin blocked")
}

func TestAutoStartPlugins(t *testing.T) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runtimeRepo := plgfakes.NewFakePluginRuntimeRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()

	svc := appplugin.NewService(pluginRepo, runtimeRepo, runner, parser, bus)

	// Pre-populate with plugins in various states
	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-running", Name: "running-plugin",
		Status: string(plugindomain.Running), Enabled: true,
	}))
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-stopped", Name: "stopped-plugin",
		Status: string(plugindomain.Stopped), Enabled: true,
	}))
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-disabled", Name: "disabled-plugin",
		Status: string(plugindomain.Running), Enabled: false,
	}))

	svc.AutoStartPlugins(ctx)

	// Only the running+enabled plugin should be started
	healthy, _ := runner.Health(ctx, "plg-running")
	assert.True(t, healthy, "running+enabled plugin should be started")

	healthy, _ = runner.Health(ctx, "plg-stopped")
	assert.False(t, healthy, "stopped plugin should not be started")

	healthy, _ = runner.Health(ctx, "plg-disabled")
	assert.False(t, healthy, "disabled plugin should not be started")
}

func TestPreviewPlugin(t *testing.T) {
	svc, _, _, parser, _, analyzer := newTestService()

	parser.Result = &plugindomain.PluginManifest{
		Name:         "preview-plugin",
		Version:      "2.0.0",
		Description:  "desc",
		Author:       "author",
		Language:     "nodejs",
		Entrypoint:   "main.js",
		Permissions:  []string{"api.library"},
		Capabilities: []string{"media.read"},
	}
	analyzer.Report = &plugindomain.AnalysisReport{
		Safe:      true,
		FileCount: 3,
	}

	preview, err := svc.PreviewPlugin(context.Background(), "/tmp/plugin.zip")
	require.NoError(t, err)
	assert.Equal(t, "preview-plugin", preview.Name)
	assert.Equal(t, "2.0.0", preview.Version)
	assert.Equal(t, []string{"api.library"}, preview.Permissions)
	assert.Equal(t, []string{"media.read"}, preview.Capabilities)
	require.NotNil(t, preview.Analysis)
	assert.True(t, preview.Analysis.Safe)
	assert.Equal(t, 3, preview.Analysis.FileCount)
}

func TestPreviewPlugin_ParseError(t *testing.T) {
	svc, _, _, parser, _, _ := newTestService()
	parser.Err = assert.AnError

	_, err := svc.PreviewPlugin(context.Background(), "/tmp/bad.zip")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse manifest")
}

func TestUninstallRemovesDir(t *testing.T) {
	svc, pluginRepo, _, parser, _, _ := newTestService()

	parser.Result = &plugindomain.PluginManifest{
		Name:       "test-plugin",
		Version:    "1.0.0",
		Language:   "go",
		Entrypoint: "./plugin",
	}

	p, err := svc.InstallPlugin(context.Background(), "/tmp/plugin.zip")
	require.NoError(t, err)

	// Verify plugin exists
	_, err = pluginRepo.GetByID(context.Background(), p.ID)
	require.NoError(t, err)

	err = svc.UninstallPlugin(context.Background(), p.ID)
	require.NoError(t, err)

	// Verify plugin is deleted
	_, err = pluginRepo.GetByID(context.Background(), p.ID)
	assert.Error(t, err)
}
