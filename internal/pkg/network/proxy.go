package network

import (
	"fmt"
	"net"
	"net/http"
	"slices"
	"strings"
)

type ProxyTrust struct {
	trusted      *LiveCIDRSet
	realIPHeader string
	trustProto   func() bool
}

// NewProxyTrust builds a boundary on a fixed list. An invalid CIDR is an error.
func NewProxyTrust(trustedCIDRs []string, realIPHeader string, trustProto bool) (*ProxyTrust, error) {
	for _, cidr := range trustedCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
		}
	}
	fixed := slices.Clone(trustedCIDRs)
	return NewLiveProxyTrust(func() []string { return fixed }, realIPHeader, func() bool { return trustProto }), nil
}

// NewLiveProxyTrust builds a boundary that reads the trusted list and the
// HTTPS flag on every request, so a change in Admin -> Remote Access applies to
// the next request. The sources must be safe to call concurrently; entries are
// expected validated upstream (an invalid one is skipped).
func NewLiveProxyTrust(trustedCIDRs func() []string, realIPHeader string, trustProto func() bool) *ProxyTrust {
	header := strings.TrimSpace(realIPHeader)
	if header == "" {
		header = "X-Forwarded-For"
	}
	return &ProxyTrust{
		trusted:      NewLiveCIDRSet(trustedCIDRs),
		realIPHeader: header,
		trustProto:   trustProto,
	}
}

func (p *ProxyTrust) isTrusted(ip net.IP) bool {
	return p.trusted.Contains(ip)
}

func (p *ProxyTrust) ResolveClientIP(r *http.Request) string {
	peerIP := extractIP(r.RemoteAddr)
	if peerIP == "" {
		return r.RemoteAddr
	}

	peer := net.ParseIP(peerIP)
	if peer == nil {
		return peerIP
	}

	if !p.isTrusted(peer) {
		return peerIP
	}

	headerVal := r.Header.Get(p.realIPHeader)
	if headerVal == "" {
		return peerIP
	}

	if p.realIPHeader == "X-Real-IP" {
		ip := strings.TrimSpace(headerVal)
		if net.ParseIP(ip) != nil {
			return ip
		}
		return peerIP
	}

	// X-Forwarded-For: walk right to left, first untrusted = client
	parts := strings.Split(headerVal, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ip := strings.TrimSpace(parts[i])
		parsed := net.ParseIP(ip)
		if parsed == nil {
			continue
		}
		if !p.isTrusted(parsed) {
			return ip
		}
	}
	// all trusted - return leftmost
	if first := strings.TrimSpace(parts[0]); net.ParseIP(first) != nil {
		return first
	}
	return peerIP
}

func (p *ProxyTrust) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := p.ResolveClientIP(r)
		// The connection's own peer is kept apart before RemoteAddr becomes
		// the client: the load test asks who handed it a request (the proxy),
		// not who sent it.
		if peer := net.ParseIP(extractIP(r.RemoteAddr)); peer != nil {
			r = r.WithContext(WithPeerIP(r.Context(), peer))
		}
		r.RemoteAddr = clientIP + ":0"

		// Collapse X-Forwarded-For to the single authoritative client IP.
		// This middleware sits at the trusted boundary and has already
		// validated the proxy chain, so anything downstream must not see the
		// raw client-supplied chain. In particular the grpc-gateway re-injects
		// X-Forwarded-For into gRPC metadata (original header + RemoteAddr),
		// and the per-IP rate limiter keys off metadata[0] - without this an
		// untrusted client could prepend a spoofed leftmost entry and poison
		// rate-limit/audit attribution. Overwriting (not appending) drops the
		// untrusted chain entirely.
		r.Header.Set("X-Forwarded-For", clientIP)

		if p.trustProto() {
			if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
				r.URL.Scheme = proto
			}
		}

		// This is the trusted boundary, so this is where the authoritative
		// client IP is minted. Downstream reads it from the context and never
		// re-parses a header (see Detect): the LAN/WAN verdict that gates
		// remote access then depends on THIS validation having run, not on a
		// middleware ordering nobody downstream can verify.
		if ip := net.ParseIP(clientIP); ip != nil {
			r = r.WithContext(WithClientIP(r.Context(), ip))
		}

		next.ServeHTTP(w, r)
	})
}

func extractIP(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// might be bare IP (no port)
		if net.ParseIP(addr) != nil {
			return addr
		}
		return ""
	}
	return host
}
