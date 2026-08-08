package main

import "testing"

// The native gRPC port is internal: the grpc-gateway in this same process is
// its only client. It must default to loopback because the remote-access gate
// - the switch deciding who outside the LAN may reach this server - is HTTP
// middleware that does NOT cover this listener. A gRPC port on every interface
// was therefore a way around that switch, with nothing warning the operator.
func TestIsLoopbackHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"localhost", true},
		{"0.0.0.0", false}, // every interface - the case that bypasses the gate
		{"", false},        // the ":port" shorthand is also every interface
		{"192.168.1.10", false},
		{"::", false},
	}
	for _, tt := range tests {
		if got := isLoopbackHost(tt.host); got != tt.want {
			t.Errorf("isLoopbackHost(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}
