package plugin_test

import (
	"context"
	"errors"
	"testing"

	appplugin "github.com/evey-project/evey/internal/app/plugin"
	plugindomain "github.com/evey-project/evey/internal/domain/plugin"
	plgfakes "github.com/evey-project/evey/internal/domain/plugin/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRuntimeService() (*appplugin.RuntimeService, *plgfakes.FakePluginRepository, *plgfakes.FakePluginRunner, *plgfakes.FakeManifestParser) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()

	svc := appplugin.NewRuntimeService(pluginRepo, plgfakes.NewFakePluginRuntimeRepository(), runner, parser, bus,
		appplugin.RuntimeWithPluginDir("/tmp/test-plugins"),
	)

	return svc, pluginRepo, runner, parser
}

// ─── Start ──────────────────────────────────────────────────────────────────

func TestStartPlugin_NotFound(t *testing.T) {
	svc, _, _, _ := newTestRuntimeService()
	err := svc.StartPlugin(context.Background(), "plg-nonexistent")
	assert.ErrorIs(t, err, appplugin.ErrPluginNotFound)
}

func TestStartPlugin_AlreadyRunning(t *testing.T) {
	svc, pluginRepo, _, _ := newTestRuntimeService()
	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-1", Name: "running", Status: string(plugindomain.Running), Enabled: true,
	}))

	err := svc.StartPlugin(ctx, "plg-1")
	assert.ErrorIs(t, err, appplugin.ErrAlreadyRunning)
}

func TestStartPlugin_CSSTheme_NoRunner(t *testing.T) {
	svc, pluginRepo, runner, _ := newTestRuntimeService()
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
	svc, pluginRepo, runner, _ := newTestRuntimeService()
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
	svc, pluginRepo, _, _ := newTestRuntimeService()
	pc := appplugin.NewPermissionChecker(pluginRepo)
	appplugin.RuntimeWithPermissionChecker(pc)(svc)
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
	svc, _, _, _ := newTestRuntimeService()
	err := svc.StopPlugin(context.Background(), "plg-nonexistent")
	assert.ErrorIs(t, err, appplugin.ErrPluginNotFound)
}

func TestStopPlugin_NotRunning(t *testing.T) {
	svc, pluginRepo, _, _ := newTestRuntimeService()
	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-stopped", Name: "stopped", Status: string(plugindomain.Stopped), Enabled: true,
	}))

	err := svc.StopPlugin(ctx, "plg-stopped")
	assert.ErrorIs(t, err, appplugin.ErrNotRunning)
}

func TestStopPlugin_InvalidatesPermissions(t *testing.T) {
	svc, pluginRepo, _, _ := newTestRuntimeService()
	pc := appplugin.NewPermissionChecker(pluginRepo)
	appplugin.RuntimeWithPermissionChecker(pc)(svc)
	ctx := context.Background()

	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-inv", Name: "inv-stop", Language: "go", Status: string(plugindomain.Running),
		Enabled: true, Permissions: []string{"api.library"},
	}))
	pc.LoadPermissions("plg-inv", []string{"api.library"})

	require.NoError(t, pc.CheckPermission("plg-inv", "api.library"))

	err := svc.StopPlugin(ctx, "plg-inv")
	require.NoError(t, err)

	assert.Error(t, pc.CheckPermission("plg-inv", "api.library"))
}

func TestStartPlugin_EmptyInstallDir_Rebuilt(t *testing.T) {
	svc, pluginRepo, _, _ := newTestRuntimeService()
	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-nodir", Name: "no-dir", Language: "go",
		Status: string(plugindomain.Stopped), Enabled: true,
		InstallDir: "",
	}))

	err := svc.StartPlugin(ctx, "plg-nodir")
	require.NoError(t, err)

	p, _ := pluginRepo.GetByID(ctx, "plg-nodir")
	assert.Equal(t, "/tmp/test-plugins/plg-nodir", p.InstallDir)
}

// ─── Enable / Disable ───────────────────────────────────────────────────────

func TestEnablePlugin_NotFound(t *testing.T) {
	svc, _, _, _ := newTestRuntimeService()
	err := svc.EnablePlugin(context.Background(), "plg-nope")
	assert.ErrorIs(t, err, appplugin.ErrPluginNotFound)
}

func TestDisablePlugin_NotFound(t *testing.T) {
	svc, _, _, _ := newTestRuntimeService()
	err := svc.DisablePlugin(context.Background(), "plg-nope")
	assert.ErrorIs(t, err, appplugin.ErrPluginNotFound)
}

func TestEnablePlugin(t *testing.T) {
	svc, pluginRepo, _, _ := newTestRuntimeService()
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
	svc, pluginRepo, runner, _ := newTestRuntimeService()
	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-dis", Name: "running-dis", Language: "go", Status: string(plugindomain.Running), Enabled: true,
	}))
	_ = runner.Start(ctx, &plugindomain.Plugin{ID: "plg-dis"})

	err := svc.DisablePlugin(ctx, "plg-dis")
	require.NoError(t, err)

	p, _ := pluginRepo.GetByID(ctx, "plg-dis")
	assert.False(t, p.Enabled)
	assert.Equal(t, string(plugindomain.Stopped), p.Status)

	healthy, _ := runner.Health(ctx, "plg-dis")
	assert.False(t, healthy)
}

func TestDisablePlugin_AlreadyStopped(t *testing.T) {
	svc, pluginRepo, runner, _ := newTestRuntimeService()
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

// ─── AutoStart ──────────────────────────────────────────────────────────────

func TestAutoStartPlugins(t *testing.T) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()

	svc := appplugin.NewRuntimeService(pluginRepo, plgfakes.NewFakePluginRuntimeRepository(), runner, parser, bus)

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
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	svc := appplugin.NewRuntimeService(pluginRepo, plgfakes.NewFakePluginRuntimeRepository(), runner, parser, bus)

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
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	svc := appplugin.NewRuntimeService(pluginRepo, plgfakes.NewFakePluginRuntimeRepository(), runner, parser, bus)

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
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	svc := appplugin.NewRuntimeService(pluginRepo, plgfakes.NewFakePluginRuntimeRepository(), runner, parser, bus,
		appplugin.RuntimeWithPluginDir("/tmp/test-plugins"),
	)

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

func TestAutoStartPlugins_StartFails_SetsErrorStatus(t *testing.T) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	svc := appplugin.NewRuntimeService(pluginRepo, plgfakes.NewFakePluginRuntimeRepository(), runner, parser, bus,
		appplugin.RuntimeWithPluginDir("/tmp/test-plugins"),
	)

	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-as-fail", Name: "auto-fail", Language: "go",
		Status: string(plugindomain.Running), Enabled: true,
		InstallDir: "/tmp/test-plugins/plg-as-fail",
	}))
	runner.Err = errors.New("runner down")

	svc.AutoStartPlugins(ctx)

	p, err := pluginRepo.GetByID(ctx, "plg-as-fail")
	require.NoError(t, err)
	assert.Equal(t, string(plugindomain.Error), p.Status)
	assert.Equal(t, "runner down", p.ErrorMessage)
}

func TestAutoStartPlugins_InitializingStatus(t *testing.T) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	svc := appplugin.NewRuntimeService(pluginRepo, plgfakes.NewFakePluginRuntimeRepository(), runner, parser, bus)

	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-init", Name: "init-plugin", Language: "go",
		Status: string(plugindomain.Initializing), Enabled: true,
		InstallDir: "/tmp/test-plugins/plg-init",
	}))

	svc.AutoStartPlugins(ctx)

	healthy, _ := runner.Health(ctx, "plg-init")
	assert.True(t, healthy, "initializing+enabled plugin should be started")
}

func TestAutoStartPlugins_LoadsPermissions(t *testing.T) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	pc := appplugin.NewPermissionChecker(pluginRepo)
	svc := appplugin.NewRuntimeService(pluginRepo, plgfakes.NewFakePluginRuntimeRepository(), runner, parser, bus,
		appplugin.RuntimeWithPermissionChecker(pc),
	)

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

// ─── HandleIPCResult ────────────────────────────────────────────────────────

func TestHandleIPCResult_Success(t *testing.T) {
	svc, pluginRepo, _, _ := newTestRuntimeService()
	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-ipc", Name: "ipc-ok", Status: string(plugindomain.Initializing),
	}))

	svc.HandleIPCResult("plg-ipc", nil)

	p, err := pluginRepo.GetByID(ctx, "plg-ipc")
	require.NoError(t, err)
	assert.Equal(t, "running", p.Status)
	assert.Empty(t, p.ErrorMessage)
}

func TestHandleIPCResult_Error(t *testing.T) {
	svc, pluginRepo, _, _ := newTestRuntimeService()
	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-ipc-err", Name: "ipc-fail", Status: string(plugindomain.Initializing),
	}))

	svc.HandleIPCResult("plg-ipc-err", errors.New("ipc timeout"))

	p, err := pluginRepo.GetByID(ctx, "plg-ipc-err")
	require.NoError(t, err)
	assert.Equal(t, "error", p.Status)
	assert.Equal(t, "ipc timeout", p.ErrorMessage)
}

func TestHandleIPCResult_NotFound(t *testing.T) {
	svc, _, _, _ := newTestRuntimeService()
	svc.HandleIPCResult("plg-ghost", nil) // should not panic
}

// ─── AddRuntime / RemoveRuntime ─────────────────────────────────────────────

func TestAddRuntime(t *testing.T) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runtimeRepo := plgfakes.NewFakePluginRuntimeRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	svc := appplugin.NewRuntimeService(pluginRepo, runtimeRepo, runner, parser, bus)

	ctx := context.Background()
	rt := &plugindomain.PluginRuntime{ID: "rt-1", Language: "go", Version: "1.22"}
	require.NoError(t, svc.AddRuntime(ctx, rt))

	list, err := runtimeRepo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "rt-1", list[0].ID)
}

func TestRemoveRuntime(t *testing.T) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runtimeRepo := plgfakes.NewFakePluginRuntimeRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	svc := appplugin.NewRuntimeService(pluginRepo, runtimeRepo, runner, parser, bus)

	ctx := context.Background()
	require.NoError(t, runtimeRepo.Create(ctx, &plugindomain.PluginRuntime{ID: "rt-del", Language: "nodejs", Version: "20"}))

	require.NoError(t, svc.RemoveRuntime(ctx, "rt-del"))

	list, err := runtimeRepo.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestRemoveRuntime_NotFound(t *testing.T) {
	svc, _, _, _ := newTestRuntimeService()
	err := svc.RemoveRuntime(context.Background(), "rt-ghost")
	assert.Error(t, err)
}
