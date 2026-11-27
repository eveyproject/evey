package network

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
)

// A relay is an address that hands over connections it did not originate:
// the server's own default gateway. A request whose client IP is a relay has
// an origin the server cannot see. It is still classified by the LAN list,
// because the same address also carries the host's own connections: Docker
// relays a connection to a port published on 127.0.0.1 through docker-proxy as
// well, so a browser or a reverse proxy on the host arrives from the gateway
// too, and nothing inside the container tells it from a relayed IPv6 client.
// A relay is therefore detected and reported (NoteRelayed), not refused.
//
// The case that makes this necessary: a container on an IPv4-only Docker
// network, whose port is published on the host's IPv6 addresses too. Docker
// reaches the container from an IPv6 client through docker-proxy, a plain TCP
// relay that opens a fresh connection from the bridge gateway. Every IPv6
// client, from the next room or from the other side of the world, then arrives
// as that one private address, and a private address used to mean LAN. A router
// doing NAT loopback hides its clients the same way.
//
// Only the IPv4 default gateway is a relay: docker-proxy speaks to an
// IPv4-only container from it, and an IPv6 router routes instead of relaying.

var (
	relays     atomic.Pointer[[]net.IP]
	relaySeen  atomic.Bool
	routeTable = "/proc/net/route"
)

// SetRelays records the relay addresses, read once at boot. Nil clears them.
func SetRelays(ips []net.IP) {
	cp := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if ip != nil {
			cp = append(cp, ip)
		}
	}
	relays.Store(&cp)
}

// IsRelay reports whether ip is one of the addresses SetRelays recorded.
func IsRelay(ip net.IP) bool {
	if ip == nil {
		return false
	}
	list := relays.Load()
	if list == nil {
		return false
	}
	for _, r := range *list {
		if r.Equal(ip) {
			return true
		}
	}
	return false
}

// NoteRelayed records that a request arrived from a relay, which is what the
// admin panel warns about: the server cannot tell its clients apart.
func NoteRelayed() { relaySeen.Store(true) }

// RelayedSeen reports whether a request has arrived from a relay since boot.
func RelayedSeen() bool { return relaySeen.Load() }

// DefaultGateways reads the IPv4 default gateways of the kernel routing table.
// A missing table (not Linux) is no gateway, not an error.
func DefaultGateways() ([]net.IP, error) {
	f, err := os.Open(routeTable)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return parseRouteTable(f)
}

// rtfGateway is RTF_GATEWAY: the route goes through a gateway.
const rtfGateway = 0x2

// parseRouteTable reads /proc/net/route: whitespace-separated columns Iface,
// Destination, Gateway, Flags, ..., Mask, with the addresses printed as the
// in-memory 32-bit value in hex, so in host byte order.
func parseRouteTable(r io.Reader) ([]net.IP, error) {
	var out []net.IP
	sc := bufio.NewScanner(r)
	first := true
	for sc.Scan() {
		if first {
			first = false
			continue
		}
		f := strings.Fields(sc.Text())
		if len(f) < 8 || f[1] != "00000000" || f[7] != "00000000" {
			continue
		}
		flags, err := strconv.ParseUint(f[3], 16, 32)
		if err != nil || flags&rtfGateway == 0 {
			continue
		}
		raw, err := hex.DecodeString(f[2])
		if err != nil || len(raw) != 4 {
			continue
		}
		b := make([]byte, 4)
		binary.NativeEndian.PutUint32(b, binary.BigEndian.Uint32(raw))
		ip := net.IPv4(b[0], b[1], b[2], b[3])
		if ip.IsUnspecified() {
			continue
		}
		dup := false
		for _, o := range out {
			if o.Equal(ip) {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, ip)
		}
	}
	return out, sc.Err()
}
