package httpapi

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apprating "github.com/gravity-zero/evey/internal/app/rating"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	ratingfakes "github.com/gravity-zero/evey/internal/domain/rating/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func newStarRatingMux(t *testing.T) (*http.ServeMux, *auth.JWTGenerator, *ratingfakes.FakeRepository, *fakeMediaGetter) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tokenGen := auth.NewJWTGenerator(key, time.Minute)

	repo := ratingfakes.NewFakeRepository()
	svc := apprating.NewService(repo, sharedfakes.NewFakeEventBus())

	getter := newFakeMediaGetter()
	getter.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-1"}

	mux := http.NewServeMux()
	registerStarRatingRoutes(mux, svc, tokenGen, getter, allowAll())
	return mux, tokenGen, repo, getter
}

func makeToken(t *testing.T, gen *auth.JWTGenerator, isAdmin bool) string {
	t.Helper()
	tok, err := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", ProfileID: "prof-1", IsAdmin: isAdmin})
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return tok
}

func TestStarRating_POST_NoAuth_401(t *testing.T) {
	mux, _, _, _ := newStarRatingMux(t)
	body, _ := json.Marshal(map[string]int{"value": 4})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/rating", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestStarRating_POST_Auth_200(t *testing.T) {
	mux, gen, _, _ := newStarRatingMux(t)
	tok := makeToken(t, gen, false)
	body, _ := json.Marshal(map[string]int{"value": 3})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/rating", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestStarRating_POST_InvalidValue_400(t *testing.T) {
	mux, gen, _, _ := newStarRatingMux(t)
	tok := makeToken(t, gen, false)
	body, _ := json.Marshal(map[string]int{"value": 11})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/rating", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid value 11, got %d", w.Code)
	}
}

func TestStarRating_DELETE_NoAuth_401(t *testing.T) {
	mux, _, _, _ := newStarRatingMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/media-1/rating", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestStarRating_DELETE_Auth_204(t *testing.T) {
	mux, gen, _, _ := newStarRatingMux(t)
	tok := makeToken(t, gen, false)

	// POST first
	body, _ := json.Marshal(map[string]int{"value": 4})
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/rating", bytes.NewReader(body))
	postReq.Header.Set("Authorization", "Bearer "+tok)
	postReq.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(httptest.NewRecorder(), postReq)

	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/media/media-1/rating", nil)
	delReq.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, delReq)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestStarRating_GET_NoAuth_401(t *testing.T) {
	mux, _, _, _ := newStarRatingMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/media-1/rating", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestStarRating_GET_Auth_200(t *testing.T) {
	mux, gen, _, _ := newStarRatingMux(t)
	tok := makeToken(t, gen, false)

	// set a rating first
	body, _ := json.Marshal(map[string]int{"value": 5})
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/rating", bytes.NewReader(body))
	postReq.Header.Set("Authorization", "Bearer "+tok)
	postReq.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(httptest.NewRecorder(), postReq)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/media-1/rating", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if int(resp["value"].(float64)) != 5 {
		t.Errorf("expected value 5, got %v", resp["value"])
	}
}

func TestStarRating_GET_NoRating_Returns0(t *testing.T) {
	mux, gen, _, _ := newStarRatingMux(t)
	tok := makeToken(t, gen, false)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/media-1/rating", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if int(resp["value"].(float64)) != 0 {
		t.Errorf("expected value 0 when no rating, got %v", resp["value"])
	}
}

func TestStarRating_Aggregate_NoAuth_401(t *testing.T) {
	mux, _, _, _ := newStarRatingMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/media-1/rating/aggregate", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestStarRating_Aggregate_Auth_200(t *testing.T) {
	mux, gen, _, _ := newStarRatingMux(t)
	tok := makeToken(t, gen, false)

	// seed two ratings
	for _, profile := range []string{"p1", "p2"} {
		body, _ := json.Marshal(map[string]int{"value": 4})
		// post directly as different profiles would need separate tokens
		// use the repo directly via the service by calling POST with the same token twice
		_ = profile
		postReq := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/rating", bytes.NewReader(body))
		postReq.Header.Set("Authorization", "Bearer "+tok)
		postReq.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(httptest.NewRecorder(), postReq)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/media-1/rating/aggregate", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["average"]; !ok {
		t.Error("response missing 'average' field")
	}
	if _, ok := resp["count"]; !ok {
		t.Error("response missing 'count' field")
	}
}

func TestStarRating_MediaNotFound_404(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	repo := ratingfakes.NewFakeRepository()
	svc := apprating.NewService(repo, sharedfakes.NewFakeEventBus())
	getter := newFakeMediaGetter() // no items seeded

	mux := http.NewServeMux()
	registerStarRatingRoutes(mux, svc, tokenGen, getter, allowAll())

	tok, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1", ProfileID: "p1"})
	body, _ := json.Marshal(map[string]int{"value": 3})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/nonexistent/rating", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing media, got %d", w.Code)
	}
}
