package plugin_test

import (
	"archive/zip"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/pluginrunner"
	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	plugindomain "github.com/eveyproject/evey/internal/domain/plugin"
	plgfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

type signer struct {
	priv        *ecdsa.PrivateKey
	pem         string
	fingerprint string
}

func newSigner(t *testing.T) signer {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	p := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	fp, _, err := plugindomain.ParsePublisherKey([]byte(p))
	require.NoError(t, err)
	return signer{priv: priv, pem: p, fingerprint: fp}
}

// pkg builds a ZIP, optionally embedding k's public key and signing the bytes
// with k (a nil k builds an unsigned package: no cosign.pub, no .sig).
func pkg(t *testing.T, k *signer, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "p.zip")
	f, err := os.Create(path)
	require.NoError(t, err)
	zw := zip.NewWriter(f)
	w, err := zw.Create("main.py")
	require.NoError(t, err)
	_, err = w.Write([]byte(body))
	require.NoError(t, err)
	if k != nil {
		w, err = zw.Create("cosign.pub")
		require.NoError(t, err)
		_, err = w.Write([]byte(k.pem))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())
	if k != nil {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		sum := sha256.Sum256(data)
		sig, err := ecdsa.SignASN1(rand.Reader, k.priv, sum[:])
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path+".sig", []byte(base64.StdEncoding.EncodeToString(sig)), 0o600))
	}
	return path
}

type publisherRig struct {
	svc     *appplugin.InstallerService
	plugins *plgfakes.FakePluginRepository
	parser  *plgfakes.FakeManifestParser
	keyring *appplugin.TrustedPublisherService
}

func newPublisherRig(t *testing.T, policy string, opts ...appplugin.InstallerOption) *publisherRig {
	t.Helper()
	plugins := plgfakes.NewFakePluginRepository()
	parser := plgfakes.NewFakeManifestParser()
	keyring := appplugin.NewTrustedPublisherService(plgfakes.NewFakeTrustedPublisherRepository(), plugins)
	all := append([]appplugin.InstallerOption{
		appplugin.InstallerWithInstaller(plgfakes.NewFakePluginInstaller()),
		appplugin.InstallerWithAnalyzer(plgfakes.NewFakePluginAnalyzer()),
		appplugin.InstallerWithPluginDir("/tmp/test-plugins"),
		appplugin.InstallerWithFileSystem(plgfakes.NewFakePluginFileSystem()),
		appplugin.InstallerWithHTTPClient(http.DefaultClient),
		appplugin.InstallerWithSignatureVerifier(pluginrunner.NewCosignVerifier(policy, keyring.Lookup)),
		appplugin.InstallerWithTrustedPublishers(keyring),
	}, opts...)
	svc := appplugin.NewInstallerService(plugins, plgfakes.NewFakePluginRunner(), parser, sharedfakes.NewFakeEventBus(), all...)
	return &publisherRig{svc: svc, plugins: plugins, parser: parser, keyring: keyring}
}

func (r *publisherRig) install(t *testing.T, version, zipPath string) (*plugindomain.Plugin, error) {
	t.Helper()
	r.parser.Result = goManifestVersioned("signed-plugin", version)
	return r.svc.InstallPlugin(context.Background(), zipPath)
}

func (r *publisherRig) update(t *testing.T, id, version, zipPath string, accept bool) (*plugindomain.Plugin, error) {
	t.Helper()
	r.parser.Result = goManifestVersioned("signed-plugin", version)
	preview, err := r.svc.PreviewUpdate(context.Background(), id, zipPath)
	require.NoError(t, err)
	return r.svc.ApplyUpdate(context.Background(), id, zipPath, preview.SourceHash, accept)
}

func TestInstall_FirstInstallPinsTheFingerprint(t *testing.T) {
	rig := newPublisherRig(t, "warn")
	k := newSigner(t)

	p, err := rig.install(t, "1.0.0", pkg(t, &k, "v1"))
	require.NoError(t, err)
	require.NotNil(t, p.Signature)
	assert.True(t, p.Signature.Verified)
	assert.Empty(t, p.Signature.Continuity, "a first install has no continuity")
	assert.Equal(t, k.fingerprint, p.PublisherKeyFingerprint)
	assert.Equal(t, k.pem, p.PublisherKeyPEM)

	stored, err := rig.plugins.GetByID(context.Background(), p.ID)
	require.NoError(t, err)
	assert.Equal(t, k.fingerprint, stored.PublisherKeyFingerprint)
}

func TestUpdate_SameKeyIsContinuous(t *testing.T) {
	rig := newPublisherRig(t, "warn")
	k := newSigner(t)
	p, err := rig.install(t, "1.0.0", pkg(t, &k, "v1"))
	require.NoError(t, err)

	p2, err := rig.update(t, p.ID, "1.1.0", pkg(t, &k, "v2"), false)
	require.NoError(t, err)
	assert.Equal(t, plugindomain.ContinuitySame, p2.Signature.Continuity)
	assert.Equal(t, k.fingerprint, p2.PublisherKeyFingerprint)
}

func TestUpdate_OtherKeyIsBlockedUntilAccepted(t *testing.T) {
	rig := newPublisherRig(t, "warn")
	k1, k2 := newSigner(t), newSigner(t)
	p, err := rig.install(t, "1.0.0", pkg(t, &k1, "v1"))
	require.NoError(t, err)
	zip2 := pkg(t, &k2, "v2")

	rig.parser.Result = goManifestVersioned("signed-plugin", "1.1.0")
	preview, err := rig.svc.PreviewUpdate(context.Background(), p.ID, zip2)
	require.NoError(t, err)
	assert.Equal(t, plugindomain.SigStatePublisherChanged, preview.SignatureStatus)
	assert.True(t, preview.RequiresAck)
	require.NotNil(t, preview.Signature)
	assert.Equal(t, plugindomain.ContinuityChanged, preview.Signature.Continuity)
	assert.Equal(t, k2.fingerprint, preview.Signature.KeyFingerprint)

	_, err = rig.svc.ApplyUpdate(context.Background(), p.ID, zip2, preview.SourceHash, false)
	require.ErrorIs(t, err, appplugin.ErrPublisherChanged)
	stored, _ := rig.plugins.GetByID(context.Background(), p.ID)
	assert.Equal(t, "1.0.0", stored.Version, "a refused update leaves the installed version alone")
	assert.Equal(t, k1.fingerprint, stored.PublisherKeyFingerprint)

	p2, err := rig.svc.ApplyUpdate(context.Background(), p.ID, zip2, preview.SourceHash, true)
	require.NoError(t, err)
	assert.Equal(t, k2.fingerprint, p2.PublisherKeyFingerprint, "accepting re-pins to the new key")
	assert.Equal(t, k2.pem, p2.PublisherKeyPEM)

	// The new key is now the one an update must carry.
	_, err = rig.update(t, p.ID, "1.2.0", pkg(t, &k1, "v3"), false)
	require.ErrorIs(t, err, appplugin.ErrPublisherChanged)
}

func TestUpdate_SignedToUnsignedIsLostUntilAccepted(t *testing.T) {
	rig := newPublisherRig(t, "warn")
	k := newSigner(t)
	p, err := rig.install(t, "1.0.0", pkg(t, &k, "v1"))
	require.NoError(t, err)
	unsigned := pkg(t, nil, "v2")

	rig.parser.Result = goManifestVersioned("signed-plugin", "1.1.0")
	preview, err := rig.svc.PreviewUpdate(context.Background(), p.ID, unsigned)
	require.NoError(t, err)
	assert.Equal(t, plugindomain.SigStateSignatureLost, preview.SignatureStatus)
	assert.True(t, preview.RequiresAck)

	_, err = rig.svc.ApplyUpdate(context.Background(), p.ID, unsigned, preview.SourceHash, false)
	require.ErrorIs(t, err, appplugin.ErrSignatureLost)

	p2, err := rig.svc.ApplyUpdate(context.Background(), p.ID, unsigned, preview.SourceHash, true)
	require.NoError(t, err)
	assert.Empty(t, p2.PublisherKeyFingerprint, "an accepted unsigned update clears the pin")
	assert.Empty(t, p2.PublisherKeyPEM)
}

func TestUpdate_InvalidSignatureCountsAsLost(t *testing.T) {
	rig := newPublisherRig(t, "warn")
	k := newSigner(t)
	p, err := rig.install(t, "1.0.0", pkg(t, &k, "v1"))
	require.NoError(t, err)
	tampered := pkg(t, &k, "v2")
	require.NoError(t, os.WriteFile(tampered+".sig", []byte(base64.StdEncoding.EncodeToString([]byte("not a signature"))), 0o600))

	_, err = rig.update(t, p.ID, "1.1.0", tampered, false)
	require.ErrorIs(t, err, appplugin.ErrSignatureLost)
}

func TestUpdate_UnsignedThenSignedHasNoContinuityAndPins(t *testing.T) {
	rig := newPublisherRig(t, "warn")
	k := newSigner(t)
	p, err := rig.install(t, "1.0.0", pkg(t, nil, "v1"))
	require.NoError(t, err)
	assert.Empty(t, p.PublisherKeyFingerprint)

	p2, err := rig.update(t, p.ID, "1.1.0", pkg(t, &k, "v2"), false)
	require.NoError(t, err)
	assert.Empty(t, p2.Signature.Continuity)
	assert.Equal(t, k.fingerprint, p2.PublisherKeyFingerprint)
}

// Policy off only stops a bad signature from BLOCKING an install: a plugin
// pinned to a key while the policy was on keeps its continuity guard, so an
// unsigned or re-keyed update still needs the admin's acknowledgement, and the
// preview says so before ApplyUpdate refuses.
func TestUpdate_PolicyOff_KeepsTheContinuityGuard(t *testing.T) {
	k, other := newSigner(t), newSigner(t)
	rig := newPublisherRig(t, "warn")
	p, err := rig.install(t, "1.0.0", pkg(t, &k, "v1"))
	require.NoError(t, err)

	off := appplugin.NewInstallerService(rig.plugins, plgfakes.NewFakePluginRunner(), rig.parser, sharedfakes.NewFakeEventBus(),
		appplugin.InstallerWithInstaller(plgfakes.NewFakePluginInstaller()),
		appplugin.InstallerWithAnalyzer(plgfakes.NewFakePluginAnalyzer()),
		appplugin.InstallerWithPluginDir("/tmp/test-plugins"),
		appplugin.InstallerWithFileSystem(plgfakes.NewFakePluginFileSystem()),
		appplugin.InstallerWithSignatureVerifier(pluginrunner.NewCosignVerifier("off", nil)))
	rig.parser.Result = goManifestVersioned("signed-plugin", "1.1.0")

	unsigned := pkg(t, nil, "v2")
	preview, err := off.PreviewUpdate(context.Background(), p.ID, unsigned)
	require.NoError(t, err)
	assert.True(t, preview.RequiresAck, "the preview must ask for the acknowledgement ApplyUpdate needs")
	_, err = off.ApplyUpdate(context.Background(), p.ID, unsigned, preview.SourceHash, false)
	require.ErrorIs(t, err, appplugin.ErrSignatureLost)

	rekeyed := pkg(t, &other, "v2")
	preview, err = off.PreviewUpdate(context.Background(), p.ID, rekeyed)
	require.NoError(t, err)
	assert.True(t, preview.RequiresAck)
	_, err = off.ApplyUpdate(context.Background(), p.ID, rekeyed, preview.SourceHash, false)
	require.ErrorIs(t, err, appplugin.ErrPublisherChanged)

	same := pkg(t, &k, "v2")
	preview, err = off.PreviewUpdate(context.Background(), p.ID, same)
	require.NoError(t, err)
	assert.False(t, preview.RequiresAck)
	p2, err := off.ApplyUpdate(context.Background(), p.ID, same, preview.SourceHash, false)
	require.NoError(t, err)
	assert.Equal(t, k.fingerprint, p2.PublisherKeyFingerprint, "the same key keeps the pin")
}

// With nothing pinned, policy off checks nothing and stores nothing.
func TestInstall_PolicyOff_UnpinnedStoresNothing(t *testing.T) {
	rig := newPublisherRig(t, "off")
	p, err := rig.install(t, "1.0.0", pkg(t, nil, "v1"))
	require.NoError(t, err)
	assert.Nil(t, p.Signature)
	assert.Empty(t, p.PublisherKeyFingerprint)
}

func TestInstall_EnforceBlocksUntrustedAndAdmitsTrusted(t *testing.T) {
	rig := newPublisherRig(t, "enforce")
	k := newSigner(t)
	signed := pkg(t, &k, "v1")

	_, err := rig.install(t, "1.0.0", signed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin blocked:")
	assert.Contains(t, err.Error(), "not a trusted publisher")

	_, err = rig.install(t, "1.0.0", pkg(t, nil, "v1"))
	require.Error(t, err, "enforce refuses an unsigned package")

	_, err = rig.keyring.AddByPEM(context.Background(), "Acme", k.pem)
	require.NoError(t, err)
	p, err := rig.install(t, "1.0.0", signed)
	require.NoError(t, err)
	assert.True(t, p.Signature.Trusted)
	assert.Equal(t, "Acme", p.Signature.Publisher)
}

func TestInstall_WarnStoresUntrustedAndPasses(t *testing.T) {
	rig := newPublisherRig(t, "warn")
	k := newSigner(t)
	p, err := rig.install(t, "1.0.0", pkg(t, &k, "v1"))
	require.NoError(t, err)
	assert.True(t, p.Signature.Verified)
	assert.False(t, p.Signature.Trusted)
	assert.Empty(t, p.Signature.Publisher)

	unsigned, err := newPublisherRig(t, "warn").install(t, "1.0.0", pkg(t, nil, "v1"))
	require.NoError(t, err)
	assert.True(t, unsigned.Signature.Unsigned())
}

func TestPreviewPlugin_ReportsTheSignatureVerdict(t *testing.T) {
	rig := newPublisherRig(t, "warn")
	k := newSigner(t)
	rig.parser.Result = goManifestVersioned("signed-plugin", "1.0.0")

	pv, err := rig.svc.PreviewPlugin(context.Background(), pkg(t, &k, "v1"))
	require.NoError(t, err)
	assert.Equal(t, plugindomain.SigStateSignedUntrusted, pv.SignatureState)
	require.NotNil(t, pv.Signature)
	assert.Equal(t, k.fingerprint, pv.Signature.KeyFingerprint)

	_, err = rig.keyring.AddByPEM(context.Background(), "Acme", k.pem)
	require.NoError(t, err)
	pv, err = rig.svc.PreviewPlugin(context.Background(), pkg(t, &k, "v1"))
	require.NoError(t, err)
	assert.Equal(t, plugindomain.SigStateSigned, pv.SignatureState)
	assert.Equal(t, "Acme", pv.Signature.Publisher)

	pv, err = rig.svc.PreviewPlugin(context.Background(), pkg(t, nil, "v1"))
	require.NoError(t, err)
	assert.Equal(t, plugindomain.SigStateUnsigned, pv.SignatureState)
}

func TestPreviewPlugin_UnderAnInstalledNameJudgesContinuity(t *testing.T) {
	rig := newPublisherRig(t, "warn")
	k1, k2 := newSigner(t), newSigner(t)
	_, err := rig.install(t, "1.0.0", pkg(t, &k1, "v1"))
	require.NoError(t, err)
	rig.parser.Result = goManifestVersioned("signed-plugin", "1.1.0")
	pv, err := rig.svc.PreviewPlugin(context.Background(), pkg(t, &k2, "v2"))
	require.NoError(t, err)
	assert.Equal(t, plugindomain.SigStatePublisherChanged, pv.SignatureState)
}

func TestPreviewPlugin_PolicyOffHasNoSignature(t *testing.T) {
	rig := newPublisherRig(t, "off")
	rig.parser.Result = goManifestVersioned("signed-plugin", "1.0.0")
	pv, err := rig.svc.PreviewPlugin(context.Background(), pkg(t, nil, "v1"))
	require.NoError(t, err)
	assert.Nil(t, pv.Signature)
	assert.Empty(t, pv.SignatureState)
}

// A URL install fetches "<url>.sig" next to the ZIP; a 404 is "unsigned".
func TestInstallFromURL_FetchesTheDetachedSignature(t *testing.T) {
	k := newSigner(t)
	zipPath := pkg(t, &k, "v1")
	zipBytes, err := os.ReadFile(zipPath)
	require.NoError(t, err)
	sigBytes, err := os.ReadFile(zipPath + ".sig")
	require.NoError(t, err)

	withSig := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/p.zip":
			_, _ = w.Write(zipBytes)
		case r.URL.Path == "/p.zip.sig" && withSig:
			_, _ = w.Write(sigBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	rig := newPublisherRig(t, "warn")
	p, err := rig.install(t, "1.0.0", srv.URL+"/p.zip")
	require.NoError(t, err)
	assert.True(t, p.Signature.Verified)
	assert.Equal(t, k.fingerprint, p.PublisherKeyFingerprint)

	withSig = false
	p2, err := newPublisherRig(t, "warn").install(t, "1.0.0", srv.URL+"/p.zip")
	require.NoError(t, err, "a missing .sig is an unsigned plugin, not a failed download")
	assert.True(t, p2.Signature.Unsigned())

	// A query string stays on the ZIP request; the signature is the same path + ".sig".
	withSig = true
	p3, err := newPublisherRig(t, "warn").install(t, "1.0.0", srv.URL+"/p.zip?token=abc")
	require.NoError(t, err)
	assert.True(t, p3.Signature.Verified)
}

func TestTrustedPublishers_AddRemoveSeed(t *testing.T) {
	ctx := context.Background()
	plugins := plgfakes.NewFakePluginRepository()
	repo := plgfakes.NewFakeTrustedPublisherRepository()
	svc := appplugin.NewTrustedPublisherService(repo, plugins)
	k, op := newSigner(t), newSigner(t)

	_, err := svc.AddByPEM(ctx, "", k.pem)
	require.ErrorIs(t, err, appplugin.ErrPublisherNameRequired)
	_, err = svc.AddByPEM(ctx, strings.Repeat("x", 101), k.pem)
	require.ErrorIs(t, err, appplugin.ErrPublisherNameTooLong)
	_, err = svc.AddByPEM(ctx, "Acme", "not a key")
	require.ErrorIs(t, err, appplugin.ErrPublisherKeyInvalid)

	added, err := svc.AddByPEM(ctx, " Acme ", k.pem)
	require.NoError(t, err)
	assert.Equal(t, "Acme", added.Name)
	assert.Equal(t, k.fingerprint, added.Fingerprint)
	name, trusted := svc.Lookup(ctx, k.fingerprint)
	assert.True(t, trusted)
	assert.Equal(t, "Acme", name)
	_, trusted = svc.Lookup(ctx, "sha256:nope")
	assert.False(t, trusted)

	// By fingerprint: only a key an installed plugin carries.
	_, err = svc.AddByFingerprint(ctx, "Ghost", "sha256:ffff")
	require.ErrorIs(t, err, appplugin.ErrPublisherKeyUnknown)
	require.NoError(t, plugins.Create(ctx, &plugindomain.Plugin{ID: "plg-1", Name: "n", PublisherKeyFingerprint: op.fingerprint, PublisherKeyPEM: op.pem}))
	byFP, err := svc.AddByFingerprint(ctx, "Operator friend", op.fingerprint)
	require.NoError(t, err)
	assert.Equal(t, op.pem, byFP.PublicKeyPEM)

	require.NoError(t, svc.Remove(ctx, k.fingerprint))
	require.ErrorIs(t, svc.Remove(ctx, k.fingerprint), shared.ErrNotFound)

	// Seeding from the configured key file; seeded rows cannot be removed.
	keyFile := filepath.Join(t.TempDir(), "cosign.pub")
	require.NoError(t, os.WriteFile(keyFile, []byte(k.pem), 0o600))
	require.NoError(t, svc.SeedOperatorKey(ctx, os.ReadFile, keyFile))
	got, err := repo.Get(ctx, k.fingerprint)
	require.NoError(t, err)
	assert.True(t, got.Seeded)
	assert.Equal(t, appplugin.OperatorKeyName, got.Name)
	require.ErrorIs(t, svc.Remove(ctx, k.fingerprint), plugindomain.ErrSeededPublisher)

	// The operator drops the setting: the seeded key stops being trusted, the
	// hand-added one stays.
	require.NoError(t, svc.SeedOperatorKey(ctx, os.ReadFile, ""))
	_, trusted = svc.Lookup(ctx, k.fingerprint)
	assert.False(t, trusted)
	_, trusted = svc.Lookup(ctx, op.fingerprint)
	assert.True(t, trusted)

	require.Error(t, svc.SeedOperatorKey(ctx, os.ReadFile, filepath.Join(t.TempDir(), "missing.pub")))
}

func TestRefreshTrust_ReadsTheKeyringNowAndCopies(t *testing.T) {
	stored := &plugindomain.SignatureStatus{Verified: true, KeyFingerprint: "sha256:aa"}
	lookup := func(_ context.Context, fp string) (string, bool) { return "Acme", fp == "sha256:aa" }

	got := appplugin.RefreshTrust(context.Background(), stored, lookup)
	assert.True(t, got.Trusted)
	assert.Equal(t, "Acme", got.Publisher)
	assert.False(t, stored.Trusted, "the stored verdict is not mutated")

	unverified := &plugindomain.SignatureStatus{KeyFingerprint: "sha256:aa", Trusted: true, Publisher: "stale"}
	got = appplugin.RefreshTrust(context.Background(), unverified, lookup)
	assert.False(t, got.Trusted, "a signature that did not verify is never trusted")
	assert.Empty(t, got.Publisher)

	assert.Nil(t, appplugin.RefreshTrust(context.Background(), nil, lookup))
}

func TestSecurityDashboard_SignatureCarriesTheLiveKeyring(t *testing.T) {
	ctx := context.Background()
	repo := plgfakes.NewFakePluginRepository()
	keys := appplugin.NewTrustedPublisherService(plgfakes.NewFakeTrustedPublisherRepository(), repo)
	k := newSigner(t)
	require.NoError(t, repo.Create(ctx, &plugindomain.Plugin{
		ID: "plg-1", Name: "n", Version: "1.0.0",
		Signature: &plugindomain.SignatureStatus{
			Verified: true, VerifiedBy: k.fingerprint, KeyFingerprint: k.fingerprint, Continuity: plugindomain.ContinuitySame,
		},
		PublisherKeyFingerprint: k.fingerprint, PublisherKeyPEM: k.pem,
	}))
	svc := appplugin.NewSecurityDashboardService(repo, sharedfakes.NewFakeEventBus(), func() string { return "basic" }).
		WithKeyLookup(keys.Lookup)

	snaps, err := svc.GetSecurityDashboard(ctx)
	require.NoError(t, err)
	require.Len(t, snaps, 1)
	assert.False(t, snaps[0].Signature.Trusted)
	assert.Equal(t, plugindomain.ContinuitySame, snaps[0].Signature.Continuity)

	_, err = keys.AddByFingerprint(ctx, "Acme", k.fingerprint)
	require.NoError(t, err)
	snaps, err = svc.GetSecurityDashboard(ctx)
	require.NoError(t, err)
	assert.True(t, snaps[0].Signature.Trusted)
	assert.Equal(t, "Acme", snaps[0].Signature.Publisher)
}
