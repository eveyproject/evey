package config

import (
	"net"
	"net/url"
	"strings"
)

// remote_access_resolve.go - make the remote-access default match the
// deployment instead of contradicting it.
//
// The 2026-07-29 incident: RemoteAccess.Enabled defaults to false, and
// networkMiddleware refuses EVERY WAN client on EVERY route but /healthz. On an
// instance published at https://evey.example.com that means every remote
// user is locked out - and the failure was invisible from both ends (no server
// log, and clients rendered raw i18n keys because even the translation
// catalogue was refused). The gate had in fact been dormant since March: the
// app only ever saw a container IP, so everyone was classified LAN. Adding
// `trusted_proxies` to the reverse proxy handed it the real client IPs and woke
// it up, with no application commit involved.
//
// The lesson is not "pick the other default": it is that "is this server
// published?" is knowable, so the server must answer it itself rather than
// require an operator to have flipped a switch they never saw.

// RemoteAccessDecision names how the effective Enabled value was arrived at, so
// boot logging and the admin panel can explain the state instead of just
// showing a boolean.
type RemoteAccessDecision string

const (
	// RemoteAccessExplicitOn / RemoteAccessExplicitOff: the operator stated a
	// choice (env var set, or saved in the panel). Always honoured.
	RemoteAccessExplicitOn  RemoteAccessDecision = "explicit-on"
	RemoteAccessExplicitOff RemoteAccessDecision = "explicit-off"
	// RemoteAccessInferredOn: nobody chose, and the server is published at a
	// publicly reachable URL - so remote clients are the point of the
	// deployment, not an exception to it.
	RemoteAccessInferredOn RemoteAccessDecision = "inferred-on"
	// RemoteAccessDefaultOff: nobody chose and nothing says this instance is
	// published. Staying closed is the safe reading of a LAN-only deployment.
	RemoteAccessDefaultOff RemoteAccessDecision = "default-off"
)

// RemoteAccessResolution is the outcome of ResolveRemoteAccess: what was
// decided, on what evidence, and whether the result is the trap this whole file
// exists to prevent.
type RemoteAccessResolution struct {
	Decision RemoteAccessDecision
	// PublicURL is the address that made this instance look published, or ""
	// when none did.
	PublicURL string
	// LockoutRisk is true when the server is published AND remote access is
	// refused: every remote user gets a 403 on everything. Only reachable by an
	// explicit operator choice now, and it must be shouted about (boot WARN +
	// admin panel) rather than discovered from a user complaint two days later.
	LockoutRisk bool
}

// ResolveRemoteAccess settles ra.Enabled, mutating ra in place, and reports how.
// baseURL is the server's configured base URL (cfg.Server.BaseURL).
//
// Order matters: an explicit choice is never overridden - an operator who wants
// a LAN-only instance keeps it, and gets a loud warning instead of a silent
// surprise. Inference only ever turns remote access ON, never off: refusing
// traffic that used to work is precisely the failure mode being fixed.
func ResolveRemoteAccess(ra *RemoteAccessConfig, baseURL string) RemoteAccessResolution {
	if ra == nil {
		return RemoteAccessResolution{Decision: RemoteAccessDefaultOff}
	}

	publicURL := ""
	for _, candidate := range []string{ra.ExternalURL, baseURL} {
		if IsPubliclyReachableURL(candidate) {
			publicURL = candidate
			break
		}
	}

	if ra.EnabledExplicit {
		if ra.Enabled {
			return RemoteAccessResolution{Decision: RemoteAccessExplicitOn, PublicURL: publicURL}
		}
		return RemoteAccessResolution{
			Decision:    RemoteAccessExplicitOff,
			PublicURL:   publicURL,
			LockoutRisk: publicURL != "",
		}
	}

	// Already on without an explicit marker: an older persisted config, or a
	// panel save from a build that predates the marker. Nothing to decide.
	if ra.Enabled {
		return RemoteAccessResolution{Decision: RemoteAccessExplicitOn, PublicURL: publicURL}
	}

	if publicURL != "" {
		ra.Enabled = true
		return RemoteAccessResolution{Decision: RemoteAccessInferredOn, PublicURL: publicURL}
	}
	return RemoteAccessResolution{Decision: RemoteAccessDefaultOff}
}

// IsPubliclyReachableURL reports whether raw is an http(s) address that a
// client OUTSIDE the home network could plausibly reach: a real hostname or a
// globally routable IP.
//
// Deliberately conservative - it decides whether to open a door, so every
// "can't tell" answers false: private and loopback ranges, CGNAT (100.64/10,
// carrier-side and not publishable), link-local, mDNS/.local names, and bare
// single-label names (container and compose service names like "evey-server",
// which is what the app talks to internally).
func IsPubliclyReachableURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := u.Hostname()
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return isGloballyRoutableIP(ip)
	}

	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	// mDNS and the conventional intranet suffixes: LAN-only by definition.
	for _, suffix := range []string{".local", ".lan", ".home", ".internal", ".home.arpa"} {
		if strings.HasSuffix(host, suffix) {
			return false
		}
	}
	// A single-label name has no public DNS meaning ("evey", "nas", "evey-server").
	return strings.Contains(host, ".")
}

// isGloballyRoutableIP excludes every range that cannot be reached from the
// public internet. net.IP's own helpers cover loopback/link-local/private;
// CGNAT and the documentation/benchmark ranges need explicit CIDRs.
func isGloballyRoutableIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	for _, cidr := range nonRoutableCIDRs {
		if cidr.Contains(ip) {
			return false
		}
	}
	return true
}

// nonRoutableCIDRs are the ranges net.IP has no predicate for: carrier NAT and
// the reserved documentation blocks (a test fixture must never be read as "this
// instance is published").
var nonRoutableCIDRs = mustParseCIDRs(
	"100.64.0.0/10", // RFC 6598 CGNAT
	"192.0.2.0/24",  // RFC 5737 documentation
	"198.51.100.0/24",
	"203.0.113.0/24",
	"198.18.0.0/15", // RFC 2544 benchmarking
	"fc00::/7",      // unique-local (IsPrivate covers this, kept explicit)
	"2001:db8::/32", // documentation
)

func mustParseCIDRs(cidrs ...string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		if _, n, err := net.ParseCIDR(c); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// RemoteAccessStatus reports the two derived facts the admin panel needs:
// whether this instance is published at a publicly reachable address, and
// whether that combines with a disabled gate into a lockout - every client
// outside the local network refused on every route.
//
// One function, called by both the HTTP and the gRPC handler, because they
// answer the same question to the same panel. The pair had already been
// computed inline in one of them; a second inline copy is how two callers start
// disagreeing about whether the household is locked out.
func RemoteAccessStatus(ra *RemoteAccessConfig, serverBaseURL string) (published, lockoutRisk bool) {
	if ra == nil {
		return false, false
	}
	published = IsPubliclyReachableURL(ra.ExternalURL) || IsPubliclyReachableURL(serverBaseURL)
	return published, published && !ra.Enabled
}
