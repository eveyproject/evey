package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
)

// --- Cookie security tests ---

func TestAuthCookieMiddleware_SetsHttpOnlyCookie(t *testing.T) {
	t.Parallel()
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"accessToken":"tok","refreshToken":"reftok"}`))
	})

	handler := authCookieMiddleware(inner, false)
	req := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// refreshToken must NOT appear in response body
	var body map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if _, ok := body["refreshToken"]; ok {
		t.Error("refreshToken must not be present in response body")
	}
	if _, ok := body["accessToken"]; !ok {
		t.Error("accessToken must remain in response body")
	}

	// Set-Cookie must be present and HttpOnly
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected Set-Cookie header")
	}
	var found bool
	for _, c := range cookies {
		if c.Name == refreshCookieName {
			found = true
			if !c.HttpOnly {
				t.Error("refresh cookie must be HttpOnly")
			}
			if c.SameSite != http.SameSiteLaxMode {
				t.Errorf("refresh cookie SameSite = %v, want Lax", c.SameSite)
			}
			if c.Value != "reftok" {
				t.Errorf("cookie value = %q, want reftok", c.Value)
			}
		}
	}
	if !found {
		t.Errorf("cookie %q not set", refreshCookieName)
	}
}

func TestAuthCookieMiddleware_HttpOnlyInHeader(t *testing.T) {
	t.Parallel()
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"accessToken":"tok","refreshToken":"secret-refresh"}`))
	})

	handler := authCookieMiddleware(inner, false)
	req := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	setCookie := w.Header().Get("Set-Cookie")
	if setCookie == "" {
		t.Fatal("Set-Cookie header missing")
	}
	if !strings.Contains(setCookie, "HttpOnly") {
		t.Errorf("Set-Cookie must contain HttpOnly, got: %s", setCookie)
	}
}

func TestAuthCookieMiddleware_NoStripOnError(t *testing.T) {
	t.Parallel()
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"bad credentials"}`))
	})

	handler := authCookieMiddleware(inner, false)
	req := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if w.Header().Get("Set-Cookie") != "" {
		t.Error("Set-Cookie must not be set on error response")
	}
}

// --- Rate limit handler tests ---

func TestLoginRateLimit_Allows_Then_Blocks(t *testing.T) {
	t.Parallel()
	// burst=3 so first 3 allowed, 4th blocked
	rl := ratelimit.New(3)
	defer rl.Stop()

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := ratelimit.MiddlewareFunc(rl, inner)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		w := httptest.NewRecorder()
		handler(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// 4th request should be rate-limited
	req := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after limit exceeded, got %d", w.Code)
	}
}

func TestLoginRateLimit_DifferentIPs_Independent(t *testing.T) {
	t.Parallel()
	rl := ratelimit.New(1)
	defer rl.Stop()

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := ratelimit.MiddlewareFunc(rl, inner)

	// exhaust IP 1
	req1 := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	req1.RemoteAddr = "10.0.0.1:1234"
	w1 := httptest.NewRecorder()
	handler(w1, req1)

	// block IP 1
	req2 := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	req2.RemoteAddr = "10.0.0.1:1234"
	w2 := httptest.NewRecorder()
	handler(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("IP 1 should be blocked, got %d", w2.Code)
	}

	// IP 2 must still be allowed (different IP, fresh bucket)
	req3 := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	req3.RemoteAddr = "10.0.0.2:1234"
	w3 := httptest.NewRecorder()
	handler(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("IP 2 should be allowed, got %d", w3.Code)
	}
}
