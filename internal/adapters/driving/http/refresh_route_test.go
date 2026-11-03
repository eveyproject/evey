package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/pkg/ratelimit"
)

// A silent renewal is not a sign-in attempt: it rides its own budget, so a
// household whose devices renew after a restart is not refused because a
// few mistyped passwords spent the login budget of the same address.
func TestRefreshRoute_OwnBudgetSurvivesExhaustedLoginLimiter(t *testing.T) {
	loginRL := ratelimit.New(1)
	defer loginRL.Stop()
	refreshRL := ratelimit.New(60)
	defer refreshRL.Stop()
	gw := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	login := ratelimit.MiddlewareFunc(loginRL, gw)
	refresh := refreshRouteHandler(gw, func() bool { return false }, refreshRL)

	for range 3 {
		login(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{}`)))
	}
	rec := httptest.NewRecorder()
	login(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{}`)))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("login budget must be spent for the test to mean anything, got %d", rec.Code)
	}

	for i := range 60 {
		ok := httptest.NewRecorder()
		refresh(ok, httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(`{"refreshToken":"x"}`)))
		if ok.Code != http.StatusOK {
			t.Fatalf("refresh %d must pass on its own budget, got %d", i, ok.Code)
		}
	}
	rec = httptest.NewRecorder()
	refresh(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(`{"refreshToken":"x"}`)))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("the refresh budget must still bound a flood, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("a refused renewal must say when to retry")
	}
}
