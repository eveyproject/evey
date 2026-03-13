package ratelimit

import (
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
}

func New(requestsPerMinute int) *Limiter {
	l := &Limiter{
		visitors: make(map[string]*entry),
		rate:     float64(requestsPerMinute) / 60.0,
		burst:    requestsPerMinute,
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
	}
	go l.cleanup()
	return l
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
	for {
		time.Sleep(5 * time.Minute)
		l.mu.Lock()
		for ip, e := range l.visitors {
			if time.Since(e.lastCheck) > 10*time.Minute {
				delete(l.visitors, ip)
			}
		}
		l.mu.Unlock()
	}
}

var trustedProxyNets = func() []*net.IPNet {
	cidrs := []string{"127.0.0.0/8", "::1/128", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, _ := net.ParseCIDR(c)
		nets = append(nets, n)
	}
	return nets
}()

func isTrustedProxy(ip net.IP) bool {
	for _, n := range trustedProxyNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

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
