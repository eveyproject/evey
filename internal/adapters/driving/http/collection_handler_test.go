package httpapi

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appcollection "github.com/gravity-zero/evey/internal/app/collection"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func newCollectionTestMux(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})

	cmdRepo := libfakes.NewFakeCollectionCommandRepo()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	queryRepo := libfakes.NewFakeCollectionQueryRepo(cmdRepo, mediaRepo)
	smartQ := libfakes.NewFakeCollectionSmartQuerier()
	svc := appcollection.NewService(cmdRepo, queryRepo, smartQ, mediaRepo)

	mux := http.NewServeMux()
	registerCollectionRoutes(mux, svc, gen)
	return mux, token
}

func newCollectionTestMuxWithAdmin(t *testing.T) (*http.ServeMux, string, string) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	userToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})
	adminToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "a1", IsAdmin: true})

	cmdRepo := libfakes.NewFakeCollectionCommandRepo()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	queryRepo := libfakes.NewFakeCollectionQueryRepo(cmdRepo, mediaRepo)
	smartQ := libfakes.NewFakeCollectionSmartQuerier()
	svc := appcollection.NewService(cmdRepo, queryRepo, smartQ, mediaRepo)

	mux := http.NewServeMux()
	registerCollectionRoutes(mux, svc, gen)
	return mux, userToken, adminToken
}

func TestCollectionRoutes_Unauthenticated(t *testing.T) {
	tv := invalidValidator()
	handler := requireAdmin(tv, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/api/v1/collections", nil)
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without token, got %d", w.Code)
	}
}

func TestListCollections_NoAuth_401(t *testing.T) {
	mux, _ := newCollectionTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/collections", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestListCollections_Auth_200(t *testing.T) {
	mux, token := newCollectionTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/collections", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestGetCollectionItems_NoAuth_401(t *testing.T) {
	mux, _ := newCollectionTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/collections/col-1/items", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPostCollection_Admin_201(t *testing.T) {
	mux, _, adminToken := newCollectionTestMuxWithAdmin(t)
	body := `{"name":"Action Movies","rules":[{"field":"genre","operator":"eq","value":"Action"}],"sortBy":"year"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/collections", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["name"] != "Action Movies" {
		t.Errorf("expected name Action Movies, got %v", resp["name"])
	}
}

func TestPostCollection_NonAdmin_403(t *testing.T) {
	mux, userToken, _ := newCollectionTestMuxWithAdmin(t)
	body := `{"name":"Action Movies","rules":[{"field":"genre","operator":"eq","value":"Action"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/collections", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestPostCollection_NoAuth_403(t *testing.T) {
	mux, _, _ := newCollectionTestMuxWithAdmin(t)
	body := `{"name":"Action Movies","rules":[{"field":"genre","operator":"eq","value":"Action"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/collections", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestPostCollection_InvalidBody_400(t *testing.T) {
	mux, _, adminToken := newCollectionTestMuxWithAdmin(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/collections", strings.NewReader("not json"))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestPostCollection_EmptyName_400(t *testing.T) {
	mux, _, adminToken := newCollectionTestMuxWithAdmin(t)
	body := `{"name":"","rules":[{"field":"genre","operator":"eq","value":"Action"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/collections", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestPutCollection_Admin_200(t *testing.T) {
	mux, _, adminToken := newCollectionTestMuxWithAdmin(t)

	// create first
	createBody := `{"name":"Action Movies","rules":[{"field":"genre","operator":"eq","value":"Action"}],"sortBy":"year"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/collections", strings.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	cw := httptest.NewRecorder()
	mux.ServeHTTP(cw, createReq)
	if cw.Code != http.StatusCreated {
		t.Fatalf("create failed: %d %s", cw.Code, cw.Body.String())
	}
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	id := created["id"].(string)

	// update
	updateBody := `{"name":"Drama Movies","rules":[{"field":"genre","operator":"eq","value":"Drama"}],"sortBy":"rating"}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/collections/"+id, strings.NewReader(updateBody))
	updateReq.Header.Set("Authorization", "Bearer "+adminToken)
	updateReq.Header.Set("Content-Type", "application/json")
	uw := httptest.NewRecorder()
	mux.ServeHTTP(uw, updateReq)
	if uw.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", uw.Code, uw.Body.String())
	}
	var updated map[string]any
	json.NewDecoder(uw.Body).Decode(&updated)
	if updated["name"] != "Drama Movies" {
		t.Errorf("expected name Drama Movies, got %v", updated["name"])
	}
}

func TestPutCollection_NonAdmin_403(t *testing.T) {
	mux, userToken, adminToken := newCollectionTestMuxWithAdmin(t)

	// create as admin
	createBody := `{"name":"Action","rules":[{"field":"genre","operator":"eq","value":"Action"}]}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/collections", strings.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	cw := httptest.NewRecorder()
	mux.ServeHTTP(cw, createReq)
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	id := created["id"].(string)

	// update as non-admin
	updateBody := `{"name":"Drama","rules":[{"field":"genre","operator":"eq","value":"Drama"}]}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/collections/"+id, strings.NewReader(updateBody))
	updateReq.Header.Set("Authorization", "Bearer "+userToken)
	uw := httptest.NewRecorder()
	mux.ServeHTTP(uw, updateReq)
	if uw.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", uw.Code)
	}
}

func TestPutCollection_NotFound_404(t *testing.T) {
	mux, _, adminToken := newCollectionTestMuxWithAdmin(t)
	body := `{"name":"Drama","rules":[{"field":"genre","operator":"eq","value":"Drama"}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/collections/nonexistent", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
