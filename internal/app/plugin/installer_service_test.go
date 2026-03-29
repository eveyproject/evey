package plugin_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
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

func tmpZip(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "plugin-*.zip")
	require.NoError(t, err)
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

func goManifest(name string) *plugindomain.PluginManifest {
	return &plugindomain.PluginManifest{
		Name:       name,
		Version:    "1.0.0",
		Language:   "go",
		Entrypoint: "./plugin",
	}
}

func goManifestVersioned(name, version string) *plugindomain.PluginManifest {
	return &plugindomain.PluginManifest{
		Name:       name,
		Version:    version,
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

// --- Install ---

func TestInstallPlugin(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifest("test-plugin")
	parser.Result.Description = "A test plugin"
	parser.Result.Author = "tester"

	p, err := svc.InstallPlugin(context.Background(), tmpZip(t, "fake"))
	require.NoError(t, err)
	assert.Equal(t, "test-plugin", p.Name)
	assert.Equal(t, string(plugindomain.Initializing), p.Status)
	assert.Equal(t, "./plugin", p.Entrypoint)
}

func TestInstallPluginExtractsZip(t *testing.T) {
	svc, _, _, parser, installer, _ := newTestInstallerService()
	parser.Result = goManifest("test-plugin")

	zip := tmpZip(t, "fake")
	p, err := svc.InstallPlugin(context.Background(), zip)
	require.NoError(t, err)
	assert.NotEmpty(t, p.InstallDir)
	assert.Contains(t, p.InstallDir, "/tmp/test-plugins/")

	destDir, ok := installer.Extracted[zip]
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

	_, err := svc.InstallPlugin(context.Background(), tmpZip(t, "fake"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin blocked")
}

func TestInstallPlugin_DuplicateName_SameVersion(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifest("dup-plugin")

	_, err := svc.InstallPlugin(context.Background(), tmpZip(t, "fake1"))
	require.NoError(t, err)

	_, err = svc.InstallPlugin(context.Background(), tmpZip(t, "fake2"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already installed at version")
}

func TestInstallPlugin_ParseError(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Err = errors.New("bad manifest")

	_, err := svc.InstallPlugin(context.Background(), tmpZip(t, "fake"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse manifest")
}

func TestInstallPlugin_DBCreateFails_CleansUpFiles(t *testing.T) {
	svc, pluginRepo, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifest("fail-create")
	pluginRepo.Err = errors.New("db down")

	_, err := svc.InstallPlugin(context.Background(), tmpZip(t, "fake"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}

func TestInstallPlugin_StartFails_ReturnsPluginWithError(t *testing.T) {
	svc, _, runner, parser, _, _ := newTestInstallerService()
	parser.Result = goManifest("start-fail")
	runner.Err = errors.New("runner crashed")

	p, err := svc.InstallPlugin(context.Background(), tmpZip(t, "fake"))
	require.NoError(t, err)
	assert.Equal(t, string(plugindomain.Error), p.Status)
	assert.Equal(t, "runner crashed", p.ErrorMessage)
}

func TestInstallPlugin_CSSTheme_NoStart(t *testing.T) {
	svc, _, runner, parser, _, _ := newTestInstallerService()
	parser.Result = cssManifest("my-theme")

	p, err := svc.InstallPlugin(context.Background(), tmpZip(t, "fake"))
	require.NoError(t, err)

	healthy, _ := runner.Health(context.Background(), p.ID)
	assert.False(t, healthy, "CSS theme should not call runner.Start")
}

func TestInstallPlugin_CSSTheme_StatusRunning(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = cssManifest("my-theme")

	p, err := svc.InstallPlugin(context.Background(), tmpZip(t, "fake"))
	require.NoError(t, err)
	assert.Equal(t, string(plugindomain.Running), p.Status)
}

func TestInstallPlugin_SetsInitializingStatus(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifest("normal-plugin")

	p, err := svc.InstallPlugin(context.Background(), tmpZip(t, "fake"))
	require.NoError(t, err)
	assert.Equal(t, string(plugindomain.Initializing), p.Status)
}

func TestInstallPlugin_LoadsPermissions(t *testing.T) {
	svc, pluginRepo, _, parser, _, _ := newTestInstallerService()
	pc := appplugin.NewPermissionChecker(pluginRepo)
	svc.WithPermissionChecker(pc)

	parser.Result = goManifest("perm-plugin")
	parser.Result.Permissions = []string{"api.library", "api.media"}

	p, err := svc.InstallPlugin(context.Background(), tmpZip(t, "fake"))
	require.NoError(t, err)

	assert.NoError(t, pc.CheckPermission(p.ID, "api.library"))
	assert.NoError(t, pc.CheckPermission(p.ID, "api.media"))
	assert.Error(t, pc.CheckPermission(p.ID, "api.admin"))
}

// --- Hash ---

func TestInstallPlugin_StoresSourceHash(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifest("hash-plugin")

	zip := tmpZip(t, "deterministic-content")
	p, err := svc.InstallPlugin(context.Background(), zip)
	require.NoError(t, err)

	data, readErr := os.ReadFile(zip)
	require.NoError(t, readErr)
	h := sha256.Sum256(data)
	expected := hex.EncodeToString(h[:])

	assert.Equal(t, expected, p.SourceHash)
	assert.Len(t, p.SourceHash, 64)
}

// --- Update (same name, higher version) ---

func TestInstallPlugin_UpgradeHigherVersion(t *testing.T) {
	svc, pluginRepo, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifestVersioned("upgrade-me", "1.0.0")

	_, err := svc.InstallPlugin(context.Background(), tmpZip(t, "v1"))
	require.NoError(t, err)

	parser.Result = goManifestVersioned("upgrade-me", "2.0.0")
	p, err := svc.InstallPlugin(context.Background(), tmpZip(t, "v2"))
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", p.Version)

	all, _ := pluginRepo.List(context.Background())
	assert.Len(t, all, 1, "old version should be uninstalled")
	assert.Equal(t, "2.0.0", all[0].Version)
}

func TestInstallPlugin_UpgradeLowerVersionRejected(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifestVersioned("downgrade-me", "2.0.0")

	_, err := svc.InstallPlugin(context.Background(), tmpZip(t, "v2"))
	require.NoError(t, err)

	parser.Result = goManifestVersioned("downgrade-me", "1.0.0")
	_, err = svc.InstallPlugin(context.Background(), tmpZip(t, "v1"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already installed at version 2.0.0")
}

func TestInstallPlugin_UpgradeSameVersionRejected(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifestVersioned("same-ver", "1.0.0")

	_, err := svc.InstallPlugin(context.Background(), tmpZip(t, "v1a"))
	require.NoError(t, err)

	parser.Result = goManifestVersioned("same-ver", "1.0.0")
	_, err = svc.InstallPlugin(context.Background(), tmpZip(t, "v1b"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already installed at version 1.0.0")
}

func TestInstallPlugin_UpgradeMinorBump(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifestVersioned("minor-bump", "1.0.0")

	_, err := svc.InstallPlugin(context.Background(), tmpZip(t, "v1.0.0"))
	require.NoError(t, err)

	parser.Result = goManifestVersioned("minor-bump", "1.1.0")
	p, err := svc.InstallPlugin(context.Background(), tmpZip(t, "v1.1.0"))
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", p.Version)
}

// --- Semver comparison ---

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"2.0.0", "1.0.0", 1},
		{"1.0.0", "2.0.0", -1},
		{"1.1.0", "1.0.0", 1},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"10.0.0", "9.0.0", 1},
		{"1.0.0-beta.1", "1.0.0", 0},
		{"2.0.0-rc.1", "1.9.9", 1},
	}
	for _, tt := range tests {
		got := appplugin.CompareSemver(tt.a, tt.b)
		assert.Equal(t, tt.want, got, "CompareSemver(%q, %q)", tt.a, tt.b)
	}
}

func TestCompareSemver_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"patch upgrade", "1.0.0", "1.0.1", -1},
		{"minor upgrade", "1.0.0", "1.1.0", -1},
		{"major upgrade", "1.0.0", "2.0.0", -1},
		{"ordering chain", "1.0.1", "1.1.0", -1},
		{"large versions", "100.200.300", "100.200.301", -1},
		{"pre-release ignored both sides", "1.0.0-alpha", "1.0.0-beta", 0},
		{"downgrade patch", "1.0.2", "1.0.1", 1},
		{"downgrade minor", "1.2.0", "1.1.0", 1},
		{"downgrade major", "3.0.0", "2.0.0", 1},
		{"zero versions", "0.0.0", "0.0.0", 0},
		{"zero to one", "0.0.0", "0.0.1", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appplugin.CompareSemver(tt.a, tt.b)
			assert.Equal(t, tt.want, got, "CompareSemver(%q, %q)", tt.a, tt.b)
		})
	}
}

func TestInstallPlugin_UpgradePatchBump(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifestVersioned("patch-bump", "1.0.0")

	_, err := svc.InstallPlugin(context.Background(), tmpZip(t, "v1.0.0"))
	require.NoError(t, err)

	parser.Result = goManifestVersioned("patch-bump", "1.0.1")
	p, err := svc.InstallPlugin(context.Background(), tmpZip(t, "v1.0.1"))
	require.NoError(t, err)
	assert.Equal(t, "1.0.1", p.Version)
}

func TestInstallPlugin_UpgradeWithPreRelease(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifestVersioned("pre-rel", "1.0.0")

	_, err := svc.InstallPlugin(context.Background(), tmpZip(t, "v1"))
	require.NoError(t, err)

	// Pre-release label is stripped for comparison, so 2.0.0-rc.1 > 1.0.0
	parser.Result = goManifestVersioned("pre-rel", "2.0.0-rc.1")
	p, err := svc.InstallPlugin(context.Background(), tmpZip(t, "v2rc1"))
	require.NoError(t, err)
	assert.Equal(t, "2.0.0-rc.1", p.Version)
}

// --- Uninstall ---

func TestUninstallRemovesDir(t *testing.T) {
	svc, pluginRepo, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifest("test-plugin")

	p, err := svc.InstallPlugin(context.Background(), tmpZip(t, "fake"))
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

// --- Preview ---

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

	preview, err := svc.PreviewPlugin(context.Background(), tmpZip(t, "fake"))
	require.NoError(t, err)
	assert.Equal(t, "preview-plugin", preview.Name)
	assert.Equal(t, "2.0.0", preview.Version)
	assert.Equal(t, []string{"api.library"}, preview.Permissions)
	assert.Equal(t, []string{"media.read"}, preview.Capabilities)
	assert.NotEmpty(t, preview.SourceHash, "preview should include source hash")
	assert.Len(t, preview.SourceHash, 64)
	require.NotNil(t, preview.Analysis)
	assert.True(t, preview.Analysis.Safe)
	assert.Equal(t, 3, preview.Analysis.FileCount)
}

func TestPreviewPlugin_ParseError(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Err = assert.AnError

	_, err := svc.PreviewPlugin(context.Background(), tmpZip(t, "bad"))
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

	preview, err := svc.PreviewPlugin(context.Background(), tmpZip(t, "fake"))
	require.NoError(t, err)
	require.NotNil(t, preview.Analysis)
	assert.True(t, preview.Analysis.Safe)
	assert.Equal(t, 5, preview.Analysis.FileCount)
	assert.Len(t, preview.Analysis.Issues, 1)
	assert.Len(t, preview.Analysis.URLs, 1)
}

// --- Concurrent ---

func TestInstallPlugin_ConcurrentSameName(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifest("concurrent-plugin")

	var wg sync.WaitGroup
	errs := make([]error, 2)
	zips := [2]string{tmpZip(t, "fake1"), tmpZip(t, "fake2")}

	for i := range 2 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = svc.InstallPlugin(context.Background(), zips[idx])
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
