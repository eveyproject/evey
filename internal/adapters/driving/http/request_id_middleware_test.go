package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	eveylogger "github.com/eveyproject/evey/internal/pkg/logger"
)

func TestRequestIDMiddlewareGenerates(t *testing.T) {
	var seen string
	h := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = eveylogger.RequestIDFromCtx(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if seen == "" {
		t.Fatal("expected generated request id in ctx")
	}
	if w.Header().Get(requestIDHeader) != seen {
		t.Fatalf("response header mismatch: header=%q ctx=%q", w.Header().Get(requestIDHeader), seen)
	}
}

func TestRequestIDMiddlewareReusesInbound(t *testing.T) {
	var seen string
	h := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = eveylogger.RequestIDFromCtx(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(requestIDHeader, "client-abc-123")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if seen != "client-abc-123" {
		t.Fatalf("want inbound id reused, got %q", seen)
	}
}

func TestRequestIDMiddlewareRejectsBadInbound(t *testing.T) {
	var seen string
	h := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = eveylogger.RequestIDFromCtx(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(requestIDHeader, "bad\nvalue")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if seen == "" || seen == "bad\nvalue" {
		t.Fatalf("expected regenerated id, got %q", seen)
	}
}
