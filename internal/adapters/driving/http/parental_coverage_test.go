package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appmedia "github.com/evey-project/evey/internal/app/media"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/evey-project/evey/internal/pkg/auth"
)

// --- shared helpers ---

// =====================================
// auth_handler.go - parental routes, push unregister, listDevices/deleteDevice happy path
// =====================================

func newParentalMux(t *testing.T) (*http.ServeMux, *auth.JWTGenerator) {
	t.Helper()
	gen, _, _ := genTokens(t)
	profileRepo := userfakes.NewFakeProfileRepository()
	_ = profileRepo.Create(context.Background(), &domainuser.Profile{
		ID:     "prof-1",
		UserID: "user-1",
		Name:   "User Profile",
	})
	_ = profileRepo.Create(context.Background(), &domainuser.Profile{
		ID:     "prof-admin",
		UserID: "admin-1",
		Name:   "Admin Profile",
	})
	hasher := userfakes.NewFakePasswordHasher()
	userRepo := userfakes.NewFakeUserRepository()
	_ = userRepo.Create(context.Background(), &domainuser.User{ID: "user-1", Username: "u1", PasswordHash: "hashed:secret"})
	_ = userRepo.Create(context.Background(), &domainuser.User{ID: "admin-1", Username: "a1", IsAdmin: true, PasswordHash: "hashed:secret"})
	mux := http.NewServeMux()
	registerParentalRoutes(mux, profileRepo, userRepo, hasher, gen)
	return mux, gen
}

func TestParentalGetSettings_NoAuth_401(t *testing.T) {
	mux, _ := newParentalMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/prof-1/parental", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestParentalGetSettings_IDOR_403(t *testing.T) {
	mux, gen := newParentalMux(t)
	// user-1 trying to access admin profile
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/prof-admin/parental", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestParentalGetSettings_OK(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/prof-1/parental", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestParentalGetSettings_NotFound_404(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", ProfileID: "prof-admin", IsAdmin: true})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/nonexistent/parental", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestParentalUpdateSettings_OK(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})
	body := `{"maxRating":"PG-13","blockedTags":["violence"],"enabled":true}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/prof-1/parental", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestParentalUpdateSettings_IDOR_403(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})
	body := `{"maxRating":"PG"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/prof-admin/parental", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestParentalVerifyPIN_NoPIN_Valid(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles/prof-1/parental/verify-pin", strings.NewReader(`{"pin":"1234"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "true") {
		t.Error("expected valid=true when no PIN set")
	}
}

func TestParentalVerifyPIN_IDOR_403(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "attacker", ProfileID: "prof-attack", IsAdmin: false})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles/prof-1/parental/verify-pin", strings.NewReader(`{"pin":"1234"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// =====================================
// auth_handler.go - parental with PIN
// =====================================

func TestParentalUpdateSettings_SetPIN_OK(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})

	body := `{"pin":"1234","currentPassword":"secret"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/prof-1/parental", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// Regression: first-time PIN set without the account password must fail -
// otherwise a stolen JWT could lock the real owner out of their profile.
func TestParentalUpdateSettings_SetPIN_NoPassword_Forbidden(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})

	body := `{"pin":"1234"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/prof-1/parental", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestParentalUpdateSettings_SetPIN_WrongPassword_Forbidden(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})

	body := `{"pin":"1234","currentPassword":"wrong"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/prof-1/parental", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestParentalUpdateSettings_ClearPIN(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})
	empty := ""
	body, _ := json.Marshal(map[string]*string{"pin": &empty})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/prof-1/parental", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// Rotating, clearing or disabling protection while a PIN exists needs proof of
// the CURRENT PIN: a stolen JWT alone must not be enough to turn parental off.
func TestParentalUpdateSettings_ExistingPIN_RequiresCurrentPIN(t *testing.T) {
	mux, gen := newParentalMux(t)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", ProfileID: "prof-1", IsAdmin: false})
	put := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/prof-1/parental", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}
	// First-time set with the account password.
	if w := put(`{"pin":"1234","currentPassword":"secret"}`); w.Code != http.StatusOK {
		t.Fatalf("set: %d %s", w.Code, w.Body.String())
	}
	for name, body := range map[string]string{
		"rotate without current pin":  `{"pin":"5678"}`,
		"rotate with wrong pin":       `{"pin":"5678","currentPin":"0000"}`,
		"clear without current pin":   `{"pin":""}`,
		"disable without current pin": `{"enabled":false}`,
	} {
		w := put(body)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s: got %d, want 403: %s", name, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "errors.auth.invalid_pin") {
			t.Errorf("%s: refusal must carry the translatable key: %s", name, w.Body.String())
		}
	}
	if w := put(`{"pin":"5678","currentPin":"1234"}`); w.Code != http.StatusOK {
		t.Fatalf("rotate with the right pin: %d %s", w.Code, w.Body.String())
	}
	if w := put(`{"enabled":false,"currentPin":"5678"}`); w.Code != http.StatusOK {
		t.Fatalf("disable with the rotated pin: %d %s", w.Code, w.Body.String())
	}
}

// ============================================================
// Stream token handler tests
// ============================================================

func TestStreamToken_ParentalBlocked_NotFound(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	userToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})

	mg := newFakeMediaGetter()
	mg.items["m1"] = &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}
	store := appmedia.NewStreamTokenStore(5 * time.Minute)

	// Parental must gate token issuance too: a kid that learns an adult media-id
	// out of band must not be handed a stream token for it.
	h := httpAuthMiddleware(gen)(streamTokenHandler(gen, store, allowAll(), mg, alwaysBlockChecker{}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/stream-token", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("parental-blocked stream-token must return 404 (existence hidden), got %d", w.Code)
	}
}
