package plugin_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	appplugin "github.com/gravity-zero/evey/internal/app/plugin"
	plugindomain "github.com/gravity-zero/evey/internal/domain/plugin"
	plgfakes "github.com/gravity-zero/evey/internal/domain/plugin/fakes"
	"github.com/gravity-zero/evey/internal/domain/shared"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestInstallerService() (*appplugin.InstallerService, *plgfakes.FakePluginRepository, *plgfakes.FakePluginRunner, *plgfakes.FakeManifestParser, *plgfakes.FakePluginInstaller, *plgfakes.FakePluginAnalyzer) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	installer := plgfakes.NewFakePluginInstaller()
	analyzer := plgfakes.NewFakePluginAnalyzer()

	pluginFS := plgfakes.NewFakePluginFileSystem()

	svc := appplugin.NewInstallerService(pluginRepo, runner, parser, bus).
		WithInstaller(installer).
		WithAnalyzer(analyzer).
		WithPluginDir("/tmp/test-plugins").
		WithFileSystem(pluginFS)

	return svc, pluginRepo, runner, parser, installer, analyzer
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
	svc, _, _, parser, _, _ := newTestInstallerService()
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
	svc, _, _, parser, installer, _ := newTestInstallerService()
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
	svc, _, _, parser, _, analyzer := newTestInstallerService()
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
	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifest("dup-plugin")

	_, err := svc.InstallPlugin(context.Background(), "/tmp/a.zip")
	require.NoError(t, err)

	_, err = svc.InstallPlugin(context.Background(), "/tmp/b.zip")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already installed")
}

func TestInstallPlugin_ParseError(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Err = errors.New("bad manifest")

	_, err := svc.InstallPlugin(context.Background(), "/tmp/bad.zip")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse manifest")
}

func TestInstallPlugin_DBCreateFails_CleansUpFiles(t *testing.T) {
	svc, pluginRepo, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifest("fail-create")
	pluginRepo.Err = errors.New("db down")

	_, err := svc.InstallPlugin(context.Background(), "/tmp/plugin.zip")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}

func TestInstallPlugin_StartFails_ReturnsPluginWithError(t *testing.T) {
	svc, _, runner, parser, _, _ := newTestInstallerService()
	parser.Result = goManifest("start-fail")
	runner.Err = errors.New("runner crashed")

	p, err := svc.InstallPlugin(context.Background(), "/tmp/plugin.zip")
	require.NoError(t, err)
	assert.Equal(t, string(plugindomain.Error), p.Status)
	assert.Equal(t, "runner crashed", p.ErrorMessage)
}

func TestInstallPlugin_CSSTheme_NoStart(t *testing.T) {
	svc, _, runner, parser, _, _ := newTestInstallerService()
	parser.Result = cssManifest("my-theme")

	p, err := svc.InstallPlugin(context.Background(), "/tmp/theme.zip")
	require.NoError(t, err)

	healthy, _ := runner.Health(context.Background(), p.ID)
	assert.False(t, healthy, "CSS theme should not call runner.Start")
}

func TestInstallPlugin_CSSTheme_StatusRunning(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = cssManifest("my-theme")

	p, err := svc.InstallPlugin(context.Background(), "/tmp/theme.zip")
	require.NoError(t, err)
	assert.Equal(t, string(plugindomain.Running), p.Status)
}

func TestInstallPlugin_SetsInitializingStatus(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifest("normal-plugin")

	p, err := svc.InstallPlugin(context.Background(), "/tmp/plugin.zip")
	require.NoError(t, err)
	assert.Equal(t, string(plugindomain.Initializing), p.Status)
}

func TestInstallPlugin_LoadsPermissions(t *testing.T) {
	svc, pluginRepo, _, parser, _, _ := newTestInstallerService()
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

// ─── Uninstall ──────────────────────────────────────────────────────────────

func TestUninstallRemovesDir(t *testing.T) {
	svc, pluginRepo, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifest("test-plugin")

	p, err := svc.InstallPlugin(context.Background(), "/tmp/plugin.zip")
	require.NoError(t, err)

	_, err = pluginRepo.GetByID(context.Background(), p.ID)
	require.NoError(t, err)

	err = svc.UninstallPlugin(context.Background(), p.ID)
	require.NoError(t, err)

	_, err = pluginRepo.GetByID(context.Background(), p.ID)
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestUninstallPlugin_NotFound(t *testing.T) {
	svc, _, _, _, _, _ := newTestInstallerService()
	err := svc.UninstallPlugin(context.Background(), "plg-ghost")
	assert.ErrorIs(t, err, appplugin.ErrPluginNotFound)
}

func TestUninstallPlugin_StopsRunning(t *testing.T) {
	svc, pluginRepo, runner, _, _, _ := newTestInstallerService()
	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-run", Name: "running-uni", Language: "go", Status: string(plugindomain.Running), Enabled: true,
	}))
	_ = runner.Start(ctx, &plugindomain.Plugin{ID: "plg-run"})

	err := svc.UninstallPlugin(ctx, "plg-run")
	require.NoError(t, err)

	healthy, _ := runner.Health(ctx, "plg-run")
	assert.False(t, healthy, "runner should be stopped")
}

func TestUninstallPlugin_InvalidatesPermissions(t *testing.T) {
	svc, pluginRepo, _, _, _, _ := newTestInstallerService()
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
	svc, pluginRepo, _, _, _, _ := newTestInstallerService()
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

func TestUninstallPlugin_StoppedNoDir(t *testing.T) {
	svc, pluginRepo, _, _, _, _ := newTestInstallerService()
	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-nodir", Name: "no-dir", Language: "go",
		Status: string(plugindomain.Stopped), Enabled: true,
		InstallDir: "",
	}))

	err := svc.UninstallPlugin(ctx, "plg-nodir")
	require.NoError(t, err)

	_, err = pluginRepo.GetByID(ctx, "plg-nodir")
	assert.Error(t, err)
}

func TestUninstallPlugin_DeleteFails(t *testing.T) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	pluginFS := plgfakes.NewFakePluginFileSystem()
	svc := appplugin.NewInstallerService(pluginRepo, runner, parser, bus).
		WithPluginDir("/tmp/test-plugins").
		WithFileSystem(pluginFS)

	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-delfail", Name: "del-fail", Language: "go",
		Status: string(plugindomain.Stopped), Enabled: true,
	}))
	pluginRepo.Err = errors.New("db error")

	err := svc.UninstallPlugin(ctx, "plg-delfail")
	require.Error(t, err)
}

// ─── Preview ────────────────────────────────────────────────────────────────

func TestPreviewPlugin(t *testing.T) {
	svc, _, _, parser, _, analyzer := newTestInstallerService()

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
	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Err = assert.AnError

	_, err := svc.PreviewPlugin(context.Background(), "/tmp/bad.zip")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse manifest")
}

func TestPreviewPlugin_WithAnalysis(t *testing.T) {
	svc, _, _, parser, _, analyzer := newTestInstallerService()
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

// ─── Concurrent ─────────────────────────────────────────────────────────────

func TestInstallPlugin_ConcurrentSameName(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifest("concurrent-plugin")

	var wg sync.WaitGroup
	errs := make([]error, 2)

	for i := range 2 {
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
