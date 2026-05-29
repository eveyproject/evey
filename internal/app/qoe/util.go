package qoe

import (
	"net/http"
	"strings"
)

// DeviceClass derives a coarse client class from User-Agent at ingest time.
// Cheap to compute up front, expensive to do at dashboard query time across
// millions of rows. Kept deliberately coarse — we only need to slice
// "web vs mobile vs TV" in the admin view, not to fingerprint individual
// browsers.
func DeviceClass(r *http.Request) string {
	ua := strings.ToLower(r.UserAgent())
	switch {
	case ua == "":
		return "unknown"
	case strings.Contains(ua, "evey-tv") || strings.Contains(ua, "androidtv") || strings.Contains(ua, "apple tv"):
		return "tv"
	case strings.Contains(ua, "evey-mobile") || strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad"):
		return "ios"
	case strings.Contains(ua, "android"):
		return "android"
	case strings.Contains(ua, "safari") && !strings.Contains(ua, "chrome"):
		return "web-safari"
	case strings.Contains(ua, "chrome") || strings.Contains(ua, "chromium") || strings.Contains(ua, "edg/"):
		return "web-chromium"
	case strings.Contains(ua, "firefox"):
		return "web-firefox"
	default:
		return "web-other"
	}
}

// RemoteIP returns the request's remote address with the port stripped.
// Trusted-proxy unwinding is already done by the proxy middleware upstream,
// so r.RemoteAddr at this layer is already the real client IP.
func RemoteIP(r *http.Request) string {
	addr := r.RemoteAddr
	if addr == "" {
		return ""
	}
	if i := strings.LastIndexByte(addr, ':'); i > 0 {
		// IPv6 literals are bracketed: "[::1]:1234" → "::1".
		host := addr[:i]
		host = strings.TrimPrefix(host, "[")
		host = strings.TrimSuffix(host, "]")
		return host
	}
	return addr
}
