package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTokenRedactMiddleware_MovesQueryToHeader(t *testing.T) {
	var gotHeader string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Authorization")
		if r.URL.Query().Get("token") != "" {
			t.Error("token query param should be removed")
		}
	})
	h := tokenRedactMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/foo?token=mytoken", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if gotHeader != "Bearer mytoken" {
		t.Errorf("expected 'Bearer mytoken', got %q", gotHeader)
	}
}

func TestTokenRedactMiddleware_DoesNotOverwriteHeader(t *testing.T) {
	var gotHeader string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Authorization")
	})
	h := tokenRedactMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/foo?token=querytoken", nil)
	req.Header.Set("Authorization", "Bearer headertoken")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if gotHeader != "Bearer headertoken" {
		t.Errorf("expected header token to win, got %q", gotHeader)
	}
}

func TestSecurityHeadersMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	h := securityHeadersMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing X-Content-Type-Options")
	}
	if w.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("missing X-Frame-Options")
	}
	if w.Header().Get("Referrer-Policy") != "strict-origin-when-cross-origin" {
		t.Error("missing Referrer-Policy")
	}
}

func TestApiCacheMiddleware_DefaultNoStore(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	h := apiCacheMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/foo", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("expected no-store, got %q", w.Header().Get("Cache-Control"))
	}
}

func TestApiCacheMiddleware_LibrariesCacheable(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	h := apiCacheMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/libraries", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Header().Get("Cache-Control") != "private, max-age=300" {
		t.Errorf("expected cacheable header, got %q", w.Header().Get("Cache-Control"))
	}
}

func TestExtractBearerToken_FromHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer mytoken")
	if got := extractBearerToken(req); got != "mytoken" {
		t.Errorf("expected 'mytoken', got %q", got)
	}
}

func TestExtractBearerToken_FromQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?token=querytoken", nil)
	if got := extractBearerToken(req); got != "querytoken" {
		t.Errorf("expected 'querytoken', got %q", got)
	}
}

func TestRequireAdmin_Forbidden_NonAdmin(t *testing.T) {
	h := requireAdmin(
		&fakeTokenVerifier{claims: testClaims(false)},
		func(w http.ResponseWriter, r *http.Request) {
			t.Error("should not be called")
		},
	)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestRequireAdmin_OK_Admin(t *testing.T) {
	called := false
	h := requireAdmin(
		&fakeTokenVerifier{claims: testClaims(true)},
		func(w http.ResponseWriter, r *http.Request) {
			called = true
		},
	)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	h(w, req)
	if !called {
		t.Error("next handler should have been called")
	}
}

func TestSafeJoin_Traversal(t *testing.T) {
	_, err := safeJoin("/tmp/safe", "../../../etc/passwd")
	if err == nil {
		t.Error("expected path traversal error")
	}
}

func TestSafeJoin_OK(t *testing.T) {
	got, err := safeJoin("/tmp/safe", "file.jpg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/tmp/safe/file.jpg" {
		t.Errorf("expected /tmp/safe/file.jpg, got %s", got)
	}
}
