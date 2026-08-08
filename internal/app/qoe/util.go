package qoe

import (
	"strings"
)

// DeviceClass derives a coarse client class from a User-Agent string at ingest
// time. Cheap to compute up front, expensive to do at dashboard query time
// across millions of rows. Kept deliberately coarse - we only need to slice
// "web vs mobile vs TV" in the admin view, not to fingerprint individual
// browsers. Takes the raw UA string (not *http.Request) so this application-
// layer helper stays free of the net/http transport dependency.
// The product tokens below must stay in step with what the clients actually
// send, which is `EveyTV (Android TV 12; MiTV-AESP0)` and
// `EveyMobile/<version> (Android 14; SM-G991B)` or `EveyMobile/<version>
// (iOS 17.4)` - built in frontend/apps/{tv,mobile}/src/lib/clientIdentity.ts.
// They used to be spelled "evey-tv" and "evey-mobile" here, with a hyphen no
// client has ever sent, so both branches were dead: the TV app fell through to
// the generic "android" match and was counted as a phone, and the iOS app
// matched nothing at all and landed in "web-other". The unit test agreed with
// the code because it asserted on invented UA strings ("Evey-TV/1.0"), never on
// the ones the apps produce.
//
// Rows already written keep the class they were stored with; this only fixes
// what gets recorded from here on.
func DeviceClass(userAgent string) string {
	ua := strings.ToLower(userAgent)
	switch {
	case ua == "":
		return "unknown"
	// Checked before the generic platform sniffing: the TV app's UA also says
	// "Android TV", so an android-first order would file every TV as a phone.
	// "crkey" is what a Chromecast puts in its UA, and it also says "Chrome":
	// without this it landed in web-chromium, i.e. every cast session was
	// counted as a desktop browser. tizen/webos are the two smart-TV browsers
	// that reach the same endpoint. The web panel's own detector already knew
	// about all three (frontend/apps/web/src/features/admin/onlineDevice.ts);
	// this side had never been brought in line.
	case strings.Contains(ua, "eveytv"), strings.Contains(ua, "androidtv"),
		strings.Contains(ua, "android tv"), strings.Contains(ua, "apple tv"),
		strings.Contains(ua, "crkey"), strings.Contains(ua, "tizen"),
		// LG ships both spellings depending on firmware: "webOS" and "Web0S"
		// with a digit zero. Matching only the first misses part of the fleet.
		strings.Contains(ua, "webos"), strings.Contains(ua, "web0s"):
		return "tv"
	// One product token covers both mobile platforms, so the class comes from
	// the platform in parentheses rather than from the token itself.
	case strings.Contains(ua, "eveymobile"):
		if strings.Contains(ua, "android") {
			return "android"
		}
		return "ios"
	case strings.Contains(ua, "iphone"), strings.Contains(ua, "ipad"):
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

// RemoteIP strips the port from a remote-address string ("host:port" →
// "host"). Trusted-proxy unwinding is already done by the proxy middleware
// upstream, so the address passed here is already the real client IP. Takes a
// string (not *http.Request) to keep this layer transport-agnostic.
func RemoteIP(remoteAddr string) string {
	addr := remoteAddr
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
