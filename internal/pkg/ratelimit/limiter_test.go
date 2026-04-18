package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLimiter(rpm int) *Limiter {
	return &Limiter{
		visitors: make(map[string]*entry),
		rate:     float64(rpm) / 60.0,
		burst:    rpm,
		done:     make(chan struct{}),
	}
}

func TestRateLimit_AllowsUpToLimit(t *testing.T) {
	t.Parallel()
	l := newTestLimiter(5)
	ip := "10.0.0.1"

	for i := range 5 {
		assert.True(t, l.Allow(ip), "request %d should be allowed", i+1)
	}
}

func TestRateLimit_BlocksOverLimit(t *testing.T) {
	t.Parallel()
	l := newTestLimiter(3)
	ip := "10.0.0.2"

	for range 3 {
		l.Allow(ip)
	}
	assert.False(t, l.Allow(ip), "request over limit should be blocked")
	assert.False(t, l.Allow(ip), "subsequent request should also be blocked")
}

func TestRateLimit_Refills(t *testing.T) {
	t.Parallel()
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

func TestRateLimit_RefillCapsAtBurst(t *testing.T) {
	t.Parallel()
	l := newTestLimiter(5)
	ip := "10.0.0.50"

	l.Allow(ip) // first request, tokens = burst-1

	// simulate long idle so refill would exceed burst
	l.mu.Lock()
	e := l.visitors[ip]
	e.lastCheck = e.lastCheck.Add(-1 * time.Hour)
	l.mu.Unlock()

	// should still only allow burst requests
	for i := range 5 {
		assert.True(t, l.Allow(ip), "request %d should be allowed after refill cap", i+1)
	}
	assert.False(t, l.Allow(ip), "should be blocked after burst exhausted")
}

func TestRateLimit_IndependentIPs(t *testing.T) {
	t.Parallel()
	l := newTestLimiter(2)

	for range 2 {
		l.Allow("ip-a")
	}
	assert.False(t, l.Allow("ip-a"))
	assert.True(t, l.Allow("ip-b"), "different IP should have its own bucket")
}

func TestNew_StartsCleanup(t *testing.T) {
	t.Parallel()
	l := New(60)
	defer l.Stop()
	assert.NotNil(t, l.visitors)
	assert.True(t, l.Allow("1.2.3.4"))
}

func TestNewWithBurst_CustomRates(t *testing.T) {
	t.Parallel()
	l := NewWithBurst(3, 120.0)
	defer l.Stop()

	for range 3 {
		assert.True(t, l.Allow("1.2.3.4"))
	}
	assert.False(t, l.Allow("1.2.3.4"), "should block after custom burst")
}

func TestStop_HaltsCleanup(t *testing.T) {
	t.Parallel()
	l := New(60)
	l.Stop()
	// calling Stop on already-stopped should not panic
	// (channel is already closed, but we just verify no hang)
}

func TestCleanup_RemovesStaleEntries(t *testing.T) {
	t.Parallel()
	l := newTestLimiter(60)

	l.Allow("stale-ip")
	l.Allow("fresh-ip")

	l.mu.Lock()
	l.visitors["stale-ip"].lastCheck = time.Now().Add(-15 * time.Minute)
	l.mu.Unlock()

	// run cleanup inline
	l.mu.Lock()
	for ip, e := range l.visitors {
		if time.Since(e.lastCheck) > 10*time.Minute {
			delete(l.visitors, ip)
		}
	}
	l.mu.Unlock()

	l.mu.Lock()
	_, hasStale := l.visitors["stale-ip"]
	_, hasFresh := l.visitors["fresh-ip"]
	l.mu.Unlock()

	assert.False(t, hasStale, "stale entry should be removed")
	assert.True(t, hasFresh, "fresh entry should remain")
}

func TestCleanup_StopsOnDone(t *testing.T) {
	t.Parallel()
	l := &Limiter{
		visitors: make(map[string]*entry),
		rate:     1.0,
		burst:    60,
		done:     make(chan struct{}),
	}
	close(l.done)
	// cleanup should return immediately when done is closed
	done := make(chan struct{})
	go func() {
		l.cleanup()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup did not exit after done closed")
	}
}

func TestRateLimit_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	l := newTestLimiter(1000)
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for range 20 {
				l.Allow("concurrent-ip")
			}
		}(i)
	}
	wg.Wait()
}

func TestClientIP_Exported(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	assert.Equal(t, "1.2.3.4", ClientIP(req))
}

func TestClientIP_XFFUntrustedProxy(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:9999"
	req.Header.Set("X-Forwarded-For", "9.9.9.9")
	assert.Equal(t, "1.2.3.4", clientIP(req), "XFF from untrusted peer must be ignored")
}

func TestClientIP_XFFTrustedProxy(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:9999"
	req.Header.Set("X-Forwarded-For", "9.9.9.9, 172.20.0.1")
	assert.Equal(t, "9.9.9.9", clientIP(req), "leftmost IP from XFF must be used when proxy is trusted")
}

func TestClientIP_XFFTrustedProxySingleIP(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:8080"
	req.Header.Set("X-Forwarded-For", "4.4.4.4")
	assert.Equal(t, "4.4.4.4", clientIP(req), "single XFF IP from trusted proxy")
}

func TestClientIP_XFFTrustedProxyInvalidXFF(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:9999"
	req.Header.Set("X-Forwarded-For", "not-an-ip")
	assert.Equal(t, "127.0.0.1", clientIP(req), "invalid XFF falls back to peer")
}

func TestClientIP_NoPort(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4" // no port
	assert.Equal(t, "1.2.3.4", clientIP(req))
}

func TestClientIP_TrustedProxyNoXFF(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:9999"
	// no XFF header
	assert.Equal(t, "192.168.1.1", clientIP(req))
}

func TestClientIP_XFFSpoofPrevented(t *testing.T) {
	t.Parallel()
	l := newTestLimiter(2)
	handler := Middleware(l)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))

	for range 2 {
		r := httptest.NewRequest("GET", "/login", nil)
		r.RemoteAddr = "5.5.5.5:1234"
		r.Header.Set("X-Forwarded-For", "1.1.1.1")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, r)
	}

	r := httptest.NewRequest("GET", "/login", nil)
	r.RemoteAddr = "5.5.5.5:1234"
	r.Header.Set("X-Forwarded-For", "2.2.2.2")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, r)
	assert.Equal(t, 429, rr.Code, "attacker should be rate-limited regardless of XFF header")
}

func TestMiddleware_ExemptPrefixes(t *testing.T) {
	t.Parallel()
	l := newTestLimiter(2)
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	handler := Middleware(l, "/api/v1/playback/progress", "/api/v1/playback/start")(ok)

	for range 2 {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest("GET", "/other", nil))
		assert.Equal(t, 200, rr.Code)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest("GET", "/other", nil))
	assert.Equal(t, 429, rr.Code)

	for _, path := range []string{"/api/v1/playback/progress", "/api/v1/playback/start"} {
		rr = httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest("POST", path, nil))
		assert.Equal(t, 200, rr.Code, "exempt path %s should bypass limiter", path)
	}
}

func TestMiddlewareFunc_AllowsAndBlocks(t *testing.T) {
	t.Parallel()
	l := newTestLimiter(2)
	called := 0
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(200)
	})
	handler := MiddlewareFunc(l, inner)

	// 2 allowed
	for range 2 {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/login", nil)
		req.RemoteAddr = "3.3.3.3:1234"
		handler.ServeHTTP(rr, req)
		assert.Equal(t, 200, rr.Code)
	}
	require.Equal(t, 2, called)

	// 3rd blocked
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/login", nil)
	req.RemoteAddr = "3.3.3.3:1234"
	handler.ServeHTTP(rr, req)
	assert.Equal(t, 429, rr.Code)
	assert.Equal(t, 2, called, "inner handler should not be called when rate limited")
}

// TestLimiter_RateExactDivisionByMinute kills ARITHMETIC_BASE on
// `requestsPerMinute / 60.0` (limiter.go:28) by asserting refill cadence
// matches rpm/60 tokens/sec, distinguishing /, *, +, - variants.
func TestLimiter_RateExactDivisionByMinute(t *testing.T) {
	t.Parallel()
	l := New(60) // rate = 1 token/sec
	defer l.Stop()
	ip := "rate-div-ip"

	for range 60 {
		l.Allow(ip)
	}
	require.False(t, l.Allow(ip), "should be exhausted")

	// advance exactly 0.5s — original yields 0.5 tokens (blocked).
	// mutated * (rate=3600): allowed. mutated + (rate=61): allowed.
	l.mu.Lock()
	l.visitors[ip].lastCheck = l.visitors[ip].lastCheck.Add(-500 * time.Millisecond)
	l.mu.Unlock()
	assert.False(t, l.Allow(ip), "0.5s elapsed @ 1tok/s should still be blocked (kills *,+ on /60)")

	// advance another 1.0s. original yields 1 token (allowed once, then blocked).
	// mutated - (rate=-59): tokens drop further, blocked.
	l.mu.Lock()
	l.visitors[ip].lastCheck = l.visitors[ip].lastCheck.Add(-1 * time.Second)
	l.mu.Unlock()
	assert.True(t, l.Allow(ip), "1s elapsed @ 1tok/s should allow once (kills - on /60)")
	assert.False(t, l.Allow(ip), "second immediate request should be blocked")
}

// TestLimiter_NewWithBurstRateExactDivision kills ARITHMETIC_BASE on
// `refilledPerMinute / 60.0` (limiter.go:41).
func TestLimiter_NewWithBurstRateExactDivision(t *testing.T) {
	t.Parallel()
	l := NewWithBurst(2, 60.0) // burst=2, rate=1 tok/s
	defer l.Stop()
	ip := "burst-rate-ip"

	l.Allow(ip)
	l.Allow(ip)
	require.False(t, l.Allow(ip), "should be exhausted at burst=2")

	l.mu.Lock()
	l.visitors[ip].lastCheck = l.visitors[ip].lastCheck.Add(-500 * time.Millisecond)
	l.mu.Unlock()
	assert.False(t, l.Allow(ip), "0.5s @ 1tok/s should be blocked (kills *,+ on /60)")

	l.mu.Lock()
	l.visitors[ip].lastCheck = l.visitors[ip].lastCheck.Add(-1 * time.Second)
	l.mu.Unlock()
	assert.True(t, l.Allow(ip), "1s @ 1tok/s should allow once (kills - on /60)")
	assert.False(t, l.Allow(ip))
}

// TestLimiter_RefillIsMultiplicative kills ARITHMETIC_BASE on
// `elapsed * l.rate` (limiter.go:73). With rate=2/s and elapsed=3s,
// original yields exactly 6 tokens; +(=5), -(=1), /(=1.5) all differ.
func TestLimiter_RefillIsMultiplicative(t *testing.T) {
	t.Parallel()
	l := New(120) // rate = 2 tok/s, burst=120
	defer l.Stop()
	ip := "mul-ip"

	for range 120 {
		l.Allow(ip)
	}
	require.False(t, l.Allow(ip), "exhausted")

	l.mu.Lock()
	l.visitors[ip].lastCheck = l.visitors[ip].lastCheck.Add(-3 * time.Second)
	l.mu.Unlock()

	allowed := 0
	for range 6 {
		if l.Allow(ip) {
			allowed++
		}
	}
	assert.Equal(t, 6, allowed, "exactly 6 refilled (kills + - / on elapsed*rate)")
	assert.False(t, l.Allow(ip), "7th should be blocked")
}

// TestLimiter_AllowsAtExactlyOneToken kills CONDITIONALS_BOUNDARY on
// `e.tokens < 1` (limiter.go:79). With tokens == 1.0 exactly, original
// (`<`) allows; mutated (`<=`) denies.
func TestLimiter_AllowsAtExactlyOneToken(t *testing.T) {
	t.Parallel()
	l := New(60) // rate=1/s, burst=60
	defer l.Stop()
	ip := "boundary-one-ip"

	for range 60 {
		l.Allow(ip)
	}
	require.False(t, l.Allow(ip))

	// Pin state: tokens=1.0 exactly, rate=0 so refill contributes 0,
	// lastCheck=now so elapsed ≈ 0. Net: tokens stays at 1.0 entering the
	// boundary check. Original `<1`: false → allow. Mutated `<=1`: true → deny.
	l.mu.Lock()
	l.rate = 0
	l.visitors[ip].tokens = 1.0
	l.visitors[ip].lastCheck = time.Now()
	l.mu.Unlock()
	assert.True(t, l.Allow(ip), "tokens==1.0 must be allowed (kills <= mutation on <1)")
}

func TestMiddleware_NoExemptPrefixes(t *testing.T) {
	t.Parallel()
	l := newTestLimiter(1)
	handler := Middleware(l)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest("GET", "/any", nil))
	assert.Equal(t, 200, rr.Code)

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest("GET", "/any", nil))
	assert.Equal(t, 429, rr.Code)
}

func TestSetTrustedProxies_DisablesXFFOnUntrustedPeer(t *testing.T) {
	// Snapshot state so the test doesn't leak into sibling tests.
	orig := trustedProxyNets
	t.Cleanup(func() {
		trustedProxyMu.Lock()
		trustedProxyNets = orig
		trustedProxyMu.Unlock()
	})

	SetTrustedProxies([]string{"203.0.113.0/24"}) // only this range is trusted

	// Peer is loopback (NOT in trusted list anymore) -> XFF should be ignored.
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")

	got := clientIP(req)
	if got != "127.0.0.1" {
		t.Fatalf("untrusted peer should ignore XFF: got %s want 127.0.0.1", got)
	}
}

func TestSetTrustedProxies_AcceptsXFFFromTrustedPeer(t *testing.T) {
	orig := trustedProxyNets
	t.Cleanup(func() {
		trustedProxyMu.Lock()
		trustedProxyNets = orig
		trustedProxyMu.Unlock()
	})

	SetTrustedProxies([]string{"203.0.113.0/24"})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.7:1234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")

	got := clientIP(req)
	if got != "1.2.3.4" {
		t.Fatalf("trusted peer should pass through XFF: got %s want 1.2.3.4", got)
	}
}

func TestSetTrustedProxies_InvalidCIDRSkipped(t *testing.T) {
	orig := trustedProxyNets
	t.Cleanup(func() {
		trustedProxyMu.Lock()
		trustedProxyNets = orig
		trustedProxyMu.Unlock()
	})

	SetTrustedProxies([]string{"not-a-cidr", "10.0.0.0/8"})

	if n := len(trustedProxyNets); n != 1 {
		t.Fatalf("invalid CIDR should be skipped: got %d trusted nets, want 1", n)
	}
}
