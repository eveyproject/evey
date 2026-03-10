package network

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func mustProxyTrust(t *testing.T, cidrs []string, header string, trustProto bool) *ProxyTrust {
	t.Helper()
	pt, err := NewProxyTrust(cidrs, header, trustProto)
	if err != nil {
		t.Fatalf("NewProxyTrust: %v", err)
	}
	return pt
}

func TestResolveClientIP_DirectConnection(t *testing.T) {
	pt := mustProxyTrust(t, []string{"127.0.0.1/32"}, "X-Forwarded-For", false)
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.5:12345"

	got := pt.ResolveClientIP(r)
	if got != "203.0.113.5" {
		t.Errorf("expected 203.0.113.5, got %s", got)
	}
}

func TestResolveClientIP_SingleTrustedProxy(t *testing.T) {
	pt := mustProxyTrust(t, []string{"127.0.0.1/32"}, "X-Forwarded-For", false)
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:8080"
	r.Header.Set("X-Forwarded-For", "203.0.113.50")

	got := pt.ResolveClientIP(r)
	if got != "203.0.113.50" {
		t.Errorf("expected 203.0.113.50, got %s", got)
	}
}

func TestResolveClientIP_ChainedProxies(t *testing.T) {
	pt := mustProxyTrust(t, []string{"10.0.0.0/8", "172.16.0.0/12"}, "X-Forwarded-For", false)
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:80"
	r.Header.Set("X-Forwarded-For", "203.0.113.10, 172.16.0.5, 10.0.0.2")

	got := pt.ResolveClientIP(r)
	if got != "203.0.113.10" {
		t.Errorf("expected 203.0.113.10, got %s", got)
	}
}

func TestResolveClientIP_UntrustedProxy(t *testing.T) {
	pt := mustProxyTrust(t, []string{"127.0.0.1/32"}, "X-Forwarded-For", false)
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.168.1.100:9999"
	r.Header.Set("X-Forwarded-For", "10.0.0.1")

	got := pt.ResolveClientIP(r)
	if got != "192.168.1.100" {
		t.Errorf("expected 192.168.1.100 (peer IP, ignoring header), got %s", got)
	}
}

func TestResolveClientIP_SpoofedHeader(t *testing.T) {
	pt := mustProxyTrust(t, []string{"127.0.0.1/32"}, "X-Forwarded-For", false)
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.99:5555"
	r.Header.Set("X-Forwarded-For", "10.0.0.1, 192.168.1.1")

	got := pt.ResolveClientIP(r)
	if got != "203.0.113.99" {
		t.Errorf("expected 203.0.113.99 (untrusted peer → ignore header), got %s", got)
	}
}

func TestResolveClientIP_XRealIP(t *testing.T) {
	pt := mustProxyTrust(t, []string{"127.0.0.1/32"}, "X-Real-IP", false)
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:80"
	r.Header.Set("X-Real-IP", "198.51.100.42")

	got := pt.ResolveClientIP(r)
	if got != "198.51.100.42" {
		t.Errorf("expected 198.51.100.42, got %s", got)
	}
}

func TestResolveClientIP_XRealIP_UntrustedPeer(t *testing.T) {
	pt := mustProxyTrust(t, []string{"127.0.0.1/32"}, "X-Real-IP", false)
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.5:80"
	r.Header.Set("X-Real-IP", "10.0.0.1")

	got := pt.ResolveClientIP(r)
	if got != "203.0.113.5" {
		t.Errorf("expected 203.0.113.5 (peer not trusted), got %s", got)
	}
}

func TestResolveClientIP_EmptyHeader(t *testing.T) {
	pt := mustProxyTrust(t, []string{"127.0.0.1/32"}, "X-Forwarded-For", false)
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:80"

	got := pt.ResolveClientIP(r)
	if got != "127.0.0.1" {
		t.Errorf("expected 127.0.0.1 (no header → peer IP), got %s", got)
	}
}

func TestResolveClientIP_CIDRRangeMatch(t *testing.T) {
	pt := mustProxyTrust(t, []string{"10.0.0.0/8"}, "X-Forwarded-For", false)
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.1.2.3:8080"
	r.Header.Set("X-Forwarded-For", "203.0.113.77")

	got := pt.ResolveClientIP(r)
	if got != "203.0.113.77" {
		t.Errorf("expected 203.0.113.77, got %s", got)
	}
}

func TestResolveClientIP_IPv6(t *testing.T) {
	pt := mustProxyTrust(t, []string{"::1/128"}, "X-Forwarded-For", false)
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "[::1]:8080"
	r.Header.Set("X-Forwarded-For", "2001:db8::1")

	got := pt.ResolveClientIP(r)
	if got != "2001:db8::1" {
		t.Errorf("expected 2001:db8::1, got %s", got)
	}
}

func TestResolveClientIP_AllTrustedReturnsLeftmost(t *testing.T) {
	pt := mustProxyTrust(t, []string{"10.0.0.0/8", "172.16.0.0/12"}, "X-Forwarded-For", false)
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:80"
	r.Header.Set("X-Forwarded-For", "10.0.0.50, 172.16.0.1, 10.0.0.2")

	got := pt.ResolveClientIP(r)
	if got != "10.0.0.50" {
		t.Errorf("expected 10.0.0.50 (leftmost when all trusted), got %s", got)
	}
}

func TestMiddleware_ReplacesRemoteAddr(t *testing.T) {
	pt := mustProxyTrust(t, []string{"127.0.0.1/32"}, "X-Forwarded-For", false)

	var capturedAddr string
	handler := pt.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAddr = r.RemoteAddr
	}))

	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:8080"
	r.Header.Set("X-Forwarded-For", "203.0.113.10")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if capturedAddr != "203.0.113.10:0" {
		t.Errorf("expected RemoteAddr=203.0.113.10:0, got %s", capturedAddr)
	}
}

func TestMiddleware_ForwardedProto(t *testing.T) {
	pt := mustProxyTrust(t, []string{"127.0.0.1/32"}, "X-Forwarded-For", true)

	var capturedScheme string
	handler := pt.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedScheme = r.URL.Scheme
	}))

	r := httptest.NewRequest("GET", "http://example.com/test", nil)
	r.RemoteAddr = "127.0.0.1:8080"
	r.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if capturedScheme != "https" {
		t.Errorf("expected scheme=https, got %s", capturedScheme)
	}
}

func TestMiddleware_ForwardedProtoDisabled(t *testing.T) {
	pt := mustProxyTrust(t, []string{"127.0.0.1/32"}, "X-Forwarded-For", false)

	var capturedScheme string
	handler := pt.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedScheme = r.URL.Scheme
	}))

	r := httptest.NewRequest("GET", "http://example.com/test", nil)
	r.RemoteAddr = "127.0.0.1:8080"
	r.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if capturedScheme != "http" {
		t.Errorf("expected scheme=http (proto trust disabled), got %s", capturedScheme)
	}
}

func TestNewProxyTrust_InvalidCIDR(t *testing.T) {
	_, err := NewProxyTrust([]string{"not-a-cidr"}, "X-Forwarded-For", false)
	if err == nil {
		t.Error("expected error for invalid CIDR")
	}
}

func TestNewProxyTrust_EmptyHeader(t *testing.T) {
	pt, err := NewProxyTrust([]string{"127.0.0.1/32"}, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pt.realIPHeader != "X-Forwarded-For" {
		t.Errorf("expected default header X-Forwarded-For, got %s", pt.realIPHeader)
	}
}
