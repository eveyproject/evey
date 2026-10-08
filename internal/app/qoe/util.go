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
	// "crkey" is what a Chromecast puts in its UA, and it also says "Chrome":
	// checked first, or every cast session is a desktop browser. Its own
	// class, not "tv": the cast receiver (CAF + Shaka, apps/web/src/cast-receiver)
	// is a third player next to the native TV app and a TV browser, and the
	// table is there to tell which one stalls.
	case strings.Contains(ua, "crkey"):
		return "cast"
	// Checked before the generic platform sniffing: the TV app's UA also says
	// "Android TV", so an android-first order would file every TV as a phone.
	// tizen/webos are the two smart-TV browsers that reach the same endpoint
	// (a /tv page there says so with X-Evey-Client, see DeviceClassOf). The
	// web panel's own detector already knew about all of them
	// (frontend/apps/web/src/features/admin/onlineDevice.ts); this side had
	// never been brought in line.
	case strings.Contains(ua, "eveytv"), strings.Contains(ua, "androidtv"),
		strings.Contains(ua, "android tv"), strings.Contains(ua, "apple tv"),
		strings.Contains(ua, "tizen"),
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

// ClientHeader is the request header a web surface sets to say which app it
// is when its User-Agent cannot: /tv in a browser is the same Chrome as a
// desktop tab, and on a webOS or Tizen set it is the same UA as any page.
const ClientHeader = "X-Evey-Client"

// ClientTVWeb is the ClientHeader value of frontend/apps/tv-web (/tv).
const ClientTVWeb = "tv-web"

// DeviceClassOf is DeviceClass with the client's own word on top: a /tv
// browser files under "web-tv-<platform>" (web-tv-webos, web-tv-tizen,
// web-tv-chromium on a PC) instead of "tv" (the native app) or "web-chromium".
// Any other or absent header leaves the UA sniffing alone.
func DeviceClassOf(userAgent, client string) string {
	if strings.EqualFold(strings.TrimSpace(client), ClientTVWeb) {
		return "web-tv-" + tvBrowserPlatform(strings.ToLower(userAgent))
	}
	return DeviceClass(userAgent)
}

// tvBrowserPlatform names the browser /tv runs in: the TV platform when the
// UA says so, the browser family otherwise. Same product tokens as
// DeviceClass, same order (Chrome also says Safari).
func tvBrowserPlatform(ua string) string {
	switch {
	case strings.Contains(ua, "tizen"):
		return "tizen"
	case strings.Contains(ua, "webos"), strings.Contains(ua, "web0s"):
		return "webos"
	case strings.Contains(ua, "firefox"):
		return "firefox"
	case strings.Contains(ua, "chrome"), strings.Contains(ua, "chromium"), strings.Contains(ua, "edg/"):
		return "chromium"
	case strings.Contains(ua, "safari"):
		return "safari"
	default:
		return "other"
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
