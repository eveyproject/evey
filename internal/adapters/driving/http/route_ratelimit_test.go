package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

// The subtitle route forks an ffmpeg process per request whenever mkvgo's fast
// path can't shortcut the container (.avi/.ts/.mov, damaged EBML), so it must
// be capped like the snapshot route - a burst of 2 admits exactly 2 requests
// from one IP before the 3rd is rejected with 429, never reaching the handler
// (and therefore never spawning ffmpeg).
func TestSubtitleRoute_RateLimited_429AfterBurst(t *testing.T) {
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		File: &domainlibrary.MediaFile{Path: "/tmp/fake.mkv"},
	}
	rl := ratelimit.NewWithBurst(2, 0) // burst 2, no refill: deterministic
	defer rl.Stop()
	h := subtitleRouteHandler(
		&fakeMediaStreamsGetter{item: item},
		"http://localhost", allowAll(), allowAllParental{}, rl,
		subtitleCache{},
	)

	mux := http.NewServeMux()
	verifier := &fakeTokenVerifier{claims: testClaims(true)}
	mux.HandleFunc("GET /api/v1/media/{id}/subtitle/{track}", httpStreamAuthMiddleware(verifier)(h))

	req := func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
		r.Header.Set("Authorization", "Bearer tok")
		return r
	}

	for i := range 2 {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req())
		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d: got 429 within the burst, want the handler to run", i+1)
		}
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req())
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("3rd request over the burst = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
}

// A nil SubtitleRL (the pre-fix state, and any deployment/test that doesn't
// wire one) must never block - the "if s.SubtitleRL != nil" gate is a
// pass-through, same as SnapshotRL's existing pattern.
func TestSubtitleRoute_NilRateLimiter_NoBlocking(t *testing.T) {
	item := &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		File: &domainlibrary.MediaFile{Path: "/tmp/fake.mkv"},
	}
	h := subtitleRouteHandler(
		&fakeMediaStreamsGetter{item: item},
		"http://localhost", allowAll(), allowAllParental{}, nil,
		subtitleCache{},
	)

	mux := http.NewServeMux()
	verifier := &fakeTokenVerifier{claims: testClaims(true)}
	mux.HandleFunc("GET /api/v1/media/{id}/subtitle/{track}", httpStreamAuthMiddleware(verifier)(h))

	for i := range 50 {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/subtitle/0", nil)
		r.Header.Set("Authorization", "Bearer tok")
		mux.ServeHTTP(w, r)
		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d: got 429 with no limiter wired, want pass-through", i+1)
		}
	}
}

// Trickplay manifest + sprite images are read-only (disk + DB, no fork) so
// they share ONE more permissive limiter across both routes - a scrub bar
// loads the manifest once then many sprite images, and both must draw from
// the same budget so a flood on either route is capped.
func TestTrickplayRoutes_SharedRateLimiter_429AfterBurst(t *testing.T) {
	item := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}
	rl := ratelimit.NewWithBurst(2, 0) // burst 2, no refill: deterministic
	defer rl.Stop()
	manifestH, imageH := trickplayRouteHandlers(
		&fakeTrickplayGetter{manifest: &domainlibrary.TrickplayManifest{MediaItemID: "m1"}},
		"http://localhost", allowAll(), &fakeMediaItemGetter{item: item}, allowAllParental{}, nil, rl,
	)

	mux := http.NewServeMux()
	verifier := &fakeTokenVerifier{claims: testClaims(true)}
	mux.HandleFunc("GET /api/v1/media/{id}/trickplay", httpStreamAuthMiddleware(verifier)(manifestH))
	mux.HandleFunc("GET /api/v1/media/{id}/trickplay/{image}", httpStreamAuthMiddleware(verifier)(imageH))

	authed := func(method, path string) *http.Request {
		r := httptest.NewRequest(method, path, nil)
		r.Header.Set("Authorization", "Bearer tok")
		return r
	}

	// Burns the shared 2-token budget across BOTH routes.
	w1 := httptest.NewRecorder()
	mux.ServeHTTP(w1, authed(http.MethodGet, "/api/v1/media/m1/trickplay"))
	if w1.Code == http.StatusTooManyRequests {
		t.Fatalf("1st request (manifest) got 429 within the burst")
	}
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, authed(http.MethodGet, "/api/v1/media/m1/trickplay/0.jpg"))
	if w2.Code == http.StatusTooManyRequests {
		t.Fatalf("2nd request (image) got 429 within the burst")
	}

	// The 3rd request, on EITHER route, must be rejected - proving one shared bucket.
	w3 := httptest.NewRecorder()
	mux.ServeHTTP(w3, authed(http.MethodGet, "/api/v1/media/m1/trickplay"))
	if w3.Code != http.StatusTooManyRequests {
		t.Errorf("3rd request over the shared burst = %d, want %d", w3.Code, http.StatusTooManyRequests)
	}
}

// A nil TrickplayRL must never block, same nil-safe pass-through as the other
// route limiters.
func TestTrickplayRoutes_NilRateLimiter_NoBlocking(t *testing.T) {
	item := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}
	manifestH, imageH := trickplayRouteHandlers(
		&fakeTrickplayGetter{manifest: &domainlibrary.TrickplayManifest{MediaItemID: "m1"}},
		"http://localhost", allowAll(), &fakeMediaItemGetter{item: item}, allowAllParental{}, nil, nil,
	)

	mux := http.NewServeMux()
	verifier := &fakeTokenVerifier{claims: testClaims(true)}
	mux.HandleFunc("GET /api/v1/media/{id}/trickplay", httpStreamAuthMiddleware(verifier)(manifestH))
	mux.HandleFunc("GET /api/v1/media/{id}/trickplay/{image}", httpStreamAuthMiddleware(verifier)(imageH))

	for i := range 50 {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/trickplay", nil)
		r.Header.Set("Authorization", "Bearer tok")
		mux.ServeHTTP(w, r)
		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d: got 429 with no limiter wired, want pass-through", i+1)
		}
	}
}
