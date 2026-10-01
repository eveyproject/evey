package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A plugin owns its install dir (Landlock -rw, symlinks included) and runs
// under the server uid: the asset route must never follow a link it planted.
func getPluginAsset(t *testing.T, mux *http.ServeMux, token, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/v1/plugins/p1/assets/"+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { mux.ServeHTTP(w, req); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("GET %s blocked (FIFO opened for reading?)", path)
	}
	return w
}

func TestPluginAsset_SymlinkToOutsideFileRefused(t *testing.T) {
	mux, token, _, dir := setupPluginAssetTest(t)
	secret := filepath.Join(t.TempDir(), "jwt.key")
	require.NoError(t, os.WriteFile(secret, []byte("TOP-SECRET-KEY"), 0o600))
	require.NoError(t, os.Symlink(secret, filepath.Join(dir, "p1", "dist", "leak.js")))

	w := getPluginAsset(t, mux, token, "leak.js")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.NotContains(t, w.Body.String(), "TOP-SECRET-KEY")
}

func TestPluginAsset_SymlinkInsidePluginRefused(t *testing.T) {
	mux, token, _, dir := setupPluginAssetTest(t)
	require.NoError(t, os.Symlink("index.js", filepath.Join(dir, "p1", "dist", "alias.js")))

	w := getPluginAsset(t, mux, token, "alias.js")
	assert.Equal(t, http.StatusNotFound, w.Code, "no link is followed, even one staying inside the plugin")
}

func TestPluginAsset_IntermediateSymlinkDirRefused(t *testing.T) {
	mux, token, _, dir := setupPluginAssetTest(t)
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "x.js"), []byte("OUTSIDE-BYTES"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "p1", "dist", "sub")))

	w := getPluginAsset(t, mux, token, "sub/x.js")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.NotContains(t, w.Body.String(), "OUTSIDE-BYTES")
}

func TestPluginAsset_WebRootSymlinkRefused(t *testing.T) {
	mux, token, _, dir := setupPluginAssetTest(t)
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "index.js"), []byte("OUTSIDE-BYTES"), 0o644))
	dist := filepath.Join(dir, "p1", "dist")
	require.NoError(t, os.RemoveAll(dist))
	require.NoError(t, os.Symlink(outside, dist))

	w := getPluginAsset(t, mux, token, "index.js")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.NotContains(t, w.Body.String(), "OUTSIDE-BYTES")
}

func TestPluginAsset_FIFORefusedWithoutBlocking(t *testing.T) {
	mux, token, _, dir := setupPluginAssetTest(t)
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "p1", "dist", "pipe.js"), 0o644))

	w := getPluginAsset(t, mux, token, "pipe.js")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestPluginAsset_RegularNestedFileServed(t *testing.T) {
	mux, token, _, dir := setupPluginAssetTest(t)
	sub := filepath.Join(dir, "p1", "dist", "assets")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "chunk.js"), []byte("export const x=1"), 0o644))

	w := getPluginAsset(t, mux, token, "assets/chunk.js")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "export const x=1", w.Body.String())
	assert.NotEmpty(t, w.Header().Get("ETag"))
}

// The themes list parses theme.css for mobile tokens: a linked theme.css
// must not leak another file's contents into the JSON.
func TestListThemes_SymlinkedThemeCSSIgnored(t *testing.T) {
	mux, token, repo, dir := setupPluginAssetTest(t)
	require.NoError(t, repo.Create(context.Background(), &plugin.Plugin{
		ID: "t1", Name: "theme", Kind: plugin.KindTheme, Enabled: true,
	}))
	secret := filepath.Join(t.TempDir(), "secret.css")
	require.NoError(t, os.WriteFile(secret, []byte(":root{--leak: TOP-SECRET}"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "t1"), 0o755))
	require.NoError(t, os.Symlink(secret, filepath.Join(dir, "t1", "theme.css")))

	req := httptest.NewRequest("GET", "/api/v1/plugins/themes", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "TOP-SECRET")
}
