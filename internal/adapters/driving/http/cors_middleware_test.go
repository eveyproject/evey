package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func corsTestHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
}

func TestCORSMiddleware_PreflightFromPackagedApp(t *testing.T) {
	h := CORSMiddleware("https://evey.example", corsTestHandler())
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	req.Header.Set("Origin", "null")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "authorization, content-type, x-persistent-session")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight must be answered here, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "null" {
		t.Errorf("Allow-Origin = %q, want the opaque origin echoed", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Headers"); got != "authorization, content-type, x-persistent-session" {
		t.Errorf("Allow-Headers = %q, want the requested headers echoed", got)
	}
	if w.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Error("a foreign origin must never get Allow-Credentials")
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("preflight must list the allowed methods")
	}
}

func TestCORSMiddleware_ForeignOriginGetsAnswerWithoutCredentials(t *testing.T) {
	h := CORSMiddleware("https://evey.example", corsTestHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	req.Header.Set("Origin", "https://tv.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusTeapot {
		t.Fatalf("a real request must reach the handler, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://tv.example" {
		t.Errorf("Allow-Origin = %q", got)
	}
	if w.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Error("a foreign origin must never get Allow-Credentials: a page must not read an answer earned by the victim's cookie")
	}
	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want Origin so a cache never serves one origin's answer to another", got)
	}
}

func TestCORSMiddleware_ConfiguredOriginKeepsCredentials(t *testing.T) {
	h := CORSMiddleware("https://evey.example/", corsTestHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	req.Header.Set("Origin", "https://evey.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Error("the configured frontend origin authenticates with cookies and keeps Allow-Credentials")
	}
}

func TestCORSMiddleware_NoOriginOrStaticPathIsUntouched(t *testing.T) {
	h := CORSMiddleware("https://evey.example", corsTestHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("no Origin header: no CORS headers")
	}

	req = httptest.NewRequest(http.MethodGet, "/assets/index.js", nil)
	req.Header.Set("Origin", "null")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("static files belong to the web app and take no part in CORS")
	}

	// A bare OPTIONS without Access-Control-Request-Method is not a preflight
	// and goes downstream like any other request.
	req = httptest.NewRequest(http.MethodOptions, "/api/v1/x", nil)
	req.Header.Set("Origin", "null")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusTeapot {
		t.Errorf("non-preflight OPTIONS reached %d, want the handler", w.Code)
	}
}

func TestCORSMiddleware_HealthzProbe(t *testing.T) {
	h := CORSMiddleware("https://evey.example", corsTestHandler())
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "null")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Header().Get("Access-Control-Allow-Origin") != "null" {
		t.Error("the server probe every client runs must be readable cross-origin")
	}
}
