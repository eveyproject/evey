package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	metafakes "github.com/gravity-zero/evey/internal/domain/metadata/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
)

func newMetadataTestMux(t *testing.T) (*http.ServeMux, string, string) {
	t.Helper()
	tokenGen := newTestJWTGen(t)

	overrides := metafakes.NewFakeMetadataOverrideRepository()
	artwork := metafakes.NewFakeArtworkRepository()
	events := sharedfakes.NewFakeEventBus()
	metaCmd := appmetadata.NewCommandService(overrides, artwork, metafakes.NewFakeArtworkDownloader(), events, nil)
	metaQuery := appmetadata.NewQueryService(overrides, artwork, nil)

	userTok, err := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})
	if err != nil {
		t.Fatalf("generate user token: %v", err)
	}
	adminTok, err := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "admin1", IsAdmin: true})
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	mux := http.NewServeMux()
	registerMetadataOverrideRoutes(mux, metaCmd, metaQuery, tokenGen)
	return mux, userTok, adminTok
}

func TestMetadataRequiresAdmin(t *testing.T) {
	tv := validUserValidator(false)
	handler := requireAdmin(tv, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("PUT", "/api/v1/media/m1/metadata", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin, got %d", w.Code)
	}
}

func TestPutMetadata_NoAuth_401(t *testing.T) {
	mux, _, _ := newMetadataTestMux(t)
	body := bytes.NewBufferString(`{"title":"New Title"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/media/m1/metadata", body)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPutMetadata_NonAdmin_401(t *testing.T) {
	mux, userTok, _ := newMetadataTestMux(t)
	body := bytes.NewBufferString(`{"title":"New Title"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/media/m1/metadata", body)
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for non-admin, got %d", w.Code)
	}
}

func TestPutMetadata_Admin_200(t *testing.T) {
	mux, _, adminTok := newMetadataTestMux(t)
	body := bytes.NewBufferString(`{"title":"New Title"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/media/m1/metadata", body)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestDeleteMetadata_NoAuth_401(t *testing.T) {
	mux, _, _ := newMetadataTestMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/m1/metadata", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestDeleteMetadata_Admin_200(t *testing.T) {
	mux, _, adminTok := newMetadataTestMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/m1/metadata", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestGetMetadataOverrides_NoAuth_401(t *testing.T) {
	mux, _, _ := newMetadataTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/metadata/overrides", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestGetMetadataOverrides_Admin_200(t *testing.T) {
	mux, _, adminTok := newMetadataTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/metadata/overrides", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
