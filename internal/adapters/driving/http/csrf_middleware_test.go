package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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

func TestCSRFMiddleware_POST_NoCookie_Forbidden(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next should not be called")
	})
	h := csrfMiddleware(false, next)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
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

func TestCSRFMiddleware_DELETE_RequiresToken(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next should not be called")
	})
	h := csrfMiddleware(false, next)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/something", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestCSRFMiddleware_PUT_RequiresToken(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next should not be called")
	})
	h := csrfMiddleware(false, next)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/something", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
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

func TestCSRFMiddleware_POST_EmptyCookieValue_Forbidden(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next should not be called")
	})
	h := csrfMiddleware(false, next)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: ""})
	req.Header.Set(csrfHeaderName, "")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestCSRFMiddleware_POST_NoHeader_Forbidden(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next should not be called")
	})
	h := csrfMiddleware(false, next)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "abc123"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}
