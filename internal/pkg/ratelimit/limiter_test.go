package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func newTestLimiter(rpm int) *Limiter {
	return &Limiter{
		visitors: make(map[string]*entry),
		rate:     float64(rpm) / 60.0,
		burst:    rpm,
	}
}

func TestRateLimit_AllowsUpToLimit(t *testing.T) {
	l := newTestLimiter(5)
	ip := "10.0.0.1"

	for i := range 5 {
		assert.True(t, l.Allow(ip), "request %d should be allowed", i+1)
	}
}

func TestRateLimit_BlocksOverLimit(t *testing.T) {
	l := newTestLimiter(3)
	ip := "10.0.0.2"

	for range 3 {
		l.Allow(ip)
	}
	assert.False(t, l.Allow(ip), "request over limit should be blocked")
	assert.False(t, l.Allow(ip), "subsequent request should also be blocked")
}

func TestRateLimit_Refills(t *testing.T) {
	l := newTestLimiter(60) // 1 token/sec
	ip := "10.0.0.3"

	for range 60 {
		l.Allow(ip)
	}
	assert.False(t, l.Allow(ip), "should be exhausted")

	l.mu.Lock()
	e := l.visitors[ip]
	e.lastCheck = e.lastCheck.Add(-2 * time.Second)
	l.mu.Unlock()

	assert.True(t, l.Allow(ip), "should have refilled after elapsed time")
}

func TestMiddleware_ExemptPrefixes(t *testing.T) {
	l := newTestLimiter(2)
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	handler := Middleware(l, "/api/v1/playback/progress", "/api/v1/playback/start")(ok)

	// exhaust the limiter
	for range 2 {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest("GET", "/other", nil))
		assert.Equal(t, 200, rr.Code)
	}

	// non-exempt path blocked
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest("GET", "/other", nil))
	assert.Equal(t, 429, rr.Code)

	// exempt paths still allowed
	for _, path := range []string{"/api/v1/playback/progress", "/api/v1/playback/start"} {
		rr = httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest("POST", path, nil))
		assert.Equal(t, 200, rr.Code, "exempt path %s should bypass limiter", path)
	}
}
