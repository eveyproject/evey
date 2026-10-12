package main

import (
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/config"
)

// `evey admin doctor` is where a self-hoster looks when "it works for me but not
// for my family". Before this check, the single most locking misconfiguration -
// a published server refusing every remote client - was the one thing the
// doctor could not tell them (2026-07-29: two days of diagnosis).

func TestCheckRemoteAccess(t *testing.T) {
	t.Run("published and refusing everyone is a FAIL with the exact fix", func(t *testing.T) {
		got := checkRemoteAccess(config.RemoteAccessConfig{Enabled: false, EnabledExplicit: true},
			"https://evey.example.com")
		if got.status != "FAIL" {
			t.Fatalf("status = %q, want FAIL: every remote user is locked out", got.status)
		}
		if !strings.Contains(got.hint, "Remote access") && !strings.Contains(got.hint, "EVEY_REMOTE_ACCESS_ENABLED") {
			t.Errorf("hint = %q, want the actual remedy", got.hint)
		}
		if !strings.Contains(got.detail, "evey.example.com") {
			t.Errorf("detail = %q, want the public URL that makes this a lockout", got.detail)
		}
	})

	t.Run("published and open is ok", func(t *testing.T) {
		got := checkRemoteAccess(config.RemoteAccessConfig{Enabled: true}, "https://evey.example.com")
		if got.status != "ok" {
			t.Errorf("status = %q, want ok", got.status)
		}
	})

	t.Run("LAN-only instance is ok, not a warning", func(t *testing.T) {
		got := checkRemoteAccess(config.RemoteAccessConfig{Enabled: false}, "http://localhost:9091")
		if got.status != "ok" {
			t.Errorf("status = %q, want ok: a LAN-only server closed to the WAN is correct", got.status)
		}
	})
}
