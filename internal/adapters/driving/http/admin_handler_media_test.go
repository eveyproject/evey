package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appmetadata "github.com/eveyproject/evey/internal/app/metadata"
	"github.com/eveyproject/evey/internal/domain/metadata"
	metafakes "github.com/eveyproject/evey/internal/domain/metadata/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/auth"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
)

func newAdminMediaMux(t *testing.T) (mux *http.ServeMux, adminTok, userTok string, provider *metafakes.FakeMetadataProvider) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	adminTok, _ = gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true})
	userTok, _ = gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", IsAdmin: false})

	overrides := metafakes.NewFakeMetadataOverrideRepository()
	artwork := metafakes.NewFakeArtworkRepository()
	downloader := metafakes.NewFakeArtworkDownloader()
	bus := sharedfakes.NewFakeEventBus()
	provider = metafakes.NewFakeMetadataProvider("tmdb")
	providers := []metadata.MetadataProvider{provider}

	metaCmd := appmetadata.NewCommandService(overrides, artwork, downloader, bus, providers)
	metaQuery := appmetadata.NewQueryService(overrides, artwork, providers)

	heavyRL := ratelimit.New(1000)
	writeRL := ratelimit.New(1000)
	t.Cleanup(func() { heavyRL.Stop(); writeRL.Stop() })

	mux = http.NewServeMux()
	registerAdminRoutes(mux, metaCmd, metaQuery, gen, heavyRL, writeRL, context.Background(), nil)
	return mux, adminTok, userTok, provider
}

// --- GET /api/v1/admin/media/{id}/search-metadata ---

func TestAdminMedia_SearchMetadata_NoAuth_401(t *testing.T) {
	mux, _, _, _ := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/mi-1/search-metadata?query=foo&type=movie", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminMedia_SearchMetadata_NonAdmin_403(t *testing.T) {
	mux, _, userTok, _ := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/mi-1/search-metadata?query=foo&type=movie", nil)
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestAdminMedia_SearchMetadata_Admin_OK(t *testing.T) {
	mux, adminTok, _, provider := newAdminMediaMux(t)
	year := 2001
	provider.SearchResults = map[string][]metadata.MetadataResult{
		"hobbit": {{
			Title:       "The Hobbit",
			Year:        &year,
			Overview:    "Bilbo's journey",
			ProviderIDs: map[string]string{"tmdb": "42"},
			Artwork: []metadata.ArtworkResult{
				{Type: "poster", URL: "https://img/p.jpg"},
			},
		}},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/mi-1/search-metadata?query=hobbit&type=movie", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var out []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
	if out[0]["tmdb_id"] != "42" || out[0]["title"] != "The Hobbit" {
		t.Errorf("unexpected result: %+v", out[0])
	}
	if out[0]["poster_url"] != "https://img/p.jpg" {
		t.Errorf("missing poster_url: %+v", out[0])
	}
}

func TestAdminMedia_SearchMetadata_MissingParams_400(t *testing.T) {
	mux, adminTok, _, _ := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/mi-1/search-metadata?query=foo", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- POST /api/v1/admin/media/{id}/refresh-metadata ---

func TestAdminMedia_RefreshMetadata_NoAuth_401(t *testing.T) {
	mux, _, _, _ := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/mi-1/refresh-metadata", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminMedia_RefreshMetadata_NonAdmin_403(t *testing.T) {
	mux, _, userTok, _ := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/mi-1/refresh-metadata", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestAdminMedia_RefreshMetadata_BadJSON_400(t *testing.T) {
	mux, adminTok, _, _ := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/mi-1/refresh-metadata", strings.NewReader("not-json"))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAdminMedia_RefreshMetadata_NoEnricher_500(t *testing.T) {
	// Enricher not configured → service returns error → 500
	mux, adminTok, _, _ := newAdminMediaMux(t)
	body := `{"tmdb_id":"42","media_type":"movie"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/mi-1/refresh-metadata", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- unknown action → 404 ---

func TestAdminMedia_UnknownAction_404(t *testing.T) {
	mux, adminTok, _, _ := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/mi-1/nope", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAdminMedia_MissingAction_404(t *testing.T) {
	mux, adminTok, _, _ := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/justid", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- POST /api/v1/admin/media/{id}/reset-metadata ---

func TestAdminMedia_ResetMetadata_NoAuth_401(t *testing.T) {
	mux, _, _, _ := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/mi-1/reset-metadata", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// Without mediaItems repository configured, ResetItemMetadata returns an
// error → 500. This verifies auth + routing reach the service layer.
func TestAdminMedia_ResetMetadata_NoMediaRepo_500(t *testing.T) {
	mux, adminTok, _, _ := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/mi-1/reset-metadata", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 (no media repo), got %d body=%s", w.Code, w.Body.String())
	}
}

// Wrong-method on POST-only route: Go 1.22+ mux returns 405 Method Not Allowed.
func TestAdminMedia_RefreshMetadata_WrongMethod_405(t *testing.T) {
	mux, adminTok, _, _ := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/media/mi-1/refresh-metadata", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for wrong method, got %d", w.Code)
	}
}

// IMDB branch: refresh by imdb_id instead of tmdb_id should also hit the service path.
func TestAdminMedia_RefreshMetadata_IMDBBranch_500NoEnricher(t *testing.T) {
	mux, adminTok, _, _ := newAdminMediaMux(t)
	body := `{"imdb_id":"tt123","media_type":"movie"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/mi-1/refresh-metadata", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 (no enricher configured), got %d", w.Code)
	}
}

// --- POST /api/v1/admin/metadata/refresh-all ---

func TestAdminRefreshAll_NoAuth_401(t *testing.T) {
	mux, _, _, _ := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/metadata/refresh-all", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminRefreshAll_Admin_202(t *testing.T) {
	mux, adminTok, _, _ := newAdminMediaMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/metadata/refresh-all", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "refresh started") {
		t.Errorf("expected 'refresh started', got %q", w.Body.String())
	}
}

// RateLimit 429 on refresh-metadata when heavyRL is saturated.
func TestAdminMedia_RefreshMetadata_RateLimit_429(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	adminTok, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true})

	overrides := metafakes.NewFakeMetadataOverrideRepository()
	artwork := metafakes.NewFakeArtworkRepository()
	downloader := metafakes.NewFakeArtworkDownloader()
	bus := sharedfakes.NewFakeEventBus()
	provider := metafakes.NewFakeMetadataProvider("tmdb")
	providers := []metadata.MetadataProvider{provider}

	metaCmd := appmetadata.NewCommandService(overrides, artwork, downloader, bus, providers)
	metaQuery := appmetadata.NewQueryService(overrides, artwork, providers)

	heavyRL := ratelimit.New(1) // burst 1
	writeRL := ratelimit.New(1000)
	t.Cleanup(func() { heavyRL.Stop(); writeRL.Stop() })

	mux := http.NewServeMux()
	registerAdminRoutes(mux, metaCmd, metaQuery, gen, heavyRL, writeRL, context.Background(), nil)

	// First request consumes the single token.
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/mi-1/refresh-metadata", strings.NewReader(`{"tmdb_id":"1","media_type":"movie"}`))
		req.Header.Set("Authorization", "Bearer "+adminTok)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if i == 1 && w.Code != http.StatusTooManyRequests {
			t.Fatalf("second call should be 429, got %d", w.Code)
		}
	}
}
