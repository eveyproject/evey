package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A login opened on another host (LAN IP while the redirect URL names the
// public domain) restarts once on the redirect URL's host: the binding cookie
// is host-only, so setting it on the LAN host would fail every such login.
func TestOIDCLogin_OtherHostRestartsOnTheRedirectHost(t *testing.T) {
	mux, idp := newPKCEFixture(t)

	req := httptest.NewRequest(http.MethodGet, "http://192.168.1.10:8080/api/v1/auth/oidc/login?flow=", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("status %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://example.com/api/v1/auth/oidc/login?") || !strings.Contains(loc, "canonical=1") {
		t.Fatalf("want one hop to the redirect host, got %q", loc)
	}
	if responseCookie(w, oidcLoginCookieName) != nil {
		t.Fatal("the binding cookie must not be set on the wrong host")
	}

	// The marked hop never hops again, even if a proxy rewrites Host.
	req = httptest.NewRequest(http.MethodGet, "http://backend:8080/api/v1/auth/oidc/login?canonical=1", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if loc := w.Header().Get("Location"); !strings.HasPrefix(loc, idp.srv.URL+"/auth") {
		t.Fatalf("marked login must go to the provider, got %q", loc)
	}
	if responseCookie(w, oidcLoginCookieName) == nil {
		t.Fatal("marked login must set the binding cookie")
	}
}
