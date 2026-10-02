package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/metrics"
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

// A rejected authentication used to vanish: 401 with no log line and no
// metric, so a credential-stuffing run against the API (unlike the login
// form, which logs per attempt) was invisible to operators. The counter is
// the alerting surface; the reason label separates probing without any
// credential from a presented-but-invalid token.
func TestHTTPAuthMiddleware_CountsRejections(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	var ran bool
	var uid string
	var admin bool
	h := httpAuthMiddleware(verifier)(recordingHandler(&ran, &uid, &admin))

	missingBefore := testutil.ToFloat64(metrics.AuthFailuresTotal.WithLabelValues("missing_bearer"))
	invalidBefore := testutil.ToFloat64(metrics.AuthFailuresTotal.WithLabelValues("invalid_token"))

	// Probe with no credential at all.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/profiles", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
	if got := testutil.ToFloat64(metrics.AuthFailuresTotal.WithLabelValues("missing_bearer")) - missingBefore; got != 1 {
		t.Errorf("missing_bearer counter delta = %v, want 1", got)
	}

	// A presented but invalid token.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles", nil)
	req.Header.Set("Authorization", "Bearer forged.jwt.value")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
	if got := testutil.ToFloat64(metrics.AuthFailuresTotal.WithLabelValues("invalid_token")) - invalidBefore; got != 1 {
		t.Errorf("invalid_token counter delta = %v, want 1", got)
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

// httpScrapeOrAdminMiddleware accepts a static scrape bearer OR an admin JWT.

func TestScrapeOrAdmin_StaticTokenPasses(t *testing.T) {
	// Verifier refuses every JWT - proves the static path bypasses JWT
	// validation entirely when the scrape token matches.
	v := &fakeTokenVerifier{err: errTest}
	var ran bool
	var uid string
	var admin bool
	h := httpScrapeOrAdminMiddleware(v, "scrape-secret")(recordingHandler(&ran, &uid, &admin))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer scrape-secret")
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK || !ran {
		t.Fatalf("static scrape token must pass: got %d ran=%v", w.Code, ran)
	}
}

func TestScrapeOrAdmin_WrongStaticTokenFallsBackToAdminAndIs401(t *testing.T) {
	v := &fakeTokenValidator{err: errTest}
	var ran bool
	var uid string
	var admin bool
	h := httpScrapeOrAdminMiddleware(v, "scrape-secret")(recordingHandler(&ran, &uid, &admin))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusUnauthorized || ran {
		t.Fatalf("wrong static token must fall through to admin and 401: got %d ran=%v", w.Code, ran)
	}
}

func TestScrapeOrAdmin_EmptyScrapeTokenAllowsAdminJWT(t *testing.T) {
	v := &fakeTokenVerifier{claims: testClaims(true)} // admin claims
	var ran bool
	var uid string
	var admin bool
	h := httpScrapeOrAdminMiddleware(v, "")(recordingHandler(&ran, &uid, &admin))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer some-jwt")
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK || !ran {
		t.Fatalf("admin JWT must pass when scrape token disabled: got %d ran=%v", w.Code, ran)
	}
	if !admin {
		t.Error("admin path must surface admin claims")
	}
}

func TestScrapeOrAdmin_NonAdminJWTRejected(t *testing.T) {
	// A normal authenticated user (non-admin) must still be 403 even when
	// the scrape token is configured but not provided. Defense-in-depth:
	// having user-level credentials must NEVER unlock /metrics.
	v := &fakeTokenVerifier{claims: testClaims(false)}
	var ran bool
	var uid string
	var admin bool
	h := httpScrapeOrAdminMiddleware(v, "scrape-secret")(recordingHandler(&ran, &uid, &admin))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer user-jwt") // valid claims but IsAdmin=false
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusForbidden || ran {
		t.Fatalf("non-admin user JWT must be 403 on /metrics: got %d ran=%v", w.Code, ran)
	}
}

func TestScrapeOrAdmin_EmptyAuthorizationRejected(t *testing.T) {
	// No header, no scrape token attempt - must be 401 from admin path.
	v := &fakeTokenVerifier{err: errTest}
	var ran bool
	var uid string
	var admin bool
	h := httpScrapeOrAdminMiddleware(v, "scrape-secret")(recordingHandler(&ran, &uid, &admin))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusUnauthorized || ran {
		t.Fatalf("no auth at all must be 401: got %d ran=%v", w.Code, ran)
	}
}

// An admin refusal used to be completely silent - no log line, no metric -
// while the invalid-token path two functions up counted and logged. That is the
// same shape as the 2026-07-29 remote-access lockout: a refused caller and zero
// server-side signal, which is precisely what made it undiagnosable.
func TestHTTPAdminMiddleware_CountsDenials(t *testing.T) {
	verifier := &fakeTokenVerifier{claims: &domainuser.Claims{UserID: "usr-1", IsAdmin: false}}
	var ran bool
	var uid string
	var admin bool
	h := httpAdminMiddleware(verifier)(recordingHandler(&ran, &uid, &admin))

	before := testutil.ToFloat64(metrics.AuthzDeniedTotal.WithLabelValues("http", "admin_required"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/server-settings", nil)
	req.Header.Set("Authorization", "Bearer good.jwt.value")
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 for a non-admin, got %d", w.Code)
	}
	if ran {
		t.Error("the wrapped admin handler must not run for a non-admin")
	}
	if got := testutil.ToFloat64(metrics.AuthzDeniedTotal.WithLabelValues("http", "admin_required")) - before; got != 1 {
		t.Errorf("admin denial counter delta = %v, want 1", got)
	}
}

func TestHTTPAdminMiddleware_AdminPasses(t *testing.T) {
	verifier := &fakeTokenVerifier{claims: &domainuser.Claims{UserID: "usr-1", IsAdmin: true}}
	var ran bool
	var uid string
	var admin bool
	h := httpAdminMiddleware(verifier)(recordingHandler(&ran, &uid, &admin))

	before := testutil.ToFloat64(metrics.AuthzDeniedTotal.WithLabelValues("http", "admin_required"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/server-settings", nil)
	req.Header.Set("Authorization", "Bearer good.jwt.value")
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK || !ran {
		t.Fatalf("an admin must pass: code=%d ran=%v", w.Code, ran)
	}
	if got := testutil.ToFloat64(metrics.AuthzDeniedTotal.WithLabelValues("http", "admin_required")) - before; got != 0 {
		t.Errorf("a successful admin call must not count as a denial (delta %v)", got)
	}
}

// An impersonation JWT marks the request, so the audit entries it writes name
// the admin behind it.
func TestHTTPAuthMiddleware_MarksImpersonation(t *testing.T) {
	claims := testClaims(true)
	claims.ImpersonatedBy = "admin-1"
	var seen string
	h := httpAuthMiddleware(&fakeTokenVerifier{claims: claims})(func(w http.ResponseWriter, r *http.Request) {
		seen = activity.ImpersonatorFrom(r.Context())
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles", nil)
	req.Header.Set("Authorization", "Bearer jwt.with.dots")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != "admin-1" {
		t.Errorf("impersonator in context = %q, want admin-1", seen)
	}
}
