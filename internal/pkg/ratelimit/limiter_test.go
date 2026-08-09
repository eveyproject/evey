package ratelimit

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

	// advance exactly 0.5s - original yields 0.5 tokens (blocked).
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

// Retry-After must never be 0 or negative: a client honoring it literally
// would retry immediately (or never), defeating the throttle. The clamp was a
// surviving mutant (limiter.go `retryAfter < 1`) - this pins it for both
// refusal writers.
func TestRefusals_ClampRetryAfterToAtLeastOneSecond(t *testing.T) {
	SetLogger(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	t.Cleanup(func() { SetLogger(nil) })

	for _, retryAfter := range []int{0, -3} {
		w := httptest.NewRecorder()
		refuse(w, httptest.NewRequest(http.MethodGet, "/x", nil), retryAfter)
		if got := w.Header().Get("Retry-After"); got != "1" {
			t.Fatalf("refuse(%d): Retry-After = %q, want \"1\"", retryAfter, got)
		}
		ws := httptest.NewRecorder()
		RefuseStreaming(ws, httptest.NewRequest(http.MethodGet, "/hls/x", nil), retryAfter, "p:x")
		if got := ws.Header().Get("Retry-After"); got != "1" {
			t.Fatalf("RefuseStreaming(%d): Retry-After = %q, want \"1\"", retryAfter, got)
		}
	}
	// And an honest value passes through untouched.
	w := httptest.NewRecorder()
	refuse(w, httptest.NewRequest(http.MethodGet, "/x", nil), 2)
	if got := w.Header().Get("Retry-After"); got != "2" {
		t.Fatalf("refuse(2): Retry-After = %q, want \"2\"", got)
	}
}

// A refused request used to leave NO trace at all: an operator investigating
// "the app refuses my login" saw an empty log and concluded the request never
// arrived (prod 2026-07-29, a beta tester locked out and invisible). Every
// block now logs WHO was refused and WHERE, at WARN.
func TestMiddleware_LogsRefusals(t *testing.T) {
	var buf bytes.Buffer
	SetLogger(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { SetLogger(nil) })

	lim := NewWithBurst(1, 0.06) // one token, refill so slow it never returns
	h := MiddlewareFunc(lim, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "203.0.113.7:5000"
	req.Header.Set("User-Agent", "EveyMobile/0.0.1-beta.19 (Android 13; TestPhone)")

	first := httptest.NewRecorder()
	h(first, req)
	if first.Code != http.StatusOK {
		t.Fatalf("first call = %d, want 200", first.Code)
	}
	if buf.Len() != 0 {
		t.Fatalf("an ALLOWED request must not log: %s", buf.String())
	}

	second := httptest.NewRecorder()
	h(second, req)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second call = %d, want 429", second.Code)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("a 429 must carry Retry-After so the client can say when to try again")
	}
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("no parseable log line for the refusal: %v (raw=%q)", err, buf.String())
	}
	if line["level"] != "WARN" {
		t.Fatalf("level = %v, want WARN", line["level"])
	}
	for field, want := range map[string]any{
		"client_ip": "203.0.113.7",
		"path":      "/api/v1/auth/login",
	} {
		if line[field] != want {
			t.Fatalf("log field %q = %v, want %v (line=%v)", field, line[field], want, line)
		}
	}
	if dev, _ := line["device"].(string); !strings.Contains(dev, "EveyMobile") {
		t.Fatalf("the refused client's user agent must be logged, got %v", line["device"])
	}
}

// A refused user must be told HOW LONG - "too many requests" with no delay
// leaves them tapping blindly (user report 2026-07-29). Retry-After carries
// the real wait computed from the bucket, not a fixed guess.
func TestRetryAfter_ReportsTheRealWait(t *testing.T) {
	lim := NewWithBurst(1, 30) // 30 tokens per MINUTE: one token every 2s
	if !lim.Allow("1.2.3.4") {
		t.Fatal("first call must pass")
	}
	if lim.Allow("1.2.3.4") {
		t.Fatal("second call must be refused")
	}
	// EXACT, not a 1..3 window: mutation testing showed that window swallowed
	// every arithmetic change in RetryAfter (swap the division for a
	// multiplication, flip `1 - tokens`, move the `>= 1` boundary - all passed).
	// The value a client is told to wait for is the whole point of the field.
	if got := lim.RetryAfter("1.2.3.4"); got != 2 {
		t.Fatalf("RetryAfter = %ds, want exactly 2s (30/min = one token every 2s)", got)
	}
	// An unknown/allowed client owes nothing.
	if n := lim.RetryAfter("9.9.9.9"); n != 0 {
		t.Fatalf("RetryAfter for an unseen client = %d, want 0", n)
	}
}

// The refusal body must carry the delay in a form a client can act on: the
// mobile app turns it into "try again in N minutes".
func TestMiddleware_RefusalBodyCarriesRetryAfter(t *testing.T) {
	lim := NewWithBurst(1, 30)
	h := MiddlewareFunc(lim, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "203.0.113.9:5000"
	h(httptest.NewRecorder(), req)

	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	var body struct {
		Message           string `json:"message"`
		RetryAfterSeconds int    `json:"retryAfterSeconds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unparseable refusal body %q: %v", rec.Body.String(), err)
	}
	if body.RetryAfterSeconds < 1 {
		t.Fatalf("retryAfterSeconds = %d, want the real wait", body.RetryAfterSeconds)
	}
	if h := rec.Header().Get("Retry-After"); h != strconv.Itoa(body.RetryAfterSeconds) {
		t.Fatalf("Retry-After header = %q, body says %d - they must agree", h, body.RetryAfterSeconds)
	}
}

// This is the exact production failure mode (2026-07-29): distinct clients
// behind the SAME trusted reverse proxy must land in distinct buckets. If the
// XFF reasoning were dropped (e.g. clientIP() always returned the peer, or
// ignored X-Forwarded-For), both requests below would resolve to the same
// peer IP and this test goes red - one client's login attempts would exhaust
// a bucket shared by every other client behind the proxy.
func TestClientIP_DistinctXFFsFromTrustedPeerGetSeparateBuckets(t *testing.T) {
	t.Parallel()
	resolve := func(xff string) string {
		req := httptest.NewRequest("GET", "/login", nil)
		req.RemoteAddr = "10.0.0.9:5000" // 10.0.0.0/8 is trusted by default
		req.Header.Set("X-Forwarded-For", xff)
		return clientIP(req)
	}

	ipA := resolve("198.51.100.11")
	ipB := resolve("198.51.100.22")
	require.NotEqual(t, ipA, ipB, "two distinct clients behind the same trusted proxy must resolve to distinct IPs")

	lim := newTestLimiter(1)
	assert.True(t, lim.Allow(ipA))
	assert.True(t, lim.Allow(ipB), "a second distinct client must not be blocked by the first client's exhausted bucket")
	assert.False(t, lim.Allow(ipA), "the first client's own bucket should now be exhausted")
}

// The refusal log must carry enough to diagnose WHY a bucket was keyed the
// way it was, not just the outcome - an operator investigating "everyone gets
// rate-limited together" needs the peer address, whether it was trusted, and
// what (if anything) arrived in X-Forwarded-For.
func TestMiddleware_LogsRefusalReasoning(t *testing.T) {
	var buf bytes.Buffer
	SetLogger(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { SetLogger(nil) })

	lim := NewWithBurst(1, 0.06)
	h := MiddlewareFunc(lim, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "10.0.0.9:5000"
	req.Header.Set("X-Forwarded-For", "198.51.100.5")

	h(httptest.NewRecorder(), req)
	h(httptest.NewRecorder(), req)

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("no parseable log line for the refusal: %v (raw=%q)", err, buf.String())
	}
	for field, want := range map[string]any{
		"client_ip":    "198.51.100.5",
		"peer_ip":      "10.0.0.9",
		"peer_trusted": true,
		"xff":          "198.51.100.5",
	} {
		if line[field] != want {
			t.Fatalf("log field %q = %v, want %v (line=%v)", field, line[field], want, line)
		}
	}
}

// The misconfiguration this warns about (2026-07-29): the reverse proxy talks
// to the backend over loopback, but EVEY_TRUSTED_PROXIES was narrowed to a
// list that no longer covers loopback - so the proxy's own X-Forwarded-For is
// ignored and every client behind it resolves to the SAME 127.0.0.1 bucket.
func TestWarnMisroutedProxy_FiresOnLoopbackWithIgnoredXFF(t *testing.T) {
	var buf bytes.Buffer
	SetLogger(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { SetLogger(nil) })
	ResetMisconfigWarnForTest()
	t.Cleanup(ResetMisconfigWarnForTest)

	orig := trustedProxyNets
	t.Cleanup(func() {
		trustedProxyMu.Lock()
		trustedProxyNets = orig
		trustedProxyMu.Unlock()
	})
	// Narrow the trusted set so it no longer covers loopback - the exact
	// misconfiguration a custom EVEY_TRUSTED_PROXIES can introduce.
	SetTrustedProxies([]string{"203.0.113.0/24"})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "127.0.0.1:5000"
	req.Header.Set("X-Forwarded-For", "198.51.100.5")

	res := resolveClientIPReasoned(req)
	WarnMisroutedProxy(res)

	if !strings.Contains(buf.String(), "EVEY_TRUSTED_PROXIES") {
		t.Fatalf("expected a WARN naming EVEY_TRUSTED_PROXIES as the cause, got %q", buf.String())
	}
}

// Correctly configured (proxy in the trusted set): no warning, no noise.
func TestWarnMisroutedProxy_SilentWhenTrustedProxyCoversPeer(t *testing.T) {
	var buf bytes.Buffer
	SetLogger(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { SetLogger(nil) })
	ResetMisconfigWarnForTest()
	t.Cleanup(ResetMisconfigWarnForTest)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "127.0.0.1:5000" // in the default trusted set
	req.Header.Set("X-Forwarded-For", "198.51.100.5")

	res := resolveClientIPReasoned(req)
	WarnMisroutedProxy(res)

	if buf.Len() != 0 {
		t.Fatalf("correctly configured trusted proxy must not warn: %s", buf.String())
	}
}

// Also silent when no XFF ever arrived: that's a different failure (no proxy
// header at all), not the "ignored despite present" misconfiguration this
// warning targets.
func TestWarnMisroutedProxy_SilentWithoutXFF(t *testing.T) {
	var buf bytes.Buffer
	SetLogger(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { SetLogger(nil) })
	ResetMisconfigWarnForTest()
	t.Cleanup(ResetMisconfigWarnForTest)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "127.0.0.1:5000"
	// no X-Forwarded-For set

	res := resolveClientIPReasoned(req)
	WarnMisroutedProxy(res)

	if buf.Len() != 0 {
		t.Fatalf("no XFF at all must not trigger the trusted-proxy misconfig warning: %s", buf.String())
	}
}

// One-shot: repeated refusals from misconfigured proxies must not spam the log.
func TestWarnMisroutedProxy_FiresOnlyOnce(t *testing.T) {
	var buf bytes.Buffer
	SetLogger(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { SetLogger(nil) })
	ResetMisconfigWarnForTest()
	t.Cleanup(ResetMisconfigWarnForTest)

	orig := trustedProxyNets
	t.Cleanup(func() {
		trustedProxyMu.Lock()
		trustedProxyNets = orig
		trustedProxyMu.Unlock()
	})
	SetTrustedProxies([]string{"203.0.113.0/24"})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "127.0.0.1:5000"
	req.Header.Set("X-Forwarded-For", "198.51.100.5")

	res := resolveClientIPReasoned(req)
	WarnMisroutedProxy(res)
	WarnMisroutedProxy(res)
	WarnMisroutedProxy(res)

	// "EVEY_TRUSTED_PROXIES" itself appears twice PER line (diagnosis + fix), so
	// count log lines (records), not substring hits.
	count := strings.Count(buf.String(), `"msg":"ratelimit: client resolved to loopback`)
	if count != 1 {
		t.Fatalf("the misconfig warning must fire once per process, got %d log lines: %s", count, buf.String())
	}
}

// RetryAfter's arithmetic, across several rates.
//
// One rate cannot distinguish a division from a multiplication (at rate 1.0 they
// agree), so a single case is exactly what let the mutants live. These four
// disagree under every mutation gremlins produced for this function.
func TestRetryAfter_ArithmeticAcrossRates(t *testing.T) {
	for _, tt := range []struct {
		rpm  float64
		want int // ceil(1 / (rpm/60)) seconds until the next token
	}{
		{rpm: 60, want: 1}, // one token per second
		{rpm: 30, want: 2},
		{rpm: 15, want: 4},
		{rpm: 12, want: 5},
	} {
		t.Run(strconv.Itoa(int(tt.rpm))+"rpm", func(t *testing.T) {
			lim := NewWithBurst(1, tt.rpm)
			if !lim.Allow("c") {
				t.Fatal("the first call must consume the single token")
			}
			if lim.Allow("c") {
				t.Fatal("the second call must be refused")
			}
			if got := lim.RetryAfter("c"); got != tt.want {
				t.Errorf("RetryAfter at %v/min = %ds, want %ds", tt.rpm, got, tt.want)
			}
		})
	}
}

// A client that still holds a whole token owes nothing - the boundary is `>= 1`,
// not `> 1`, and moving it by one is a silent change that made a refused client
// retry immediately into the wall.
func TestRetryAfter_BoundaryAtOneWholeToken(t *testing.T) {
	lim := NewWithBurst(2, 60) // burst 2, one token per second
	if !lim.Allow("c") {
		t.Fatal("first call must pass")
	}
	// One token left: nothing owed.
	if got := lim.RetryAfter("c"); got != 0 {
		t.Errorf("with a full token in hand, RetryAfter = %d, want 0", got)
	}
	if !lim.Allow("c") {
		t.Fatal("second call must pass, the burst is 2")
	}
	// Now empty: a real wait.
	if got := lim.RetryAfter("c"); got != 1 {
		t.Errorf("with an empty bucket at 60/min, RetryAfter = %d, want 1", got)
	}
}

// A limiter with no rate cannot compute a wait - dividing by it would be a
// division by zero, so the guard returns 0 rather than +Inf seconds.
func TestRetryAfter_ZeroRateOwesNothing(t *testing.T) {
	lim := NewWithBurst(1, 0)
	lim.Allow("c")
	if got := lim.RetryAfter("c"); got != 0 {
		t.Errorf("RetryAfter with a zero rate = %d, want 0", got)
	}
}

// SetLimit converts requests-per-MINUTE into a per-second rate. Nothing asserted
// that conversion, so the /60 could become anything and every test stayed green.
func TestSetLimit_ConvertsPerMinuteToPerSecond(t *testing.T) {
	lim := NewWithBurst(1, 60)
	if !lim.Allow("c") {
		t.Fatal("first call must pass")
	}
	lim.SetLimit(30) // 30/min -> one token every 2s
	if got := lim.RetryAfter("c"); got != 2 {
		t.Errorf("after SetLimit(30), RetryAfter = %ds, want 2s", got)
	}
	lim.SetLimit(12) // 12/min -> one token every 5s
	if got := lim.RetryAfter("c"); got != 5 {
		t.Errorf("after SetLimit(12), RetryAfter = %ds, want 5s", got)
	}
}

// Every unseen key allocates an entry, and the sweep only runs every 5 minutes:
// a caller rotating source addresses (one /64 of IPv6 holds 2^64) could grow
// the table by roughly one entry per request until then, on each of the several
// limiters the server runs. The table is now bounded.
func TestLimiter_VisitorTableIsBounded(t *testing.T) {
	l := New(60)
	defer l.Stop()

	for i := range maxVisitors + 5_000 {
		l.Allow("2001:db8::" + strconv.Itoa(i))
	}

	l.mu.Lock()
	n := len(l.visitors)
	l.mu.Unlock()
	if n > maxVisitors {
		t.Errorf("table holds %d keys, above the %d cap", n, maxVisitors)
	}
}

// The cap must not cost an active caller its bucket while an idle one keeps
// theirs: eviction takes the coldest entry, so a client that just called is the
// last to go.
func TestLimiter_EvictionTakesTheColdestEntry(t *testing.T) {
	l := New(60)
	defer l.Stop()

	l.mu.Lock()
	old := time.Now().Add(-time.Minute)
	for i := range maxVisitors {
		l.visitors["cold-"+strconv.Itoa(i)] = &entry{tokens: 1, lastCheck: old}
	}
	// One caller that just spoke - younger than every other entry.
	l.visitors["warm"] = &entry{tokens: 1, lastCheck: time.Now()}
	delete(l.visitors, "cold-0") // back to exactly the cap
	l.mu.Unlock()

	l.Allow("newcomer")

	l.mu.Lock()
	_, warmSurvived := l.visitors["warm"]
	_, newcomerAdmitted := l.visitors["newcomer"]
	n := len(l.visitors)
	l.mu.Unlock()

	if !warmSurvived {
		t.Error("the most recently active caller was evicted: eviction must take the coldest")
	}
	if !newcomerAdmitted {
		t.Error("the newcomer got no bucket at all")
	}
	if n > maxVisitors {
		t.Errorf("table holds %d keys, above the %d cap", n, maxVisitors)
	}
}

// Eviction claims to drop "everything the periodic sweep would have dropped
// anyway" before touching a live entry. Mutation testing (2026-08-01) showed
// nothing verified that: the retention comparison could be inverted or moved
// and every test stayed green, leaving eviction to throw out ACTIVE callers
// one at a time while a table full of stale ones sat untouched.
func TestLimiter_EvictionDropsEveryStaleEntryBeforeTouchingALiveOne(t *testing.T) {
	l := New(60)
	defer l.Stop()

	stale := time.Now().Add(-visitorRetention - time.Minute)
	l.mu.Lock()
	for i := range maxVisitors - 1 {
		l.visitors["stale-"+strconv.Itoa(i)] = &entry{tokens: 1, lastCheck: stale}
	}
	l.visitors["live"] = &entry{tokens: 1, lastCheck: time.Now()}
	l.mu.Unlock()

	l.Allow("newcomer") // table is at the cap: this triggers eviction

	l.mu.Lock()
	n := len(l.visitors)
	_, liveSurvived := l.visitors["live"]
	_, newcomerAdmitted := l.visitors["newcomer"]
	l.mu.Unlock()

	if !liveSurvived {
		t.Error("the only active caller was evicted while the table was full of stale entries")
	}
	if !newcomerAdmitted {
		t.Error("the newcomer got no bucket")
	}
	// The FULL purge moved to the periodic sweeper: doing it here scanned the
	// whole table under the lock every request takes. On the hot path an
	// eviction now samples and frees ONE slot. What still holds - and is the
	// point - is the victim choice: a stale entry goes before the active caller.
	if n > maxVisitors {
		t.Errorf("table grew past its cap: %d > %d", n, maxVisitors)
	}
	if n != maxVisitors {
		t.Errorf("table at %d entries: an eviction must free exactly one slot and "+
			"admit the newcomer, not purge the whole table under the lock", n)
	}
}

// The token bucket must not let an idle caller BANK requests. Without the cap
// at burst, someone who waits an hour would come back holding thousands of
// tokens and could fire them all at once - the exact burst the limiter exists
// to prevent, handed over for free by patience.
//
// Mutation testing (2026-08-01): the cap comparison survived the suite, so it
// could have been loosened or removed unnoticed.
func TestLimiter_IdleCallerCannotBankMoreThanTheBurst(t *testing.T) {
	l := New(60) // burst 60, refill 1/s
	defer l.Stop()

	l.Allow("ip") // creates the entry
	// Pretend that caller went quiet for an hour: 3600 tokens' worth of refill.
	l.mu.Lock()
	l.visitors["ip"].lastCheck = time.Now().Add(-time.Hour)
	l.visitors["ip"].tokens = 0
	l.mu.Unlock()

	allowed := 0
	for range 200 {
		if l.Allow("ip") {
			allowed++
		}
	}
	// 60 from the capped bucket, plus the handful the refill adds during the
	// loop itself. Anything near 200 means the cap is gone.
	if allowed > 70 {
		t.Errorf("an idle caller fired %d requests in a row; the bucket is capped at %d", allowed, 60)
	}
	if allowed < 50 {
		t.Errorf("only %d allowed: the refill after an hour idle should have filled the bucket", allowed)
	}
}

// RetryAfter must return a delay a client can act on: zero while tokens
// remain, and a positive number of seconds once the bucket is empty. Returning
// 0 on an empty bucket is the "retry blindly and stay blocked" report of
// 2026-07-29.
func TestLimiter_RetryAfterIsActionableOnceDrained(t *testing.T) {
	l := New(60)
	defer l.Stop()

	if got := l.RetryAfter("fresh"); got != 0 {
		t.Errorf("unknown caller: RetryAfter = %d, want 0", got)
	}
	for range 60 {
		l.Allow("drained")
	}
	if l.Allow("drained") {
		t.Fatal("premise: the bucket should be empty after burst requests")
	}
	if got := l.RetryAfter("drained"); got < 1 {
		t.Errorf("RetryAfter = %d on an empty bucket, want at least 1 second", got)
	}
}
