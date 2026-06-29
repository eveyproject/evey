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
		{"evey tv", "Evey-TV/1.0", "tv"},
		{"android tv", "Mozilla/5.0 (Linux; Android 11; AndroidTV)", "tv"},
		{"apple tv", "AppleCoreMedia/1.0 (Apple TV)", "tv"},
		{"iphone", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)", "ios"},
		{"ipad", "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X)", "ios"},
		{"evey mobile", "Evey-Mobile/2.1", "ios"},
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
