// Package ssrf provides defense-in-depth against Server-Side Request Forgery
// (SSRF) attacks on outbound HTTP clients. The safe dialer rejects connections
// to private, loopback, link-local, multicast, unspecified, or otherwise
// reserved IP ranges after DNS resolution but before the TCP handshake.
package ssrf

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"
)

// ErrBlockedPrivateIP is returned by the safe dialer when a connection target
// resolves to a private or reserved IP address.
var ErrBlockedPrivateIP = errors.New("ssrf: blocked connection to private or reserved IP")

var reservedV4Ranges []*net.IPNet
var reservedV6Ranges []*net.IPNet

func init() {
	v4 := []string{
		"0.0.0.0/8",       // current network / unspecified
		"10.0.0.0/8",      // RFC1918
		"100.64.0.0/10",   // CGNAT
		"127.0.0.0/8",     // loopback
		"169.254.0.0/16",  // link-local
		"172.16.0.0/12",   // RFC1918
		"192.0.0.0/24",    // IETF protocol assignments
		"192.0.2.0/24",    // TEST-NET-1
		"192.168.0.0/16",  // RFC1918
		"198.18.0.0/15",   // benchmarking
		"198.51.100.0/24", // TEST-NET-2
		"203.0.113.0/24",  // TEST-NET-3
		"224.0.0.0/4",     // multicast
		"240.0.0.0/4",     // reserved / broadcast
	}
	v6 := []string{
		"::/128",  // unspecified
		"::1/128", // loopback
		// ::/96 - IPv4-compatible (deprecated, RFC 4291). ::7f00:1 embeds
		// 127.0.0.1, and To4() returns nil on it, so NO v4 range above applies
		// and ::/128 only covers the unspecified address itself. Deprecated and
		// unrouted on modern Linux, so barely exploitable - but this is a DENY
		// list, and what is not on it is allowed.
		"::/96",
		"::ffff:0:0/96", // IPv4-mapped (handled via To4 but keep for safety)
		"64:ff9b::/96",  // IPv4/IPv6 translation
		"100::/64",      // discard-only
		// 2002::/16 - 6to4. 2002:7f00:0001:: encapsulates 127.0.0.1, and nothing
		// else here catches it (IsPrivate() over v6 only knows fc00::/7). Same
		// reasoning as ::/96 above.
		"2002::/16",
		"2001:db8::/32", // documentation
		"fc00::/7",      // unique local (includes fd00::/8)
		"fe80::/10",     // link-local
		"ff00::/8",      // multicast
	}
	for _, c := range v4 {
		if _, n, err := net.ParseCIDR(c); err == nil {
			reservedV4Ranges = append(reservedV4Ranges, n)
		}
	}
	for _, c := range v6 {
		if _, n, err := net.ParseCIDR(c); err == nil {
			reservedV6Ranges = append(reservedV6Ranges, n)
		}
	}
}

// IsPrivateOrReserved returns true if the IP is loopback, link-local,
// private, multicast, unspecified, or in reserved ranges. IPv4-mapped IPv6
// addresses are evaluated against their embedded IPv4 representation.
func IsPrivateOrReserved(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() ||
		ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsPrivate() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		for _, r := range reservedV4Ranges {
			if r.Contains(v4) {
				return true
			}
		}
		return false
	}
	for _, r := range reservedV6Ranges {
		if r.Contains(ip) {
			return true
		}
	}
	return false
}

// NewSafeDialer returns a *net.Dialer with a Control callback that rejects
// connections whose resolved destination is a private or reserved IP. The
// dialer uses a 10s connect timeout by default.
func NewSafeDialer() *net.Dialer {
	return &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   safeControl,
	}
}

func safeControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return ErrBlockedPrivateIP
	}
	if IsPrivateOrReserved(ip) {
		return ErrBlockedPrivateIP
	}
	return nil
}

// SafeHTTPClient returns an *http.Client with a Transport whose DialContext
// rejects private/reserved IPs. timeout is the overall request timeout. It
// honors an operator-configured HTTP(S)_PROXY, which is fine for FIRST-PARTY
// egress (metadata enrichers, admin-configured webhooks) where the operator
// chose both the proxy and the destinations.
func SafeHTTPClient(timeout time.Duration) *http.Client {
	return safeClient(timeout, http.ProxyFromEnvironment)
}

// SafeHTTPClientNoProxy is SafeHTTPClient with proxying disabled. Use it for
// egress on UNTRUSTED, caller-supplied URLs (the plugin fetch broker): with a
// proxy in play the Transport would ship the real destination to the proxy as
// a literal URL, so a private/metadata target would bypass the IP filter
// entirely. Pinning Proxy=nil forces every connection through the safe dialer,
// which sees - and rejects - the true destination IP.
func SafeHTTPClientNoProxy(timeout time.Duration) *http.Client {
	return safeClient(timeout, nil)
}

func safeClient(timeout time.Duration, proxy func(*http.Request) (*url.URL, error)) *http.Client {
	dialer := NewSafeDialer()
	transport := &http.Transport{
		Proxy: proxy,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		},
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}
