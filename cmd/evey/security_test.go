package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- authCookieMiddleware tests ---

func TestAuthCookieMiddleware_SetsHttpOnlyCookie(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"accessToken":"at123","refreshToken":"rt456"}`))
	})

	handler := authCookieMiddleware(inner, false)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	handler.ServeHTTP(rec, req)

	// refreshToken must be stripped from body
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal("invalid JSON body:", err)
	}
	if _, ok := body["refreshToken"]; ok {
		t.Error("refreshToken should be stripped from response body")
	}
	if _, ok := body["accessToken"]; !ok {
		t.Error("accessToken should remain in response body")
	}

	// cookie must be set
	cookies := rec.Result().Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == refreshCookieName {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("evey_refresh cookie not set")
	}
	if found.Value != "rt456" {
		t.Errorf("cookie value = %q, want %q", found.Value, "rt456")
	}
	if !found.HttpOnly {
		t.Error("cookie must be HttpOnly")
	}
	if found.Secure {
		t.Error("Secure flag should be false when httpsOnly=false")
	}
	if found.SameSite != http.SameSiteLaxMode {
		t.Error("SameSite must be Lax")
	}
}

func TestAuthCookieMiddleware_SecureFlagWhenHTTPS(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"accessToken":"a","refreshToken":"r"}`))
	})
	handler := authCookieMiddleware(inner, true)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))

	for _, c := range rec.Result().Cookies() {
		if c.Name == refreshCookieName {
			if !c.Secure {
				t.Error("Secure flag should be true when httpsOnly=true")
			}
			return
		}
	}
	t.Error("cookie not found")
}

func TestAuthCookieMiddleware_PassthroughOnError(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"bad creds"}`))
	})
	handler := authCookieMiddleware(inner, false)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if len(rec.Result().Cookies()) > 0 {
		t.Error("no cookies should be set on error response")
	}
}

func TestAuthCookieMiddleware_NoRefreshToken(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"accessToken":"at"}`))
	})
	handler := authCookieMiddleware(inner, false)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))

	if len(rec.Result().Cookies()) > 0 {
		t.Error("no cookie should be set when refreshToken absent")
	}
}

// --- injectRefreshTokenFromCookie tests ---

func TestInjectRefreshTokenFromCookie_InjectsToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(`{}`))
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "mytoken"})

	r2 := injectRefreshTokenFromCookie(req)

	var m map[string]string
	if err := json.NewDecoder(r2.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	if m["refreshToken"] != "mytoken" {
		t.Errorf("refreshToken = %q, want %q", m["refreshToken"], "mytoken")
	}
}

func TestInjectRefreshTokenFromCookie_DoesNotOverwrite(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"refreshToken":"existing"}`))
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "cookie-token"})

	r2 := injectRefreshTokenFromCookie(req)

	var m map[string]string
	if err := json.NewDecoder(r2.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	if m["refreshToken"] != "existing" {
		t.Errorf("existing refreshToken should not be overwritten, got %q", m["refreshToken"])
	}
}

func TestInjectRefreshTokenFromCookie_NoCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	r2 := injectRefreshTokenFromCookie(req)
	// should return same request, no panic
	if r2 == nil {
		t.Error("should return non-nil request")
	}
}

// --- clearRefreshCookie tests ---

func TestClearRefreshCookie_SetsMaxAgeZero(t *testing.T) {
	rec := httptest.NewRecorder()
	clearRefreshCookie(rec, false)

	for _, c := range rec.Result().Cookies() {
		if c.Name == refreshCookieName {
			if c.MaxAge >= 0 {
				// MaxAge negative means delete; net/http serializes MaxAge<0 as "Max-Age=0"
				// The raw Set-Cookie header should contain Max-Age=0
				raw := rec.Header().Get("Set-Cookie")
				if !strings.Contains(raw, "Max-Age=0") {
					t.Errorf("expected Max-Age=0 in Set-Cookie, got: %s", raw)
				}
			}
			return
		}
	}
	t.Error("evey_refresh cookie not found in response")
}
