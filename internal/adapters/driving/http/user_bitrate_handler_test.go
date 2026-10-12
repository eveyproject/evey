package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eveyproject/evey/internal/pkg/ratelimit"
)

// stubBitrateSetter records the arguments SetUserBitrateLimit was called with
// and returns the configured error.
type stubBitrateSetter struct {
	err            error
	capturedUserID string
	capturedKbps   int
}

func (s *stubBitrateSetter) SetUserBitrateLimit(_ context.Context, userID string, maxBitrateKbps int) error {
	s.capturedUserID = userID
	s.capturedKbps = maxBitrateKbps
	return s.err
}

// buildUserBitrateMux wires just the bitrate-limit routes against the given
// getter/setter stubs, gated by the real admin middleware (a real JWTGenerator
// via genTokens so IsAdmin is actually enforced, not stubbed away).
func buildUserBitrateMux(t *testing.T, getter UserBitrateGetter, setter userBitrateSetter) (mux *http.ServeMux, adminTok, userTok string) {
	t.Helper()
	jwtGen, adminTok, userTok := genTokens(t)
	mux = http.NewServeMux()
	registerUserBitrateRoutes(mux, getter, setter, jwtGen, ratelimit.New(600))
	return mux, adminTok, userTok
}

// TestGetUserBitrateLimit_RouteMissingOnEmptyMux pins the bug: before this
// route was registered anywhere (neither the manual mux in server.go nor the
// proto, which declares no bitrate-limit RPC at all - grep proof in the
// report), GET .../bitrate-limit 404'd. A bare mux with nothing registered
// reproduces exactly that.
func TestGetUserBitrateLimit_RouteMissingOnEmptyMux(t *testing.T) {
	mux := http.NewServeMux()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/u1/bitrate-limit", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 (route not registered), got %d", w.Code)
	}
}

func TestGetUserBitrateLimit_HappyPath(t *testing.T) {
	getter := &stubBitrateSvc{kbps: 8000}
	mux, adminTok, _ := buildUserBitrateMux(t, getter, &stubBitrateSetter{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/u1/bitrate-limit", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]int
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if body["max_bitrate_kbps"] != 8000 {
		t.Errorf("expected max_bitrate_kbps=8000, got %d", body["max_bitrate_kbps"])
	}
}

func TestGetUserBitrateLimit_NonAdmin_Forbidden(t *testing.T) {
	mux, _, userTok := buildUserBitrateMux(t, &stubBitrateSvc{kbps: 1000}, &stubBitrateSetter{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/u1/bitrate-limit", nil)
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestGetUserBitrateLimit_Unauthenticated(t *testing.T) {
	mux, _, _ := buildUserBitrateMux(t, &stubBitrateSvc{kbps: 1000}, &stubBitrateSetter{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/u1/bitrate-limit", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestGetUserBitrateLimit_UserNotFound(t *testing.T) {
	mux, adminTok, _ := buildUserBitrateMux(t, &stubBitrateSvc{err: errors.New("not found")}, &stubBitrateSetter{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/ghost/bitrate-limit", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPatchUserBitrateLimit_HappyPath(t *testing.T) {
	setter := &stubBitrateSetter{}
	mux, adminTok, _ := buildUserBitrateMux(t, &stubBitrateSvc{}, setter)

	body, _ := json.Marshal(map[string]int{"max_bitrate_kbps": 4000})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/u1/bitrate-limit", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if setter.capturedUserID != "u1" {
		t.Errorf("expected user id u1, got %q", setter.capturedUserID)
	}
	if setter.capturedKbps != 4000 {
		t.Errorf("expected 4000 kbps, got %d", setter.capturedKbps)
	}
}

func TestPatchUserBitrateLimit_NonAdmin_Forbidden(t *testing.T) {
	mux, _, userTok := buildUserBitrateMux(t, &stubBitrateSvc{}, &stubBitrateSetter{})

	body, _ := json.Marshal(map[string]int{"max_bitrate_kbps": 4000})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/u1/bitrate-limit", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestPatchUserBitrateLimit_InvalidJSON(t *testing.T) {
	mux, adminTok, _ := buildUserBitrateMux(t, &stubBitrateSvc{}, &stubBitrateSetter{})

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/u1/bitrate-limit", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPatchUserBitrateLimit_Negative_Rejected(t *testing.T) {
	mux, adminTok, _ := buildUserBitrateMux(t, &stubBitrateSvc{}, &stubBitrateSetter{})

	body, _ := json.Marshal(map[string]int{"max_bitrate_kbps": -1})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/u1/bitrate-limit", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPatchUserBitrateLimit_SetterError(t *testing.T) {
	mux, adminTok, _ := buildUserBitrateMux(t, &stubBitrateSvc{}, &stubBitrateSetter{err: errors.New("user not found")})

	body, _ := json.Marshal(map[string]int{"max_bitrate_kbps": 4000})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/ghost/bitrate-limit", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
