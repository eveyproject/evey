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
