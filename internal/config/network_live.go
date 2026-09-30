package config

import (
	"fmt"
	"strings"
)

// network_live.go - the two proxy facts Admin -> Remote Access can now change
// while the server runs. Both used to be read once at boot from the
// environment only: an operator who set an https External URL or a proxy
// range in the panel saw nothing happen, because the panel held look-alikes
// that decided neither.

// EffectiveTrustedProxies is the trusted-proxy list in force now: the panel's
// when it saved one, else the environment's. Loopback is always part of it.
func (c *Config) EffectiveTrustedProxies() []string {
	c.mu.RLock()
	panel := c.RemoteAccess.TrustedProxies
	env := c.Server.TrustedProxies
	c.mu.RUnlock()
	if len(panel) == 0 {
		return env
	}
	norm, err := NormalizeTrustedProxies(panel)
	if err != nil {
		// Saved entries are validated on write; a list that no longer parses
		// falls back to the environment rather than trusting nothing.
		return env
	}
	return norm
}

// EffectiveHTTPS reports whether the server is published over HTTPS, which
// sets the Secure flag on cookies, sends HSTS and trusts X-Forwarded-Proto from
// a trusted proxy. True when EVEY_SERVER_BASE_URL is https (or the deprecated
// EVEY_FORWARDED_PROTO forces it), or when the External URL saved in the panel
// is.
func (c *Config) EffectiveHTTPS() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Server.ForwardedProto || strings.HasPrefix(strings.ToLower(c.RemoteAccess.ExternalURL), "https://")
}

// PanelTrustedProxies validates a list typed in Admin -> Remote Access and
// returns it canonical, deduplicated and without the loopback baseline (always
// trusted anyway). A malformed entry is refused, and so is a range covering
// every address: trusting it would let any client choose the address it is
// seen with, which disarms the remote-access gate and the rate limits.
func PanelTrustedProxies(entries []string) ([]string, error) {
	norm, err := NormalizeTrustedProxies(entries)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(norm))
	for _, cidr := range norm {
		if IsCatchAllProxyRange(cidr) {
			return nil, fmt.Errorf("%s would trust every address: list the proxy's own address or network", cidr)
		}
		if isMandatoryProxy(cidr) {
			continue
		}
		out = append(out, cidr)
	}
	return out, nil
}

// TrustedProxiesView is the list in force as the panel shows it: without the
// loopback baseline, and whether it comes from the panel or the environment.
// Not to be called under the config lock.
func (c *Config) TrustedProxiesView() (list []string, fromPanel bool) {
	c.mu.RLock()
	fromPanel = len(c.RemoteAccess.TrustedProxies) > 0
	c.mu.RUnlock()
	for _, cidr := range c.EffectiveTrustedProxies() {
		if !isMandatoryProxy(cidr) {
			list = append(list, cidr)
		}
	}
	return list, fromPanel
}

func isMandatoryProxy(cidr string) bool {
	for _, base := range mandatoryTrustedProxies {
		if norm, err := normalizeProxyCIDR(base); err == nil && norm == cidr {
			return true
		}
	}
	return false
}
