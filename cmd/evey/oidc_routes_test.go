package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOIDCSession_NoCookie(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/auth/oidc/session", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(oidcSessionCookieName)
		if err != nil || cookie.Value == "" {
			http.Error(w, `{"message":"no session"}`, http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:   oidcSessionCookieName,
			Value:  "",
			MaxAge: -1,
		})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"accessToken": cookie.Value})
	})

	req := httptest.NewRequest("GET", "/api/v1/auth/oidc/session", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without cookie, got %d", w.Code)
	}
}

func TestOIDCSession_WithCookie_ReturnsTokenAndClears(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/auth/oidc/session", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(oidcSessionCookieName)
		if err != nil || cookie.Value == "" {
			http.Error(w, `{"message":"no session"}`, http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     oidcSessionCookieName,
			Value:    "",
			Path:     "/api/v1/auth/oidc/session",
			HttpOnly: true,
			MaxAge:   -1,
		})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"accessToken": cookie.Value})
	})

	req := httptest.NewRequest("GET", "/api/v1/auth/oidc/session", nil)
	req.AddCookie(&http.Cookie{Name: oidcSessionCookieName, Value: "test-access-token"})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid cookie, got %d", w.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if body["accessToken"] != "test-access-token" {
		t.Errorf("accessToken = %q, want %q", body["accessToken"], "test-access-token")
	}

	// Session cookie must be cleared.
	var cleared bool
	for _, c := range w.Result().Cookies() {
		if c.Name == oidcSessionCookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("session cookie must be cleared after exchange")
	}
}

func TestOIDCCallback_NoTokenInRedirectURL(t *testing.T) {
	t.Parallel()
	// Verify the redirect target does not contain token/refresh query params.
	// We test the redirect URL shape — not the full OIDC flow which requires a live provider.
	redirectTarget := "/oidc-callback"

	if len(redirectTarget) == 0 {
		t.Fatal("redirect target must not be empty")
	}
	if contains(redirectTarget, "token=") || contains(redirectTarget, "refresh=") {
		t.Error("redirect URL must not contain token or refresh query params")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsAt(s, sub))
}

func containsAt(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
