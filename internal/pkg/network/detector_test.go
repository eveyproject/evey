package network

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDetector_LAN(t *testing.T) {
	d := NewDetector([]string{"192.168.0.0/16", "10.0.0.0/8"})
	tests := []struct {
		name     string
		remoteAddr string
		xff      string
		realIP   string
		want     NetworkType
	}{
		{"private 192.168", "192.168.1.100:12345", "", "", LAN},
		{"private 10.x", "10.0.0.5:9999", "", "", LAN},
		{"loopback", "127.0.0.1:1234", "", "", LAN},
		{"public IP", "8.8.8.8:1234", "", "", WAN},
		{"xff private", "8.8.8.8:1234", "192.168.1.50", "", LAN},
		{"xff public", "192.168.1.1:1234", "203.0.113.5", "", WAN},
		{"x-real-ip private", "8.8.8.8:1234", "", "10.0.0.1", LAN},
		{"x-real-ip public", "192.168.1.1:1234", "", "203.0.113.5", WAN},
		{"xff takes priority", "8.8.8.8:1234", "10.0.0.1", "203.0.113.5", LAN},
		{"172.16 not configured", "172.16.0.1:1234", "", "", WAN},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.realIP != "" {
				r.Header.Set("X-Real-IP", tt.realIP)
			}
			got := d.Detect(r)
			if got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestDetector_AllPrivateRanges(t *testing.T) {
	d := NewDetector([]string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "172.16.0.1:1234"
	if got := d.Detect(r); got != LAN {
		t.Errorf("172.16 should be LAN with full private ranges, got %s", got)
	}
}

func TestDetector_EmptyCIDRs(t *testing.T) {
	d := NewDetector(nil)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.168.1.1:1234"
	if got := d.Detect(r); got != WAN {
		t.Errorf("no CIDRs should make all non-loopback IPs WAN, got %s", got)
	}
}

func TestDetector_InvalidCIDR(t *testing.T) {
	d := NewDetector([]string{"not-a-cidr", "192.168.0.0/16"})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.168.1.1:1234"
	if got := d.Detect(r); got != LAN {
		t.Errorf("valid CIDR should still work after invalid one, got %s", got)
	}
}

func TestContextRoundtrip(t *testing.T) {
	ctx := context.Background()
	if got := GetNetworkType(ctx); got != LAN {
		t.Errorf("default should be LAN, got %s", got)
	}
	ctx = WithNetworkType(ctx, WAN)
	if got := GetNetworkType(ctx); got != WAN {
		t.Errorf("expected WAN, got %s", got)
	}
}

func TestNetworkInterfaces(t *testing.T) {
	ifaces, err := NetworkInterfaces()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// just verify it returns without error; actual interfaces vary
	_ = ifaces
}
