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

func TestClientIP_XFFUntrustedProxy(t *testing.T) {
	// XFF from untrusted peer must be ignored
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:9999" // public IP — not a trusted proxy
	req.Header.Set("X-Forwarded-For", "9.9.9.9")
	assert.Equal(t, "1.2.3.4", clientIP(req), "XFF from untrusted peer must be ignored")
}

func TestClientIP_XFFTrustedProxy(t *testing.T) {
	// XFF from trusted proxy (loopback/private) must be honoured
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:9999"
	req.Header.Set("X-Forwarded-For", "9.9.9.9, 172.20.0.1")
	assert.Equal(t, "9.9.9.9", clientIP(req), "leftmost IP from XFF must be used when proxy is trusted")
}

func TestClientIP_XFFSpoofPrevented(t *testing.T) {
	// attacker from public net cannot bypass rate limit by forging XFF
	l := newTestLimiter(2)
	handler := Middleware(l)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))

	for range 2 {
		r := httptest.NewRequest("GET", "/login", nil)
		r.RemoteAddr = "5.5.5.5:1234"
		r.Header.Set("X-Forwarded-For", "1.1.1.1") // attacker rotates IP in XFF
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, r)
	}

	r := httptest.NewRequest("GET", "/login", nil)
	r.RemoteAddr = "5.5.5.5:1234"
	r.Header.Set("X-Forwarded-For", "2.2.2.2") // different forged IP
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, r)
	assert.Equal(t, 429, rr.Code, "attacker should be rate-limited regardless of XFF header")
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
