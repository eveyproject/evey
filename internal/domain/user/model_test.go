package user_test

import (
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/domain/user"
)

func TestQuickConnectStatusConstants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		status user.QuickConnectStatus
		want   string
	}{
		{user.Pending, "pending"},
		{user.Authorized, "authorized"},
		{user.Expired, "expired"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			t.Parallel()
			if string(c.status) != c.want {
				t.Errorf("QuickConnectStatus = %q, want %q", c.status, c.want)
			}
		})
	}
}

func TestUser_AdminFlag(t *testing.T) {
	t.Parallel()

	admin := &user.User{
		ID:       "u1",
		Username: "admin",
		IsAdmin:  true,
		Enabled:  true,
	}
	regular := &user.User{
		ID:       "u2",
		Username: "alice",
		IsAdmin:  false,
		Enabled:  true,
	}

	if !admin.IsAdmin {
		t.Error("admin user should have IsAdmin=true")
	}
	if regular.IsAdmin {
		t.Error("regular user should have IsAdmin=false")
	}
}

func TestUser_OIDCFields(t *testing.T) {
	t.Parallel()

	// local user: no OIDC fields
	local := &user.User{ID: "u1", Username: "local"}
	if local.OIDCSubject != nil {
		t.Error("local user should have no OIDCSubject")
	}
	if local.OIDCIssuer != nil {
		t.Error("local user should have no OIDCIssuer")
	}

	// OIDC user: both fields set
	sub := "sub-123"
	iss := "https://idp.example.com"
	oidcUser := &user.User{
		ID:          "u2",
		OIDCSubject: &sub,
		OIDCIssuer:  &iss,
	}
	if *oidcUser.OIDCSubject != "sub-123" {
		t.Errorf("OIDCSubject = %q, want %q", *oidcUser.OIDCSubject, "sub-123")
	}
	if *oidcUser.OIDCIssuer != "https://idp.example.com" {
		t.Errorf("OIDCIssuer = %q, want %q", *oidcUser.OIDCIssuer, "https://idp.example.com")
	}
}

func TestUser_MaxStreams(t *testing.T) {
	t.Parallel()

	// nil = unlimited
	unlimited := &user.User{ID: "u1"}
	if unlimited.MaxStreams != nil {
		t.Error("default MaxStreams should be nil (unlimited)")
	}

	n := 2
	limited := &user.User{ID: "u2", MaxStreams: &n}
	if *limited.MaxStreams != 2 {
		t.Errorf("MaxStreams = %d, want 2", *limited.MaxStreams)
	}
}

func TestProfile_ParentalSettings(t *testing.T) {
	t.Parallel()

	t.Run("no parental control", func(t *testing.T) {
		t.Parallel()
		p := &user.Profile{
			ID:     "p1",
			UserID: "u1",
			Name:   "Alice",
		}
		if p.Parental != nil {
			t.Error("Parental should be nil when not configured")
		}
	})

	t.Run("parental enabled", func(t *testing.T) {
		t.Parallel()
		p := &user.Profile{
			ID:     "p2",
			UserID: "u1",
			Name:   "Kid",
			Parental: &user.ParentalSettings{
				MaxRating:   "PG",
				BlockedTags: []string{"violence", "adult"},
				PINHash:     "hashed-pin",
				Enabled:     true,
			},
		}
		if !p.Parental.Enabled {
			t.Error("expected parental controls to be enabled")
		}
		if p.Parental.MaxRating != "PG" {
			t.Errorf("MaxRating = %q, want %q", p.Parental.MaxRating, "PG")
		}
		if len(p.Parental.BlockedTags) != 2 {
			t.Errorf("BlockedTags len = %d, want 2", len(p.Parental.BlockedTags))
		}
	})

	t.Run("parental disabled but struct present", func(t *testing.T) {
		t.Parallel()
		p := &user.Profile{
			ID:     "p3",
			UserID: "u1",
			Name:   "Teen",
			Parental: &user.ParentalSettings{
				MaxRating: "R",
				Enabled:   false,
			},
		}
		if p.Parental.Enabled {
			t.Error("expected parental controls disabled")
		}
	})
}

func TestProfile_AudioSubtitleLanguages(t *testing.T) {
	t.Parallel()

	p := &user.Profile{
		ID:                "p1",
		UserID:            "u1",
		Name:              "French user",
		AudioLanguages:    []string{"fr", "en"},
		SubtitleLanguages: []string{"fr", "en"},
	}

	if len(p.AudioLanguages) != 2 {
		t.Errorf("AudioLanguages len = %d, want 2", len(p.AudioLanguages))
	}
	if p.AudioLanguages[0] != "fr" {
		t.Errorf("first AudioLanguage = %q, want %q", p.AudioLanguages[0], "fr")
	}
	if p.SubtitleLanguages[0] != "fr" {
		t.Errorf("first SubtitleLanguage = %q, want %q", p.SubtitleLanguages[0], "fr")
	}
}

func TestProfile_MaxBitrate(t *testing.T) {
	t.Parallel()

	// nil = no limit
	p := &user.Profile{ID: "p1", UserID: "u1", Name: "A"}
	if p.MaxBitrateKbps != nil {
		t.Error("MaxBitrateKbps should be nil (no limit) by default")
	}

	n := 4000
	p.MaxBitrateKbps = &n
	if *p.MaxBitrateKbps != 4000 {
		t.Errorf("MaxBitrateKbps = %d, want 4000", *p.MaxBitrateKbps)
	}
}

func TestRefreshToken_Expiry(t *testing.T) {
	t.Parallel()

	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	expired := &user.RefreshToken{
		ID:        "rt1",
		UserID:    "u1",
		TokenHash: "hash",
		ExpiresAt: past,
		Used:      false,
	}
	valid := &user.RefreshToken{
		ID:        "rt2",
		UserID:    "u1",
		TokenHash: "hash2",
		ExpiresAt: future,
		Used:      false,
	}

	if !expired.ExpiresAt.Before(now) {
		t.Error("expired token should have ExpiresAt in the past")
	}
	if !valid.ExpiresAt.After(now) {
		t.Error("valid token should have ExpiresAt in the future")
	}
}

func TestAPIKeyModel_Permissions(t *testing.T) {
	t.Parallel()

	key := &user.APIKeyModel{
		ID:          "k1",
		UserID:      "u1",
		KeyHash:     "hash",
		Name:        "CI Key",
		Permissions: []string{"library:read", "playback:start"},
	}

	if len(key.Permissions) != 2 {
		t.Errorf("Permissions len = %d, want 2", len(key.Permissions))
	}

	// nil LastUsedAt before first use
	if key.LastUsedAt != nil {
		t.Error("LastUsedAt should be nil before first use")
	}

	// nil ExpiresAt = no expiry
	if key.ExpiresAt != nil {
		t.Error("ExpiresAt should be nil when no expiry is set")
	}
}

func TestInviteLink_UsageTracking(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		maxUses   int
		usedCount int
		exhausted bool
	}{
		{"not used", 5, 0, false},
		{"partially used", 5, 3, false},
		{"exactly exhausted", 5, 5, true},
		{"unlimited (0)", 0, 999, false}, // 0 means unlimited by convention
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			link := &user.InviteLink{
				ID:        "inv1",
				CreatedBy: "u1",
				Token:     "tok",
				MaxUses:   c.maxUses,
				UsedCount: c.usedCount,
			}
			exhausted := c.maxUses > 0 && link.UsedCount >= link.MaxUses
			if exhausted != c.exhausted {
				t.Errorf("exhausted = %v, want %v (maxUses=%d, usedCount=%d)", exhausted, c.exhausted, c.maxUses, c.usedCount)
			}
		})
	}
}

func TestRole_Permissions(t *testing.T) {
	t.Parallel()

	role := &user.Role{
		ID:          "r1",
		Name:        "editor",
		Permissions: []string{"library:write", "metadata:edit"},
		IsSystem:    false,
	}

	if role.IsSystem {
		t.Error("custom role should not be a system role")
	}
	if len(role.Permissions) != 2 {
		t.Errorf("Permissions len = %d, want 2", len(role.Permissions))
	}

	sysRole := &user.Role{
		ID:       "r2",
		Name:     "admin",
		IsSystem: true,
	}
	if !sysRole.IsSystem {
		t.Error("system role should have IsSystem=true")
	}
}
