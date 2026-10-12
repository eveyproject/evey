package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/eveyproject/evey/internal/pkg/i18n"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupI18nDir(t *testing.T) *i18n.Translator {
	t.Helper()
	dir := t.TempDir()
	enDir := filepath.Join(dir, "en")
	require.NoError(t, os.Mkdir(enDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(enDir, "common.yaml"), []byte(`
save: Save
cancel: Cancel
`), 0o644))
	frDir := filepath.Join(dir, "fr")
	require.NoError(t, os.Mkdir(frDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(frDir, "common.yaml"), []byte(`
save: Enregistrer
cancel: Annuler
`), 0o644))
	tr, err := i18n.NewFromDir(dir)
	require.NoError(t, err)
	return tr
}

func TestI18nHandler_OK(t *testing.T) {
	tr := setupI18nDir(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/i18n/{locale}", i18nHandler(tr))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/i18n/en", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Header().Get("Cache-Control"), "max-age=3600")
	assert.NotEmpty(t, rec.Header().Get("ETag"))

	var msgs map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &msgs))
	assert.Equal(t, "Save", msgs["common.save"])
}

func TestI18nHandler_French(t *testing.T) {
	tr := setupI18nDir(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/i18n/{locale}", i18nHandler(tr))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/i18n/fr", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var msgs map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &msgs))
	assert.Equal(t, "Enregistrer", msgs["common.save"])
}

func TestI18nHandler_ETag304(t *testing.T) {
	tr := setupI18nDir(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/i18n/{locale}", i18nHandler(tr))

	// First request to get ETag
	req := httptest.NewRequest(http.MethodGet, "/api/v1/i18n/en", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	etag := rec.Header().Get("ETag")
	require.NotEmpty(t, etag)

	// Second request with If-None-Match
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/i18n/en", nil)
	req2.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)

	assert.Equal(t, http.StatusNotModified, rec2.Code)
	assert.Empty(t, rec2.Body.Bytes())
}

func TestI18nHandler_DevMode_NoCache(t *testing.T) {
	tr := setupI18nDir(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/i18n/{locale}", i18nHandler(tr, true))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/i18n/en", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.Empty(t, rec.Header().Get("ETag"), "dev mode must not set ETag")

	var msgs map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &msgs))
	assert.Equal(t, "Save", msgs["common.save"])
}

func TestI18nHandler_ProdMode_CacheAndETag(t *testing.T) {
	tr := setupI18nDir(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/i18n/{locale}", i18nHandler(tr, false))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/i18n/en", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Cache-Control"), "max-age=3600")
	assert.NotEmpty(t, rec.Header().Get("ETag"), "prod mode must set ETag")
}

func TestI18nHandler_DevMode_NoETag304(t *testing.T) {
	tr := setupI18nDir(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/i18n/{locale}", i18nHandler(tr, true))

	// Even if client sends If-None-Match, dev mode ignores it (no ETag comparison)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/i18n/en", nil)
	req.Header.Set("If-None-Match", `"anything"`)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "dev mode should always return 200")
}
