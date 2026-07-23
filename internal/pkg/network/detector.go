package network

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type NetworkType string

const (
	LAN NetworkType = "lan"
	WAN NetworkType = "wan"
)

type contextKey struct{}

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

func (d *Detector) Detect(r *http.Request) NetworkType {
	return d.DetectIP(clientIP(r))
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

func clientIP(r *http.Request) net.IP {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		if ip := net.ParseIP(strings.TrimSpace(parts[0])); ip != nil {
			return ip
		}
	}
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		if ip := net.ParseIP(strings.TrimSpace(realIP)); ip != nil {
			return ip
		}
	}
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
				Name:    iface.Name,
				Address: addr.String(),
			})
		}
	}
	return result, nil
}

type InterfaceInfo struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}
