package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Exported wrapper just calls the internal helper; assert it's wired.
func TestCSRFMiddleware_ExportedWrapper(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	CSRFMiddleware(false, next).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	found := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == "_csrf" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected _csrf cookie set via exported CSRFMiddleware")
	}
}

func TestCSRFMiddleware_GET_SetsToken(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := csrfMiddleware(false, next)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/something", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	cookies := w.Result().Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == csrfCookieName {
			found = true
			if c.HttpOnly {
				t.Error("csrf cookie must not be HttpOnly")
			}
			if len(c.Value) != csrfTokenBytes*2 {
				t.Errorf("expected token length %d hex chars, got %d", csrfTokenBytes*2, len(c.Value))
			}
		}
	}
	if !found {
		t.Error("expected _csrf cookie to be set")
	}
}

func TestCSRFMiddleware_POST_NoCookieNoHeader_SkipCSRF(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	h := csrfMiddleware(false, next)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (mobile/API skip), got %d", w.Code)
	}
	if !called {
		t.Error("next handler should have been called for non-browser client")
	}
}

func TestCSRFMiddleware_POST_CookieButNoHeader_SkipCSRF(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	h := csrfMiddleware(false, next)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "abc123"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (no CSRF header = mobile/API), got %d", w.Code)
	}
	if !called {
		t.Error("next should be called when no X-CSRF-Token header")
	}
}

func TestCSRFMiddleware_POST_MismatchHeader_Forbidden(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next should not be called")
	})
	h := csrfMiddleware(false, next)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "rt-xyz"})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "abc123"})
	req.Header.Set(csrfHeaderName, "wrong-value")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestCSRFMiddleware_POST_SessionNoHeader_Forbidden(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next should not be called - session request without CSRF header must fail-closed")
	})
	h := csrfMiddleware(false, next)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/something", nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "rt-xyz"})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "abc123"})
	// no X-CSRF-Token header
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 (session cookie + no header = fail-closed), got %d", w.Code)
	}
}

func TestCSRFMiddleware_POST_NoCsrfCookie_Bypassed(t *testing.T) {
	// A client that does NOT carry our _csrf cookie cannot be a browser
	// CSRF target - browsers auto-attach our cookies on cross-origin POST,
	// so the absence of the cookie means there is no session to forge
	// against. Bypass and let the request through, regardless of any
	// Sec-Fetch-Site or Origin header the client may have set on its own.
	called := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true })
	h := csrfMiddleware(false, next)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/something", nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "rt-xyz"})
	req.Header.Set(csrfHeaderName, "some-header")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if !called {
		t.Fatalf("expected next to run (no _csrf cookie = native client), got code %d", w.Code)
	}
}

func TestCSRFMiddleware_POST_MatchingToken_OK(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	h := csrfMiddleware(false, next)
	token := "validtoken123456"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	req.Header.Set(csrfHeaderName, token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !called {
		t.Error("next handler should have been called")
	}
}

func TestCSRFMiddleware_DELETE_NoCookieNoHeader_SkipCSRF(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	h := csrfMiddleware(false, next)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/something", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (mobile/API skip), got %d", w.Code)
	}
	if !called {
		t.Error("next should be called for non-browser client")
	}
}

func TestCSRFMiddleware_DELETE_WithCookie_NoHeader_SkipCSRF(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	h := csrfMiddleware(false, next)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/something", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "abc123"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (no header = skip), got %d", w.Code)
	}
	if !called {
		t.Error("next should be called")
	}
}

func TestCSRFMiddleware_PUT_NoCookieNoHeader_SkipCSRF(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	h := csrfMiddleware(false, next)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/something", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (mobile/API skip), got %d", w.Code)
	}
	if !called {
		t.Error("next should be called for non-browser client")
	}
}

func TestCSRFMiddleware_GET_ExistingCookie_NoNewCookie(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := csrfMiddleware(false, next)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/something", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "existing"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	for _, c := range w.Result().Cookies() {
		if c.Name == csrfCookieName {
			t.Error("should not set new csrf cookie when one exists")
		}
	}
}

func TestCSRFMiddleware_POST_EmptyCookieValue_Bypassed(t *testing.T) {
	// An empty _csrf cookie is functionally indistinguishable from no
	// cookie at all - same conclusion: not a browser session, bypass.
	called := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true })
	h := csrfMiddleware(false, next)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "rt-xyz"})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: ""})
	req.Header.Set(csrfHeaderName, "some-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if !called {
		t.Fatalf("expected next to run (empty _csrf cookie = native client), got %d", w.Code)
	}
}

// Mobile clients (React Native, Expo) sometimes carry a stale refresh-token
// cookie in their fetch jar after a forced logout. They never set Sec-Fetch-*
// or Origin headers, so they must bypass CSRF - otherwise login is impossible
// until the cookie naturally expires.
func TestCSRFMiddleware_POST_StaleRefreshCookie_NoBrowserHeaders_SkipCSRF(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	h := csrfMiddleware(false, next)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "rt-stale"})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "abc123"})
	// no Sec-Fetch-Site, no Origin → not a browser
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (native client without browser headers), got %d", w.Code)
	}
	if !called {
		t.Error("next should be called for native client even with stale refresh cookie")
	}
}

func TestCSRFMiddleware_POST_CookieNoHeader_SkipCSRF2(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	h := csrfMiddleware(false, next)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "abc123"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (no header = skip CSRF), got %d", w.Code)
	}
	if !called {
		t.Error("next should be called")
	}
}
