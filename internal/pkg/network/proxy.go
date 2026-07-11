package network

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

type ProxyTrust struct {
	trustedCIDRs []*net.IPNet
	realIPHeader string
	trustProto   bool
}

func NewProxyTrust(trustedCIDRs []string, realIPHeader string, trustProto bool) (*ProxyTrust, error) {
	nets := make([]*net.IPNet, 0, len(trustedCIDRs))
	for _, cidr := range trustedCIDRs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
		}
		nets = append(nets, ipNet)
	}
	header := strings.TrimSpace(realIPHeader)
	if header == "" {
		header = "X-Forwarded-For"
	}
	return &ProxyTrust{
		trustedCIDRs: nets,
		realIPHeader: header,
		trustProto:   trustProto,
	}, nil
}

func (p *ProxyTrust) isTrusted(ip net.IP) bool {
	for _, cidr := range p.trustedCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
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

		if p.trustProto {
			if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
				r.URL.Scheme = proto
			}
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
