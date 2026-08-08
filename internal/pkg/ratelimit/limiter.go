package ratelimit

import (
	"github.com/evey-project/evey/internal/pkg/bgloop"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type entry struct {
	tokens    float64
	lastCheck time.Time
}

type Limiter struct {
	mu       sync.Mutex
	visitors map[string]*entry
	rate     float64 // tokens per second
	burst    int
	done     chan struct{}
}

func New(requestsPerMinute int) *Limiter {
	l := &Limiter{
		visitors: make(map[string]*entry),
		rate:     float64(requestsPerMinute) / 60.0,
		burst:    requestsPerMinute,
		done:     make(chan struct{}),
	}
	go l.cleanup()
	return l
}

// NewWithBurst creates a limiter with a custom burst size and refill rate.
// burstSize: max allowed at once; refilledPerMinute: how fast tokens refill.
func NewWithBurst(burstSize int, refilledPerMinute float64) *Limiter {
	l := &Limiter{
		visitors: make(map[string]*entry),
		rate:     refilledPerMinute / 60.0,
		burst:    burstSize,
		done:     make(chan struct{}),
	}
	go l.cleanup()
	return l
}

func (l *Limiter) SetLimit(requestsPerMinute int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rate = float64(requestsPerMinute) / 60.0
	l.burst = requestsPerMinute
	slog.Info("ratelimit: updated limit", slog.Int("rpm", requestsPerMinute))
}

func (l *Limiter) Stop() {
	close(l.done)
}

// maxVisitors bounds the per-key table. Every unseen key allocates an entry,
// and the sweep only runs every 5 minutes with a 10-minute idle rule - so until
// 2026-08-01 a caller rotating source addresses (a /64 of IPv6 is 2^64 of them,
// and each limiter keeps its own table) could grow it by roughly one entry per
// request for ten minutes with nothing to stop it.
//
// 50k entries is far beyond any real household - a self-hosted server sees tens
// of clients - while capping one table at a couple of megabytes.
const maxVisitors = 50_000

// evictSample bounds the work of ONE eviction. The full retention pass lives in
// the periodic sweeper, off the hot path; here we sample. Go randomises map
// iteration order, so these few entries are a fair draw. Approximate by choice:
// the goal is bounding memory, not electing the exact oldest key, and a full
// scan would cost more than it buys.
const evictSample = 64

// evictScanned counts entries examined by the last eviction, so a test can
// check that this cost does NOT track table size.
var evictScanned int

func (l *Limiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	e, ok := l.visitors[ip]
	if !ok {
		if len(l.visitors) >= maxVisitors {
			l.evictLocked(now)
		}
		l.visitors[ip] = &entry{tokens: float64(l.burst) - 1, lastCheck: now}
		return true
	}

	elapsed := now.Sub(e.lastCheck).Seconds()
	e.tokens += elapsed * l.rate
	if e.tokens > float64(l.burst) {
		e.tokens = float64(l.burst)
	}
	e.lastCheck = now

	if e.tokens < 1 {
		return false
	}
	e.tokens--
	return true
}

// RetryAfter returns how many seconds ip must wait for its next token, or 0
// when it can call right now. A refusal without a delay is unactionable: the
// user retries blindly and stays blocked (report 2026-07-29).
func (l *Limiter) RetryAfter(ip string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.visitors[ip]
	if !ok || l.rate <= 0 {
		return 0
	}
	tokens := e.tokens + time.Since(e.lastCheck).Seconds()*l.rate
	if tokens >= 1 {
		return 0
	}
	secs := (1 - tokens) / l.rate
	return int(math.Ceil(secs))
}

// evictLocked frees ONE slot in a full table, examining at most evictSample
// entries. Purging every stale entry belongs to the periodic sweeper: doing it
// here scanned the whole table under l.mu - the lock every request takes - so a
// caller rotating addresses paid one packet and cost the server 50k iterations.
//
// Victim preference:
//  1. a stale entry met in the sample - net win, stop there;
//  2. else the coldest with a token left: it would be allowed anyway, so
//     evicting it hands back no permission;
//  3. only as a last resort, the coldest of any state.
//
// 2 before 3 is not cosmetic. A caller refused and then quiet becomes the
// coldest entry as soon as the table fills with fresh addresses; evicting it
// hands it a full bucket on its next request, undoing the very throttling just
// applied. The "an active caller is protected by its lastCheck" guarantee only
// covers the one that retries.
//
// None of this claims to stop an attacker rotating addresses: the per-key model
// already hands them a fresh bucket per address, cap or no cap. The cap decides
// what that costs us in memory; this policy decides who pays for it.
//
// Called with l.mu held.
func (l *Limiter) evictLocked(now time.Time) {
	// Two candidates. solvent = coldest entry still holding a token (evicting it
	// hands back nothing). any = coldest of any state, the fallback for a sample
	// containing only callers in debt.
	solventKey, solvent := "", time.Time{}
	anyKey, any := "", time.Time{}
	scanned := 0
	for ip, e := range l.visitors {
		scanned++
		// A stale entry is a net win: take it and stop.
		if now.Sub(e.lastCheck) > visitorRetention {
			delete(l.visitors, ip)
			evictScanned = scanned
			return
		}
		if anyKey == "" || e.lastCheck.Before(any) {
			anyKey, any = ip, e.lastCheck
		}
		if e.tokens >= 1 && (solventKey == "" || e.lastCheck.Before(solvent)) {
			solventKey, solvent = ip, e.lastCheck
		}
		if scanned >= evictSample {
			break
		}
	}
	evictScanned = scanned
	victim := solventKey
	if victim == "" {
		victim = anyKey
	}
	if victim != "" {
		delete(l.visitors, victim)
	}
}

// visitorRetention is how long an idle key keeps its bucket. Shared by the
// periodic sweep and the at-capacity eviction so both agree on what "stale"
// means.
const visitorRetention = 10 * time.Minute

func (l *Limiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-l.done:
			return
		case <-ticker.C:
			bgloop.Tick("ratelimit.visitorSweep", func() {
				l.mu.Lock()
				defer l.mu.Unlock()
				for ip, e := range l.visitors {
					if time.Since(e.lastCheck) > visitorRetention {
						delete(l.visitors, ip)
					}
				}
			})
		}
	}
}

var (
	trustedProxyMu   sync.RWMutex
	trustedProxyNets = parseTrustedCIDRs([]string{"127.0.0.0/8", "::1/128", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"})
)

// SetTrustedProxies overrides the package-level trusted proxy list (CIDRs).
// Invalid entries are skipped and logged. Passing nil/empty resets to an empty
// set, which makes clientIP() ignore X-Forwarded-For entirely.
func SetTrustedProxies(cidrs []string) {
	nets := parseTrustedCIDRs(cidrs)
	trustedProxyMu.Lock()
	trustedProxyNets = nets
	trustedProxyMu.Unlock()
}

func parseTrustedCIDRs(cidrs []string) []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			slog.Warn("parse trusted proxy CIDR", slog.String("cidr", c), slog.Any("err", err))
			continue
		}
		nets = append(nets, n)
	}
	return nets
}

func isTrustedProxy(ip net.IP) bool {
	trustedProxyMu.RLock()
	defer trustedProxyMu.RUnlock()
	for _, n := range trustedProxyNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// IsTrustedProxy reports whether ip is within the configured trusted-proxy set.
// Exposed for non-HTTP boundaries (the gRPC client-IP resolver) that must apply
// the same trust rule before honoring proxy-supplied IP headers.
func IsTrustedProxy(ip net.IP) bool { return isTrustedProxy(ip) }

// ProxyResolution captures the reasoning behind a resolved client IP, not just
// the outcome. Before this, a refusal log showed only the winning IP: when
// that IP was wrong (every external client resolving to the SAME loopback
// bucket because the reverse proxy wasn't in EVEY_TRUSTED_PROXIES, prod
// 2026-07-29), the log gave no way to tell WHY - it read like the requests
// simply never arrived. HTTP (clientIP) and gRPC (resolveClientIP) both build
// one of these so the same diagnostic reasoning is available at either edge.
type ProxyResolution struct {
	IP          string // the IP callers should key the rate limiter on
	PeerIP      string // the immediate peer's address, before any header is applied
	PeerTrusted bool   // whether PeerIP is in the trusted-proxy set
	XFF         string // the raw X-Forwarded-For / x-real-ip value seen, "" if none arrived
	XFFHonored  bool   // whether XFF actually decided IP (false => IP falls back to PeerIP)
}

func ClientIP(r *http.Request) string { return clientIP(r) }

func clientIP(r *http.Request) string { return resolveClientIPReasoned(r).IP }

func resolveClientIPReasoned(r *http.Request) ProxyResolution {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	res := ProxyResolution{IP: host, PeerIP: host}
	if peer := net.ParseIP(host); peer != nil {
		res.PeerTrusted = isTrustedProxy(peer)
	}
	res.XFF = r.Header.Get("X-Forwarded-For")
	if res.PeerTrusted && res.XFF != "" {
		// take leftmost (client) IP from XFF
		fwd := res.XFF
		if idx := strings.Index(fwd, ","); idx != -1 {
			fwd = fwd[:idx]
		}
		if ip := strings.TrimSpace(fwd); net.ParseIP(ip) != nil {
			res.IP = ip
			res.XFFHonored = true
		}
	}
	return res
}

var (
	misconfigWarnMu   sync.Mutex
	misconfigWarnOnce = &sync.Once{}
)

// WarnMisroutedProxy flags the exact 2026-07-29 production failure mode: the
// resolved IP is loopback even though a forwarded-for value arrived, which
// only happens when the immediate peer (the reverse proxy) isn't covered by
// EVEY_TRUSTED_PROXIES. When that's true, every external client keys the rate
// limiter on the SAME loopback bucket - a single shared login budget for
// every client behind the proxy, not one budget per client. Shared between
// the HTTP (clientIP) and gRPC (resolveClientIP) resolvers so the same
// misconfiguration is caught regardless of which edge sees the request first.
//
// One-shot per process: this is a static config fact, not a per-request one,
// so repeating it on every refusal would just be log spam that operators
// learn to ignore. A restart - already required to change
// EVEY_TRUSTED_PROXIES - re-arms it if the misconfiguration is still there.
func WarnMisroutedProxy(res ProxyResolution) {
	if res.XFF == "" || res.XFFHonored {
		return
	}
	ip := net.ParseIP(res.IP)
	if ip == nil || !ip.IsLoopback() {
		return
	}
	misconfigWarnMu.Lock()
	once := misconfigWarnOnce
	misconfigWarnMu.Unlock()
	once.Do(func() {
		refusalLogger().Warn("ratelimit: client resolved to loopback despite a forwarded-for header being present - "+
			"EVEY_TRUSTED_PROXIES likely does not cover the reverse proxy's address, so every remote client shares "+
			"this one rate-limit bucket (fix: add the proxy's address to EVEY_TRUSTED_PROXIES)",
			slog.String("peer_ip", res.PeerIP),
			slog.Bool("peer_trusted", res.PeerTrusted),
			slog.String("xff", res.XFF))
	})
}

// ResetMisconfigWarnForTest re-arms WarnMisroutedProxy's one-shot guard.
// Test-only: without it, whichever test happens to run first would consume
// the Once and silently make every later "the warning must fire" assertion
// pass for the wrong reason.
func ResetMisconfigWarnForTest() {
	misconfigWarnMu.Lock()
	misconfigWarnOnce = &sync.Once{}
	misconfigWarnMu.Unlock()
}

// logger is the sink for refusal lines. Package-level (the middlewares are
// constructed all over wire-up and threading a logger through every call site
// would touch dozens of lines); nil = slog.Default().
var logger atomic.Pointer[slog.Logger]

// SetLogger overrides the refusal logger (tests). nil restores slog.Default().
func SetLogger(l *slog.Logger) { logger.Store(l) }

func refusalLogger() *slog.Logger {
	if l := logger.Load(); l != nil {
		return l
	}
	return slog.Default()
}

// logRefusal emits the canonical refusal line. Every refusal in the process
// funnels through here so an operator greps ONE message whatever the limiter:
// before this, a rate-limited client was completely invisible server-side: a
// beta tester locked out of the mobile app produced an empty log, which read
// as "the request never arrived" and cost an afternoon of misdirected
// diagnosis (2026-07-29).
func logRefusal(r *http.Request, extra ...slog.Attr) {
	res := resolveClientIPReasoned(r)
	WarnMisroutedProxy(res)
	// Copy rather than append onto `extra`: appending to a caller-owned slice
	// writes into ITS backing array whenever it has spare capacity, so a caller
	// reusing its buffer would find these fields stamped over its own. Today
	// every caller passes literal variadic args (exact capacity, no aliasing),
	// which is precisely why the day one of them stops would be silent.
	attrs := make([]slog.Attr, 0, len(extra)+7)
	attrs = append(attrs, extra...)
	attrs = append(attrs,
		slog.String("client_ip", res.IP),
		slog.String("path", r.URL.Path),
		slog.String("method", r.Method),
		slog.String("device", r.UserAgent()),
		slog.String("peer_ip", res.PeerIP),
		slog.Bool("peer_trusted", res.PeerTrusted),
		slog.String("xff", res.XFF))
	refusalLogger().LogAttrs(r.Context(), slog.LevelWarn, "rate limit: request refused", attrs...)
}

// refuse answers 429 AND logs it. Retry-After tells the client when to come
// back instead of leaving it to guess.
func refuse(w http.ResponseWriter, r *http.Request, retryAfter int) {
	logRefusal(r)
	if retryAfter < 1 {
		retryAfter = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	// retryAfterSeconds in the BODY too: a client that cannot read headers
	// (or a proxy that strips them) still tells the user when to come back.
	_, _ = w.Write([]byte(`{"message":"too many requests","retryAfterSeconds":` + strconv.Itoa(retryAfter) + `}`))
}

// RefuseStreaming answers a refused STREAMING request with 503 + Retry-After
// (hls.js retries a 503 transparently where a 429 surfaces as a fatal network
// error, see the streaming middleware) and logs the same canonical refusal
// line, plus the rate bucket: behind one household IP, only the bucket says
// WHICH profile was being throttled. The 07-31 cast stall investigation lost
// hours because this refusal existed only client-side.
func RefuseStreaming(w http.ResponseWriter, r *http.Request, retryAfter int, bucket string) {
	logRefusal(r, slog.String("bucket", bucket))
	if retryAfter < 1 {
		retryAfter = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte(`{"message":"streaming rate limit exceeded, retry","retryAfterSeconds":` + strconv.Itoa(retryAfter) + `}`))
}

func Middleware(limiter *Limiter, exemptPrefixes ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, p := range exemptPrefixes {
				if strings.HasPrefix(r.URL.Path, p) {
					next.ServeHTTP(w, r)
					return
				}
			}
			if !limiter.Allow(clientIP(r)) {
				refuse(w, r, limiter.RetryAfter(clientIP(r)))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func MiddlewareFunc(limiter *Limiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow(clientIP(r)) {
			refuse(w, r, limiter.RetryAfter(clientIP(r)))
			return
		}
		next(w, r)
	}
}
