package user_test

import (
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/user"
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

func TestNewUser_HappyPath(t *testing.T) {
	t.Parallel()
	u, err := user.NewUser("alice", "a@b.com", "hash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.ID == "" || u.Username != "alice" || u.Email != "a@b.com" || u.PasswordHash != "hash" {
		t.Errorf("unexpected user: %+v", u)
	}
	if !u.Enabled {
		t.Error("expected Enabled=true by default")
	}
	if u.CreatedAt.IsZero() {
		t.Error("expected CreatedAt set")
	}
}

func TestNewUser_RejectsEmptyUsername(t *testing.T) {
	t.Parallel()
	if _, err := user.NewUser("", "a@b.com", "hash"); err == nil {
		t.Fatal("expected error for empty username")
	}
}

func TestNewUser_RejectsInvalidEmail(t *testing.T) {
	t.Parallel()
	if _, err := user.NewUser("alice", "not-an-email", "hash"); err == nil {
		t.Fatal("expected error for invalid email")
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

func TestUser_CanLogin(t *testing.T) {
	t.Parallel()
	if !((&user.User{Enabled: true}).CanLogin()) {
		t.Error("enabled user should be able to login")
	}
	if (&user.User{Enabled: false}).CanLogin() {
		t.Error("disabled user should not be able to login")
	}
}

func TestUser_IsOIDCUser(t *testing.T) {
	t.Parallel()
	local := &user.User{ID: "u1"}
	if local.IsOIDCUser() {
		t.Error("local user should not be OIDC")
	}
	sub, iss := "sub", "iss"
	oidc := &user.User{OIDCSubject: &sub, OIDCIssuer: &iss}
	if !oidc.IsOIDCUser() {
		t.Error("OIDC user should be recognized")
	}
}

func TestUser_HasStreamsLimit(t *testing.T) {
	t.Parallel()
	u := &user.User{}
	if u.HasStreamsLimit() {
		t.Error("nil MaxStreams = no limit")
	}
	zero := 0
	u.MaxStreams = &zero
	if u.HasStreamsLimit() {
		t.Error("zero MaxStreams = no limit")
	}
	n := 3
	u.MaxStreams = &n
	if !u.HasStreamsLimit() {
		t.Error("positive MaxStreams = has limit")
	}
}

func TestUser_StreamQuotaExceeded(t *testing.T) {
	t.Parallel()
	u := &user.User{}
	if u.StreamQuotaExceeded(100) {
		t.Error("no configured limit should never exceed")
	}
	n := 3
	u.MaxStreams = &n
	if u.StreamQuotaExceeded(2) {
		t.Error("2 active < 3 max = not exceeded")
	}
	if !u.StreamQuotaExceeded(3) {
		t.Error("3 active == 3 max = exceeded (boundary)")
	}
	if !u.StreamQuotaExceeded(10) {
		t.Error("10 active > 3 max = exceeded")
	}
}

func TestRefreshToken_IsValid(t *testing.T) {
	t.Parallel()
	valid := &user.RefreshToken{ExpiresAt: time.Now().Add(time.Hour), Used: false}
	if !valid.IsValid() {
		t.Error("fresh unexpired token should be valid")
	}
	expired := &user.RefreshToken{ExpiresAt: time.Now().Add(-time.Hour), Used: false}
	if expired.IsValid() {
		t.Error("expired token should not be valid")
	}
	used := &user.RefreshToken{ExpiresAt: time.Now().Add(time.Hour), Used: true}
	if used.IsValid() {
		t.Error("used token should not be valid")
	}
}

func TestInviteLink_IsUsable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		link user.InviteLink
		want bool
	}{
		{"fresh unlimited", user.InviteLink{MaxUses: 0, UsedCount: 0}, true},
		{"uses remaining", user.InviteLink{MaxUses: 5, UsedCount: 3}, true},
		{"one use left (UsedCount == MaxUses-1)", user.InviteLink{MaxUses: 5, UsedCount: 4}, true},
		{"exactly exhausted (UsedCount == MaxUses)", user.InviteLink{MaxUses: 5, UsedCount: 5}, false},
		{"over-used", user.InviteLink{MaxUses: 5, UsedCount: 6}, false},
		{"unlimited with high usage", user.InviteLink{MaxUses: 0, UsedCount: 999}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.link.IsUsable(); got != tt.want {
				t.Errorf("IsUsable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAPIKeyModel_HasPermission(t *testing.T) {
	t.Parallel()
	k := &user.APIKeyModel{Permissions: []string{"library:read", "playback:start"}}
	if !k.HasPermission("library:read") {
		t.Error("should have library:read")
	}
	if k.HasPermission("admin:delete") {
		t.Error("should not have admin:delete")
	}
	wildcard := &user.APIKeyModel{Permissions: []string{"*"}}
	if !wildcard.HasPermission("anything") {
		t.Error("wildcard should match any permission")
	}
}

func TestAPIKeyModel_IsExpired(t *testing.T) {
	t.Parallel()
	k := &user.APIKeyModel{}
	if k.IsExpired() {
		t.Error("nil ExpiresAt = not expired")
	}
	past := time.Now().Add(-time.Hour)
	k.ExpiresAt = &past
	if !k.IsExpired() {
		t.Error("past ExpiresAt = expired")
	}
}

func TestUser_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		user    user.User
		wantErr string
	}{
		{"valid", user.User{Username: "alice"}, ""},
		{"empty username", user.User{Username: ""}, "username is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.user.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil || err.Error() != tt.wantErr {
					t.Errorf("Validate() = %v, want %q", err, tt.wantErr)
				}
			}
		})
	}
}

func TestProfile_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		profile user.Profile
		wantErr string
	}{
		{"valid", user.Profile{UserID: "u1", Name: "Main"}, ""},
		{"no user ID", user.Profile{UserID: "", Name: "Main"}, "profile user ID is required"},
		{"no name", user.Profile{UserID: "u1", Name: ""}, "profile name is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.profile.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil || err.Error() != tt.wantErr {
					t.Errorf("Validate() = %v, want %q", err, tt.wantErr)
				}
			}
		})
	}
}

func TestInviteLink_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		link    user.InviteLink
		wantErr string
	}{
		{"valid", user.InviteLink{CreatedBy: "u1", Token: "tok", MaxUses: 5}, ""},
		{"no created_by", user.InviteLink{CreatedBy: "", Token: "tok"}, "invite link created_by is required"},
		{"no token", user.InviteLink{CreatedBy: "u1", Token: ""}, "invite link token is required"},
		{"negative max uses", user.InviteLink{CreatedBy: "u1", Token: "tok", MaxUses: -1}, "invite link max uses must not be negative"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.link.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil || err.Error() != tt.wantErr {
					t.Errorf("Validate() = %v, want %q", err, tt.wantErr)
				}
			}
		})
	}
}

func TestAPIKeyModel_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		key     user.APIKeyModel
		wantErr string
	}{
		{"valid", user.APIKeyModel{UserID: "u1", Name: "CI", KeyHash: "h"}, ""},
		{"no user ID", user.APIKeyModel{UserID: "", Name: "CI", KeyHash: "h"}, "API key user ID is required"},
		{"no name", user.APIKeyModel{UserID: "u1", Name: "", KeyHash: "h"}, "API key name is required"},
		{"no hash", user.APIKeyModel{UserID: "u1", Name: "CI", KeyHash: ""}, "API key hash is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.key.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil || err.Error() != tt.wantErr {
					t.Errorf("Validate() = %v, want %q", err, tt.wantErr)
				}
			}
		})
	}
}

func TestInviteLink_IsUsable_Expired(t *testing.T) {
	t.Parallel()
	past := time.Now().Add(-time.Hour)
	link := user.InviteLink{MaxUses: 0, ExpiresAt: &past}
	if link.IsUsable() {
		t.Error("expired invite link should not be usable")
	}
}

func TestAPIKeyModel_IsExpired_Future(t *testing.T) {
	t.Parallel()
	future := time.Now().Add(time.Hour)
	k := &user.APIKeyModel{ExpiresAt: &future}
	if k.IsExpired() {
		t.Error("future ExpiresAt = not expired")
	}
}

func TestAPIKeyModel_HasPermission_Empty(t *testing.T) {
	t.Parallel()
	k := &user.APIKeyModel{Permissions: []string{}}
	if k.HasPermission("anything") {
		t.Error("empty permissions should not match")
	}
}

func TestRefreshToken_IsExpired(t *testing.T) {
	t.Parallel()
	expired := &user.RefreshToken{ExpiresAt: time.Now().Add(-time.Hour)}
	if !expired.IsExpired() {
		t.Error("past ExpiresAt = expired")
	}
	valid := &user.RefreshToken{ExpiresAt: time.Now().Add(time.Hour)}
	if valid.IsExpired() {
		t.Error("future ExpiresAt = not expired")
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
