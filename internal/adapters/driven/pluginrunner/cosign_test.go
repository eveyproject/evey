package pluginrunner

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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainplugin "github.com/eveyproject/evey/internal/domain/plugin"
)

// cosignFixtureDir resolves the repo-root tests/plugins/cosign directory from
// this package's dir (go test runs with CWD = package dir). Skips the test if
// the fixtures are absent (e.g. a sparse checkout).
func cosignFixtureDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "..", "tests", "plugins", "cosign")
	if _, err := os.Stat(filepath.Join(dir, "test.pub")); err != nil {
		t.Skipf("cosign fixtures not found at %s: %v", dir, err)
	}
	return dir
}

// fixtureFingerprint is the fingerprint of the committed test.pub, computed by
// the same domain helper the verifier uses.
func fixtureFingerprint(t *testing.T, dir string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "test.pub"))
	require.NoError(t, err)
	fp, _, err := domainplugin.ParsePublisherKey(raw)
	require.NoError(t, err)
	return fp
}

func lookupOf(trusted map[string]string) domainplugin.KeyLookup {
	return func(_ context.Context, fp string) (string, bool) {
		name, ok := trusted[fp]
		return name, ok
	}
}

// testKey is a throwaway P-256 key pair with its PEM and fingerprint.
type testKey struct {
	priv        *ecdsa.PrivateKey
	pem         string
	fingerprint string
}

func newTestKey(t *testing.T) testKey {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	pemStr := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	fp, _, err := domainplugin.ParsePublisherKey([]byte(pemStr))
	require.NoError(t, err)
	return testKey{priv: priv, pem: pemStr, fingerprint: fp}
}

// writeZip builds a ZIP from name -> content and returns its path.
func writeZip(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "p.zip")
	f, err := os.Create(path)
	require.NoError(t, err)
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())
	return path
}

func signZip(t *testing.T, k testKey, zipPath string) string {
	t.Helper()
	data, err := os.ReadFile(zipPath)
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	sig, err := ecdsa.SignASN1(rand.Reader, k.priv, sum[:])
	require.NoError(t, err)
	sigPath := zipPath + ".sig"
	require.NoError(t, os.WriteFile(sigPath, []byte(base64.StdEncoding.EncodeToString(sig)), 0o600))
	return sigPath
}

func TestCosignVerifier_PolicyOff_IsNoop(t *testing.T) {
	st, err := NewCosignVerifier("off", nil).VerifySignature(context.Background(), "/x.zip", "")
	require.NoError(t, err)
	assert.Nil(t, st, "policy=off must skip verification entirely")
}

func TestCosignVerifier_EmbeddedKey_Valid_Trusted(t *testing.T) {
	dir := cosignFixtureDir(t)
	fp := fixtureFingerprint(t, dir)
	for _, policy := range []string{"warn", "enforce"} {
		v := NewCosignVerifier(policy, lookupOf(map[string]string{fp: "Fixture Publisher"}))
		st, err := v.VerifySignature(context.Background(), filepath.Join(dir, "plugin-valid.zip"), "")
		require.NoError(t, err, policy)
		require.NotNil(t, st)
		assert.True(t, st.Verified)
		assert.True(t, st.Trusted)
		assert.Equal(t, "Fixture Publisher", st.Publisher)
		assert.Equal(t, fp, st.KeyFingerprint)
		assert.Equal(t, fp, st.VerifiedBy)
		assert.Equal(t, domainplugin.SigStateSigned, st.State())
		assert.Contains(t, st.KeyPEM, "BEGIN PUBLIC KEY")
	}
}

func TestCosignVerifier_FingerprintFormat(t *testing.T) {
	dir := cosignFixtureDir(t)
	st, err := NewCosignVerifier("warn", nil).VerifySignature(context.Background(), filepath.Join(dir, "plugin-valid.zip"), "")
	require.NoError(t, err)
	assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, st.KeyFingerprint)
}

// A valid signature under a key the instance does not list: warn records it,
// enforce blocks it.
func TestCosignVerifier_ValidButUntrusted(t *testing.T) {
	dir := cosignFixtureDir(t)
	zipPath := filepath.Join(dir, "plugin-valid.zip")
	st, err := NewCosignVerifier("warn", lookupOf(nil)).VerifySignature(context.Background(), zipPath, "")
	require.NoError(t, err)
	assert.True(t, st.Verified)
	assert.False(t, st.Trusted)
	assert.Empty(t, st.Publisher)
	assert.Equal(t, domainplugin.SigStateSignedUntrusted, st.State())

	st, err = NewCosignVerifier("enforce", lookupOf(nil)).VerifySignature(context.Background(), zipPath, "")
	require.Error(t, err)
	require.NotNil(t, st)
	assert.True(t, st.Verified)
}

func TestCosignVerifier_Enforce_NilLookup_TrustsNothing(t *testing.T) {
	dir := cosignFixtureDir(t)
	_, err := NewCosignVerifier("enforce", nil).VerifySignature(context.Background(), filepath.Join(dir, "plugin-valid.zip"), "")
	require.Error(t, err)
}

func TestCosignVerifier_MissingKey_Unsigned(t *testing.T) {
	k := newTestKey(t)
	zipPath := writeZip(t, map[string]string{"plugin.yaml": "name: x\n"})
	signZip(t, k, zipPath) // a .sig without a key in the package is still unsigned

	st, err := NewCosignVerifier("warn", nil).VerifySignature(context.Background(), zipPath, "")
	require.NoError(t, err)
	require.NotNil(t, st)
	assert.False(t, st.Verified)
	assert.Empty(t, st.KeyFingerprint)
	assert.Equal(t, domainplugin.SigErrNoKey, st.Error)
	assert.Equal(t, domainplugin.SigStateUnsigned, st.State())

	_, err = NewCosignVerifier("enforce", nil).VerifySignature(context.Background(), zipPath, "")
	require.Error(t, err)
}

func TestCosignVerifier_NestedKeyIgnored(t *testing.T) {
	k := newTestKey(t)
	zipPath := writeZip(t, map[string]string{"plugin.yaml": "name: x\n", "sub/cosign.pub": k.pem})
	signZip(t, k, zipPath)
	st, err := NewCosignVerifier("warn", nil).VerifySignature(context.Background(), zipPath, "")
	require.NoError(t, err)
	assert.Equal(t, domainplugin.SigErrNoKey, st.Error)
}

func TestCosignVerifier_MissingSignature_Unsigned(t *testing.T) {
	k := newTestKey(t)
	zipPath := writeZip(t, map[string]string{"plugin.yaml": "name: x\n", "cosign.pub": k.pem})

	st, err := NewCosignVerifier("warn", nil).VerifySignature(context.Background(), zipPath, "")
	require.NoError(t, err)
	assert.False(t, st.Verified)
	assert.Equal(t, k.fingerprint, st.KeyFingerprint, "the key is reported even without a signature")
	assert.Equal(t, domainplugin.SigErrNoSignature, st.Error)
	assert.Equal(t, domainplugin.SigStateUnsigned, st.State())

	_, err = NewCosignVerifier("enforce", lookupOf(map[string]string{k.fingerprint: "k"})).VerifySignature(context.Background(), zipPath, "")
	require.Error(t, err)

	_, err = NewCosignVerifier("enforce", nil).VerifySignature(context.Background(), zipPath, filepath.Join(t.TempDir(), "nope.sig"))
	require.Error(t, err)
}

// A signature made by another key than the one the package carries is invalid,
// and a trusted name must not attach to it.
func TestCosignVerifier_WrongKey_Invalid(t *testing.T) {
	dir := cosignFixtureDir(t)
	fp := fixtureFingerprint(t, dir)
	zipPath := filepath.Join(dir, "plugin-wrongkey.zip")

	st, err := NewCosignVerifier("warn", lookupOf(map[string]string{fp: "Fixture Publisher"})).VerifySignature(context.Background(), zipPath, "")
	require.NoError(t, err, "warn must never block")
	assert.False(t, st.Verified)
	assert.False(t, st.Trusted)
	assert.Empty(t, st.Publisher)
	assert.NotEmpty(t, st.Error)
	assert.Equal(t, domainplugin.SigStateInvalid, st.State())

	_, err = NewCosignVerifier("enforce", lookupOf(map[string]string{fp: "Fixture Publisher"})).VerifySignature(context.Background(), zipPath, "")
	require.Error(t, err)
}

func TestCosignVerifier_TamperedZip_Invalid(t *testing.T) {
	dir := cosignFixtureDir(t)
	fp := fixtureFingerprint(t, dir)
	zipPath := filepath.Join(dir, "plugin-tampered.zip")

	st, err := NewCosignVerifier("warn", lookupOf(map[string]string{fp: "x"})).VerifySignature(context.Background(), zipPath, "")
	require.NoError(t, err)
	assert.False(t, st.Verified)
	assert.Equal(t, fp, st.KeyFingerprint)
	assert.Equal(t, domainplugin.SigStateInvalid, st.State())

	_, err = NewCosignVerifier("enforce", lookupOf(map[string]string{fp: "x"})).VerifySignature(context.Background(), zipPath, "")
	require.Error(t, err, "a ZIP modified after signing must be rejected")
}

func TestCosignVerifier_UnsupportedKey_Invalid(t *testing.T) {
	rsaLike := "-----BEGIN PUBLIC KEY-----\nMFwwDQYJKoZIhvcNAQEBBQADSwAwSAJBAMxN0ExampleNotARealKeyxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n-----END PUBLIC KEY-----\n"
	zipPath := writeZip(t, map[string]string{"plugin.yaml": "name: x\n", "cosign.pub": rsaLike})
	st, err := NewCosignVerifier("warn", nil).VerifySignature(context.Background(), zipPath, "")
	require.NoError(t, err)
	assert.False(t, st.Verified)
	assert.Empty(t, st.KeyFingerprint)
	assert.Equal(t, domainplugin.SigStateInvalid, st.State())
	_, err = NewCosignVerifier("enforce", nil).VerifySignature(context.Background(), zipPath, "")
	require.Error(t, err)
}

func TestCosignVerifier_OversizedKey_Invalid(t *testing.T) {
	big := make([]byte, 5000)
	for i := range big {
		big[i] = 'A'
	}
	zipPath := writeZip(t, map[string]string{"plugin.yaml": "name: x\n", "cosign.pub": string(big)})
	st, err := NewCosignVerifier("warn", nil).VerifySignature(context.Background(), zipPath, "")
	require.NoError(t, err)
	assert.False(t, st.Verified)
	assert.Contains(t, st.Error, "4 KiB")
}

func TestCosignVerifier_ExplicitSigPath(t *testing.T) {
	k := newTestKey(t)
	zipPath := writeZip(t, map[string]string{"plugin.yaml": "name: x\n", "cosign.pub": k.pem})
	sigPath := signZip(t, k, zipPath)
	moved := filepath.Join(t.TempDir(), "elsewhere.sig")
	require.NoError(t, os.Rename(sigPath, moved))
	st, err := NewCosignVerifier("warn", lookupOf(map[string]string{k.fingerprint: "K"})).VerifySignature(context.Background(), zipPath, moved)
	require.NoError(t, err)
	assert.True(t, st.Verified)
	assert.Equal(t, "K", st.Publisher)
}

func TestCosignVerifier_NotAZip_Invalid(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.zip")
	require.NoError(t, os.WriteFile(p, []byte("not a zip"), 0o600))
	st, err := NewCosignVerifier("warn", nil).VerifySignature(context.Background(), p, "")
	require.NoError(t, err)
	assert.False(t, st.Verified)
	assert.Equal(t, domainplugin.SigStateInvalid, st.State())
}
