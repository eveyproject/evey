package plugin_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/pluginrunner"
	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	plugindomain "github.com/eveyproject/evey/internal/domain/plugin"
	plgfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

const testRepo = "https://github.com/acme/thing"

// fakeReleases is the repository: whatever was published last.
type fakeReleases struct {
	mu  sync.Mutex
	rel *plugindomain.Release
	err error
}

func (f *fakeReleases) Latest(_ context.Context, repo string) (*plugindomain.Release, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if repo != testRepo {
		return nil, plugindomain.ErrUnsupportedRepository
	}
	if f.err != nil {
		return nil, f.err
	}
	r := *f.rel
	return &r, nil
}

type autoRig struct {
	svc      *appplugin.InstallerService
	upd      *appplugin.AutoUpdater
	plugins  *plgfakes.FakePluginRepository
	parser   *plgfakes.FakeManifestParser
	analyzer *plgfakes.FakePluginAnalyzer
	bus      *sharedfakes.FakeEventBus
	releases *fakeReleases
	srv      *httptest.Server
	dir      string
	key      signer
	keyring  *appplugin.TrustedPublisherService
}

func newAutoRig(t *testing.T, policy string) *autoRig {
	t.Helper()
	dir := t.TempDir()
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	t.Cleanup(srv.Close)
	plugins := plgfakes.NewFakePluginRepository()
	parser := plgfakes.NewFakeManifestParser()
	analyzer := plgfakes.NewFakePluginAnalyzer()
	bus := sharedfakes.NewFakeEventBus()
	releases := &fakeReleases{}
	keyring := appplugin.NewTrustedPublisherService(plgfakes.NewFakeTrustedPublisherRepository(), plugins)
	svc := appplugin.NewInstallerService(plugins, plgfakes.NewFakePluginRunner(), parser, bus,
		appplugin.InstallerWithInstaller(plgfakes.NewFakePluginInstaller()),
		appplugin.InstallerWithAnalyzer(analyzer),
		appplugin.InstallerWithPluginDir("/tmp/test-plugins"),
		appplugin.InstallerWithFileSystem(plgfakes.NewFakePluginFileSystem()),
		appplugin.InstallerWithHTTPClient(http.DefaultClient),
		appplugin.InstallerWithSignatureVerifier(pluginrunner.NewCosignVerifier(policy, keyring.Lookup)),
		appplugin.InstallerWithTrustedPublishers(keyring),
		appplugin.InstallerWithReleaseSource(releases),
	)
	return &autoRig{svc: svc, upd: appplugin.NewAutoUpdater(svc), plugins: plugins, parser: parser,
		analyzer: analyzer, bus: bus, releases: releases, srv: srv, dir: dir, key: newSigner(t), keyring: keyring}
}

// signed builds a package signed with the rig's key, the publisher's.
func (r *autoRig) signed(t *testing.T, body string) string {
	t.Helper()
	return pkg(t, &r.key, body)
}

// publish makes zipPath (and its .sig when there is one) the repository's
// latest release, with m as the manifest the server will read from it.
func (r *autoRig) publish(t *testing.T, m *plugindomain.PluginManifest, zipPath string) {
	t.Helper()
	name := m.Name + "-" + m.Version + ".zip"
	data, err := os.ReadFile(zipPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(r.dir, name), data, 0o600))
	if sig, err := os.ReadFile(zipPath + ".sig"); err == nil {
		require.NoError(t, os.WriteFile(filepath.Join(r.dir, name+".sig"), sig, 0o600))
	}
	sum := sha256.Sum256(data)
	r.releases.mu.Lock()
	r.releases.rel = &plugindomain.Release{Version: m.Version, Tag: "v" + m.Version,
		ZipURL: r.srv.URL + "/" + name, SHA256: hex.EncodeToString(sum[:])}
	r.releases.mu.Unlock()
	r.parser.Result = m
}

// installFromRepo installs v1.0.0 the way an admin does: the repository link.
func (r *autoRig) installFromRepo(t *testing.T, k *signer) *plugindomain.Plugin {
	t.Helper()
	r.publish(t, goManifestVersioned("thing", "1.0.0"), pkg(t, k, "v1"))
	p, err := r.svc.InstallPlugin(context.Background(), testRepo)
	require.NoError(t, err)
	return p
}

func (r *autoRig) check(t *testing.T, id string) plugindomain.UpdateState {
	t.Helper()
	st, err := r.upd.Check(context.Background(), id)
	require.NoError(t, err)
	return st
}

func (r *autoRig) stored(t *testing.T, id string) *plugindomain.Plugin {
	t.Helper()
	p, err := r.plugins.GetByID(context.Background(), id)
	require.NoError(t, err)
	return p
}

func (r *autoRig) attention() []plugindomain.PluginUpdateAttention {
	var out []plugindomain.PluginUpdateAttention
	for _, ev := range r.bus.PublishedEvents() {
		if e, ok := ev.(plugindomain.PluginUpdateAttention); ok {
			out = append(out, e)
		}
	}
	return out
}

func TestAutoUpdate_InstallFromTheRepositoryLinkFollowsIt(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)
	stored := rig.stored(t, p.ID)
	assert.Equal(t, "1.0.0", stored.Version)
	assert.Equal(t, testRepo, stored.UpdateSource)
	assert.True(t, stored.AutoUpdate, "automatic updates are on by default")
}

func TestAutoUpdate_AppliesASafeRelease(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)

	rig.publish(t, goManifestVersioned("thing", "1.0.1"), rig.signed(t, "v101"))
	st := rig.check(t, p.ID)

	assert.Equal(t, plugindomain.UpdateApplied, st.Outcome, st.Reason)
	assert.Equal(t, "1.0.1", rig.stored(t, p.ID).Version)
	assert.Empty(t, rig.attention(), "an applied release pages nobody")
	assert.Equal(t, testRepo, rig.stored(t, p.ID).UpdateSource, "the upgrade keeps the repository it follows")
}

func TestAutoUpdate_UpToDateInstallsNothing(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)
	st := rig.check(t, p.ID)
	assert.Equal(t, plugindomain.UpdateUpToDate, st.Outcome)
	assert.Equal(t, "1.0.0", st.LatestVersion)
	assert.Equal(t, st, rig.stored(t, p.ID).UpdateState, "the state is recorded on the plugin")
}

func TestAutoUpdate_NewPermissionWaitsForAnAdminAndPagesOnce(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)

	m := goManifestVersioned("thing", "1.1.0")
	m.Permissions = []string{"network"}
	m.ExternalFetch = []string{"api.example.com"}
	rig.publish(t, m, rig.signed(t, "v110"))
	st := rig.check(t, p.ID)

	assert.Equal(t, plugindomain.UpdateHeld, st.Outcome)
	assert.Contains(t, st.Reason, "permission:network")
	assert.Contains(t, st.Reason, "external_fetch:api.example.com")
	assert.Equal(t, "1.0.0", rig.stored(t, p.ID).Version, "a held release is not installed")
	require.Len(t, rig.attention(), 1)
	assert.Equal(t, plugindomain.UpdateHeld, rig.attention()[0].Outcome)

	rig.check(t, p.ID)
	assert.Len(t, rig.attention(), 1, "the same held release pages once, not every hour")
}

func TestAutoUpdate_OffWaitsForAnAdmin(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)
	_, err := rig.svc.SetUpdateFollow(context.Background(), p.ID, testRepo, false)
	require.NoError(t, err)

	rig.publish(t, goManifestVersioned("thing", "1.0.1"), rig.signed(t, "v101"))
	st := rig.check(t, p.ID)

	assert.Equal(t, plugindomain.UpdateHeld, st.Outcome)
	assert.Contains(t, st.Reason, "automatic updates are off")
	assert.Equal(t, "1.0.0", rig.stored(t, p.ID).Version)
	assert.Len(t, rig.attention(), 1)
}

func TestAutoUpdate_MaliciousCodeIsRefusedAndPaged(t *testing.T) {
	cases := map[string]*plugindomain.AnalysisReport{
		"critical finding": {Safe: false, RawFindings: []plugindomain.PluginFinding{
			{Category: "shell_exec", Severity: "CRITICAL", File: "main.py", Line: 3}}},
		"blocking warning": {Safe: true, RawFindings: []plugindomain.PluginFinding{
			{Category: "obfuscation", Severity: "WARNING", File: "main.py", Line: 9}}},
	}
	for name, report := range cases {
		t.Run(name, func(t *testing.T) {
			rig := newAutoRig(t, "warn")
			p := rig.installFromRepo(t, &rig.key)

			rig.analyzer.Report = report
			rig.publish(t, goManifestVersioned("thing", "1.0.1"), rig.signed(t, "evil"))
			st := rig.check(t, p.ID)

			assert.Equal(t, plugindomain.UpdateRefused, st.Outcome)
			assert.Contains(t, st.Reason, "the analysis refuses this code")
			assert.Equal(t, "1.0.0", rig.stored(t, p.ID).Version, "refused code is never installed")
			require.Len(t, rig.attention(), 1)
			assert.Equal(t, plugindomain.UpdateRefused, rig.attention()[0].Outcome)
		})
	}
}

func TestAutoUpdate_RefusedEvenWithAutomaticUpdatesOff(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)
	_, err := rig.svc.SetUpdateFollow(context.Background(), p.ID, testRepo, false)
	require.NoError(t, err)

	rig.analyzer.Report = &plugindomain.AnalysisReport{Safe: false, RawFindings: []plugindomain.PluginFinding{
		{Category: "shell_exec", Severity: "CRITICAL"}}}
	rig.publish(t, goManifestVersioned("thing", "1.0.1"), rig.signed(t, "evil"))

	assert.Equal(t, plugindomain.UpdateRefused, rig.check(t, p.ID).Outcome,
		"the admin is told the release is malicious, not merely that it waits")
}

func TestAutoUpdate_NewKindOfWarningWaitsForAnAdmin(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)

	rig.analyzer.Report = &plugindomain.AnalysisReport{Safe: true, RawFindings: []plugindomain.PluginFinding{
		{Category: "outbound_fetch", Severity: "WARNING", File: "main.py"}}}
	rig.publish(t, goManifestVersioned("thing", "1.0.1"), rig.signed(t, "v101"))
	st := rig.check(t, p.ID)

	assert.Equal(t, plugindomain.UpdateHeld, st.Outcome)
	assert.Contains(t, st.Reason, "outbound_fetch")
}

func TestAutoUpdate_AWarningTheInstalledVersionHadIsNotNew(t *testing.T) {
	rig := newAutoRig(t, "warn")
	rig.analyzer.Report = &plugindomain.AnalysisReport{Safe: true, RawFindings: []plugindomain.PluginFinding{
		{Category: "outbound_fetch", Severity: "WARNING", File: "main.py"}}}
	p := rig.installFromRepo(t, &rig.key)

	rig.publish(t, goManifestVersioned("thing", "1.0.1"), rig.signed(t, "v101"))
	assert.Equal(t, plugindomain.UpdateApplied, rig.check(t, p.ID).Outcome)
}

func TestAutoUpdate_SigningKey(t *testing.T) {
	a, b := newSigner(t), newSigner(t)

	t.Run("same key applies", func(t *testing.T) {
		rig := newAutoRig(t, "warn")
		p := rig.installFromRepo(t, &a)
		rig.publish(t, goManifestVersioned("thing", "1.0.1"), pkg(t, &a, "v101"))
		st := rig.check(t, p.ID)
		assert.Equal(t, plugindomain.UpdateApplied, st.Outcome, st.Reason)
	})
	t.Run("another key waits", func(t *testing.T) {
		rig := newAutoRig(t, "warn")
		p := rig.installFromRepo(t, &a)
		rig.publish(t, goManifestVersioned("thing", "1.0.1"), pkg(t, &b, "v101"))
		st := rig.check(t, p.ID)
		assert.Equal(t, plugindomain.UpdateHeld, st.Outcome)
		assert.Contains(t, st.Reason, "another key")
		assert.Equal(t, "1.0.0", rig.stored(t, p.ID).Version)
	})
	t.Run("an unsigned release waits", func(t *testing.T) {
		rig := newAutoRig(t, "warn")
		p := rig.installFromRepo(t, &a)
		rig.publish(t, goManifestVersioned("thing", "1.0.1"), pkg(t, nil, "v101"))
		st := rig.check(t, p.ID)
		assert.Equal(t, plugindomain.UpdateHeld, st.Outcome)
		assert.Contains(t, st.Reason, "no longer validly signed")
	})
}

func TestAutoUpdate_ADownloadThatDoesNotMatchItsDigestIsNotInstalled(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)
	rig.publish(t, goManifestVersioned("thing", "1.0.1"), rig.signed(t, "v101"))
	rig.releases.rel.SHA256 = hex.EncodeToString(make([]byte, 32))

	st := rig.check(t, p.ID)
	assert.Equal(t, plugindomain.UpdateFailed, st.Outcome)
	assert.Contains(t, st.Reason, "does not match")
	assert.Equal(t, "1.0.0", rig.stored(t, p.ID).Version)
}

func TestAutoUpdate_AReleaseOfAnotherPluginIsRefused(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)
	rig.publish(t, goManifestVersioned("other", "2.0.0"), rig.signed(t, "other"))

	st := rig.check(t, p.ID)
	assert.Equal(t, plugindomain.UpdateRefused, st.Outcome)
	assert.Equal(t, "1.0.0", rig.stored(t, p.ID).Version)
}

func TestAutoUpdate_UnreachableRepositoryIsRecordedNotPaged(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)
	rig.releases.err = assert.AnError

	st := rig.check(t, p.ID)
	assert.Equal(t, plugindomain.UpdateFailed, st.Outcome)
	assert.Empty(t, rig.attention())
}

func TestAutoUpdate_CheckAllSkipsPluginsThatFollowNothing(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)
	_, err := rig.svc.SetUpdateFollow(context.Background(), p.ID, "", true)
	require.NoError(t, err)

	rig.publish(t, goManifestVersioned("thing", "1.0.1"), rig.signed(t, "v101"))
	rig.upd.CheckAll(context.Background())
	assert.Equal(t, "1.0.0", rig.stored(t, p.ID).Version)

	_, err = rig.upd.Check(context.Background(), p.ID)
	require.ErrorIs(t, err, appplugin.ErrNoUpdateSource)
}

func TestSetUpdateFollow_RefusesAnUnsupportedLink(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)
	_, err := rig.svc.SetUpdateFollow(context.Background(), p.ID, "https://example.com/acme/thing", true)
	require.ErrorIs(t, err, plugindomain.ErrUnsupportedRepository)
	assert.Equal(t, testRepo, rig.stored(t, p.ID).UpdateSource)
}

// The scheduled check is what makes the update automatic: nobody presses
// anything, the release is there on the next tick.
func TestAutoUpdate_TheScheduledCheckAppliesTheReleaseOnItsOwn(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)
	rig.publish(t, goManifestVersioned("thing", "1.0.1"), rig.signed(t, "v101"))

	rig.upd.WithSchedule(10*time.Millisecond, 10*time.Millisecond).Start(t.Context())

	require.Eventually(t, func() bool {
		got, err := rig.plugins.GetByID(context.Background(), p.ID)
		return err == nil && got.Version == "1.0.1"
	}, 5*time.Second, 10*time.Millisecond, "the scheduled check never applied the release")
}

// An admin who approves a held release by hand ends the wait: the plugin keeps
// following its repository and the state stops saying a release waits.
func TestAutoUpdate_ApprovingAHeldReleaseKeepsTheFollowAndSettlesTheState(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)
	_, err := rig.svc.SetUpdateFollow(context.Background(), p.ID, testRepo, false)
	require.NoError(t, err)

	m := goManifestVersioned("thing", "1.1.0")
	m.Permissions = []string{"network"}
	m.ExternalFetch = []string{"api.example.com"}
	rig.publish(t, m, rig.signed(t, "v110"))
	require.Equal(t, plugindomain.UpdateHeld, rig.check(t, p.ID).Outcome)

	preview, err := rig.svc.PreviewUpdate(context.Background(), p.ID, testRepo)
	require.NoError(t, err)
	assert.True(t, preview.RequiresAck)
	got, err := rig.svc.ApplyUpdate(context.Background(), p.ID, testRepo, preview.SourceHash, false)
	require.NoError(t, err)

	assert.Equal(t, "1.1.0", got.Version)
	assert.Equal(t, testRepo, got.UpdateSource, "the returned record still follows the repository")
	assert.False(t, got.AutoUpdate, "and keeps the admin's switch")
	stored := rig.stored(t, p.ID)
	assert.Equal(t, plugindomain.UpdateUpToDate, stored.UpdateState.Outcome)
	assert.Equal(t, "1.1.0", stored.UpdateState.LatestVersion)
}

// Whoever takes over an unsigned plugin's repository could ship code the
// analysis does not catch; without a key to check, nothing installs alone.
func TestAutoUpdate_AnUnsignedPluginNeverUpdatesOnItsOwn(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, nil)

	rig.publish(t, goManifestVersioned("thing", "1.0.1"), pkg(t, nil, "v101"))
	st := rig.check(t, p.ID)

	assert.Equal(t, plugindomain.UpdateHeld, st.Outcome)
	assert.Contains(t, st.Reason, "not signed")
	assert.Equal(t, "1.0.0", rig.stored(t, p.ID).Version)
	assert.Len(t, rig.attention(), 1, "the admin is told a release waits")
}

// With verification off, no key is pinned at install: the same rule holds.
func TestAutoUpdate_WithVerificationOffNothingUpdatesOnItsOwn(t *testing.T) {
	rig := newAutoRig(t, "off")
	p := rig.installFromRepo(t, &rig.key)
	rig.publish(t, goManifestVersioned("thing", "1.0.1"), rig.signed(t, "v101"))
	assert.Equal(t, plugindomain.UpdateHeld, rig.check(t, p.ID).Outcome)
}

// A key pinned while verification was on keeps guarding the plugin after an
// admin turns verification off: an unsigned release still waits.
func TestAutoUpdate_APinnedKeyStillGuardsWithVerificationOff(t *testing.T) {
	rig := newAutoRig(t, "off")
	p := rig.installFromRepo(t, &rig.key)
	stored := rig.stored(t, p.ID)
	stored.PublisherKeyFingerprint, stored.PublisherKeyPEM = rig.key.fingerprint, rig.key.pem
	require.NoError(t, rig.plugins.Update(context.Background(), stored))

	rig.publish(t, goManifestVersioned("thing", "1.0.1"), pkg(t, nil, "v101"))
	st := rig.check(t, p.ID)
	assert.Equal(t, plugindomain.UpdateHeld, st.Outcome)
	assert.Contains(t, st.Reason, "no longer validly signed")
	assert.Equal(t, "1.0.0", rig.stored(t, p.ID).Version)
}

// Under enforce, a release signed by a key that is no longer trusted waits for
// the admin: it is a matter of trust, not malicious code, so it must not be
// reported as refused.
func TestAutoUpdate_AnUntrustedKeyUnderEnforceIsHeldNotRefused(t *testing.T) {
	rig := newAutoRig(t, "enforce")
	_, err := rig.keyring.AddByPEM(context.Background(), "Acme", rig.key.pem)
	require.NoError(t, err)
	p := rig.installFromRepo(t, &rig.key)
	require.NoError(t, rig.keyring.Remove(context.Background(), rig.key.fingerprint))

	rig.publish(t, goManifestVersioned("thing", "1.0.1"), rig.signed(t, "v101"))
	st := rig.check(t, p.ID)
	assert.Equal(t, plugindomain.UpdateHeld, st.Outcome, st.Reason)
	assert.Contains(t, st.Reason, "not a trusted publisher")
	assert.Equal(t, "1.0.0", rig.stored(t, p.ID).Version)
}
