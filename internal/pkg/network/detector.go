package network

import (
	"context"
	"net"
	"net/http"
)

type NetworkType string

const (
	LAN NetworkType = "lan"
	WAN NetworkType = "wan"
)

type contextKey struct{}

// clientIPKey carries the ONE authoritative client IP for a request, put there
// by ProxyTrust.Middleware after it has validated the proxy chain. It is the
// only channel the LAN/WAN decision reads: see Detect.
type clientIPKey struct{}

// WithClientIP records the authoritative client IP resolved at the trusted
// boundary. Called by ProxyTrust.Middleware; nothing else should.
func WithClientIP(ctx context.Context, ip net.IP) context.Context {
	return context.WithValue(ctx, clientIPKey{}, ip)
}

// ClientIPFromContext returns the IP WithClientIP stored, or nil when the
// request never crossed the trusted boundary.
func ClientIPFromContext(ctx context.Context) net.IP {
	ip, _ := ctx.Value(clientIPKey{}).(net.IP)
	return ip
}

// peerKey carries the address the connection came from (the proxy's, when
// the request came through one), before ProxyTrust.Middleware rewrites
// RemoteAddr to the client behind it.
type peerKey struct{}

// WithPeerIP records the connection's own peer. Called by
// ProxyTrust.Middleware.
func WithPeerIP(ctx context.Context, ip net.IP) context.Context {
	return context.WithValue(ctx, peerKey{}, ip)
}

// PeerIPFromContext returns the IP WithPeerIP stored, or nil when the request
// never crossed the trusted boundary (RemoteAddr is then the peer).
func PeerIPFromContext(ctx context.Context) net.IP {
	ip, _ := ctx.Value(peerKey{}).(net.IP)
	return ip
}

type Detector struct {
	lanNets []*net.IPNet
}

func NewDetector(cidrs []string) *Detector {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil {
			nets = append(nets, ipNet)
		}
	}
	return &Detector{lanNets: nets}
}

// Detect classifies the request's client as LAN or WAN. It reads ONLY the IP
// that ProxyTrust.Middleware validated at the trusted boundary (see
// WithClientIP) - never a request header.
//
// That distinction is the whole point. This function gates remote access: a
// LAN verdict opens the server to the caller. When it parsed X-Forwarded-For
// itself, its safety rested entirely on the middleware ORDER upstream - correct
// in the shipped chain, but a property no reader of THIS file could check, and
// one that any new mux or debug endpoint would silently break by letting a
// client claim `X-Forwarded-For: 127.0.0.1` and be classified LAN.
//
// No boundary crossed means no IP, and no IP means WAN: unknown is remote, so a
// gate that never saw the middleware refuses instead of opening. Same contract
// as DetectIP's nil case, and the same one the gRPC side already follows
// (interceptors.ClientIPFromCtx).
func (d *Detector) Detect(r *http.Request) NetworkType {
	return d.DetectIP(ClientIPFromContext(r.Context()))
}

// DetectIP classifies an already-resolved client IP as LAN or WAN. Used where
// the real client IP is resolved upstream (the gRPC client-info interceptor)
// rather than from an *http.Request. A nil IP is WAN (unknown = treat as
// remote), matching Detect.
func (d *Detector) DetectIP(ip net.IP) NetworkType {
	if ip == nil {
		return WAN
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return LAN
	}
	for _, n := range d.lanNets {
		if n.Contains(ip) {
			return LAN
		}
	}
	return WAN
}

// ClientIP resolves the request's real client IP for LOGGING, presence and
// discovery hints - never for an authorization decision (use Detect, which is
// deliberately stricter). It answers with the IP the trusted boundary
// validated; without one it falls back to the raw peer address, and never to a
// client-supplied header: a caller must not be able to choose the address that
// lands in the audit log or the presence panel.
func ClientIP(r *http.Request) net.IP {
	if ip := ClientIPFromContext(r.Context()); ip != nil {
		return ip
	}
	return peerIP(r)
}

// peerIP is the transport-level remote address, the only client identity that
// survives without the trusted boundary.
func peerIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return net.ParseIP(r.RemoteAddr)
	}
	return net.ParseIP(host)
}

func WithNetworkType(ctx context.Context, nt NetworkType) context.Context {
	return context.WithValue(ctx, contextKey{}, nt)
}

func NetworkInterfaces() ([]InterfaceInfo, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var result []InterfaceInfo
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			result = append(result, InterfaceInfo{
				Name: iface.Name,
				// The IP, not addr.String(): on a *net.IPNet the latter is the
				// CIDR ("172.18.0.5/16"), and this list answers "where can I
				// reach this server" in the admin panel - a mask is noise the
				// operator has to strip before typing the address. The loopback
				// filter just above already works on `ip`; only the value stored
				// disagreed with it.
				Address: ip.String(),
			})
		}
	}
	return result, nil
}

type InterfaceInfo struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}
