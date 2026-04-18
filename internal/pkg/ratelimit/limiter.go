package ratelimit

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
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
				http.Error(w, `{"message":"too many requests"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func MiddlewareFunc(limiter *Limiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow(clientIP(r)) {
			http.Error(w, `{"message":"too many requests"}`, http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}
