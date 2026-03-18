package plugin_test

import (
	"context"
	"errors"
	"sync"
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

func newTestServiceWithBus() (*appplugin.Service, *plgfakes.FakePluginRepository, *plgfakes.FakePluginRunner, *plgfakes.FakeManifestParser, *plgfakes.FakePluginInstaller, *plgfakes.FakePluginAnalyzer, *sharedfakes.FakeEventBus) {
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

	return svc, pluginRepo, runner, parser, installer, analyzer, bus
}

func goManifest(name string) *plugindomain.PluginManifest {
	return &plugindomain.PluginManifest{
		Name:       name,
		Version:    "1.0.0",
		Language:   "go",
		Entrypoint: "./plugin",
	}
}

func cssManifest(name string) *plugindomain.PluginManifest {
	return &plugindomain.PluginManifest{
		Name:     name,
		Version:  "1.0.0",
		Language: "css",
	}
}

// ─── Install ────────────────────────────────────────────────────────────────

func TestInstallPlugin(t *testing.T) {
	svc, _, _, parser, _, _ := newTestService()
	parser.Result = goManifest("test-plugin")
	parser.Result.Description = "A test plugin"
	parser.Result.Author = "tester"

	p, err := svc.InstallPlugin(context.Background(), "/tmp/plugin.zip")
	require.NoError(t, err)
	assert.Equal(t, "test-plugin", p.Name)
	assert.Equal(t, string(plugindomain.Initializing), p.Status)
	assert.Equal(t, "./plugin", p.Entrypoint)
}

func TestInstallPluginExtractsZip(t *testing.T) {
	svc, _, _, parser, installer, _ := newTestService()
	parser.Result = goManifest("test-plugin")

	p, err := svc.InstallPlugin(context.Background(), "/tmp/plugin.zip")
	require.NoError(t, err)
	assert.NotEmpty(t, p.InstallDir)
	assert.Contains(t, p.InstallDir, "/tmp/test-plugins/")

	destDir, ok := installer.Extracted["/tmp/plugin.zip"]
	require.True(t, ok)
	assert.Equal(t, p.InstallDir, destDir)
}

func TestInstallPluginBlocksUnsafe(t *testing.T) {
	svc, _, _, parser, _, analyzer := newTestService()
	parser.Result = goManifest("unsafe-plugin")
	analyzer.Report = &plugindomain.AnalysisReport{
		Safe:   false,
		Issues: []string{"contains os/exec"},
	}

	_, err := svc.InstallPlugin(context.Background(), "/tmp/plugin.zip")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin blocked")
}

func TestInstallPlugin_DuplicateName(t *testing.T) {
	svc, _, _, parser, _, _ := newTestService()
	parser.Result = goManifest("dup-plugin")

	_, err := svc.InstallPlugin(context.Background(), "/tmp/a.zip")
	require.NoError(t, err)

	_, err = svc.InstallPlugin(context.Background(), "/tmp/b.zip")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already installed")
}

func TestInstallPlugin_ParseError(t *testing.T) {
	svc, _, _, parser, _, _ := newTestService()
	parser.Err = errors.New("bad manifest")

	_, err := svc.InstallPlugin(context.Background(), "/tmp/bad.zip")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse manifest")
}

func TestInstallPlugin_DBCreateFails_CleansUpFiles(t *testing.T) {
	svc, pluginRepo, _, parser, _, _ := newTestService()
	parser.Result = goManifest("fail-create")
	pluginRepo.Err = errors.New("db down")

	_, err := svc.InstallPlugin(context.Background(), "/tmp/plugin.zip")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}

func TestInstallPlugin_StartFails_ReturnsPluginWithError(t *testing.T) {
	svc, _, runner, parser, _, _ := newTestService()
	parser.Result = goManifest("start-fail")
	runner.Err = errors.New("runner crashed")

	p, err := svc.InstallPlugin(context.Background(), "/tmp/plugin.zip")
	require.NoError(t, err)
	assert.Equal(t, string(plugindomain.Error), p.Status)
	assert.Equal(t, "runner crashed", p.ErrorMessage)
}

func TestInstallPlugin_CSSTheme_NoStart(t *testing.T) {
	svc, _, runner, parser, _, _ := newTestService()
	parser.Result = cssManifest("my-theme")

	p, err := svc.InstallPlugin(context.Background(), "/tmp/theme.zip")
	require.NoError(t, err)

	healthy, _ := runner.Health(context.Background(), p.ID)
	assert.False(t, healthy, "CSS theme should not call runner.Start")
}

func TestInstallPlugin_CSSTheme_StatusRunning(t *testing.T) {
	svc, _, _, parser, _, _ := newTestService()
	parser.Result = cssManifest("my-theme")

	p, err := svc.InstallPlugin(context.Background(), "/tmp/theme.zip")
	require.NoError(t, err)
	assert.Equal(t, string(plugindomain.Running), p.Status)
}

func TestInstallPlugin_SetsInitializingStatus(t *testing.T) {
	svc, _, _, parser, _, _ := newTestService()
	parser.Result = goManifest("normal-plugin")

	p, err := svc.InstallPlugin(context.Background(), "/tmp/plugin.zip")
	require.NoError(t, err)
	assert.Equal(t, string(plugindomain.Initializing), p.Status)
}

func TestInstallPlugin_LoadsPermissions(t *testing.T) {
	svc, pluginRepo, _, parser, _, _ := newTestService()
	pc := appplugin.NewPermissionChecker(pluginRepo)
	svc.WithPermissionChecker(pc)

	parser.Result = goManifest("perm-plugin")
	parser.Result.Permissions = []string{"api.library", "api.media"}

	p, err := svc.InstallPlugin(context.Background(), "/tmp/plugin.zip")
	require.NoError(t, err)

	assert.NoError(t, pc.CheckPermission(p.ID, "api.library"))
	assert.NoError(t, pc.CheckPermission(p.ID, "api.media"))
	assert.Error(t, pc.CheckPermission(p.ID, "api.admin"))
}

// ─── Start ──────────────────────────────────────────────────────────────────

func TestStartPlugin_NotFound(t *testing.T) {
	svc, _, _, _, _, _ := newTestService()
	err := svc.StartPlugin(context.Background(), "plg-nonexistent")
	assert.ErrorIs(t, err, appplugin.ErrPluginNotFound)
}

func TestStartPlugin_AlreadyRunning(t *testing.T) {
	svc, pluginRepo, _, _, _, _ := newTestService()
	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-1", Name: "running", Status: string(plugindomain.Running), Enabled: true,
	}))

	err := svc.StartPlugin(ctx, "plg-1")
	assert.ErrorIs(t, err, appplugin.ErrAlreadyRunning)
}

func TestStartPlugin_CSSTheme_NoRunner(t *testing.T) {
	svc, pluginRepo, runner, _, _, _ := newTestService()
	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-css", Name: "theme", Language: "css", Status: string(plugindomain.Stopped), Enabled: true,
	}))

	err := svc.StartPlugin(ctx, "plg-css")
	require.NoError(t, err)

	healthy, _ := runner.Health(ctx, "plg-css")
	assert.False(t, healthy, "CSS plugin should not call runner")

	p, _ := pluginRepo.GetByID(ctx, "plg-css")
	assert.Equal(t, string(plugindomain.Running), p.Status)
}

func TestStartPlugin_Failure_SetsErrorStatus(t *testing.T) {
	svc, pluginRepo, runner, _, _, _ := newTestService()
	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-fail", Name: "failing", Language: "go", Status: string(plugindomain.Stopped), Enabled: true,
		InstallDir: "/tmp/test-plugins/plg-fail",
	}))
	runner.Err = errors.New("start failed")

	err := svc.StartPlugin(ctx, "plg-fail")
	require.Error(t, err)

	p, _ := pluginRepo.GetByID(ctx, "plg-fail")
	assert.Equal(t, string(plugindomain.Error), p.Status)
	assert.Equal(t, "start failed", p.ErrorMessage)
}

func TestStartPlugin_LoadsPermissions(t *testing.T) {
	svc, pluginRepo, _, _, _, _ := newTestService()
	pc := appplugin.NewPermissionChecker(pluginRepo)
	svc.WithPermissionChecker(pc)
	ctx := context.Background()

	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-perm", Name: "perm-start", Language: "go", Status: string(plugindomain.Stopped),
		Enabled: true, Permissions: []string{"api.library"},
		InstallDir: "/tmp/test-plugins/plg-perm",
	}))

	err := svc.StartPlugin(ctx, "plg-perm")
	require.NoError(t, err)
	assert.NoError(t, pc.CheckPermission("plg-perm", "api.library"))
}

// ─── Stop ───────────────────────────────────────────────────────────────────

func TestStopPlugin_NotFound(t *testing.T) {
	svc, _, _, _, _, _ := newTestService()
	err := svc.StopPlugin(context.Background(), "plg-nonexistent")
	assert.ErrorIs(t, err, appplugin.ErrPluginNotFound)
}

func TestStopPlugin_NotRunning(t *testing.T) {
	svc, pluginRepo, _, _, _, _ := newTestService()
	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-stopped", Name: "stopped", Status: string(plugindomain.Stopped), Enabled: true,
	}))

	err := svc.StopPlugin(ctx, "plg-stopped")
	assert.ErrorIs(t, err, appplugin.ErrNotRunning)
}

func TestStopPlugin_InvalidatesPermissions(t *testing.T) {
	svc, pluginRepo, _, _, _, _ := newTestService()
	pc := appplugin.NewPermissionChecker(pluginRepo)
	svc.WithPermissionChecker(pc)
	ctx := context.Background()

	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-inv", Name: "inv-stop", Language: "go", Status: string(plugindomain.Running),
		Enabled: true, Permissions: []string{"api.library"},
	}))
	pc.LoadPermissions("plg-inv", []string{"api.library"})

	assert.NoError(t, pc.CheckPermission("plg-inv", "api.library"))

	err := svc.StopPlugin(ctx, "plg-inv")
	require.NoError(t, err)

	assert.Error(t, pc.CheckPermission("plg-inv", "api.library"))
}

// ─── Enable / Disable ───────────────────────────────────────────────────────

func TestEnablePlugin(t *testing.T) {
	svc, pluginRepo, _, _, _, _ := newTestService()
	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-en", Name: "disabled-one", Enabled: false, Status: string(plugindomain.Stopped),
	}))

	err := svc.EnablePlugin(ctx, "plg-en")
	require.NoError(t, err)

	p, _ := pluginRepo.GetByID(ctx, "plg-en")
	assert.True(t, p.Enabled)
}

func TestDisablePlugin_StopsRunningPlugin(t *testing.T) {
	svc, pluginRepo, runner, _, _, _ := newTestService()
	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-dis", Name: "running-dis", Language: "go", Status: string(plugindomain.Running), Enabled: true,
	}))
	// Mark as running in runner
	runner.Start(ctx, &plugindomain.Plugin{ID: "plg-dis"}) //nolint:errcheck

	err := svc.DisablePlugin(ctx, "plg-dis")
	require.NoError(t, err)

	p, _ := pluginRepo.GetByID(ctx, "plg-dis")
	assert.False(t, p.Enabled)
	assert.Equal(t, string(plugindomain.Stopped), p.Status)

	healthy, _ := runner.Health(ctx, "plg-dis")
	assert.False(t, healthy)
}

func TestDisablePlugin_AlreadyStopped(t *testing.T) {
	svc, pluginRepo, runner, _, _, _ := newTestService()
	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-ds", Name: "already-stopped", Status: string(plugindomain.Stopped), Enabled: true,
	}))

	err := svc.DisablePlugin(ctx, "plg-ds")
	require.NoError(t, err)

	p, _ := pluginRepo.GetByID(ctx, "plg-ds")
	assert.False(t, p.Enabled)

	healthy, _ := runner.Health(ctx, "plg-ds")
	assert.False(t, healthy, "should not have been started")
}

// ─── Uninstall ──────────────────────────────────────────────────────────────

func TestUninstallRemovesDir(t *testing.T) {
	svc, pluginRepo, _, parser, _, _ := newTestService()
	parser.Result = goManifest("test-plugin")

	p, err := svc.InstallPlugin(context.Background(), "/tmp/plugin.zip")
	require.NoError(t, err)

	_, err = pluginRepo.GetByID(context.Background(), p.ID)
	require.NoError(t, err)

	err = svc.UninstallPlugin(context.Background(), p.ID)
	require.NoError(t, err)

	_, err = pluginRepo.GetByID(context.Background(), p.ID)
	assert.Error(t, err)
}

func TestUninstallPlugin_NotFound(t *testing.T) {
	svc, _, _, _, _, _ := newTestService()
	err := svc.UninstallPlugin(context.Background(), "plg-ghost")
	assert.ErrorIs(t, err, appplugin.ErrPluginNotFound)
}

func TestUninstallPlugin_StopsRunning(t *testing.T) {
	svc, pluginRepo, runner, _, _, _ := newTestService()
	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-run", Name: "running-uni", Language: "go", Status: string(plugindomain.Running), Enabled: true,
	}))
	runner.Start(ctx, &plugindomain.Plugin{ID: "plg-run"}) //nolint:errcheck

	err := svc.UninstallPlugin(ctx, "plg-run")
	require.NoError(t, err)

	healthy, _ := runner.Health(ctx, "plg-run")
	assert.False(t, healthy, "runner should be stopped")
}

func TestUninstallPlugin_InvalidatesPermissions(t *testing.T) {
	svc, pluginRepo, _, _, _, _ := newTestService()
	pc := appplugin.NewPermissionChecker(pluginRepo)
	svc.WithPermissionChecker(pc)
	ctx := context.Background()

	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-uinv", Name: "uni-perm", Language: "go", Status: string(plugindomain.Stopped),
		Enabled: true, Permissions: []string{"api.library"},
	}))
	pc.LoadPermissions("plg-uinv", []string{"api.library"})

	err := svc.UninstallPlugin(ctx, "plg-uinv")
	require.NoError(t, err)

	assert.Error(t, pc.CheckPermission("plg-uinv", "api.library"))
}

func TestUninstallPlugin_RemovesDir(t *testing.T) {
	svc, pluginRepo, _, _, _, _ := newTestService()
	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-rmdir", Name: "dir-plugin", Language: "go",
		Status: string(plugindomain.Stopped), Enabled: true,
		InstallDir: "/tmp/test-plugins/plg-rmdir",
	}))

	err := svc.UninstallPlugin(ctx, "plg-rmdir")
	require.NoError(t, err)

	_, err = pluginRepo.GetByID(ctx, "plg-rmdir")
	assert.Error(t, err, "plugin should be deleted from repo")
}

// ─── AutoStart ──────────────────────────────────────────────────────────────

func TestAutoStartPlugins(t *testing.T) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runtimeRepo := plgfakes.NewFakePluginRuntimeRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()

	svc := appplugin.NewService(pluginRepo, runtimeRepo, runner, parser, bus)

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

	healthy, _ := runner.Health(ctx, "plg-running")
	assert.True(t, healthy, "running+enabled plugin should be started")

	healthy, _ = runner.Health(ctx, "plg-stopped")
	assert.False(t, healthy, "stopped plugin should not be started")

	healthy, _ = runner.Health(ctx, "plg-disabled")
	assert.False(t, healthy, "disabled plugin should not be started")
}

func TestAutoStartPlugins_SkipsDisabled(t *testing.T) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runtimeRepo := plgfakes.NewFakePluginRuntimeRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	svc := appplugin.NewService(pluginRepo, runtimeRepo, runner, parser, bus)

	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-dis-auto", Name: "disabled-auto", Language: "go",
		Status: string(plugindomain.Running), Enabled: false,
	}))

	svc.AutoStartPlugins(ctx)

	healthy, _ := runner.Health(ctx, "plg-dis-auto")
	assert.False(t, healthy)
}

func TestAutoStartPlugins_SkipsCSS(t *testing.T) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runtimeRepo := plgfakes.NewFakePluginRuntimeRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	svc := appplugin.NewService(pluginRepo, runtimeRepo, runner, parser, bus)

	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-css-auto", Name: "css-theme", Language: "css",
		Status: string(plugindomain.Running), Enabled: true,
	}))

	svc.AutoStartPlugins(ctx)

	healthy, _ := runner.Health(ctx, "plg-css-auto")
	assert.False(t, healthy, "CSS theme should not be started via runner")
}

func TestAutoStartPlugins_RebuildsMissingFields(t *testing.T) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runtimeRepo := plgfakes.NewFakePluginRuntimeRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	svc := appplugin.NewService(pluginRepo, runtimeRepo, runner, parser, bus).
		WithPluginDir("/tmp/test-plugins")

	parser.Result = &plugindomain.PluginManifest{
		Name:       "rebuild",
		Entrypoint: "./rebuilt-entry",
	}

	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-rebuild", Name: "rebuild", Language: "go",
		Status: string(plugindomain.Running), Enabled: true,
		InstallDir: "", Entrypoint: "",
	}))

	svc.AutoStartPlugins(ctx)

	p, err := pluginRepo.GetByID(ctx, "plg-rebuild")
	require.NoError(t, err)
	assert.Equal(t, "/tmp/test-plugins/plg-rebuild", p.InstallDir)
	assert.Equal(t, "./rebuilt-entry", p.Entrypoint)
}

func TestAutoStartPlugins_LoadsPermissions(t *testing.T) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runtimeRepo := plgfakes.NewFakePluginRuntimeRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	pc := appplugin.NewPermissionChecker(pluginRepo)
	svc := appplugin.NewService(pluginRepo, runtimeRepo, runner, parser, bus).
		WithPermissionChecker(pc)

	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-apl", Name: "auto-perm", Language: "go",
		Status: string(plugindomain.Running), Enabled: true,
		Permissions: []string{"api.library"},
		InstallDir:  "/tmp/test-plugins/plg-apl",
	}))

	svc.AutoStartPlugins(ctx)

	assert.NoError(t, pc.CheckPermission("plg-apl", "api.library"))
}

// ─── Preview ────────────────────────────────────────────────────────────────

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

func TestPreviewPlugin_WithAnalysis(t *testing.T) {
	svc, _, _, parser, _, analyzer := newTestService()
	parser.Result = goManifest("analyzed")
	analyzer.Report = &plugindomain.AnalysisReport{
		Safe:      true,
		FileCount: 5,
		Issues:    []string{"minor: unused import"},
		URLs:      []plugindomain.ExtractedURL{{URL: "http://example.com", File: "main.go", Line: 10, Type: "literal"}},
	}

	preview, err := svc.PreviewPlugin(context.Background(), "/tmp/plugin.zip")
	require.NoError(t, err)
	require.NotNil(t, preview.Analysis)
	assert.True(t, preview.Analysis.Safe)
	assert.Equal(t, 5, preview.Analysis.FileCount)
	assert.Len(t, preview.Analysis.Issues, 1)
	assert.Len(t, preview.Analysis.URLs, 1)
}

// ─── Config ─────────────────────────────────────────────────────────────────

func TestGetPluginConfig(t *testing.T) {
	svc, pluginRepo, _, _, _, _ := newTestService()
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
	svc, _, _, _, _, _ := newTestService()
	_, err := svc.GetPluginConfig(context.Background(), "plg-nope")
	assert.ErrorIs(t, err, appplugin.ErrPluginNotFound)
}

func TestUpdatePluginConfig(t *testing.T) {
	svc, pluginRepo, _, _, _, _ := newTestService()
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
	svc, _, _, _, _, _ := newTestService()
	err := svc.UpdatePluginConfig(context.Background(), "plg-nope", map[string]any{})
	assert.ErrorIs(t, err, appplugin.ErrPluginNotFound)
}

// ─── Concurrent ─────────────────────────────────────────────────────────────

func TestInstallPlugin_ConcurrentSameName(t *testing.T) {
	svc, _, _, parser, _, _ := newTestService()
	parser.Result = goManifest("concurrent-plugin")

	var wg sync.WaitGroup
	errs := make([]error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = svc.InstallPlugin(context.Background(), "/tmp/plugin.zip")
		}(i)
	}
	wg.Wait()

	successes := 0
	failures := 0
	for _, err := range errs {
		if err == nil {
			successes++
		} else {
			failures++
			assert.Contains(t, err.Error(), "already installed")
		}
	}
	assert.Equal(t, 1, successes, "exactly one install should succeed")
	assert.Equal(t, 1, failures, "exactly one install should fail")
}
