package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/pluginrunner"
	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	"github.com/eveyproject/evey/internal/domain/plugin"
	pluginfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/auth"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
)

type sigKey struct {
	priv *ecdsa.PrivateKey
	pem  string
	fp   string
}

func newSigKey(t *testing.T) sigKey {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	p := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	fp, _, err := plugin.ParsePublisherKey([]byte(p))
	require.NoError(t, err)
	return sigKey{priv: priv, pem: p, fp: fp}
}

// signedPackage returns the ZIP bytes (carrying k's public key) and the
// detached signature over them.
func signedPackage(t *testing.T, k sigKey, body string) (zipBytes, sig []byte) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range map[string]string{"main.py": body, "cosign.pub": k.pem} {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	sum := sha256.Sum256(buf.Bytes())
	raw, err := ecdsa.SignASN1(rand.Reader, k.priv, sum[:])
	require.NoError(t, err)
	return buf.Bytes(), []byte(base64.StdEncoding.EncodeToString(raw))
}

type sigRoutes struct {
	mux    *http.ServeMux
	token  string
	parser *pluginfakes.FakeManifestParser
	keys   *appplugin.TrustedPublisherService
}

func newSigRoutes(t *testing.T, policy string) *sigRoutes {
	t.Helper()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	gen := auth.NewJWTGenerator(rsaKey, time.Minute)
	token, err := gen.GenerateAccessToken(domainuser.Claims{UserID: "a1", IsAdmin: true})
	require.NoError(t, err)

	repo := pluginfakes.NewFakePluginRepository()
	parser := pluginfakes.NewFakeManifestParser()
	keys := appplugin.NewTrustedPublisherService(pluginfakes.NewFakeTrustedPublisherRepository(), repo)
	svc := appplugin.NewInstallerService(repo, pluginfakes.NewFakePluginRunner(), parser, sharedfakes.NewFakeEventBus(),
		appplugin.InstallerWithInstaller(pluginfakes.NewFakePluginInstaller()),
		appplugin.InstallerWithAnalyzer(pluginfakes.NewFakePluginAnalyzer()),
		appplugin.InstallerWithPluginDir(t.TempDir()),
		appplugin.InstallerWithFileSystem(pluginfakes.NewFakePluginFileSystem()),
		appplugin.InstallerWithSignatureVerifier(pluginrunner.NewCosignVerifier(policy, keys.Lookup)),
	)
	mux := http.NewServeMux()
	registerPluginUploadRoute(mux, gen, svc, ratelimit.New(100))
	registerPluginPreviewRoute(mux, gen, svc, ratelimit.New(100))
	return &sigRoutes{mux: mux, token: token, parser: parser, keys: keys}
}

func (r *sigRoutes) post(t *testing.T, path string, zipBytes, sig []byte, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("plugin", "p.zip")
	require.NoError(t, err)
	_, _ = part.Write(zipBytes)
	if sig != nil {
		sp, err := mw.CreateFormFile("sig", "p.zip.sig")
		require.NoError(t, err)
		_, _ = sp.Write(sig)
	}
	for k, v := range fields {
		require.NoError(t, mw.WriteField(k, v))
	}
	require.NoError(t, mw.Close())
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+r.token)
	w := httptest.NewRecorder()
	r.mux.ServeHTTP(w, req)
	return w
}

func (r *sigRoutes) manifest(version string) {
	r.parser.Result = &plugin.PluginManifest{Name: "signed", Kind: plugin.KindPlugin, Version: version, Language: "go", Entrypoint: "./plugin"}
}

// The install preview carries the signature verdict when the "sig" form field
// was uploaded beside the package; the key it trusts shows its publisher name.
func TestPluginPreview_UploadedSignatureVerdict(t *testing.T) {
	r := newSigRoutes(t, "warn")
	k := newSigKey(t)
	r.manifest("1.0.0")
	zipBytes, sig := signedPackage(t, k, "v1")

	type previewBody struct {
		Signature *struct {
			SignatureStatus string `json:"signature_status"`
			Verified        bool   `json:"verified"`
			KeyFingerprint  string `json:"key_fingerprint"`
			Publisher       string `json:"publisher"`
			Trusted         bool   `json:"trusted"`
			Continuity      string `json:"continuity"`
			Error           string `json:"error"`
		} `json:"signature"`
	}
	decode := func(w *httptest.ResponseRecorder) previewBody {
		t.Helper()
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var b previewBody
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &b))
		return b
	}

	b := decode(r.post(t, "/api/v1/admin/plugins/preview", zipBytes, sig, nil))
	require.NotNil(t, b.Signature)
	assert.Equal(t, "signed-untrusted", b.Signature.SignatureStatus)
	assert.True(t, b.Signature.Verified)
	assert.Equal(t, k.fp, b.Signature.KeyFingerprint)
	assert.False(t, b.Signature.Trusted)

	_, err := r.keys.AddByPEM(context.Background(), "Acme", k.pem)
	require.NoError(t, err)
	b = decode(r.post(t, "/api/v1/admin/plugins/preview", zipBytes, sig, nil))
	assert.Equal(t, "signed", b.Signature.SignatureStatus)
	assert.Equal(t, "Acme", b.Signature.Publisher)
	assert.True(t, b.Signature.Trusted)

	b = decode(r.post(t, "/api/v1/admin/plugins/preview", zipBytes, nil, nil))
	assert.Equal(t, "unsigned", b.Signature.SignatureStatus)
	assert.False(t, b.Signature.Verified)
	assert.Equal(t, "no signature", b.Signature.Error)
}

func TestPluginPreview_PolicyOffHasNoSignatureObject(t *testing.T) {
	r := newSigRoutes(t, "off")
	k := newSigKey(t)
	r.manifest("1.0.0")
	zipBytes, sig := signedPackage(t, k, "v1")
	w := r.post(t, "/api/v1/admin/plugins/preview", zipBytes, sig, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	_, has := raw["signature"]
	assert.False(t, has)
}

// An upload that replaces an installed plugin with one signed by another key
// is answered 428 with the continuity; accept_new_publisher=true lets it through.
func TestPluginUpload_PublisherChangeNeedsAcceptance(t *testing.T) {
	r := newSigRoutes(t, "warn")
	k1, k2 := newSigKey(t), newSigKey(t)

	r.manifest("1.0.0")
	zip1, sig1 := signedPackage(t, k1, "v1")
	w := r.post(t, "/api/v1/admin/plugins/upload", zip1, sig1, nil)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	r.manifest("1.1.0")
	zip2, sig2 := signedPackage(t, k2, "v2")
	w = r.post(t, "/api/v1/admin/plugins/upload", zip2, sig2, nil)
	require.Equal(t, http.StatusPreconditionRequired, w.Code, w.Body.String())
	var body struct {
		PublisherChanged bool   `json:"publisher_changed"`
		Continuity       string `json:"continuity"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.True(t, body.PublisherChanged)
	assert.Equal(t, "changed", body.Continuity)

	w = r.post(t, "/api/v1/admin/plugins/upload", zip2, sig2, map[string]string{"accept_new_publisher": "true"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	// Signed -> unsigned is "lost" until accepted too.
	r.manifest("1.2.0")
	zip3, _ := signedPackage(t, k2, "v3")
	w = r.post(t, "/api/v1/admin/plugins/upload", zip3, nil, nil)
	require.Equal(t, http.StatusPreconditionRequired, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "lost", body.Continuity)
}
