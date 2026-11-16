package plugin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appplugin "github.com/eveyproject/evey/internal/app/plugin"
)

// A repository link needs something that reads releases: a server without it
// says so instead of trying to download the repository's page as a zip.
func TestInstallFromRepositoryLink_WithoutAReleaseSource(t *testing.T) {
	svc, repo, _, _, _, _ := newTestInstallerService()
	_, err := svc.InstallPlugin(context.Background(), testRepo)
	require.ErrorIs(t, err, appplugin.ErrNoReleaseSource)
	all, listErr := repo.List(context.Background())
	require.NoError(t, listErr)
	assert.Empty(t, all)
}

// A repository that cannot be identified, or whose latest release cannot be
// read, is an unavailable source, and nothing is installed.
func TestInstallFromRepositoryLink_AnUnreadableRepositoryIsUnavailable(t *testing.T) {
	down := errors.New("github is down")
	for name, breakIt := range map[string]func(*fakeReleases){
		"identify": func(f *fakeReleases) { f.err = down },
		"latest":   func(f *fakeReleases) { f.latestErr = down },
	} {
		t.Run(name, func(t *testing.T) {
			rig := newAutoRig(t, "warn")
			rig.publish(t, goManifestVersioned("thing", "1.0.0"), rig.signed(t, "v1"))
			rig.releases.mu.Lock()
			breakIt(rig.releases)
			rig.releases.mu.Unlock()

			_, err := rig.svc.InstallPlugin(context.Background(), testRepo)
			require.ErrorIs(t, err, appplugin.ErrSourceUnavailable)
			require.ErrorIs(t, err, down)
			all, listErr := rig.plugins.List(context.Background())
			require.NoError(t, listErr)
			assert.Empty(t, all)
		})
	}
}

// A release that publishes no digest is installed without a pin, and the
// plugin still follows its repository by its permanent id.
func TestInstallFromRepositoryLink_AReleaseWithoutADigestInstallsUnpinned(t *testing.T) {
	rig := newAutoRig(t, "warn")
	rig.publish(t, goManifestVersioned("thing", "1.0.0"), rig.signed(t, "v1"))
	rig.releases.mu.Lock()
	rig.releases.rel.SHA256 = ""
	rig.releases.mu.Unlock()

	p, err := rig.svc.InstallPlugin(context.Background(), testRepo)
	require.NoError(t, err)
	got := rig.stored(t, p.ID)
	assert.Equal(t, "1.0.0", got.Version)
	assert.Equal(t, testRepo, got.UpdateSource)
	assert.Equal(t, testRepoID, got.UpdateSourceID)
}

// A release's download link of a repository follows that repository, by the
// id it is identified under at that moment.
func TestInstallFromAReleaseLink_FollowsItsRepository(t *testing.T) {
	rig := newAutoRig(t, "warn")
	rig.publish(t, goManifestVersioned("thing", "1.0.0"), rig.signed(t, "v1"))
	rig.releases.mu.Lock()
	zipURL := rig.releases.rel.ZipURL
	rig.releases.mu.Unlock()

	p, err := rig.svc.InstallPlugin(context.Background(), zipURL)
	require.NoError(t, err)
	got := rig.stored(t, p.ID)
	assert.Equal(t, testRepo, got.UpdateSource)
	assert.Equal(t, testRepoID, got.UpdateSourceID)
}

// A release link whose repository cannot be identified installs all the
// same and follows nothing: no repository is followed by its name alone.
func TestInstallFromAReleaseLink_AnUnidentifiedRepositoryIsNotFollowed(t *testing.T) {
	rig := newAutoRig(t, "warn")
	rig.publish(t, goManifestVersioned("thing", "1.0.0"), rig.signed(t, "v1"))
	rig.releases.mu.Lock()
	zipURL := rig.releases.rel.ZipURL
	rig.releases.err = errors.New("github is down")
	rig.releases.mu.Unlock()

	p, err := rig.svc.InstallPlugin(context.Background(), zipURL)
	require.NoError(t, err)
	got := rig.stored(t, p.ID)
	assert.Empty(t, got.UpdateSource)
	assert.Zero(t, got.UpdateSourceID)
}

// A local package belongs to no repository and follows none.
func TestInstallFromALocalPackage_FollowsNothing(t *testing.T) {
	rig := newAutoRig(t, "warn")
	rig.parser.Result = goManifestVersioned("thing", "1.0.0")
	p, err := rig.svc.InstallPlugin(context.Background(), rig.signed(t, "v1"))
	require.NoError(t, err)
	got := rig.stored(t, p.ID)
	assert.Empty(t, got.UpdateSource)
	assert.Zero(t, got.UpdateSourceID)
}
