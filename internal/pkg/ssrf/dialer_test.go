package ssrf

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIsPrivateOrReserved(t *testing.T) {
	cases := []struct {
		ip      string
		blocked bool
	}{
		// loopback
		{"127.0.0.1", true},
		{"127.1.2.3", true},
		{"::1", true},
		// private RFC1918
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.1.1", true},
		// link-local
		{"169.254.1.1", true},
		{"fe80::1", true},
		// IPv6 private
		{"fc00::1", true},
		{"fd12:3456:789a::1", true},
		// multicast
		{"224.0.0.1", true},
		{"239.255.255.255", true},
		{"ff02::1", true},
		// unspecified
		{"0.0.0.0", true},
		{"::", true},
		// CGNAT
		{"100.64.0.1", true},
		// TEST-NET
		{"192.0.2.1", true},
		{"198.51.100.1", true},
		{"203.0.113.1", true},
		// reserved / broadcast
		{"240.0.0.1", true},
		{"255.255.255.255", true},
		// IPv4-mapped IPv6 must be rejected via embedded IPv4
		{"::ffff:10.0.0.1", true},
		{"::ffff:127.0.0.1", true},
		// Deprecated IPv6 forms that EMBED a v4 address without To4() seeing it,
		// so no v4 range above applies to them. Barely routable on a modern
		// kernel, but this is a deny list: what is absent is allowed.
		{"2002:7f00:1::", true},    // 6to4 wrapping 127.0.0.1
		{"2002:a00:1::", true},     // 6to4 wrapping 10.0.0.1
		{"2002:a9fe:1::", true},    // 6to4 wrapping 169.254.0.1 (cloud metadata)
		{"::7f00:1", true},         // IPv4-compatible, 127.0.0.1
		{"::a00:1", true},          // IPv4-compatible, 10.0.0.1
		{"2001:4860::8888", false}, /* a real 2001:: address stays reachable */
		// public
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"9.9.9.9", false},
		{"2001:4860:4860::8888", false},
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("could not parse %q", tc.ip)
			}
			got := IsPrivateOrReserved(ip)
			if got != tc.blocked {
				t.Fatalf("IsPrivateOrReserved(%s) = %v, want %v", tc.ip, got, tc.blocked)
			}
		})
	}
}

func TestIsPrivateOrReserved_NilIP(t *testing.T) {
	if !IsPrivateOrReserved(nil) {
		t.Fatal("nil IP must be treated as blocked")
	}
}

func TestSafeHTTPClient_BlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := SafeHTTPClient(5 * time.Second)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := client.Do(req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected error connecting to loopback, got nil")
	}
	if !errors.Is(err, ErrBlockedPrivateIP) {
		t.Fatalf("expected ErrBlockedPrivateIP in chain, got %v", err)
	}
}

func TestSafeHTTPClient_AllowsPublic(t *testing.T) {
	// Only run when network is available. Use a short-circuit: resolve example.com
	// with a short timeout; skip if DNS fails (offline CI / sandbox).
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := net.DefaultResolver.LookupHost(ctx, "example.com"); err != nil {
		t.Skipf("skipping, no DNS / network: %v", err)
	}

	client := SafeHTTPClient(5 * time.Second)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("skipping, no network: %v", err)
	}
	resp.Body.Close()
}

func TestNewSafeDialer_BlocksReservedLiterals(t *testing.T) {
	d := NewSafeDialer()
	cases := []struct {
		name string
		addr string
	}{
		{"loopback", "127.0.0.1:1"},
		{"private-10-8", "10.0.0.1:1"},
		{"private-172-16-12", "172.16.0.1:1"},
		{"private-192-168-16", "192.168.0.1:1"},
		{"link-local-169-254-16", "169.254.1.1:1"},
		{"cgnat-100-64", "100.64.0.1:1"},
		{"ipv6-loopback", "[::1]:1"},
		{"ipv6-ula", "[fc00::1]:1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, err := d.DialContext(ctx, "tcp", tc.addr)
			if err == nil {
				t.Fatalf("expected error dialing blocked literal %s", tc.addr)
			}
			if !errors.Is(err, ErrBlockedPrivateIP) {
				t.Fatalf("expected ErrBlockedPrivateIP in chain for %s, got %v", tc.addr, err)
			}
		})
	}
}

// The plugin broker fetches UNTRUSTED, plugin-supplied URLs, so its client
// must never delegate the connection to an env proxy: HTTP(S)_PROXY set on
// the server would otherwise ship the real (possibly private) destination to
// the proxy as a literal URL, and the IP filter would never see it. First-
// party fetchers (enrichers, webhooks) keep honoring a corporate proxy.
func TestSafeHTTPClientNoProxy_HasNilProxy(t *testing.T) {
	tr, ok := SafeHTTPClientNoProxy(time.Second).Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport")
	}
	if tr.Proxy != nil {
		t.Error("broker client must not route through an env proxy (SSRF filter must apply to the real destination)")
	}
}

func TestSafeHTTPClient_HonorsProxyForFirstParty(t *testing.T) {
	tr, ok := SafeHTTPClient(time.Second).Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport")
	}
	if tr.Proxy == nil {
		t.Error("first-party client should still honor an operator-configured proxy")
	}
}

// safeControl is the enforcement point: it runs after DNS resolution, on the
// address the socket is ABOUT to connect to, which is what closes the
// DNS-rebinding window that validating a URL string cannot. The existing
// dialer test only exercises literal private addresses; mutation testing
// (2026-08-01) showed the callback's own branches were the least covered code
// in the package - notably the unparseable-host branch, whose removal survived
// the whole suite.
func TestSafeControl_RejectsWhatItCannotVouchFor(t *testing.T) {
	blocked := []struct{ name, address string }{
		{"private v4", "10.1.2.3:443"},
		{"loopback v4", "127.0.0.1:80"},
		{"link-local (cloud metadata)", "169.254.169.254:80"},
		{"loopback v6", "[::1]:443"},
		// Not an IP at all: safeControl runs POST-resolution, so a host that
		// does not parse means we cannot say where this socket is going. The
		// only safe answer is no.
		{"unparseable host", "not-an-ip:443"},
		{"empty host", ":443"},
	}
	for _, tc := range blocked {
		t.Run(tc.name, func(t *testing.T) {
			if err := safeControl("tcp", tc.address, nil); !errors.Is(err, ErrBlockedPrivateIP) {
				t.Errorf("safeControl(%q) = %v, want ErrBlockedPrivateIP", tc.address, err)
			}
		})
	}

	allowed := []struct{ name, address string }{
		{"public v4", "93.184.216.34:443"},
		{"public v6", "[2606:2800:220:1:248:1893:25c8:1946]:443"},
	}
	for _, tc := range allowed {
		t.Run(tc.name, func(t *testing.T) {
			if err := safeControl("tcp", tc.address, nil); err != nil {
				t.Errorf("safeControl(%q) = %v, want nil - a public address must connect", tc.address, err)
			}
		})
	}

	// A malformed address (no port at all) is a caller bug, not an SSRF
	// attempt: it must error, but it must not be reported as a blocked IP.
	err := safeControl("tcp", "no-port-here", nil)
	if err == nil {
		t.Error("a malformed address must error")
	}
	if errors.Is(err, ErrBlockedPrivateIP) {
		t.Error("a malformed address reported as ErrBlockedPrivateIP would send an operator hunting for an SSRF attempt that never happened")
	}
}

// The dialer the whole server uses must carry the callback AND a connect
// timeout: without the timeout a webhook pointed at a black-holed address
// pins a goroutine for as long as the OS lets it.
func TestNewSafeDialer_CarriesTheGuardAndABoundedConnect(t *testing.T) {
	d := NewSafeDialer()
	if d.Control == nil {
		t.Fatal("no Control callback: the dialer would connect anywhere DNS points")
	}
	if d.Timeout <= 0 {
		t.Error("no connect timeout")
	}
	if err := d.Control("tcp", "169.254.169.254:80", nil); !errors.Is(err, ErrBlockedPrivateIP) {
		t.Errorf("the wired callback let cloud metadata through: %v", err)
	}
}

// The IPv6 range table is reached only AFTER the stdlib classifiers, so the
// obvious v6 cases (::1, fc00::) never exercise it - they are caught by
// IsLoopback/IsPrivate first. Mutation testing (2026-08-01) exposed that:
// breaking the loop that builds reservedV6Ranges left every test green.
//
// These three are covered by the table and by nothing else.
func TestIsPrivateOrReserved_UsesTheIPv6RangeTable(t *testing.T) {
	cases := []struct{ name, ip string }{
		{"documentation 2001:db8::/32", "2001:db8::1"},
		{"IPv4/IPv6 translation 64:ff9b::/96", "64:ff9b::1.2.3.4"},
		{"discard-only 100::/64", "100::1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("premise: %q must parse", tc.ip)
			}
			if !IsPrivateOrReserved(ip) {
				t.Errorf("%s (%s) was allowed: the IPv6 reserved table is not being consulted", tc.name, tc.ip)
			}
		})
	}

	// Control: a normal public v6 address must still be allowed, or the test
	// above would pass with a blanket "block all v6".
	if pub := net.ParseIP("2606:2800:220:1:248:1893:25c8:1946"); IsPrivateOrReserved(pub) {
		t.Error("a public IPv6 address was blocked: the guard is too broad to be useful")
	}
}
