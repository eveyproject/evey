// Package extfetch validates external_fetch host-whitelist entries declared in plugin manifests.
// Each entry is a string of the form [scheme://]host[/path].
package extfetch

import (
	"fmt"
	"net"
	"strings"
)

// Validate checks that all entries in the external_fetch list follow the
// strict host-whitelist rules. Returns the first invalid entry with explanation.
func Validate(entries []string) error {
	for _, entry := range entries {
		if err := validateEntry(entry); err != nil {
			return fmt.Errorf("external_fetch entry %q: %w", entry, err)
		}
	}
	return nil
}

func validateEntry(raw string) error {
	if raw == "" {
		return fmt.Errorf("empty entry not allowed")
	}

	// Must not start with a bare slash (path without host).
	if strings.HasPrefix(raw, "/") {
		return fmt.Errorf("must specify a host, not a bare path")
	}

	// Parse scheme.
	rest := raw
	if idx := strings.Index(raw, "://"); idx != -1 {
		scheme := strings.ToLower(raw[:idx])
		if scheme != "https" {
			return fmt.Errorf("scheme %q not allowed, only https is permitted", scheme)
		}
		rest = raw[idx+3:]
	}
	// rest is now "host[/path...]"

	// Split host from optional path.
	hostPart := rest
	if slash := strings.Index(rest, "/"); slash != -1 {
		hostPart = rest[:slash]
		// path := rest[slash:] — wildcards in path are allowed, no further validation needed
	}

	if hostPart == "" {
		return fmt.Errorf("host must not be empty")
	}

	// Normalise to lowercase for validation.
	hostPart = strings.ToLower(hostPart)

	if err := validateHost(hostPart); err != nil {
		return err
	}
	return nil
}

// validateHost enforces the host label rules.
func validateHost(host string) error {
	// Reject bare IP literals.
	if isIPLiteral(host) {
		return fmt.Errorf("IP address %q is not allowed; use a domain name", host)
	}
	// Reject localhost.
	if host == "localhost" {
		return fmt.Errorf("localhost is not allowed")
	}

	labels := strings.Split(host, ".")

	// A single label with no dots: even if it's "*", it is a catch-all — reject.
	if len(labels) == 1 {
		return fmt.Errorf("host %q must contain at least two labels (e.g. example.com)", host)
	}

	// The two rightmost labels (TLD + SLD) must be concrete (no wildcard).
	tld := labels[len(labels)-1]
	sld := labels[len(labels)-2]
	if tld == "*" || sld == "*" {
		return fmt.Errorf("wildcard in TLD or SLD is not allowed (got %q)", host)
	}

	// Wildcard is only allowed as the leftmost label.
	for i, label := range labels {
		if label == "*" {
			if i != 0 {
				return fmt.Errorf("wildcard must be the leftmost label (got %q)", host)
			}
			// Only one wildcard, and it must be leftmost — already enforced above.
			continue
		}
		// Labels containing embedded wildcard (e.g. "foo*bar") are rejected.
		if strings.Contains(label, "*") {
			return fmt.Errorf("label %q contains an embedded wildcard — only a standalone '*' leftmost label is allowed", label)
		}
	}

	// Reject *.tld pattern: wildcard + only one concrete label after it (e.g. *.com).
	// We require at least two concrete labels after the wildcard.
	if labels[0] == "*" && len(labels) < 3 {
		return fmt.Errorf("wildcard %q matches too broadly — need at least two concrete labels (e.g. *.example.com)", host)
	}

	return nil
}

// isIPLiteral returns true for IPv4, IPv6, and private/loopback ranges.
func isIPLiteral(host string) bool {
	// Strip IPv6 brackets.
	h := host
	if strings.HasPrefix(h, "[") && strings.HasSuffix(h, "]") {
		h = h[1 : len(h)-1]
	}
	ip := net.ParseIP(h)
	return ip != nil
}
