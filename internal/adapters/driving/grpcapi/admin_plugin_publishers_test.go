package grpcapi

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	plugindomain "github.com/eveyproject/evey/internal/domain/plugin"
	pluginfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/eveyproject/evey/internal/pkg/demo"
)

func newPublisherKey(t *testing.T) (pemStr, fingerprint string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	pemStr = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	fingerprint, _, err = plugindomain.ParsePublisherKey([]byte(pemStr))
	require.NoError(t, err)
	return pemStr, fingerprint
}

func newPublisherHandler(t *testing.T) (*adminPluginHandler, *pluginfakes.FakePluginRepository, *pluginfakes.FakeTrustedPublisherRepository) {
	t.Helper()
	plugins := pluginfakes.NewFakePluginRepository()
	keys := pluginfakes.NewFakeTrustedPublisherRepository()
	runtimeRepo := pluginfakes.NewFakePluginRuntimeRepository()
	svc := appplugin.NewInstallerService(plugins, pluginfakes.NewFakePluginRunner(), pluginfakes.NewFakeManifestParser(),
		sharedfakes.NewFakeEventBus(),
		appplugin.InstallerWithTrustedPublishers(appplugin.NewTrustedPublisherService(keys, plugins)))
	return &adminPluginHandler{
		pluginInstaller: svc,
		pluginQuery:     appplugin.NewQueryService(plugins, runtimeRepo),
		enabled:         true,
	}, plugins, keys
}

func TestTrustedPublishers_AddByPEM_List_Remove(t *testing.T) {
	h, _, _ := newPublisherHandler(t)
	ctx := context.Background()
	keyPEM, fp := newPublisherKey(t)

	added, err := h.AddTrustedPublisher(ctx, &eveyv1.AddTrustedPublisherRequest{Name: "Acme", PublicKeyPem: keyPEM})
	require.NoError(t, err)
	assert.Equal(t, fp, added.Fingerprint)
	assert.Equal(t, "Acme", added.Name)
	assert.False(t, added.Seeded)
	assert.NotEmpty(t, added.AddedAt)
	assert.Contains(t, added.PublicKeyPem, "BEGIN PUBLIC KEY")

	list, err := h.ListTrustedPublishers(ctx, &eveyv1.ListTrustedPublishersRequest{})
	require.NoError(t, err)
	require.Len(t, list.Publishers, 1)
	assert.Equal(t, fp, list.Publishers[0].Fingerprint)

	_, err = h.RemoveTrustedPublisher(ctx, &eveyv1.RemoveTrustedPublisherRequest{Fingerprint: fp})
	require.NoError(t, err)
	list, err = h.ListTrustedPublishers(ctx, &eveyv1.ListTrustedPublishersRequest{})
	require.NoError(t, err)
	assert.Empty(t, list.Publishers)

	_, err = h.RemoveTrustedPublisher(ctx, &eveyv1.RemoveTrustedPublisherRequest{Fingerprint: fp})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestTrustedPublishers_AddByFingerprint_FromAnInstalledPlugin(t *testing.T) {
	h, plugins, _ := newPublisherHandler(t)
	ctx := context.Background()
	keyPEM, fp := newPublisherKey(t)
	require.NoError(t, plugins.Create(ctx, &plugindomain.Plugin{
		ID: "plg-1", Name: "signed", PublisherKeyFingerprint: fp, PublisherKeyPEM: keyPEM,
	}))

	added, err := h.AddTrustedPublisher(ctx, &eveyv1.AddTrustedPublisherRequest{Name: "Acme", Fingerprint: fp})
	require.NoError(t, err)
	assert.Equal(t, keyPEM, added.PublicKeyPem)

	_, err = h.AddTrustedPublisher(ctx, &eveyv1.AddTrustedPublisherRequest{Name: "Ghost", Fingerprint: "sha256:ffff"})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestTrustedPublishers_AddRejectsBadInput(t *testing.T) {
	h, _, _ := newPublisherHandler(t)
	ctx := context.Background()
	keyPEM, fp := newPublisherKey(t)
	for name, req := range map[string]*eveyv1.AddTrustedPublisherRequest{
		"no name":         {PublicKeyPem: keyPEM},
		"no key at all":   {Name: "Acme"},
		"both key forms":  {Name: "Acme", PublicKeyPem: keyPEM, Fingerprint: fp},
		"garbage pem":     {Name: "Acme", PublicKeyPem: "not a key"},
		"name too long":   {Name: string(make([]byte, 101)) + "x", PublicKeyPem: keyPEM},
		"blank both only": {Name: "Acme", PublicKeyPem: "  ", Fingerprint: " "},
	} {
		_, err := h.AddTrustedPublisher(ctx, req)
		assert.Equal(t, codes.InvalidArgument, status.Code(err), name)
	}
}

func TestTrustedPublishers_SeededCannotBeRemoved(t *testing.T) {
	h, _, keys := newPublisherHandler(t)
	ctx := context.Background()
	require.NoError(t, keys.Upsert(ctx, plugindomain.TrustedPublisher{
		Fingerprint: "sha256:op", Name: appplugin.OperatorKeyName, PublicKeyPEM: "pem", Seeded: true,
	}))

	list, err := h.ListTrustedPublishers(ctx, &eveyv1.ListTrustedPublishersRequest{})
	require.NoError(t, err)
	require.Len(t, list.Publishers, 1)
	assert.True(t, list.Publishers[0].Seeded)

	_, err = h.RemoveTrustedPublisher(ctx, &eveyv1.RemoveTrustedPublisherRequest{Fingerprint: "sha256:op"})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestTrustedPublishers_DisabledSystemRefusesMutationsOnly(t *testing.T) {
	h, _, _ := newPublisherHandler(t)
	h.enabled = false
	ctx := context.Background()
	keyPEM, _ := newPublisherKey(t)

	_, err := h.AddTrustedPublisher(ctx, &eveyv1.AddTrustedPublisherRequest{Name: "Acme", PublicKeyPem: keyPEM})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	_, err = h.RemoveTrustedPublisher(ctx, &eveyv1.RemoveTrustedPublisherRequest{Fingerprint: "sha256:x"})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	_, err = h.ListTrustedPublishers(ctx, &eveyv1.ListTrustedPublishersRequest{})
	assert.NoError(t, err)
}

func TestTrustedPublishers_NotWiredIsUnimplementedOrEmpty(t *testing.T) {
	h := &adminPluginHandler{enabled: true}
	ctx := context.Background()
	list, err := h.ListTrustedPublishers(ctx, &eveyv1.ListTrustedPublishersRequest{})
	require.NoError(t, err)
	assert.Empty(t, list.Publishers)
	_, err = h.AddTrustedPublisher(ctx, &eveyv1.AddTrustedPublisherRequest{Name: "A", PublicKeyPem: "x"})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

// Plugin.signature is the install-time verdict with Publisher/Trusted read
// from the keyring now: trusting a key from the card flips it at once.
func TestListPlugins_SignatureMappingFollowsTheKeyring(t *testing.T) {
	h, plugins, _ := newPublisherHandler(t)
	ctx := context.Background()
	keyPEM, fp := newPublisherKey(t)
	require.NoError(t, plugins.Create(ctx, &plugindomain.Plugin{
		ID: "plg-1", Name: "signed", PublisherKeyFingerprint: fp, PublisherKeyPEM: keyPEM,
		Signature: &plugindomain.SignatureStatus{Verified: true, VerifiedBy: fp, KeyFingerprint: fp, Continuity: plugindomain.ContinuitySame},
	}))
	require.NoError(t, plugins.Create(ctx, &plugindomain.Plugin{ID: "plg-2", Name: "plain"}))

	find := func(resp *eveyv1.ListPluginsResponse, id string) *eveyv1.Plugin {
		for _, p := range resp.Plugins {
			if p.Id == id {
				return p
			}
		}
		t.Fatalf("plugin %s not listed", id)
		return nil
	}

	resp, err := h.ListPlugins(ctx, &eveyv1.ListPluginsRequest{})
	require.NoError(t, err)
	sig := find(resp, "plg-1").Signature
	require.NotNil(t, sig)
	assert.True(t, sig.Verified)
	assert.Equal(t, fp, sig.KeyFingerprint)
	assert.False(t, sig.Trusted)
	assert.Empty(t, sig.Publisher)
	assert.Equal(t, "same", sig.Continuity)
	assert.Nil(t, find(resp, "plg-2").Signature, "no verdict stays null")

	_, err = h.AddTrustedPublisher(ctx, &eveyv1.AddTrustedPublisherRequest{Name: "Acme", Fingerprint: fp})
	require.NoError(t, err)
	resp, err = h.ListPlugins(ctx, &eveyv1.ListPluginsRequest{})
	require.NoError(t, err)
	sig = find(resp, "plg-1").Signature
	assert.True(t, sig.Trusted)
	assert.Equal(t, "Acme", sig.Publisher)

	stored, _ := plugins.GetByID(ctx, "plg-1")
	assert.False(t, stored.Signature.Trusted, "the stored install-time verdict is untouched")
}

func TestSignatureToProto(t *testing.T) {
	assert.Nil(t, signatureToProto(nil))
	got := signatureToProto(&plugindomain.SignatureStatus{
		Verified: false, KeyFingerprint: "sha256:aa", Publisher: "P", Trusted: true, Continuity: "changed", Error: "boom", VerifiedBy: "ignored",
	})
	assert.Equal(t, &eveyv1.PluginSignature{KeyFingerprint: "sha256:aa", Publisher: "P", Trusted: true, Continuity: "changed", Error: "boom"}, got)
}

func TestApplyPluginUpdate_PublisherChangeMapsToFailedPrecondition(t *testing.T) {
	assert.Equal(t, codes.FailedPrecondition, status.Code(installPluginError(context.Background(), appplugin.ErrPublisherChanged)))
	assert.Equal(t, codes.FailedPrecondition, status.Code(installPluginError(context.Background(), appplugin.ErrSignatureLost)))
}

// The three RPCs are admin-only and, on the demo, only the read stays open: a
// guest administrator must not decide whose code the shared instance trusts.
func TestTrustedPublishers_DemoClassification(t *testing.T) {
	list := "/evey.v1.AdminPluginService/ListTrustedPublishers"
	add := "/evey.v1.AdminPluginService/AddTrustedPublisher"
	remove := "/evey.v1.AdminPluginService/RemoveTrustedPublisher"

	assert.Contains(t, demo.OpenAdminRPCs, list)
	assert.NotContains(t, demo.LockedAdminRPCs, list)
	for _, m := range []string{add, remove} {
		assert.Contains(t, demo.LockedAdminRPCs, m)
		assert.NotContains(t, demo.OpenAdminRPCs, m)
		assert.NotContains(t, demo.ExtendedStaysLockedRPCs, m, "the extended demo lets a guest admin trust a key")
	}
}

type fixedVerifier struct{ status plugindomain.SignatureStatus }

func (v fixedVerifier) VerifySignature(context.Context, string, string) (*plugindomain.SignatureStatus, error) {
	st := v.status
	return &st, nil
}

// PreviewPluginUpdate maps the verdict string and the full signature, and the
// publisher-changed verdict forces the acknowledgement; ApplyPluginUpdate
// refuses it until accept_new_publisher is set.
func TestPluginUpdate_PublisherChangeRoundTrip(t *testing.T) {
	ctx := context.Background()
	plugins := pluginfakes.NewFakePluginRepository()
	parser := pluginfakes.NewFakeManifestParser()
	runner := pluginfakes.NewFakePluginRunner()
	bus := sharedfakes.NewFakeEventBus()
	svc := appplugin.NewInstallerService(plugins, runner, parser, bus,
		appplugin.InstallerWithInstaller(pluginfakes.NewFakePluginInstaller()),
		appplugin.InstallerWithAnalyzer(pluginfakes.NewFakePluginAnalyzer()),
		appplugin.InstallerWithPluginDir(t.TempDir()),
		appplugin.InstallerWithFileSystem(pluginfakes.NewFakePluginFileSystem()),
		appplugin.InstallerWithSignatureVerifier(fixedVerifier{plugindomain.SignatureStatus{
			Verified: true, KeyFingerprint: "sha256:new", VerifiedBy: "sha256:new",
		}}))
	h := &adminPluginHandler{pluginInstaller: svc, enabled: true}

	require.NoError(t, plugins.Create(ctx, &plugindomain.Plugin{
		ID: "plg-1", Name: "signed", Version: "1.0.0", Kind: plugindomain.KindPlugin, Language: "go",
		PublisherKeyFingerprint: "sha256:old",
	}))
	parser.Result = &plugindomain.PluginManifest{Name: "signed", Kind: plugindomain.KindPlugin, Version: "1.1.0", Language: "go", Entrypoint: "./plugin"}
	zip := tmpZipGRPC(t, "v2")

	preview, err := h.PreviewPluginUpdate(ctx, &eveyv1.PreviewPluginUpdateRequest{PluginId: "plg-1", Source: zip})
	require.NoError(t, err)
	assert.Equal(t, "publisher-changed", preview.SignatureStatus)
	assert.True(t, preview.RequiresAck)
	require.NotNil(t, preview.Signature)
	assert.Equal(t, "sha256:new", preview.Signature.KeyFingerprint)
	assert.Equal(t, "changed", preview.Signature.Continuity)
	assert.True(t, preview.Signature.Verified)

	req := &eveyv1.ApplyPluginUpdateRequest{PluginId: "plg-1", Source: zip, AcknowledgedHash: preview.SourceHash}
	_, err = h.ApplyPluginUpdate(ctx, req)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))

	req.AcceptNewPublisher = true
	applied, err := h.ApplyPluginUpdate(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", applied.Version)
	require.NotNil(t, applied.Signature)
	assert.Equal(t, "changed", applied.Signature.Continuity)
	stored, _ := plugins.GetByID(ctx, "plg-1")
	assert.Equal(t, "sha256:new", stored.PublisherKeyFingerprint)
}
