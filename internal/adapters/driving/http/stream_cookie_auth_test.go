package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appmedia "github.com/eveyproject/evey/internal/app/media"
)

// Plan B P1/P2: a valid opaque stream token authenticates an HLS request whether
// it arrives by ?token= (legacy web + cast), the evey_hls httpOnly cookie
// (native <video>/hls.js), or an opaque Bearer header (mobile expo-video); a
// missing or bad token is rejected; and every authenticated HLS request
// (re)issues the sliding cookie with a Max-Age tracking the token's TTL. This
// exercises the full tokenRedactMiddleware -> httpStreamAuthMiddleware chain and
// mirrors the 200-vs-401 matrix validated by hand against the dev server.
func TestStreamAuthChain_CookieBearerQueryAccept(t *testing.T) {
	store := appmedia.NewStreamTokenStore(5 * time.Minute)
	// A native/cast-style long-lived token (12h): the native player and the
	// cookie's Max-Age must carry this window, not the 5-minute default.
	const nativeTTL = 12 * time.Hour
	tok := store.CreateWithTTL("u1", "p1", "m1", nativeTTL)

	probe := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	streamAuth := httpStreamAuthMiddleware(&fakeTokenVerifier{}) // JWT path unused for these cases
	chain := tokenRedactMiddleware(store, false, streamAuth(probe))

	const hlsURL = "/api/v1/media/m1/hls/master.m3u8"
	serve := func(mutate func(*http.Request)) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, hlsURL, nil)
		mutate(req)
		w := httptest.NewRecorder()
		chain.ServeHTTP(w, req)
		return w
	}
	issuedCookie := func(w *httptest.ResponseRecorder) *http.Cookie {
		for _, c := range w.Result().Cookies() {
			if c.Name == hlsCookieName {
				return c
			}
		}
		return nil
	}

	cases := []struct {
		name   string
		mutate func(*http.Request)
		want   int
	}{
		{"evey_hls cookie (native/hls.js)", func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: hlsCookieName, Value: tok})
		}, http.StatusOK},
		{"opaque Bearer (mobile)", func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+tok)
		}, http.StatusOK},
		{"legacy ?token= (dual-accept)", func(r *http.Request) {
			q := r.URL.Query()
			q.Set("token", tok)
			r.URL.RawQuery = q.Encode()
		}, http.StatusOK},
		{"no auth", func(*http.Request) {}, http.StatusUnauthorized},
		{"invalid cookie", func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: hlsCookieName, Value: "not-a-real-token"})
		}, http.StatusUnauthorized},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := serve(c.mutate)
			if w.Code != c.want {
				t.Fatalf("status = %d, want %d", w.Code, c.want)
			}
			if c.want != http.StatusOK {
				return
			}
			ck := issuedCookie(w)
			if ck == nil || ck.Value != tok {
				t.Fatalf("expected evey_hls Set-Cookie carrying the token on an authenticated HLS response")
			}
			if !ck.HttpOnly {
				t.Error("evey_hls cookie must be HttpOnly")
			}
			if ck.Path != "/api/v1/media/m1/hls" {
				t.Errorf("cookie Path = %q, want /api/v1/media/m1/hls", ck.Path)
			}
			if want := int(nativeTTL.Seconds()); ck.MaxAge != want {
				t.Errorf("cookie Max-Age = %d, want %d (the token's sliding TTL)", ck.MaxAge, want)
			}
		})
	}
}
