package plugin_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	plugindomain "github.com/eveyproject/evey/internal/domain/plugin"
	plgfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
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

	svc := appplugin.NewInstallerService(pluginRepo, runner, parser, bus,
		appplugin.InstallerWithInstaller(installer),
		appplugin.InstallerWithAnalyzer(analyzer),
		appplugin.InstallerWithPluginDir("/tmp/test-plugins"),
		appplugin.InstallerWithFileSystem(pluginFS),
		appplugin.InstallerWithHTTPClient(http.DefaultClient),
	)

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
		Kind:       plugindomain.KindPlugin,
		Version:    "1.0.0",
		Language:   "go",
		Entrypoint: "./plugin",
	}
}

func goManifestVersioned(name, version string) *plugindomain.PluginManifest {
	return &plugindomain.PluginManifest{
		Name:       name,
		Kind:       plugindomain.KindPlugin,
		Version:    version,
		Language:   "go",
		Entrypoint: "./plugin",
	}
}

func cssManifest(name string) *plugindomain.PluginManifest {
	return &plugindomain.PluginManifest{
		Kind:    plugindomain.KindTheme,
		Name:    name,
		Version: "1.0.0",
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
	assert.Equal(t, plugindomain.Initializing, p.Status)
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
	pluginRepo.CreateErr = errors.New("db down")

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
	assert.Equal(t, plugindomain.Error, p.Status)
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
	assert.Equal(t, plugindomain.Running, p.Status)
}

func TestInstallPlugin_SetsInitializingStatus(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifest("normal-plugin")

	p, err := svc.InstallPlugin(context.Background(), tmpZip(t, "fake"))
	require.NoError(t, err)
	assert.Equal(t, plugindomain.Initializing, p.Status)
}

func TestInstallPlugin_LoadsPermissions(t *testing.T) {
	svc, pluginRepo, _, parser, _, _ := newTestInstallerService()
	pc := appplugin.NewPermissionChecker(pluginRepo)
	appplugin.InstallerWithPermissionChecker(pc)(svc)

	parser.Result = goManifest("perm-plugin")
	parser.Result.Permissions = []string{"api.library", "api.media"}

	p, err := svc.InstallPlugin(context.Background(), tmpZip(t, "fake"))
	require.NoError(t, err)

	assert.NoError(t, pc.CheckPermission(p.ID, "api.library"))
	assert.NoError(t, pc.CheckPermission(p.ID, "api.media"))
	assert.Error(t, pc.CheckPermission(p.ID, "api.admin"))
}

func TestInstall_PluginWithoutBackend_Rejected(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	// kind:plugin with an entrypoint but no runtime language - passes manifest
	// validation but must be rejected by the installer before touching the FS.
	parser.Result = &plugindomain.PluginManifest{
		Kind:       plugindomain.KindPlugin,
		Name:       "no-lang-plugin",
		Version:    "1.0.0",
		Entrypoint: "./plugin",
		// Language intentionally empty
	}

	_, err := svc.InstallPlugin(context.Background(), tmpZip(t, "fake"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kind:plugin requires a backend")
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

// An upgrade must keep the SAME plugin id and Update the row in place, never
// delete+recreate it. plugin_data (per-user/profile config) FK-cascades on
// plugins(id) ON DELETE, so a fresh id + row delete silently wipes every user's
// settings on every update. A stable id + in-place Update is what lets config
// survive an update whose settings schema is unchanged.
func TestInstallPlugin_UpgradePreservesPluginID(t *testing.T) {
	svc, pluginRepo, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifestVersioned("keep-id", "1.0.0")

	v1, err := svc.InstallPlugin(context.Background(), tmpZip(t, "v1"))
	require.NoError(t, err)

	parser.Result = goManifestVersioned("keep-id", "2.0.0")
	v2, err := svc.InstallPlugin(context.Background(), tmpZip(t, "v2"))
	require.NoError(t, err)

	assert.Equal(t, v1.ID, v2.ID, "upgrade must reuse the plugin id, not mint a new one")

	// The original row still lives under the same id, updated in place - the id
	// was never deleted (which would cascade-wipe plugin_data).
	got, err := pluginRepo.GetByID(context.Background(), v1.ID)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", got.Version)
}

// The server-level config the admin set and the enabled state must survive an
// update (the manifest resets Config to empty / Enabled to true). Together with
// the stable id preserving plugin_data, an update keeps both user and server
// config when the settings schema is unchanged.
func TestInstallPlugin_UpgradePreservesServerConfigAndDisabledState(t *testing.T) {
	svc, pluginRepo, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifestVersioned("keep-cfg", "1.0.0")

	v1, err := svc.InstallPlugin(context.Background(), tmpZip(t, "v1"))
	require.NoError(t, err)

	// Admin sets a global config value and disables the plugin.
	stored, err := pluginRepo.GetByID(context.Background(), v1.ID)
	require.NoError(t, err)
	stored.Config = map[string]any{"apiKey": "secret"}
	stored.Enabled = false
	require.NoError(t, pluginRepo.Update(context.Background(), stored))

	parser.Result = goManifestVersioned("keep-cfg", "2.0.0")
	_, err = svc.InstallPlugin(context.Background(), tmpZip(t, "v2"))
	require.NoError(t, err)

	got, err := pluginRepo.GetByID(context.Background(), v1.ID)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", got.Version)
	assert.Equal(t, map[string]any{"apiKey": "secret"}, got.Config, "server config must survive the update")
	assert.False(t, got.Enabled, "disabled state must survive the update")
	assert.Equal(t, plugindomain.Stopped, got.Status, "a disabled plugin must not be re-started on update")
}

// A FAILED upgrade must be atomic: the live version's dir, DB row and config
// all stay intact and only the staging dir is touched. Here the new version is
// extracted to staging and then rejected by the analyzer - the live plugin dir
// must not be renamed away or removed.
func TestInstallPlugin_FailedUpgradeIsAtomic(t *testing.T) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	installer := plgfakes.NewFakePluginInstaller()
	analyzer := plgfakes.NewFakePluginAnalyzer()
	pluginFS := plgfakes.NewFakePluginFileSystem()
	svc := appplugin.NewInstallerService(pluginRepo, runner, parser, bus,
		appplugin.InstallerWithInstaller(installer),
		appplugin.InstallerWithAnalyzer(analyzer),
		appplugin.InstallerWithPluginDir("/tmp/test-plugins"),
		appplugin.InstallerWithFileSystem(pluginFS),
		appplugin.InstallerWithHTTPClient(http.DefaultClient),
	)
	ctx := context.Background()

	parser.Result = goManifestVersioned("atomic", "1.0.0")
	v1, err := svc.InstallPlugin(ctx, tmpZip(t, "v1"))
	require.NoError(t, err)

	// Admin config on the live version that must survive a failed update.
	stored, err := pluginRepo.GetByID(ctx, v1.ID)
	require.NoError(t, err)
	stored.Config = map[string]any{"apiKey": "secret"}
	require.NoError(t, pluginRepo.Update(ctx, stored))

	finalDir := "/tmp/test-plugins/" + v1.ID
	staging := finalDir + ".upgrade"

	// The new version is rejected by the analyzer, after extraction to staging.
	analyzer.Report = &plugindomain.AnalysisReport{Safe: false, Issues: []string{"malware"}}
	parser.Result = goManifestVersioned("atomic", "2.0.0")
	_, err = svc.InstallPlugin(ctx, tmpZip(t, "v2"))
	require.Error(t, err)

	// Live version fully intact: old version + config, never swapped/removed.
	got, err := pluginRepo.GetByID(ctx, v1.ID)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", got.Version, "failed upgrade must not bump the version")
	assert.Equal(t, map[string]any{"apiKey": "secret"}, got.Config, "config must survive a failed upgrade")
	assert.Empty(t, pluginFS.Renamed, "no swap on a failed upgrade")
	assert.NotContains(t, pluginFS.Removed, finalDir, "the live plugin dir must not be removed")
	assert.Contains(t, pluginFS.Removed, staging, "the staging dir must be cleaned up")
}

// A SUCCESSFUL upgrade swaps the validated staging dir over the live dir.
func TestInstallPlugin_SuccessfulUpgradeSwapsStaging(t *testing.T) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	installer := plgfakes.NewFakePluginInstaller()
	analyzer := plgfakes.NewFakePluginAnalyzer()
	pluginFS := plgfakes.NewFakePluginFileSystem()
	svc := appplugin.NewInstallerService(pluginRepo, runner, parser, bus,
		appplugin.InstallerWithInstaller(installer),
		appplugin.InstallerWithAnalyzer(analyzer),
		appplugin.InstallerWithPluginDir("/tmp/test-plugins"),
		appplugin.InstallerWithFileSystem(pluginFS),
		appplugin.InstallerWithHTTPClient(http.DefaultClient),
	)
	ctx := context.Background()

	parser.Result = goManifestVersioned("swap", "1.0.0")
	v1, err := svc.InstallPlugin(ctx, tmpZip(t, "v1"))
	require.NoError(t, err)

	finalDir := "/tmp/test-plugins/" + v1.ID
	staging := finalDir + ".upgrade"

	parser.Result = goManifestVersioned("swap", "2.0.0")
	_, err = svc.InstallPlugin(ctx, tmpZip(t, "v2"))
	require.NoError(t, err)

	assert.Contains(t, pluginFS.Renamed, [2]string{staging, finalDir}, "staging must be renamed into the live dir")
	assert.Contains(t, pluginFS.Removed, finalDir, "the old dir must be removed before the swap")
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
		ID: "plg-run", Name: "running-uni", Language: "go", Status: plugindomain.Running, Enabled: true,
	}))
	_ = runner.Start(ctx, &plugindomain.Plugin{ID: "plg-run"})

	err := svc.UninstallPlugin(ctx, "plg-run")
	require.NoError(t, err)

	healthy, _ := runner.Health(ctx, "plg-run")
	assert.False(t, healthy, "runner should be stopped")
}

// A plugin auto-started on install is persisted as Initializing until its IPC
// handshake completes; one that never handshakes (broken or hostile) stays
// there with a live process. Uninstall must terminate that process too - gating
// the stop on Status==Running leaked it (row + dir removed, process ran on).
func TestUninstallPlugin_StopsInitializing(t *testing.T) {
	svc, pluginRepo, runner, _, _, _ := newTestInstallerService()
	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-init", Name: "initializing-uni", Language: "go", Status: plugindomain.Initializing, Enabled: true,
	}))
	_ = runner.Start(ctx, &plugindomain.Plugin{ID: "plg-init"})

	err := svc.UninstallPlugin(ctx, "plg-init")
	require.NoError(t, err)

	healthy, _ := runner.Health(ctx, "plg-init")
	assert.False(t, healthy, "an Initializing plugin's process must be stopped on uninstall, not leaked")
}

func TestUninstallPlugin_InvalidatesPermissions(t *testing.T) {
	svc, pluginRepo, _, _, _, _ := newTestInstallerService()
	pc := appplugin.NewPermissionChecker(pluginRepo)
	appplugin.InstallerWithPermissionChecker(pc)(svc)
	ctx := context.Background()

	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-uinv", Name: "uni-perm", Language: "go", Status: plugindomain.Stopped,
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
		Status: plugindomain.Stopped, Enabled: true,
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
		Status: plugindomain.Stopped, Enabled: true,
		InstallDir: "",
	}))

	err := svc.UninstallPlugin(ctx, "plg-nodir")
	require.NoError(t, err)

	_, err = pluginRepo.GetByID(ctx, "plg-nodir")
	assert.Error(t, err)
}

// deleteFailingRepo wraps a FakePluginRepository so only Delete fails,
// leaving GetByID (called first by UninstallPlugin) working - the shared
// fake's Err field would short-circuit that earlier lookup too.
type deleteFailingRepo struct {
	*plgfakes.FakePluginRepository
	deleteErr error
}

func (r *deleteFailingRepo) Delete(ctx context.Context, id string) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	return r.FakePluginRepository.Delete(ctx, id)
}

func TestUninstallPlugin_DeleteFails(t *testing.T) {
	pluginRepo := &deleteFailingRepo{FakePluginRepository: plgfakes.NewFakePluginRepository()}
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	pluginFS := plgfakes.NewFakePluginFileSystem()
	svc := appplugin.NewInstallerService(pluginRepo, runner, parser, bus,
		appplugin.InstallerWithPluginDir("/tmp/test-plugins"),
		appplugin.InstallerWithFileSystem(pluginFS),
	)

	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-delfail", Name: "del-fail", Language: "go",
		Status: plugindomain.Stopped, Enabled: true,
	}))
	pluginRepo.deleteErr = errors.New("db error")

	err := svc.UninstallPlugin(ctx, "plg-delfail")
	require.Error(t, err, "DB delete failure must surface, not be swallowed")

	assert.Empty(t, bus.PublishedEvents(), "PluginUninstalled must not fire when the DB row still exists")
}

// TestUninstallPlugin_DeleteSucceeds_PublishesEvent is the DeleteFails
// counterpart's happy path: Delete succeeds → nil error and the
// PluginUninstalled event fires.
func TestUninstallPlugin_DeleteSucceeds_PublishesEvent(t *testing.T) {
	pluginRepo := &deleteFailingRepo{FakePluginRepository: plgfakes.NewFakePluginRepository()}
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	pluginFS := plgfakes.NewFakePluginFileSystem()
	svc := appplugin.NewInstallerService(pluginRepo, runner, parser, bus,
		appplugin.InstallerWithPluginDir("/tmp/test-plugins"),
		appplugin.InstallerWithFileSystem(pluginFS),
	)

	ctx := context.Background()
	require.NoError(t, pluginRepo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-delok", Name: "del-ok", Language: "go",
		Status: plugindomain.Stopped, Enabled: true,
	}))

	err := svc.UninstallPlugin(ctx, "plg-delok")
	require.NoError(t, err)

	events := bus.PublishedEvents()
	require.Len(t, events, 1)
	_, ok := events[0].(plugindomain.PluginUninstalled)
	assert.True(t, ok)
}

// --- Preview ---

func TestPreviewPlugin(t *testing.T) {
	svc, _, _, parser, _, analyzer := newTestInstallerService()

	parser.Result = &plugindomain.PluginManifest{
		Name:         "preview-plugin",
		Kind:         plugindomain.KindPlugin,
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

// --- native-binary opt-in gate ---

func TestInstallPlugin_BlocksCompiledBinaryWithoutPermission(t *testing.T) {
	svc, _, _, parser, _, analyzer := newTestInstallerService()
	parser.Result = goManifest("native-without-perm")
	analyzer.Report = &plugindomain.AnalysisReport{
		Safe:       true,
		Categories: []string{"compiled_binary"},
	}

	zip := tmpZip(t, "fake-binary-zip")
	_, err := svc.InstallPlugin(context.Background(), zip)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "native-binary")
}

func TestInstallPlugin_BlocksEmbeddedArchiveWithoutPermission(t *testing.T) {
	svc, _, _, parser, _, analyzer := newTestInstallerService()
	parser.Result = goManifest("archive-without-perm")
	analyzer.Report = &plugindomain.AnalysisReport{
		Safe:       true,
		Categories: []string{"embedded_archive"},
	}

	zip := tmpZip(t, "fake-archive-zip")
	_, err := svc.InstallPlugin(context.Background(), zip)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "native-binary")
}

func TestInstallPlugin_AllowsCompiledBinaryWithPermission(t *testing.T) {
	svc, _, _, parser, _, analyzer := newTestInstallerService()
	manifest := goManifest("native-with-perm")
	manifest.Permissions = []string{"native-binary"}
	parser.Result = manifest
	analyzer.Report = &plugindomain.AnalysisReport{
		Safe:       true,
		Categories: []string{"compiled_binary"},
	}

	zip := tmpZip(t, "fake-binary-zip-allowed")
	p, err := svc.InstallPlugin(context.Background(), zip)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Contains(t, p.Permissions, "native-binary")
}

// makeMinimalPluginZip builds a valid zip archive in memory with a single
// manifest.json so it can be served over HTTP in tests.
func makeMinimalPluginZip(t *testing.T, name, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("manifest.json")
	require.NoError(t, err)
	_, err = fmt.Fprintf(f, `{"name":%q,"version":%q,"language":"go","entrypoint":"./plugin"}`, name, version)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

// --- URL install ---

func TestInstallPlugin_WithURL_Success(t *testing.T) {
	zipContent := makeMinimalPluginZip(t, "url-plugin", "1.0.0")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(zipContent)
	}))
	defer srv.Close()

	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifest("url-plugin")

	p, err := svc.InstallPlugin(context.Background(), srv.URL)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "url-plugin", p.Name)
}

func TestInstallPlugin_WithURL_HashMismatch(t *testing.T) {
	zipContent := makeMinimalPluginZip(t, "url-plugin-hash", "1.0.0")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(zipContent)
	}))
	defer srv.Close()

	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifest("url-plugin-hash")

	badHash := strings.Repeat("0", 64)
	_, err := svc.InstallPlugin(context.Background(), srv.URL+"#sha256:"+badHash)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hash mismatch")
}

func TestInstallPlugin_WithURL_HashMatch(t *testing.T) {
	zipContent := makeMinimalPluginZip(t, "url-plugin-hashok", "1.0.0")
	h := sha256.Sum256(zipContent)
	correctHash := hex.EncodeToString(h[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(zipContent)
	}))
	defer srv.Close()

	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifest("url-plugin-hashok")

	p, err := svc.InstallPlugin(context.Background(), srv.URL+"#sha256:"+correctHash)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, correctHash, p.SourceHash)
}

func TestInstallPlugin_WithURL_SizeLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		// Stream slightly more than 100 MB so the size limit is hit.
		const maxBytes = 100 << 20
		_, _ = io.Copy(w, io.LimitReader(zeroReader{}, maxBytes+1024))
	}))
	defer srv.Close()

	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifest("big-plugin")

	_, err := svc.InstallPlugin(context.Background(), srv.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds 100 MB")
}

func TestInstallPlugin_WithLocalPath_Unchanged(t *testing.T) {
	// downloadIfURL must leave local paths untouched and return empty hash.
	zip := tmpZip(t, "local-content")

	svc, _, _, parser, _, _ := newTestInstallerService()
	parser.Result = goManifest("local-plugin")

	p, err := svc.InstallPlugin(context.Background(), zip)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "local-plugin", p.Name)

	// Verify hash matches what hashFile would compute on the local file.
	data, readErr := os.ReadFile(zip)
	require.NoError(t, readErr)
	sum := sha256.Sum256(data)
	assert.Equal(t, hex.EncodeToString(sum[:]), p.SourceHash)
}

// --- Permission escalation guard ---

func TestInstallPlugin_Update_EscalatesPermissions_Blocked(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()

	// v1: api.library only
	parser.Result = goManifestVersioned("escalate-me", "1.0.0")
	parser.Result.Permissions = []string{"api.library"}
	_, err := svc.InstallPlugin(context.Background(), tmpZip(t, "v1"))
	require.NoError(t, err)

	// v2: adds network + subprocess - should be blocked without explicit allow
	parser.Result = goManifestVersioned("escalate-me", "2.0.0")
	parser.Result.Permissions = []string{"api.library", "network", "subprocess"}
	parser.Result.ExternalFetch = []string{"example.com"}
	_, err = svc.InstallPlugin(context.Background(), tmpZip(t, "v2"))
	require.Error(t, err)
	require.ErrorIs(t, err, appplugin.ErrPluginPermissionEscalation)
	assert.Contains(t, err.Error(), "permission:network")
}

func TestInstallPlugin_Update_SamePermissions_Allowed(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()

	// v1: api.library
	parser.Result = goManifestVersioned("stable-perms", "1.0.0")
	parser.Result.Permissions = []string{"api.library"}
	_, err := svc.InstallPlugin(context.Background(), tmpZip(t, "v1"))
	require.NoError(t, err)

	// v2: same permissions set - must succeed
	parser.Result = goManifestVersioned("stable-perms", "2.0.0")
	parser.Result.Permissions = []string{"api.library"}
	p, err := svc.InstallPlugin(context.Background(), tmpZip(t, "v2"))
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", p.Version)
}

func TestInstallPlugin_Update_EscalationAllowedByContext(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()

	// v1: api.library
	parser.Result = goManifestVersioned("ctx-escalate", "1.0.0")
	parser.Result.Permissions = []string{"api.library"}
	_, err := svc.InstallPlugin(context.Background(), tmpZip(t, "v1"))
	require.NoError(t, err)

	// v2: adds subprocess - allowed because context carries explicit admin ack
	parser.Result = goManifestVersioned("ctx-escalate", "2.0.0")
	parser.Result.Permissions = []string{"api.library", "subprocess"}
	ctx := appplugin.WithAllowPermissionEscalation(context.Background())
	p, err := svc.InstallPlugin(ctx, tmpZip(t, "v2"))
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", p.Version)
}

// zeroReader is an infinite stream of zero bytes, used to simulate a
// response body larger than the installer's 100 MB cap.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

// --- PreviewUpdate ---

func TestPreviewUpdate_NewPermission(t *testing.T) {
	svc, pluginRepo, _, parser, _, _ := newTestInstallerService()
	ctx := context.Background()

	// Install v1 with api.library only.
	parser.Result = goManifestVersioned("perm-update", "1.0.0")
	parser.Result.Permissions = []string{"api.library"}
	p, err := svc.InstallPlugin(ctx, tmpZip(t, "v1"))
	require.NoError(t, err)

	// Preview v2 that adds network permission.
	parser.Result = goManifestVersioned("perm-update", "2.0.0")
	parser.Result.Permissions = []string{"api.library", "network"}

	preview, err := svc.PreviewUpdate(ctx, p.ID, tmpZip(t, "v2"))
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", preview.CurrentVersion)
	assert.Equal(t, "2.0.0", preview.CandidateVersion)
	assert.Equal(t, []string{"network"}, preview.PermissionsDiff.Added)
	assert.Empty(t, preview.PermissionsDiff.Removed)
	assert.True(t, preview.RequiresAck)
	assert.NotEmpty(t, preview.SourceHash)

	// Confirm the repo still has the old plugin (preview mutates nothing).
	still, _ := pluginRepo.GetByID(ctx, p.ID)
	assert.Equal(t, "1.0.0", still.Version)
}

func TestPreviewUpdate_NoChange(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	ctx := context.Background()

	parser.Result = goManifestVersioned("stable-update", "1.0.0")
	parser.Result.Permissions = []string{"api.library"}
	p, err := svc.InstallPlugin(ctx, tmpZip(t, "v1"))
	require.NoError(t, err)

	parser.Result = goManifestVersioned("stable-update", "2.0.0")
	parser.Result.Permissions = []string{"api.library"}

	preview, err := svc.PreviewUpdate(ctx, p.ID, tmpZip(t, "v2"))
	require.NoError(t, err)
	assert.Empty(t, preview.PermissionsDiff.Added)
	assert.Empty(t, preview.PermissionsDiff.Removed)
	assert.False(t, preview.RequiresAck)
}

func TestPreviewUpdate_ExternalFetchAdded(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	ctx := context.Background()

	parser.Result = goManifestVersioned("fetch-update", "1.0.0")
	p, err := svc.InstallPlugin(ctx, tmpZip(t, "v1"))
	require.NoError(t, err)

	parser.Result = goManifestVersioned("fetch-update", "2.0.0")
	parser.Result.ExternalFetch = []string{"cdn.example.com"}

	preview, err := svc.PreviewUpdate(ctx, p.ID, tmpZip(t, "v2"))
	require.NoError(t, err)
	assert.Equal(t, []string{"cdn.example.com"}, preview.ExternalFetchDiff.Added)
	assert.True(t, preview.RequiresAck)
}

func TestPreviewUpdate_PluginNotFound(t *testing.T) {
	svc, _, _, _, _, _ := newTestInstallerService()
	_, err := svc.PreviewUpdate(context.Background(), "plg-ghost", tmpZip(t, "v1"))
	assert.ErrorIs(t, err, appplugin.ErrPluginNotFound)
}

// --- ApplyUpdate ---

func TestApplyUpdate_HashMismatch(t *testing.T) {
	svc, _, _, parser, _, _ := newTestInstallerService()
	ctx := context.Background()

	parser.Result = goManifestVersioned("hash-guard", "1.0.0")
	p, err := svc.InstallPlugin(ctx, tmpZip(t, "v1"))
	require.NoError(t, err)

	parser.Result = goManifestVersioned("hash-guard", "2.0.0")
	zip2 := tmpZip(t, "v2")

	// Pass a wrong hash.
	_, err = svc.ApplyUpdate(ctx, p.ID, zip2, strings.Repeat("0", 64))
	require.Error(t, err)
	assert.ErrorIs(t, err, appplugin.ErrCandidateChanged)
}

func TestApplyUpdate_RequiresAckWhenPermsAdded(t *testing.T) {
	svc, pluginRepo, _, parser, _, _ := newTestInstallerService()
	ctx := context.Background()

	// v1: no permissions.
	parser.Result = goManifestVersioned("ack-guard", "1.0.0")
	p, err := svc.InstallPlugin(ctx, tmpZip(t, "v1"))
	require.NoError(t, err)

	// v2: adds subprocess.
	parser.Result = goManifestVersioned("ack-guard", "2.0.0")
	parser.Result.Permissions = []string{"subprocess"}
	zip2 := tmpZip(t, "v2")

	// Preview to get the real hash.
	preview, err := svc.PreviewUpdate(ctx, p.ID, zip2)
	require.NoError(t, err)
	assert.True(t, preview.RequiresAck)

	// ApplyUpdate with the correct acknowledged_hash succeeds.
	p2, err := svc.ApplyUpdate(ctx, p.ID, zip2, preview.SourceHash)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", p2.Version)

	all, _ := pluginRepo.List(ctx)
	assert.Len(t, all, 1)
	assert.Equal(t, "2.0.0", all[0].Version)
}

func TestApplyUpdate_PluginNotFound(t *testing.T) {
	svc, _, _, _, _, _ := newTestInstallerService()
	_, err := svc.ApplyUpdate(context.Background(), "plg-ghost", tmpZip(t, "v1"), strings.Repeat("0", 64))
	assert.ErrorIs(t, err, appplugin.ErrPluginNotFound)
}

// --- RuntimeChecker integration ---

func TestInstallPlugin_RuntimeMissing_Refused(t *testing.T) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	installer := plgfakes.NewFakePluginInstaller()
	analyzer := plgfakes.NewFakePluginAnalyzer()
	pluginFS := plgfakes.NewFakePluginFileSystem()
	// Host only has python - installing a node plugin must fail with a hint.
	checker := plgfakes.NewRuntimeChecker("python", "go")

	svc := appplugin.NewInstallerService(pluginRepo, runner, parser, bus,
		appplugin.InstallerWithInstaller(installer),
		appplugin.InstallerWithAnalyzer(analyzer),
		appplugin.InstallerWithPluginDir(t.TempDir()),
		appplugin.InstallerWithFileSystem(pluginFS),
		appplugin.InstallerWithHTTPClient(http.DefaultClient),
		appplugin.InstallerWithRuntimeChecker(checker),
	)

	parser.Result = &plugindomain.PluginManifest{
		Name:       "needs-node",
		Kind:       plugindomain.KindPlugin,
		Version:    "1.0.0",
		Language:   "nodejs",
		Entrypoint: "node main.js",
	}

	_, err := svc.InstallPlugin(context.Background(), tmpZip(t, "fake"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"nodejs"`)
	assert.Contains(t, err.Error(), "evey:beta") // hint at image variants
	// repo must remain empty - install must not have any side effect.
	all, _ := pluginRepo.List(context.Background())
	assert.Empty(t, all)
}

func TestInstallPlugin_RuntimeAvailable_Allowed(t *testing.T) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	installer := plgfakes.NewFakePluginInstaller()
	analyzer := plgfakes.NewFakePluginAnalyzer()
	pluginFS := plgfakes.NewFakePluginFileSystem()
	checker := plgfakes.NewRuntimeChecker("python", "go", "nodejs")

	svc := appplugin.NewInstallerService(pluginRepo, runner, parser, bus,
		appplugin.InstallerWithInstaller(installer),
		appplugin.InstallerWithAnalyzer(analyzer),
		appplugin.InstallerWithPluginDir(t.TempDir()),
		appplugin.InstallerWithFileSystem(pluginFS),
		appplugin.InstallerWithHTTPClient(http.DefaultClient),
		appplugin.InstallerWithRuntimeChecker(checker),
	)

	parser.Result = &plugindomain.PluginManifest{
		Name:       "node-ok",
		Kind:       plugindomain.KindPlugin,
		Version:    "1.0.0",
		Language:   "nodejs",
		Entrypoint: "node main.js",
	}

	p, err := svc.InstallPlugin(context.Background(), tmpZip(t, "fake"))
	require.NoError(t, err)
	assert.Equal(t, "node-ok", p.Name)
}

func TestInstallPlugin_ThemeSkipsRuntimeCheck(t *testing.T) {
	// Themes have no backend runtime; checker must not gate them even when
	// the host is bare.
	pluginRepo := plgfakes.NewFakePluginRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	installer := plgfakes.NewFakePluginInstaller()
	analyzer := plgfakes.NewFakePluginAnalyzer()
	pluginFS := plgfakes.NewFakePluginFileSystem()
	checker := plgfakes.NewRuntimeChecker() // empty: nothing available

	svc := appplugin.NewInstallerService(pluginRepo, runner, parser, bus,
		appplugin.InstallerWithInstaller(installer),
		appplugin.InstallerWithAnalyzer(analyzer),
		appplugin.InstallerWithPluginDir(t.TempDir()),
		appplugin.InstallerWithFileSystem(pluginFS),
		appplugin.InstallerWithHTTPClient(http.DefaultClient),
		appplugin.InstallerWithRuntimeChecker(checker),
	)

	parser.Result = &plugindomain.PluginManifest{
		Kind:    plugindomain.KindTheme,
		Name:    "bare-theme",
		Version: "1.0.0",
	}

	p, err := svc.InstallPlugin(context.Background(), tmpZip(t, "fake"))
	require.NoError(t, err)
	assert.Equal(t, "bare-theme", p.Name)
}

// --- Orphaned install dir cleanup ---

// newTestInstallerServiceWithFS is like newTestInstallerService but also
// returns the FakePluginFileSystem so callers can inspect RemoveAll calls.
func newTestInstallerServiceWithFS() (
	*appplugin.InstallerService,
	*plgfakes.FakePluginRepository,
	*plgfakes.FakePluginRunner,
	*plgfakes.FakeManifestParser,
	*plgfakes.FakePluginInstaller,
	*plgfakes.FakePluginAnalyzer,
	*plgfakes.FakePluginFileSystem,
) {
	pluginRepo := plgfakes.NewFakePluginRepository()
	runner := plgfakes.NewFakePluginRunner()
	parser := plgfakes.NewFakeManifestParser()
	bus := sharedfakes.NewFakeEventBus()
	installer := plgfakes.NewFakePluginInstaller()
	analyzer := plgfakes.NewFakePluginAnalyzer()
	pluginFS := plgfakes.NewFakePluginFileSystem()

	svc := appplugin.NewInstallerService(pluginRepo, runner, parser, bus,
		appplugin.InstallerWithInstaller(installer),
		appplugin.InstallerWithAnalyzer(analyzer),
		appplugin.InstallerWithPluginDir("/tmp/test-plugins"),
		appplugin.InstallerWithFileSystem(pluginFS),
		appplugin.InstallerWithHTTPClient(http.DefaultClient),
	)

	return svc, pluginRepo, runner, parser, installer, analyzer, pluginFS
}

// TestInstallPlugin_FailAfterExtract_CleansUpDir verifies that when the
// analyzer fails (after extraction), the deferred cleanup removes the install
// directory exactly once.
func TestInstallPlugin_FailAfterExtract_CleansUpDir(t *testing.T) {
	svc, _, _, parser, _, analyzer, pluginFS := newTestInstallerServiceWithFS()
	parser.Result = goManifest("orphan-test")
	analyzer.Err = errors.New("analyzer exploded")

	_, err := svc.InstallPlugin(context.Background(), tmpZip(t, "fake"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "static analysis failed")

	// The deferred cleanup must have removed the install directory.
	require.Len(t, pluginFS.Removed, 1, "exactly one dir must be removed on failure")
	assert.Contains(t, pluginFS.Removed[0], "/tmp/test-plugins/", "removed path must be under pluginDir")
}

// TestInstallPlugin_FailUnsafe_CleansUpDir verifies that an unsafe analysis
// report (report.Safe == false) also triggers the deferred cleanup.
func TestInstallPlugin_FailUnsafe_CleansUpDir(t *testing.T) {
	svc, _, _, parser, _, analyzer, pluginFS := newTestInstallerServiceWithFS()
	parser.Result = goManifest("unsafe-orphan")
	analyzer.Report = &plugindomain.AnalysisReport{
		Safe:   false,
		Issues: []string{"dangerous pattern"},
	}

	_, err := svc.InstallPlugin(context.Background(), tmpZip(t, "fake"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin blocked")

	require.Len(t, pluginFS.Removed, 1, "exactly one dir must be removed on failure")
	assert.Contains(t, pluginFS.Removed[0], "/tmp/test-plugins/")
}

// TestInstallPlugin_DBFail_CleansUpDir verifies that a DB create failure
// (after extraction and analysis) triggers the deferred cleanup.
func TestInstallPlugin_DBFail_CleansUpDir(t *testing.T) {
	svc, pluginRepo, _, parser, _, _, pluginFS := newTestInstallerServiceWithFS()
	parser.Result = goManifest("db-fail-orphan")
	pluginRepo.CreateErr = errors.New("db unavailable")

	_, err := svc.InstallPlugin(context.Background(), tmpZip(t, "fake"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db unavailable")

	require.Len(t, pluginFS.Removed, 1, "exactly one dir must be removed on failure")
	assert.Contains(t, pluginFS.Removed[0], "/tmp/test-plugins/")
}

// TestInstallPlugin_Success_KeepsDir verifies that a successful install does
// NOT remove the install directory.
func TestInstallPlugin_Success_KeepsDir(t *testing.T) {
	svc, _, _, parser, _, _, pluginFS := newTestInstallerServiceWithFS()
	parser.Result = goManifest("keep-dir-plugin")

	p, err := svc.InstallPlugin(context.Background(), tmpZip(t, "fake"))
	require.NoError(t, err)
	assert.NotEmpty(t, p.InstallDir)

	// RemoveAll must NOT have been called on the install dir after a success.
	for _, removed := range pluginFS.Removed {
		assert.NotEqual(t, p.InstallDir, removed, "successful install must not remove its own dir")
	}
}

// TestInstallPlugin_NoBinaryPerm_CleansUpDir verifies that the
// compiled_binary gate (after analysis) triggers the deferred cleanup.
func TestInstallPlugin_NoBinaryPerm_CleansUpDir(t *testing.T) {
	svc, _, _, parser, _, analyzer, pluginFS := newTestInstallerServiceWithFS()
	parser.Result = goManifest("binary-orphan")
	analyzer.Report = &plugindomain.AnalysisReport{
		Safe:       true,
		Categories: []string{"compiled_binary"},
	}

	_, err := svc.InstallPlugin(context.Background(), tmpZip(t, "fake"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "native-binary")

	require.Len(t, pluginFS.Removed, 1, "exactly one dir must be removed on failure")
	assert.Contains(t, pluginFS.Removed[0], "/tmp/test-plugins/")
}
