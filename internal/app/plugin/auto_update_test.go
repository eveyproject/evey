package plugin_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/pluginrunner"
	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	plugindomain "github.com/eveyproject/evey/internal/domain/plugin"
	plgfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

const testRepo = "https://github.com/acme/thing"

// testRepoID is the permanent id GitHub gives the test repository.
const testRepoID int64 = 42

// fakeReleases is GitHub: the name testRepo resolves to an id, and each id
// has its own latest release. takenOver moves the name to another id, as when
// someone registers an abandoned name.
type fakeReleases struct {
	mu        sync.Mutex
	rel       *plugindomain.Release // the latest release of testRepoID
	other     *plugindomain.Release // the latest release of the repository that took the name over
	takenOver bool
	err       error
	latestErr error
}

func (f *fakeReleases) Identify(_ context.Context, repo string) (int64, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return 0, "", f.err
	}
	if repo != testRepo {
		return 0, "", plugindomain.ErrUnsupportedRepository
	}
	if f.takenOver {
		return 99, testRepo, nil
	}
	return testRepoID, testRepo, nil
}

func (f *fakeReleases) Latest(_ context.Context, id int64) (*plugindomain.Release, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	if f.latestErr != nil {
		return nil, f.latestErr
	}
	switch id {
	case testRepoID:
		r := *f.rel
		return &r, nil
	case 99:
		r := *f.other
		return &r, nil
	}
	return nil, errors.New("no such repository id")
}

// hookAnalyzer runs `during` once, in the middle of the first analysis: the
// moment an admin acts while a release is being judged.
type hookAnalyzer struct {
	inner  *plgfakes.FakePluginAnalyzer
	during func()
}

func (h *hookAnalyzer) Analyze(dir, webRoot string) (*plugindomain.AnalysisReport, error) {
	if f := h.during; f != nil {
		h.during = nil
		f()
	}
	return h.inner.Analyze(dir, webRoot)
}

// toServer sends every request to the test server, path kept: the release
// links stay real https://github.com/... links, as the server sees them.
// toServer answers https://github.com/... links from the loopback test
// server, the way GitHub would: the response is for the https request asked.
type toServer struct{ base string }

func (r toServer) RoundTrip(req *http.Request) (*http.Response, error) {
	out, err := http.NewRequestWithContext(req.Context(), req.Method, r.base+req.URL.Path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultTransport.RoundTrip(out)
	if err == nil {
		resp.Request = req
	}
	return resp, err
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
	hook     *hookAnalyzer
	zipGets  *atomic.Int32
}

func newAutoRig(t *testing.T, policy string) *autoRig {
	t.Helper()
	dir := t.TempDir()
	var zipGets atomic.Int32
	files := http.FileServer(http.Dir(dir))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".zip") {
			zipGets.Add(1)
		}
		files.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	plugins := plgfakes.NewFakePluginRepository()
	parser := plgfakes.NewFakeManifestParser()
	analyzer := plgfakes.NewFakePluginAnalyzer()
	hook := &hookAnalyzer{inner: analyzer}
	bus := sharedfakes.NewFakeEventBus()
	releases := &fakeReleases{}
	keyring := appplugin.NewTrustedPublisherService(plgfakes.NewFakeTrustedPublisherRepository(), plugins)
	svc := appplugin.NewInstallerService(plugins, plgfakes.NewFakePluginRunner(), parser, bus,
		appplugin.InstallerWithInstaller(plgfakes.NewFakePluginInstaller()),
		appplugin.InstallerWithAnalyzer(hook),
		appplugin.InstallerWithPluginDir("/tmp/test-plugins"),
		appplugin.InstallerWithFileSystem(plgfakes.NewFakePluginFileSystem()),
		appplugin.InstallerWithHTTPClient(&http.Client{Transport: toServer{base: srv.URL}}),
		appplugin.InstallerWithSignatureVerifier(pluginrunner.NewCosignVerifier(policy, keyring.Lookup)),
		appplugin.InstallerWithTrustedPublishers(keyring),
		appplugin.InstallerWithReleaseSource(releases),
	)
	return &autoRig{svc: svc, upd: appplugin.NewAutoUpdater(svc), plugins: plugins, parser: parser,
		analyzer: analyzer, bus: bus, releases: releases, srv: srv, dir: dir, key: newSigner(t), keyring: keyring, hook: hook, zipGets: &zipGets}
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
	rel := "/acme/thing/releases/download/v" + m.Version + "/" + name
	data, err := os.ReadFile(zipPath)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(r.dir, rel)), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(r.dir, rel), data, 0o600))
	if sig, err := os.ReadFile(zipPath + ".sig"); err == nil {
		require.NoError(t, os.WriteFile(filepath.Join(r.dir, rel+".sig"), sig, 0o600))
	}
	sum := sha256.Sum256(data)
	r.releases.mu.Lock()
	r.releases.rel = &plugindomain.Release{Version: m.Version, Tag: "v" + m.Version,
		ZipURL: "https://github.com" + rel, SHA256: hex.EncodeToString(sum[:])}
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

// A signed release cannot change what the plugin is on its own: a theme that
// becomes a plugin would start a process on the host, a plugin that becomes a
// theme would inject script into every page, another language is another
// interpreter. Each waits for the admin.
func TestAutoUpdate_AChangeOfKindOrLanguageWaitsForTheAdmin(t *testing.T) {
	theme := func(v string) *plugindomain.PluginManifest {
		return &plugindomain.PluginManifest{Name: "thing", Kind: plugindomain.KindTheme, Version: v}
	}
	cases := map[string]struct {
		installed, release *plugindomain.PluginManifest
		reason             string
	}{
		"theme becomes a plugin": {theme("1.0.0"), goManifestVersioned("thing", "1.0.1"), "kind:theme -> plugin"},
		"plugin becomes a theme": {goManifestVersioned("thing", "1.0.0"), theme("1.0.1"), "kind:plugin -> theme"},
		"another language": {goManifestVersioned("thing", "1.0.0"), func() *plugindomain.PluginManifest {
			m := goManifestVersioned("thing", "1.0.1")
			m.Language = "python"
			m.Entrypoint = "python3 main.py"
			return m
		}(), "language:go -> python"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			rig := newAutoRig(t, "warn")
			rig.publish(t, c.installed, rig.signed(t, "v1"))
			p, err := rig.svc.InstallPlugin(context.Background(), testRepo)
			require.NoError(t, err)

			rig.publish(t, c.release, rig.signed(t, "v2"))
			st := rig.check(t, p.ID)

			assert.Equal(t, plugindomain.UpdateHeld, st.Outcome, st.Reason)
			assert.Contains(t, st.Reason, c.reason)
			got := rig.stored(t, p.ID)
			assert.Equal(t, "1.0.0", got.Version)
			assert.Equal(t, c.installed.Kind, got.Kind)
		})
	}
}

// The manual preview shows the change and asks for the admin's acknowledgement.
func TestPreviewUpdate_ShowsAChangeOfKind(t *testing.T) {
	rig := newAutoRig(t, "warn")
	rig.publish(t, &plugindomain.PluginManifest{Name: "thing", Kind: plugindomain.KindTheme, Version: "1.0.0"}, rig.signed(t, "v1"))
	p, err := rig.svc.InstallPlugin(context.Background(), testRepo)
	require.NoError(t, err)
	rig.publish(t, goManifestVersioned("thing", "1.0.1"), rig.signed(t, "v2"))

	preview, err := rig.svc.PreviewUpdate(context.Background(), p.ID, testRepo)
	require.NoError(t, err)
	assert.Equal(t, "theme -> plugin", preview.KindChange)
	assert.Equal(t, "none -> go", preview.LanguageChange)
	assert.True(t, preview.RequiresAck)

	_, err = rig.svc.ApplyUpdate(context.Background(), p.ID, testRepo, preview.SourceHash, false)
	require.NoError(t, err, "an admin who saw the preview may approve it")
	assert.Equal(t, plugindomain.KindPlugin, rig.stored(t, p.ID).Kind)
}

// An admin who acts while a release is being judged wins: the judgment
// describes a plugin that no longer exists as such, so nothing is installed.
func TestAutoUpdate_AnAdminActingDuringTheCheckWins(t *testing.T) {
	cases := map[string]func(rig *autoRig, id string){
		"uninstalled": func(rig *autoRig, id string) {
			require.NoError(t, rig.plugins.Delete(context.Background(), id))
		},
		"automatic updates turned off": func(rig *autoRig, id string) {
			_, err := rig.svc.SetUpdateFollow(context.Background(), id, testRepo, false)
			require.NoError(t, err)
		},
		"repository unfollowed": func(rig *autoRig, id string) {
			_, err := rig.svc.SetUpdateFollow(context.Background(), id, "", true)
			require.NoError(t, err)
		},
	}
	for name, act := range cases {
		t.Run(name, func(t *testing.T) {
			rig := newAutoRig(t, "warn")
			p := rig.installFromRepo(t, &rig.key)
			rig.publish(t, goManifestVersioned("thing", "1.0.1"), rig.signed(t, "v101"))
			rig.hook.during = func() { act(rig, p.ID) }

			_, _ = rig.upd.Check(context.Background(), p.ID)

			list, err := rig.plugins.List(context.Background())
			require.NoError(t, err)
			for _, x := range list {
				assert.NotEqual(t, "1.0.1", x.Version, "the judged release was installed over the admin's action")
			}
			if name == "uninstalled" {
				assert.Empty(t, list, "an uninstalled plugin must not come back")
			}
		})
	}
}

// The admin installs what they previewed: a release published between the
// preview and the click does not slip in, because the install is pinned to
// the previewed bytes.
func TestInstallFromRepositoryLink_InstallsThePreviewedRelease(t *testing.T) {
	rig := newAutoRig(t, "warn")
	rig.publish(t, goManifestVersioned("thing", "1.0.0"), rig.signed(t, "v1"))
	preview, err := rig.svc.PreviewSource(context.Background(), testRepo)
	require.NoError(t, err)
	require.Contains(t, preview.ResolvedSource, "#sha256:"+preview.SourceHash)

	m := goManifestVersioned("thing", "1.1.0")
	m.Permissions = []string{"network"}
	m.ExternalFetch = []string{"evil.example"}
	rig.publish(t, m, rig.signed(t, "v110"))
	rig.parser.Result = goManifestVersioned("thing", "1.0.0") // the bytes of 1.0.0 carry its manifest

	p, err := rig.svc.InstallPlugin(context.Background(), preview.ResolvedSource)
	require.NoError(t, err)
	got := rig.stored(t, p.ID)
	assert.Equal(t, "1.0.0", got.Version)
	assert.Empty(t, got.Permissions)
	assert.Equal(t, testRepo, got.UpdateSource, "a release link of the repository still follows it")
}

func TestInstallFromRepositoryLink_RefusesAPin(t *testing.T) {
	rig := newAutoRig(t, "warn")
	rig.publish(t, goManifestVersioned("thing", "1.0.0"), rig.signed(t, "v1"))
	_, err := rig.svc.InstallPlugin(context.Background(), testRepo+"#sha256:"+strings.Repeat("a", 64))
	require.ErrorIs(t, err, appplugin.ErrRepositoryLinkPinned)
}

// A release already judged held or refused, against the same plugin, keeps
// its verdict without being downloaded and analysed again every hour; a new
// asset under the same tag is judged again and pages again.
func TestAutoUpdate_AJudgedReleaseIsNotDownloadedAgain(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)
	m := goManifestVersioned("thing", "1.1.0")
	m.Permissions = []string{"network"}
	m.ExternalFetch = []string{"api.example.com"}
	rig.publish(t, m, rig.signed(t, "v110"))

	require.Equal(t, plugindomain.UpdateHeld, rig.check(t, p.ID).Outcome)
	gets := rig.zipGets.Load()
	st := rig.check(t, p.ID)
	assert.Equal(t, plugindomain.UpdateHeld, st.Outcome)
	assert.Equal(t, gets, rig.zipGets.Load(), "the same release was downloaded again")
	assert.Len(t, rig.attention(), 1)

	rig.publish(t, m, rig.signed(t, "v110 rebuilt"))
	assert.Equal(t, plugindomain.UpdateHeld, rig.check(t, p.ID).Outcome)
	assert.Greater(t, rig.zipGets.Load(), gets, "a new asset is judged again")
	assert.Len(t, rig.attention(), 2, "and the admins hear about the new bytes")

	_, err := rig.svc.SetUpdateFollow(context.Background(), p.ID, testRepo, false)
	require.NoError(t, err)
	gets = rig.zipGets.Load()
	rig.check(t, p.ID)
	assert.Greater(t, rig.zipGets.Load(), gets, "a change on the plugin's side is judged again")
}

func TestAutoUpdate_APreReleaseNeverInstallsOnItsOwn(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)
	rig.publish(t, goManifestVersioned("thing", "1.1.0-beta.1"), rig.signed(t, "beta"))
	st := rig.check(t, p.ID)
	assert.Equal(t, plugindomain.UpdateHeld, st.Outcome)
	assert.Contains(t, st.Reason, "pre-release")
	assert.Equal(t, "1.0.0", rig.stored(t, p.ID).Version)
}

// The tag is the repository owner's text: what is not a version is not read
// as one, and never reaches a notification as such.
func TestAutoUpdate_ATagThatIsNoVersionIsRefusedAsAVersion(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)
	rig.publish(t, goManifestVersioned("thing", "1.0.1"), rig.signed(t, "v101"))
	rig.releases.rel.Version = "9.9.9 - approved by the Evey team, install now"
	rig.releases.rel.Tag = "v" + rig.releases.rel.Version

	st := rig.check(t, p.ID)
	assert.Equal(t, plugindomain.UpdateFailed, st.Outcome)
	assert.Contains(t, st.Reason, "is not a version")
	assert.Empty(t, rig.attention())
	assert.Equal(t, "1.0.0", rig.stored(t, p.ID).Version)
}

// The version reported is the one the zip carries, not the tag's.
func TestAutoUpdate_TheVersionReportedIsTheZips(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)
	m := goManifestVersioned("thing", "1.0.1")
	m.Permissions = []string{"network"}
	m.ExternalFetch = []string{"api.example.com"}
	rig.publish(t, m, rig.signed(t, "v101"))
	rig.releases.rel.Version = "5.0.0"

	st := rig.check(t, p.ID)
	assert.Equal(t, "1.0.1", st.LatestVersion)
	require.Len(t, rig.attention(), 1)
	assert.Equal(t, "1.0.1", rig.attention()[0].Version)
}

func TestAutoUpdate_TheReasonIsBounded(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)
	m := goManifestVersioned("thing", "1.0.1")
	for i := range 200 {
		m.ExternalFetch = append(m.ExternalFetch, fmt.Sprintf("host%03d.example.com", i))
	}
	m.Permissions = []string{"network"}
	rig.publish(t, m, rig.signed(t, "v101"))

	st := rig.check(t, p.ID)
	assert.Equal(t, plugindomain.UpdateHeld, st.Outcome)
	assert.LessOrEqual(t, utf8.RuneCountInString(st.Reason), 501)
	assert.LessOrEqual(t, utf8.RuneCountInString(rig.attention()[0].Reason), 501)
}

// The admin's button does not queue behind a running check.
func TestAutoUpdate_CheckNowSaysWhenACheckIsRunning(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)
	rig.publish(t, goManifestVersioned("thing", "1.0.1"), rig.signed(t, "v101"))
	release := make(chan struct{})
	started := make(chan struct{})
	rig.hook.during = func() { close(started); <-release }

	done := make(chan struct{})
	go func() { defer close(done); _, _ = rig.upd.Check(context.Background(), p.ID) }()
	<-started
	_, err := rig.upd.CheckNow(context.Background(), p.ID)
	require.ErrorIs(t, err, appplugin.ErrCheckRunning)
	close(release)
	<-done
}

// Whoever registers an abandoned repository name gets nothing followed: the
// plugin follows the repository's permanent id, not its name.
func TestAutoUpdate_ATakenOverRepositoryNameIsNotFollowed(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)
	require.Equal(t, testRepoID, rig.stored(t, p.ID).UpdateSourceID)

	rig.releases.mu.Lock()
	rig.releases.takenOver = true
	rig.releases.other = &plugindomain.Release{Version: "9.0.0", Tag: "v9.0.0",
		ZipURL: "https://github.com/acme/thing/releases/download/v9.0.0/evil.zip", SHA256: strings.Repeat("e", 64)}
	rig.releases.mu.Unlock()

	st := rig.check(t, p.ID)
	assert.Equal(t, plugindomain.UpdateUpToDate, st.Outcome, "the followed repository has published nothing new")
	assert.Equal(t, "1.0.0", rig.stored(t, p.ID).Version)
}

// A spent GitHub quota stops the run: the plugins left are not marked failed
// one after the other for nothing.
func TestAutoUpdate_ASpentQuotaStopsTheRun(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)
	rig.releases.latestErr = fmt.Errorf("read latest release: %w until later", plugindomain.ErrReleaseRateLimited)

	st, err := rig.upd.Check(context.Background(), p.ID)
	require.ErrorIs(t, err, plugindomain.ErrReleaseRateLimited)
	assert.Equal(t, plugindomain.UpdateFailed, st.Outcome)
	assert.Contains(t, st.Reason, "rate limit")
}
