package urlvalidator

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

var privateRanges []*net.IPNet

func init() {
	cidrs := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"0.0.0.0/8",
		"::1/128",
		"fd00::/8",
		"fe80::/10",
	}
	for _, cidr := range cidrs {
		_, ipNet, _ := net.ParseCIDR(cidr) //nolint:errcheck // hardcoded valid CIDRs
		if ipNet != nil {
			privateRanges = append(privateRanges, ipNet)
		}
	}
}

func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	for _, r := range privateRanges {
		if r.Contains(ip) {
			return true
		}
	}
	return false
}

// ValidateRedirectURL checks that a URL is safe for open-redirect responses.
// Unlike ValidateWebhookURL, it does not resolve DNS because the browser makes
// the request — no server-side SSRF risk. It still blocks non-http(s) schemes
// and literal private-IP hosts to prevent javascript:/file:// injection and
// redirects to internal infrastructure.
func ValidateRedirectURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("scheme %q not allowed, must be http or https", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("empty host")
	}
	if ip := net.ParseIP(host); ip != nil && isPrivateIP(ip) {
		return fmt.Errorf("IP %s is private/internal", ip)
	}
	return nil
}

// ValidateWebhookURL checks that a URL is safe for outbound webhook delivery.
// Blocks private/internal IPs, non-HTTP(S) schemes, and resolves DNS
// to prevent DNS rebinding attacks.
func ValidateWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("scheme %q not allowed, must be http or https", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("empty host")
	}

	blockedHosts := []string{"localhost", "metadata.google.internal", "100.100.100.200"}
	for _, b := range blockedHosts {
		if strings.EqualFold(host, b) {
			return fmt.Errorf("host %q is blocked", host)
		}
	}

	// Try parsing as IP literal first
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("IP %s is private/internal", ip)
		}
		return nil
	}

	// Resolve DNS to get actual IPs (prevents DNS rebinding)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("cannot resolve host %q: %w", host, err)
	}

	for _, addr := range addrs {
		if isPrivateIP(addr.IP) {
			return fmt.Errorf("host %q resolves to private IP %s", host, addr.IP)
		}
	}

	return nil
}
