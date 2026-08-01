package network

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// requestFromBoundary is a request as the gate sees it in production: it has
// crossed ProxyTrust.Middleware, which validated the proxy chain and put the
// one authoritative client IP in the context.
func requestFromBoundary(clientIP string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	return r.WithContext(WithClientIP(r.Context(), net.ParseIP(clientIP)))
}

func TestDetector_LAN(t *testing.T) {
	d := NewDetector([]string{"192.168.0.0/16", "10.0.0.0/8"})
	tests := []struct {
		name     string
		clientIP string
		want     NetworkType
	}{
		{"private 192.168", "192.168.1.100", LAN},
		{"private 10.x", "10.0.0.5", LAN},
		{"loopback", "127.0.0.1", LAN},
		{"public IP", "8.8.8.8", WAN},
		{"172.16 not configured", "172.16.0.1", WAN},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.Detect(requestFromBoundary(tt.clientIP))
			if got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

// The gate must not be openable by a header. Before the boundary carried the
// verdict's input, Detect parsed X-Forwarded-For itself: anyone could claim to
// be 127.0.0.1 and be classified LAN if the request ever reached a mux without
// the proxy middleware in front. The headers are now inert by construction.
func TestDetector_IgnoresSpoofedHeaders(t *testing.T) {
	d := NewDetector([]string{"192.168.0.0/16", "10.0.0.0/8"})
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		t.Run(header, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = "203.0.113.5:4444"
			r.Header.Set(header, "127.0.0.1")
			// No boundary crossed: an unvalidated request is remote, whatever
			// it claims about itself.
			if got := d.Detect(r); got != WAN {
				t.Errorf("%s=127.0.0.1 must not buy a LAN verdict, got %s", header, got)
			}
			// Even with the boundary crossed, the validated IP wins over the
			// header a client can still set.
			withBoundary := r.WithContext(WithClientIP(r.Context(), net.ParseIP("203.0.113.5")))
			if got := d.Detect(withBoundary); got != WAN {
				t.Errorf("%s must not override the validated client IP, got %s", header, got)
			}
		})
	}
}

// A request that never crossed the trusted boundary has no known client, and
// unknown is remote: the gate refuses rather than opening. Fails LOUD (every
// request denied) instead of silently public.
func TestDetector_NoBoundary_IsWAN(t *testing.T) {
	d := NewDetector([]string{"192.168.0.0/16"})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.168.1.10:1234" // a LAN peer, but unvalidated
	if got := d.Detect(r); got != WAN {
		t.Errorf("no validated client IP must be WAN (fail closed), got %s", got)
	}
}

func TestDetector_AllPrivateRanges(t *testing.T) {
	d := NewDetector([]string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})
	if got := d.Detect(requestFromBoundary("172.16.0.1")); got != LAN {
		t.Errorf("172.16 should be LAN with full private ranges, got %s", got)
	}
}

func TestDetector_EmptyCIDRs(t *testing.T) {
	d := NewDetector(nil)
	if got := d.Detect(requestFromBoundary("192.168.1.1")); got != WAN {
		t.Errorf("no CIDRs should make all non-loopback IPs WAN, got %s", got)
	}
}

func TestDetector_InvalidCIDR(t *testing.T) {
	d := NewDetector([]string{"not-a-cidr", "192.168.0.0/16"})
	if got := d.Detect(requestFromBoundary("192.168.1.1")); got != LAN {
		t.Errorf("valid CIDR should still work after invalid one, got %s", got)
	}
}

// ClientIP feeds logs, the presence panel and the cast subnet probe. It must
// answer with the validated IP when there is one, and otherwise with the raw
// peer - never with a header, or a caller could choose what lands in the audit
// trail.
func TestClientIP_PrefersBoundaryThenPeer_NeverHeaders(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.9:5555"
	r.Header.Set("X-Forwarded-For", "10.0.0.1")
	r.Header.Set("X-Real-IP", "10.0.0.2")

	if got := ClientIP(r); got == nil || got.String() != "203.0.113.9" {
		t.Errorf("without a boundary, ClientIP must be the peer, got %v", got)
	}

	validated := r.WithContext(WithClientIP(r.Context(), net.ParseIP("198.51.100.7")))
	if got := ClientIP(validated); got == nil || got.String() != "198.51.100.7" {
		t.Errorf("with a boundary, ClientIP must be the validated IP, got %v", got)
	}
}

func TestWithNetworkType_Roundtrip(t *testing.T) {
	ctx := WithNetworkType(context.Background(), WAN)
	if nt, ok := ctx.Value(contextKey{}).(NetworkType); !ok || nt != WAN {
		t.Errorf("expected WAN in context, got %v", nt)
	}
}

// The admin network panel lists the addresses an operator can reach this
// server on. The host's real interfaces vary, so the assertions are on the
// CONTRACT rather than on any particular NIC: whatever comes back must be
// usable as "connect to Evey here".
//
// The previous version ended on `_ = ifaces` - it only proved the call did not
// error, so the filtering was untested and mutation testing killed nothing in
// it. Loopback is the assertion that matters: telling an operator to reach the
// server on 127.0.0.1 is advice that works only from the machine itself.
func TestNetworkInterfaces(t *testing.T) {
	ifaces, err := NetworkInterfaces()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, i := range ifaces {
		if i.Name == "" {
			t.Error("interface with no name: nothing for the panel to label the address with")
		}
		ip := net.ParseIP(i.Address)
		if ip == nil {
			t.Errorf("interface %q carries %q, which is not an address anyone can dial", i.Name, i.Address)
			continue
		}
		if ip.IsLoopback() {
			t.Errorf("interface %q reported loopback %s: unreachable from any other machine", i.Name, i.Address)
		}
	}
}
