package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evey-project/evey/internal/domain/shared"
)

// inner records whether it ran and the claims it observed, so the middleware
// tests can assert both the auth decision and the synthesized identity.
func recordingHandler(ran *bool, seenUserID *string, seenAdmin *bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*ran = true
		if c := HTTPClaimsFromContext(r.Context()); c != nil {
			*seenUserID = c.UserID
			*seenAdmin = c.IsAdmin
		}
		w.WriteHeader(http.StatusOK)
	}
}

func withStreamToken(r *http.Request, mediaID, userID string) *http.Request {
	st := &shared.StreamToken{MediaItemID: shared.MediaItemID(mediaID), UserID: shared.UserID(userID)}
	return r.WithContext(context.WithValue(r.Context(), streamTokenCtxKey, st))
}

// THE FIX: a stream token must NOT authenticate a generic (non-stream) endpoint.
// In production TokenRedactMiddlewareWithStore sets Authorization: Bearer <opaque>
// and the stream-token context value; httpAuthMiddleware must ignore the stream
// token and reject the opaque value as an invalid JWT (401).
func TestHTTPAuthMiddleware_RejectsStreamToken(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest} // opaque token is not a valid JWT
	var ran bool
	var uid string
	var admin bool
	h := httpAuthMiddleware(verifier)(recordingHandler(&ran, &uid, &admin))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles", nil)
	req.Header.Set("Authorization", "Bearer opaque-stream-token")
	req = withStreamToken(req, "m1", "u1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("stream token must not authenticate a generic endpoint: got %d, want 401", w.Code)
	}
	if ran {
		t.Error("handler must not run for a stream token on a generic endpoint")
	}
}

func TestHTTPAuthMiddleware_AcceptsJWT(t *testing.T) {
	verifier := &fakeTokenVerifier{claims: testClaims(false)}
	var ran bool
	var uid string
	var admin bool
	h := httpAuthMiddleware(verifier)(recordingHandler(&ran, &uid, &admin))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles", nil)
	req.Header.Set("Authorization", "Bearer jwt.with.dots")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK || !ran {
		t.Fatalf("valid JWT must authenticate: got %d ran=%v", w.Code, ran)
	}
	if uid != "u1" {
		t.Errorf("expected claims UserID u1, got %q", uid)
	}
}

func TestHTTPAuthMiddleware_NoToken_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	var ran bool
	var uid string
	var admin bool
	h := httpAuthMiddleware(verifier)(recordingHandler(&ran, &uid, &admin))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized || ran {
		t.Fatalf("no token must be 401: got %d ran=%v", w.Code, ran)
	}
}

// httpStreamAuthMiddleware accepts a stream token and synthesizes UserID-only
// claims (never admin), so the wrapped media handler runs and can scope-check.
func TestHTTPStreamAuthMiddleware_AcceptsStreamToken(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest} // no JWT; must still pass via stream token
	var ran bool
	var uid string
	var admin bool
	h := httpStreamAuthMiddleware(verifier)(recordingHandler(&ran, &uid, &admin))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/stream", nil)
	req.Header.Set("Authorization", "Bearer opaque-stream-token")
	req = withStreamToken(req, "m1", "u1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK || !ran {
		t.Fatalf("stream token must authenticate a stream endpoint: got %d ran=%v", w.Code, ran)
	}
	if uid != "u1" {
		t.Errorf("expected synthesized UserID u1, got %q", uid)
	}
	if admin {
		t.Error("stream-token claims must never be admin")
	}
}

// With no stream token present, httpStreamAuthMiddleware falls back to JWT.
func TestHTTPStreamAuthMiddleware_FallsBackToJWT(t *testing.T) {
	verifier := &fakeTokenVerifier{claims: testClaims(false)}
	var ran bool
	var uid string
	var admin bool
	h := httpStreamAuthMiddleware(verifier)(recordingHandler(&ran, &uid, &admin))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/stream", nil)
	req.Header.Set("Authorization", "Bearer jwt.with.dots")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK || !ran {
		t.Fatalf("JWT must authenticate a stream endpoint when no stream token: got %d ran=%v", w.Code, ran)
	}
	if uid != "u1" {
		t.Errorf("expected JWT UserID u1, got %q", uid)
	}
}

func TestHTTPStreamAuthMiddleware_NoToken_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	var ran bool
	var uid string
	var admin bool
	h := httpStreamAuthMiddleware(verifier)(recordingHandler(&ran, &uid, &admin))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/stream", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized || ran {
		t.Fatalf("no token must be 401 on stream endpoint too: got %d ran=%v", w.Code, ran)
	}
}
