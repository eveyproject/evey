package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// theme.css is public (login page): a theme that links theme.css or theme.js
// to another file must never get that file served.
func getThemeAsset(t *testing.T, mux *http.ServeMux, token, asset string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/v1/themes/dark/"+asset, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { mux.ServeHTTP(w, req); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("GET %s blocked (FIFO opened for reading?)", asset)
	}
	return w
}

func TestThemeAssets_SymlinkRefused(t *testing.T) {
	for _, asset := range []string{"theme.css", "theme.js"} {
		t.Run(asset, func(t *testing.T) {
			mux, token, _, dir := setupThemeAssetTest(t)
			secret := filepath.Join(t.TempDir(), "jwt.key")
			require.NoError(t, os.WriteFile(secret, []byte("TOP-SECRET-KEY"), 0o600))
			target := filepath.Join(dir, "dark", asset)
			require.NoError(t, os.Remove(target))
			require.NoError(t, os.Symlink(secret, target))

			w := getThemeAsset(t, mux, token, asset)
			assert.Equal(t, http.StatusNotFound, w.Code)
			assert.NotContains(t, w.Body.String(), "TOP-SECRET-KEY")
		})
	}
}

func TestThemeAssets_SymlinkedThemeDirRefused(t *testing.T) {
	mux, token, _, dir := setupThemeAssetTest(t)
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "theme.css"), []byte("OUTSIDE-BYTES"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "theme.js"), []byte("OUTSIDE-BYTES"), 0o644))
	require.NoError(t, os.RemoveAll(filepath.Join(dir, "dark")))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "dark")))

	for _, asset := range []string{"theme.css", "theme.js"} {
		w := getThemeAsset(t, mux, token, asset)
		assert.Equal(t, http.StatusNotFound, w.Code, asset)
		assert.NotContains(t, w.Body.String(), "OUTSIDE-BYTES", asset)
	}
}

func TestThemeAssets_FIFORefusedWithoutBlocking(t *testing.T) {
	for _, asset := range []string{"theme.css", "theme.js"} {
		t.Run(asset, func(t *testing.T) {
			mux, token, _, dir := setupThemeAssetTest(t)
			target := filepath.Join(dir, "dark", asset)
			require.NoError(t, os.Remove(target))
			require.NoError(t, syscall.Mkfifo(target, 0o644))

			assert.Equal(t, http.StatusNotFound, getThemeAsset(t, mux, token, asset).Code)
		})
	}
}
