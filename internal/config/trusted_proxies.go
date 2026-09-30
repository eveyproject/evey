package config

import (
	"fmt"
	"net"
	"os"
	"strings"
)

// trusted_proxies.go - EVEY_TRUSTED_PROXIES ADDS to the loopback baseline
// instead of replacing it.
//
// The bug this file exists to prevent: the variable used to overwrite the
// default `127.0.0.1/32,::1/128`. Production sets it to `172.16.0.0/12` (the
// Docker network), so loopback stopped being a trusted proxy while the reverse
// proxy talks to Evey over exactly that path. Two protections were disarmed at
// once, silently:
//
//  1. Every client's X-Forwarded-For was dropped and the rate limiter keyed on
//     127.0.0.1 - one shared login budget for the whole household, so four
//     automated clients closed the login page for everyone.
//  2. An IP of 127.0.0.1 is LAN (network.Detector.DetectIP), so every visitor
//     from the internet was classified LAN and the remote-access gate, which
//     reads that verdict, opened for all of them.
//
// Nothing in the name "trusted proxies" suggests it removes trust, and no
// operator reads a defaulted list they never typed. Adding is the reading
// everyone has; it is also the only one that cannot disarm a guard by accident.

// mandatoryTrustedProxies is always part of the effective list. This is not a
// default an operator may narrow: the server's own HTTP->gRPC gateway dials the
// gRPC listener over loopback (cmd/evey/gateway.go, EVEY_GRPC_BIND defaults to
// 127.0.0.1), so every gateway-forwarded request has 127.0.0.1 as its gRPC
// peer. Distrusting loopback does not harden anything - it only blinds the
// server to every client's real address.
//
// It costs nothing either: a packet with a 127.0.0.0/8 source is a martian and
// is dropped on any non-loopback interface, so "the peer is loopback" means
// "the peer is a process on this host" - and such a process can read the JWT
// signing key and the database socket long before it needs to forge a header.
// Hence no opt-out: there is no deployment that gains from one.
var mandatoryTrustedProxies = []string{"127.0.0.1/32", "::1/128"}

// trustedProxiesFromEnv returns the loopback baseline plus the operator's
// entries, deduplicated and in a canonical form. A malformed entry is an error:
// the list decides who may rename a client, and a silently dropped entry means
// the proxy is not trusted, which is the failure mode above.
func trustedProxiesFromEnv(key string) ([]string, error) {
	out, err := NormalizeTrustedProxies(strings.Split(os.Getenv(key), ","))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	return out, nil
}

// NormalizeTrustedProxies is the one reading of a trusted-proxy list, whether
// it comes from EVEY_TRUSTED_PROXIES or from Admin -> Remote Access: the
// loopback baseline, then the entries, canonical and deduplicated. Empty
// entries are skipped; a malformed one is an error naming it.
func NormalizeTrustedProxies(entries []string) ([]string, error) {
	out := make([]string, 0, len(mandatoryTrustedProxies)+len(entries))
	seen := make(map[string]struct{}, len(mandatoryTrustedProxies)+len(entries))
	add := func(cidr string) {
		if _, dup := seen[cidr]; dup {
			return
		}
		seen[cidr] = struct{}{}
		out = append(out, cidr)
	}
	for _, base := range mandatoryTrustedProxies {
		norm, err := normalizeProxyCIDR(base)
		if err != nil {
			return nil, fmt.Errorf("built-in entry %q is not a CIDR: %w", base, err)
		}
		add(norm)
	}
	var bad []string
	for _, raw := range entries {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		norm, err := normalizeProxyCIDR(entry)
		if err != nil {
			bad = append(bad, entry)
			continue
		}
		add(norm)
	}
	if len(bad) > 0 {
		return nil, fmt.Errorf("invalid entries %v - each entry must be a CIDR (172.16.0.0/12) or a bare address (172.18.0.2)", bad)
	}
	return out, nil
}

// IsCatchAllProxyRange reports a normalized entry that covers every address:
// trusting it lets any client choose the address it is seen with.
func IsCatchAllProxyRange(cidr string) bool {
	return cidr == "0.0.0.0/0" || cidr == "::/0"
}

// normalizeProxyCIDR canonicalises one entry. A bare address is read as a
// single-host prefix: it is the notation operators type for one proxy, and it
// used to abort the boot from deep inside the HTTP wiring (network.NewProxyTrust)
// without naming the variable at fault.
func normalizeProxyCIDR(entry string) (string, error) {
	if ip := net.ParseIP(entry); ip != nil {
		bits := 128
		if ip.To4() != nil {
			ip = ip.To4()
			bits = 32
		}
		return (&net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}).String(), nil
	}
	_, ipNet, err := net.ParseCIDR(entry)
	if err != nil {
		return "", err
	}
	return ipNet.String(), nil
}
