package ratelimit

import (
	"log/slog"
	"net"
	"net/http"
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

func (l *Limiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	e, ok := l.visitors[ip]
	if !ok {
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

func (l *Limiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-l.done:
			return
		case <-ticker.C:
			l.mu.Lock()
			for ip, e := range l.visitors {
				if time.Since(e.lastCheck) > 10*time.Minute {
					delete(l.visitors, ip)
				}
			}
			l.mu.Unlock()
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

func ClientIP(r *http.Request) string { return clientIP(r) }

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer := net.ParseIP(host)
	if peer != nil && isTrustedProxy(peer) {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			// take leftmost (client) IP from XFF
			if idx := strings.Index(fwd, ","); idx != -1 {
				fwd = fwd[:idx]
			}
			if ip := strings.TrimSpace(fwd); net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	if host != "" {
		return host
	}
	return r.RemoteAddr
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

// refuse answers 429 AND logs it. Before this, a rate-limited client was
// completely invisible server-side: a beta tester locked out of the mobile app
// produced an empty log, which read as "the request never arrived" and cost an
// afternoon of misdirected diagnosis (2026-07-29). Retry-After tells the client
// when to come back instead of leaving it to guess.
func refuse(w http.ResponseWriter, r *http.Request) {
	refusalLogger().WarnContext(r.Context(), "rate limit: request refused",
		slog.String("client_ip", clientIP(r)),
		slog.String("path", r.URL.Path),
		slog.String("method", r.Method),
		slog.String("device", r.UserAgent()))
	w.Header().Set("Retry-After", "60")
	http.Error(w, `{"message":"too many requests"}`, http.StatusTooManyRequests)
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
				refuse(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func MiddlewareFunc(limiter *Limiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow(clientIP(r)) {
			refuse(w, r)
			return
		}
		next(w, r)
	}
}
