package pluginrunner

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
)

// buildHostsFile resolves all concrete hosts from externalFetch and writes a
// minimal /etc/hosts file to a temp path. Returns the path + cleanup func.
// Wildcard entries (*.example.com) are skipped in V1 (can't enumerate subdomains)
// and logged as a warning.
// Returns ("", noop, nil) when externalFetch is empty.
func buildHostsFile(pluginID string, externalFetch []string) (path string, cleanup func(), err error) {
	noop := func() {}
	if len(externalFetch) == 0 {
		return "", noop, nil
	}

	type hostEntry struct {
		host string
		ips  []string
	}

	var entries []hostEntry
	for _, raw := range externalFetch {
		host := extractHost(raw)
		if strings.HasPrefix(host, "*.") || host == "*" {
			slog.Warn("wildcard host skipped in V1 DNS whitelist",
				slog.String("plugin_id", pluginID),
				slog.String("entry", raw))
			continue
		}
		if host == "" {
			continue
		}
		ips, lookupErr := net.LookupHost(host)
		if lookupErr != nil {
			slog.Warn("DNS lookup failed for external_fetch host",
				slog.String("plugin_id", pluginID),
				slog.String("host", host),
				slog.Any("err", lookupErr))
			continue
		}
		entries = append(entries, hostEntry{host: host, ips: ips})
	}

	if len(entries) == 0 {
		// All entries were wildcards or lookup failures — non-blocking in V1.
		return "", noop, nil
	}

	f, tmpErr := os.CreateTemp("", "evey-plugin-hosts-*")
	if tmpErr != nil {
		return "", noop, fmt.Errorf("buildHostsFile: create temp: %w", tmpErr)
	}
	tmpPath := f.Name()

	var b strings.Builder
	b.WriteString("127.0.0.1 localhost\n")
	b.WriteString("::1 localhost\n")
	for _, e := range entries {
		for _, ip := range e.ips {
			b.WriteString(ip)
			b.WriteByte(' ')
			b.WriteString(e.host)
			b.WriteByte('\n')
		}
	}

	if _, writeErr := f.WriteString(b.String()); writeErr != nil {
		f.Close()  //nolint:errcheck
		os.Remove(tmpPath) //nolint:errcheck
		return "", noop, fmt.Errorf("buildHostsFile: write: %w", writeErr)
	}
	if closeErr := f.Close(); closeErr != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return "", noop, fmt.Errorf("buildHostsFile: close: %w", closeErr)
	}

	return tmpPath, func() { os.Remove(tmpPath) }, nil //nolint:errcheck
}

// extractHost strips scheme and path from an entry, returning the bare hostname.
func extractHost(entry string) string {
	if idx := strings.Index(entry, "://"); idx >= 0 {
		rest := entry[idx+3:]
		if slash := strings.IndexByte(rest, '/'); slash >= 0 {
			rest = rest[:slash]
		}
		// strip port
		if colon := strings.LastIndexByte(rest, ':'); colon >= 0 {
			rest = rest[:colon]
		}
		return rest
	}
	// bare hostname — strip port if present
	if colon := strings.LastIndexByte(entry, ':'); colon >= 0 {
		return entry[:colon]
	}
	return entry
}
