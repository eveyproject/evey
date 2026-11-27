package network

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withRelays sets the relay list for one test and restores it after.
func withRelays(t *testing.T, ips ...string) {
	t.Helper()
	prev := relays.Load()
	list := make([]net.IP, 0, len(ips))
	for _, s := range ips {
		list = append(list, net.ParseIP(s))
	}
	SetRelays(list)
	t.Cleanup(func() { relays.Store(prev) })
}

// routeHex prints an IPv4 address the way the kernel does in /proc/net/route:
// the in-memory 32-bit value in hex, so in host byte order.
func routeHex(ip string) string {
	v4 := net.ParseIP(ip).To4()
	return fmt.Sprintf("%08X", binary.NativeEndian.Uint32(v4))
}

func routeTableOf(rows ...string) string {
	return "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n" +
		strings.Join(rows, "\n") + "\n"
}

func route(iface, dst, gw, flags, mask string) string {
	return fmt.Sprintf("%s\t%s\t%s\t%s\t0\t0\t0\t%s\t0\t0\t0", iface, dst, gw, flags, mask)
}

func TestParseRouteTable_DockerBridgeDefaultRoute(t *testing.T) {
	table := routeTableOf(
		route("eth0", "00000000", routeHex("172.17.0.1"), "0003", "00000000"),
		route("eth0", routeHex("172.17.0.0"), "00000000", "0001", routeHex("255.255.0.0")),
	)
	got, err := parseRouteTable(strings.NewReader(table))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].Equal(net.ParseIP("172.17.0.1")) {
		t.Fatalf("gateways = %v, want [172.17.0.1]", got)
	}
}

func TestParseRouteTable_IgnoresLinkRoutesAndDuplicates(t *testing.T) {
	table := routeTableOf(
		// A default route without RTF_GATEWAY is a device route, not a relay.
		route("wg0", "00000000", "00000000", "0001", "00000000"),
		// A route through a gateway to one subnet is not the default route.
		route("eth0", routeHex("10.8.0.0"), routeHex("192.168.1.254"), "0003", routeHex("255.255.255.0")),
		route("eth0", "00000000", routeHex("192.168.1.1"), "0003", "00000000"),
		route("eth0", "00000000", routeHex("192.168.1.1"), "0003", "00000000"),
	)
	got, err := parseRouteTable(strings.NewReader(table))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].Equal(net.ParseIP("192.168.1.1")) {
		t.Fatalf("gateways = %v, want [192.168.1.1]", got)
	}
}

func TestDefaultGateways_MissingTableIsNoGateway(t *testing.T) {
	prev := routeTable
	routeTable = filepath.Join(t.TempDir(), "absent")
	t.Cleanup(func() { routeTable = prev })
	got, err := DefaultGateways()
	if err != nil || got != nil {
		t.Fatalf("got %v, %v; want nil, nil", got, err)
	}
}

func TestDefaultGateways_ReadsTheTable(t *testing.T) {
	p := filepath.Join(t.TempDir(), "route")
	if err := os.WriteFile(p, []byte(routeTableOf(route("eth0", "00000000", routeHex("172.18.0.1"), "0003", "00000000"))), 0o600); err != nil {
		t.Fatal(err)
	}
	prev := routeTable
	routeTable = p
	t.Cleanup(func() { routeTable = prev })
	got, err := DefaultGateways()
	if err != nil || len(got) != 1 || !got[0].Equal(net.ParseIP("172.18.0.1")) {
		t.Fatalf("got %v, %v; want [172.18.0.1]", got, err)
	}
}

// The hole this closes: an IPv6 client relayed by docker-proxy arrives as the
// bridge gateway, which sits in the default LAN list. It must be WAN, while
// the other addresses of the same range stay LAN.
func TestDetectIP_RelayIsWANEvenInsideTheLANList(t *testing.T) {
	withRelays(t, "172.17.0.1")
	d := NewDetector([]string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})
	cases := map[string]NetworkType{
		"172.17.0.1":  WAN,
		"172.17.0.5":  LAN,
		"192.168.1.5": LAN,
		"127.0.0.1":   LAN,
		"203.0.113.9": WAN,
	}
	for ip, want := range cases {
		if got := d.DetectIP(net.ParseIP(ip)); got != want {
			t.Errorf("DetectIP(%s) = %s, want %s", ip, got, want)
		}
	}
}

func TestDetectIP_NoRelayKeepsTheLANVerdict(t *testing.T) {
	withRelays(t)
	d := NewDetector([]string{"172.16.0.0/12"})
	if got := d.DetectIP(net.ParseIP("172.17.0.1")); got != LAN {
		t.Fatalf("DetectIP = %s, want LAN with no relay recorded", got)
	}
}

func TestIsRelay(t *testing.T) {
	withRelays(t, "172.17.0.1")
	if !IsRelay(net.ParseIP("172.17.0.1")) || !IsRelay(net.ParseIP("::ffff:172.17.0.1")) {
		t.Error("the gateway, in either form, must be a relay")
	}
	if IsRelay(net.ParseIP("172.17.0.2")) || IsRelay(nil) {
		t.Error("another address, or none, is not a relay")
	}
}
