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
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "abc123"})
	req.Header.Set(csrfHeaderName, "wrong-value")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
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

func TestCSRFMiddleware_POST_EmptyCookieValue_WithHeader_Forbidden(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next should not be called")
	})
	h := csrfMiddleware(false, next)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: ""})
	req.Header.Set(csrfHeaderName, "some-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
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
