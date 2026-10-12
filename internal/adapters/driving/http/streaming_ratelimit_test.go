package httpapi

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
)

func withClaims(r *http.Request, c *domainuser.Claims) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), httpClaimsCtxKey{}, c))
}

// streamingRateLimitKey prefers profile, then user, then IP - and the three
// spaces are disjoint so identical raw ids in different roles never collide.
func TestStreamingRateLimitKey(t *testing.T) {
	base := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/stream_0_000.m4s", nil)
	base.RemoteAddr = "203.0.113.7:50000"

	assert.Equal(t, "p:prof-1",
		streamingRateLimitKey(withClaims(base, &domainuser.Claims{UserID: "u1", ProfileID: "prof-1"})))
	assert.Equal(t, "u:u1",
		streamingRateLimitKey(withClaims(base, &domainuser.Claims{UserID: "u1"})),
		"no profile (legacy token) falls back to user")
	assert.Equal(t, "ip:203.0.113.7",
		streamingRateLimitKey(base), "no claims (share link) falls back to IP")

	// Same raw string as a profile vs a user vs an IP must not share a bucket.
	pk := streamingRateLimitKey(withClaims(base, &domainuser.Claims{ProfileID: "x"}))
	uk := streamingRateLimitKey(withClaims(base, &domainuser.Claims{UserID: "x"}))
	assert.NotEqual(t, pk, uk)
}

// The limiter blocks a single profile's flood with 503+Retry-After (NOT 429),
// and an UNRELATED profile keeps streaming - proving per-profile isolation
// (the multi-profile household case).
func TestStreamingRateLimit_PerProfileIsolationAnd503(t *testing.T) {
	lim := ratelimit.NewWithBurst(3, 0) // 3 tokens, no refill → deterministic
	defer lim.Stop()
	served := 0
	h := streamingRateLimit(lim, nil, func(w http.ResponseWriter, _ *http.Request) {
		served++
		w.WriteHeader(http.StatusOK)
	})

	reqFor := func(profile string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/seg.m4s", nil)
		return withClaims(r, &domainuser.Claims{UserID: "u1", ProfileID: profile})
	}

	// Profile A burns its 3 tokens, the 4th is throttled.
	var lastCode int
	var lastRetry string
	for range 4 {
		w := httptest.NewRecorder()
		h(w, reqFor("A"))
		lastCode = w.Code
		lastRetry = w.Header().Get("Retry-After")
	}
	assert.Equal(t, http.StatusServiceUnavailable, lastCode, "4th request for profile A is throttled with 503")
	assert.Equal(t, "2", lastRetry, "503 carries Retry-After so hls.js retries")
	assert.Equal(t, 3, served, "exactly 3 of A's requests passed")

	// Profile B (same user, different profile) is unaffected - its own bucket.
	w := httptest.NewRecorder()
	h(w, reqFor("B"))
	assert.Equal(t, http.StatusOK, w.Code, "a different profile keeps its full budget")
}

// A refused streaming request must be VISIBLE server-side: the 07-31 cast
// stall investigation lost hours because this 503 left no trace in the logs
// (the refusal existed only client-side). The log line reuses the canonical
// ratelimit refusal shape and adds the bucket, so an operator can tell WHICH
// profile is being throttled behind a shared household IP.
func TestStreamingRateLimit_RefusalIsLogged(t *testing.T) {
	var buf bytes.Buffer
	ratelimit.SetLogger(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer ratelimit.SetLogger(nil)

	lim := ratelimit.NewWithBurst(1, 0) // 1 token, no refill → 2nd request refused
	defer lim.Stop()
	h := streamingRateLimit(lim, nil, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for range 2 {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/seg.m4s", nil)
		h(w, withClaims(r, &domainuser.Claims{UserID: "u1", ProfileID: "prof-throttled"}))
	}

	logged := buf.String()
	require.Contains(t, logged, "rate limit: request refused", "refusal must produce the canonical refusal log line")
	assert.Contains(t, logged, `"bucket":"p:prof-throttled"`, "the log must name the throttled bucket")
	assert.Contains(t, logged, "/api/v1/media/m1/hls/seg.m4s", "the log must carry the refused path")
}

// A nil limiter is a transparent pass-through (no-RL deployment / tests).
func TestStreamingRateLimit_NilIsPassthrough(t *testing.T) {
	served := 0
	h := streamingRateLimit(nil, nil, func(w http.ResponseWriter, _ *http.Request) {
		served++
		w.WriteHeader(http.StatusOK)
	})
	for range 100 {
		w := httptest.NewRecorder()
		h(w, httptest.NewRequest(http.MethodGet, "/x", nil))
		require.Equal(t, http.StatusOK, w.Code)
	}
	assert.Equal(t, 100, served, "nil limiter never blocks")
}
