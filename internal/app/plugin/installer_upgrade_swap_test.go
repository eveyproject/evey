package plugin_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/filesystem"
	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	plugindomain "github.com/eveyproject/evey/internal/domain/plugin"
	plgfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

type swapFixture struct {
	svc      *appplugin.InstallerService
	repo     *plgfakes.FakePluginRepository
	parser   *plgfakes.FakeManifestParser
	fs       *plgfakes.FakePluginFileSystem
	id       string
	finalDir string
	staging  string
	backup   string
}

// newSwapFixture installs v1 of a plugin so the next install is an upgrade.
func newSwapFixture(t *testing.T) *swapFixture {
	t.Helper()
	f := &swapFixture{
		repo:   plgfakes.NewFakePluginRepository(),
		parser: plgfakes.NewFakeManifestParser(),
		fs:     plgfakes.NewFakePluginFileSystem(),
	}
	f.svc = appplugin.NewInstallerService(f.repo, plgfakes.NewFakePluginRunner(), f.parser, sharedfakes.NewFakeEventBus(),
		appplugin.InstallerWithInstaller(plgfakes.NewFakePluginInstaller()),
		appplugin.InstallerWithAnalyzer(plgfakes.NewFakePluginAnalyzer()),
		appplugin.InstallerWithPluginDir("/tmp/test-plugins"),
		appplugin.InstallerWithFileSystem(f.fs),
		appplugin.InstallerWithHTTPClient(http.DefaultClient),
	)
	f.parser.Result = goManifestVersioned("swap", "1.0.0")
	v1, err := f.svc.InstallPlugin(context.Background(), tmpZip(t, "v1"))
	require.NoError(t, err)
	f.id = v1.ID
	f.finalDir = "/tmp/test-plugins/" + v1.ID
	f.staging = f.finalDir + ".upgrade"
	f.backup = f.finalDir + ".old"
	f.parser.Result = goManifestVersioned("swap", "2.0.0")
	return f
}

// backupRemovals counts deletions of <id>.old. The first is the pre-swap
// cleanup of a leftover; any later one would destroy the live version.
func (f *swapFixture) backupRemovals() int {
	n := 0
	for _, d := range f.fs.Removed {
		if d == f.backup {
			n++
		}
	}
	return n
}

func (f *swapFixture) storedVersion(t *testing.T) string {
	t.Helper()
	got, err := f.repo.GetByID(context.Background(), f.id)
	require.NoError(t, err)
	return got.Version
}

// The live dir is moved aside, never deleted, until the new version is in
// place and its row written: the old version survives every failed step.
func TestUpgradeSwap_Success_MovesLiveAsideThenDropsIt(t *testing.T) {
	f := newSwapFixture(t)

	_, err := f.svc.InstallPlugin(context.Background(), tmpZip(t, "v2"))
	require.NoError(t, err)

	assert.Equal(t, [][2]string{{f.finalDir, f.backup}, {f.staging, f.finalDir}}, f.fs.Renamed)
	assert.Equal(t, 2, f.backupRemovals(), "the previous version is dropped once the upgrade is persisted")
	assert.NotContains(t, f.fs.Removed, f.finalDir, "the live dir is never deleted in place")
	assert.Equal(t, "2.0.0", f.storedVersion(t))
}

// Staging cannot be renamed into place: the old version is renamed back, so
// the plugin keeps its files instead of losing both dirs.
func TestUpgradeSwap_StagingRenameFails_RestoresLiveDir(t *testing.T) {
	f := newSwapFixture(t)
	f.fs.RenameErrFor = map[string]error{f.staging: errors.New("rename failed")}

	_, err := f.svc.InstallPlugin(context.Background(), tmpZip(t, "v2"))
	require.Error(t, err)

	assert.Equal(t, [][2]string{{f.finalDir, f.backup}, {f.backup, f.finalDir}}, f.fs.Renamed)
	assert.NotContains(t, f.fs.Removed, f.finalDir)
	assert.Equal(t, 1, f.backupRemovals(), "the moved-aside live version is never deleted")
	assert.Contains(t, f.fs.Removed, f.staging, "the rejected staging dir is cleaned up")
	assert.Equal(t, "1.0.0", f.storedVersion(t))
}

// The live dir cannot be moved aside: nothing was touched, the upgrade stops.
func TestUpgradeSwap_MoveAsideFails_LeavesLiveDirUntouched(t *testing.T) {
	f := newSwapFixture(t)
	f.fs.RenameErrFor = map[string]error{f.finalDir: errors.New("busy")}

	_, err := f.svc.InstallPlugin(context.Background(), tmpZip(t, "v2"))
	require.Error(t, err)

	assert.Empty(t, f.fs.Renamed)
	assert.NotContains(t, f.fs.Removed, f.finalDir)
	assert.Contains(t, f.fs.Removed, f.staging)
	assert.Equal(t, "1.0.0", f.storedVersion(t))
}

// The row cannot be written after the swap: the files go back to the old
// version so disk and database keep describing the same plugin.
func TestUpgradeSwap_PersistFails_RollsFilesBack(t *testing.T) {
	f := newSwapFixture(t)
	f.repo.UpdateErr = errors.New("db down")

	_, err := f.svc.InstallPlugin(context.Background(), tmpZip(t, "v2"))
	require.Error(t, err)

	assert.Equal(t, [][2]string{{f.finalDir, f.backup}, {f.staging, f.finalDir}, {f.backup, f.finalDir}}, f.fs.Renamed)
	assert.Contains(t, f.fs.Removed, f.finalDir, "the new version is removed before the old one comes back")
	assert.Equal(t, 1, f.backupRemovals(), "the moved-aside live version is never deleted")
	f.repo.UpdateErr = nil
	assert.Equal(t, "1.0.0", f.storedVersion(t))
}

// A crash between moving the live dir aside and renaming staging into place
// leaves only <id>.old on disk. The boot sweep must put it back, not delete
// it as an orphan along with the unfinished staging dir.
func TestCleanupOrphans_RestoresInterruptedUpgrade(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	repo := plgfakes.NewFakePluginRepository()
	for _, id := range []string{"plg-crashed", "plg-done"} {
		require.NoError(t, repo.Create(ctx, &plugindomain.Plugin{ID: id, Name: id}))
	}
	for _, d := range []string{"plg-crashed.old", "plg-crashed.upgrade", "plg-done", "plg-done.old", "plg-gone"} {
		require.NoError(t, os.Mkdir(filepath.Join(base, d), 0o755))
	}
	svc := appplugin.NewInstallerService(repo, plgfakes.NewFakePluginRunner(), plgfakes.NewFakeManifestParser(), sharedfakes.NewFakeEventBus(),
		appplugin.InstallerWithPluginDir(base),
		appplugin.InstallerWithFileSystem(filesystem.NewPluginFileSystem()),
	)

	svc.CleanupOrphans(ctx)

	entries, err := os.ReadDir(base)
	require.NoError(t, err)
	var left []string
	for _, e := range entries {
		left = append(left, e.Name())
	}
	assert.ElementsMatch(t, []string{"plg-crashed", "plg-done"}, left)
}
