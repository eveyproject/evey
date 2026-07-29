package config

import "testing"

// The 2026-07-29 lockout: remote access defaulted OFF on an instance published
// at https://evey.example.com, so networkMiddleware refused EVERY remote
// client on EVERY route. Nobody could see it (no log, no panel warning) and the
// clients showed raw i18n keys. These tests pin the inference that makes the
// default match what the operator actually deployed.

func TestIsPubliclyReachableURL(t *testing.T) {
	public := []string{
		"https://evey.example.com",
		"http://evey.example.com:8096",
		"https://media.example.co.uk/",
		"http://93.184.216.34",
		"https://[2606:2800:220:1:248:1893:25c8:1946]:9091",
	}
	for _, u := range public {
		if !IsPubliclyReachableURL(u) {
			t.Errorf("IsPubliclyReachableURL(%q) = false, want true", u)
		}
	}

	private := []string{
		"",
		"http://localhost:9091",
		"http://127.0.0.1:9091",
		"https://[::1]:9091",
		"http://192.168.1.50:9091",
		"http://10.0.0.5",
		"http://172.16.4.4",
		"http://100.64.0.1", // CGNAT: carrier-side, not publishable
		"http://169.254.10.1",
		"http://nas.local:9091",
		"http://evey:9091",     // bare container/host name
		"http://evey-server",   // compose service name
		"not-a-url-at-all",
		"ftp://evey.example.com", // not an HTTP endpoint
		"http://203.0.113.9",     // RFC 5737 documentation range: never a real deployment
		"https://[2001:db8::1]",  // idem, IPv6
	}
	for _, u := range private {
		if IsPubliclyReachableURL(u) {
			t.Errorf("IsPubliclyReachableURL(%q) = true, want false", u)
		}
	}
}

func TestResolveRemoteAccess(t *testing.T) {
	t.Run("public base URL turns remote access on when nobody chose", func(t *testing.T) {
		ra := RemoteAccessConfig{}
		got := ResolveRemoteAccess(&ra, "https://evey.example.com")
		if !ra.Enabled {
			t.Fatal("Enabled = false, want true: a published instance must serve its remote users")
		}
		if got.Decision != RemoteAccessInferredOn {
			t.Errorf("Decision = %q, want %q", got.Decision, RemoteAccessInferredOn)
		}
		if got.PublicURL != "https://evey.example.com" {
			t.Errorf("PublicURL = %q, want the base URL", got.PublicURL)
		}
	})

	t.Run("external URL alone is enough", func(t *testing.T) {
		ra := RemoteAccessConfig{ExternalURL: "https://evey.example.com"}
		got := ResolveRemoteAccess(&ra, "http://localhost:9091")
		if !ra.Enabled || got.Decision != RemoteAccessInferredOn {
			t.Fatalf("Enabled=%v Decision=%q, want true/%q", ra.Enabled, got.Decision, RemoteAccessInferredOn)
		}
	})

	t.Run("LAN-only deployment stays off", func(t *testing.T) {
		ra := RemoteAccessConfig{}
		got := ResolveRemoteAccess(&ra, "http://192.168.1.50:9091")
		if ra.Enabled {
			t.Fatal("Enabled = true: a LAN-only instance must not be opened by inference")
		}
		if got.Decision != RemoteAccessDefaultOff {
			t.Errorf("Decision = %q, want %q", got.Decision, RemoteAccessDefaultOff)
		}
	})

	t.Run("an explicit operator choice always wins - even off, even when published", func(t *testing.T) {
		ra := RemoteAccessConfig{EnabledExplicit: true, Enabled: false}
		got := ResolveRemoteAccess(&ra, "https://evey.example.com")
		if ra.Enabled {
			t.Fatal("Enabled = true: an explicit LAN-only choice must be honoured")
		}
		if got.Decision != RemoteAccessExplicitOff {
			t.Errorf("Decision = %q, want %q", got.Decision, RemoteAccessExplicitOff)
		}
		if !got.LockoutRisk {
			t.Error("LockoutRisk = false: published + refused is exactly what must be shouted about")
		}
	})

	t.Run("explicit on is reported as explicit, not inferred", func(t *testing.T) {
		ra := RemoteAccessConfig{EnabledExplicit: true, Enabled: true}
		got := ResolveRemoteAccess(&ra, "https://evey.example.com")
		if !ra.Enabled || got.Decision != RemoteAccessExplicitOn {
			t.Fatalf("Enabled=%v Decision=%q, want true/%q", ra.Enabled, got.Decision, RemoteAccessExplicitOn)
		}
		if got.LockoutRisk {
			t.Error("LockoutRisk = true while remote access is ON")
		}
	})

	t.Run("already-on config is left alone and never flagged", func(t *testing.T) {
		ra := RemoteAccessConfig{Enabled: true}
		got := ResolveRemoteAccess(&ra, "http://192.168.1.50:9091")
		if !ra.Enabled || got.LockoutRisk {
			t.Fatalf("Enabled=%v LockoutRisk=%v, want true/false", ra.Enabled, got.LockoutRisk)
		}
	})

	t.Run("nil config does not panic", func(t *testing.T) {
		if got := ResolveRemoteAccess(nil, "https://evey.example.com"); got.Decision != RemoteAccessDefaultOff {
			t.Errorf("Decision = %q, want %q", got.Decision, RemoteAccessDefaultOff)
		}
	})
}
