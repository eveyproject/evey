package qoe

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeviceClass(t *testing.T) {
	cases := []struct {
		name string
		ua   string
		want string
	}{
		{"empty UA", "", "unknown"},
		// The four cases below are the strings the shipped apps really send,
		// copied from frontend/apps/{tv,mobile}/src/lib/clientIdentity.ts. The
		// previous version of this table asserted on "Evey-TV/1.0" and
		// "Evey-Mobile/2.1", which no client has ever emitted: it passed while
		// the TV app was being counted as a phone and the iOS app as a desktop
		// browser. Any change to the UA builders must be mirrored here.
		{"evey tv app", "EveyTV (Android TV 12; MiTV-AESP0)", "tv"},
		{"evey tv app, no model", "EveyTV (Android TV)", "tv"},
		{"evey mobile on android", "EveyMobile/0.0.1-beta.37 (Android 14; SM-G991B)", "android"},
		{"evey mobile on ios", "EveyMobile/0.0.1-beta.37 (iOS 17.4)", "ios"},
		{"android tv", "Mozilla/5.0 (Linux; Android 11; AndroidTV)", "tv"},
		{"apple tv", "AppleCoreMedia/1.0 (Apple TV)", "tv"},
		// A Chromecast asks for segments with a UA that says both CrKey and
		// Chrome. Chrome must not win, or every cast session is filed as a
		// desktop browser.
		{"chromecast", "Mozilla/5.0 (X11; Linux armv7l) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 CrKey/1.56.500000", "tv"},
		{"samsung tizen tv", "Mozilla/5.0 (SMART-TV; Linux; Tizen 6.0) AppleWebKit/537.36 Chrome/76.0 Safari/537.36", "tv"},
		// LG writes it "Web0S" with a zero on some firmwares and "webOS" on
		// others; both are in the field, so both are pinned here.
		{"lg web0s tv (zero)", "Mozilla/5.0 (Web0S; Linux/SmartTV) AppleWebKit/537.36 Chrome/87.0 Safari/537.36", "tv"},
		{"lg webos tv (letter o)", "Mozilla/5.0 (webOS; Linux/SmartTV) AppleWebKit/537.36 Chrome/87.0 Safari/537.36", "tv"},
		{"iphone", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)", "ios"},
		{"ipad", "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X)", "ios"},
		// A real desktop-Chrome UA contains BOTH "Chrome" and "Safari"; the
		// safari branch requires !chrome, so Chrome must win → web-chromium.
		{"desktop chrome (also says safari)", "Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36", "web-chromium"},
		{"edge", "Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 Edg/120.0", "web-chromium"},
		{"chromium", "Mozilla/5.0 Chromium/120", "web-chromium"},
		// Real Safari: "Safari" present, no "Chrome".
		{"desktop safari", "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15", "web-safari"},
		{"firefox", "Mozilla/5.0 (Windows NT 10.0; rv:120.0) Gecko/20100101 Firefox/120.0", "web-firefox"},
		// Mobile Chrome on Android carries "Android" AND "Chrome"; the android
		// branch is checked before chrome → classed as android, not web-chromium.
		{"android chrome → android", "Mozilla/5.0 (Linux; Android 13) AppleWebKit/537.36 Chrome/120 Mobile Safari/537.36", "android"},
		{"unknown desktop", "curl/8.0", "web-other"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, DeviceClass(c.ua))
		})
	}
}

func TestRemoteIP(t *testing.T) {
	cases := []struct {
		name string
		addr string
		want string
	}{
		{"ipv4 with port", "1.2.3.4:5678", "1.2.3.4"},
		{"ipv4 no port", "1.2.3.4", "1.2.3.4"},
		{"ipv6 bracketed with port", "[::1]:1234", "::1"},
		{"ipv6 bracketed long", "[2001:db8::1]:443", "2001:db8::1"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, RemoteIP(c.addr))
		})
	}
}
