package user_test

import (
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
)

func intPtr(n int) *int { return &n }

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

func TestQuickConnectStatus_IsValid(t *testing.T) {
	t.Parallel()
	cases := map[user.QuickConnectStatus]bool{
		user.Pending:                true,
		user.Authorized:             true,
		user.Expired:                true,
		user.QuickConnectStatus(""): false,
		"garbage":                   false,
	}
	for s, want := range cases {
		if s.IsValid() != want {
			t.Errorf("%q.IsValid() = %v, want %v", s, !want, want)
		}
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

	maxStreams := 2
	limited := &user.User{ID: "u2", MaxStreams: &maxStreams}
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
				MaxAge:      intPtr(10),
				BlockedTags: []string{"violence", "adult"},
				PINHash:     "hashed-pin",
				Enabled:     true,
			},
		}
		if !p.Parental.Enabled {
			t.Error("expected parental controls to be enabled")
		}
		if p.Parental.MaxAge == nil || *p.Parental.MaxAge != 10 {
			t.Errorf("MaxAge = %v, want 10", p.Parental.MaxAge)
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
				MaxAge:  intPtr(17),
				Enabled: false,
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
		{"valid email", user.User{Username: "alice", Email: "a@b.com"}, ""},
		{"bare host email", user.User{Username: "alice", Email: "x@x"}, ""},
		{"surrounding whitespace tolerated", user.User{Username: "alice", Email: " a@b.com "}, ""},
		// "@" alone used to pass: an address is a local part AND a domain.
		{"missing domain", user.User{Username: "alice", Email: "a@"}, "email is invalid"},
		{"missing local part", user.User{Username: "alice", Email: "@b.com"}, "email is invalid"},
		{"space in address", user.User{Username: "alice", Email: "a b@c.d"}, "email is invalid"},
		// Display names and lists are not addresses: the SMTP sender would
		// otherwise mail whatever the operator typed.
		{"display name form", user.User{Username: "alice", Email: "Bob <a@b.com>"}, "email is invalid"},
		{"header injection", user.User{Username: "alice", Email: "a@b.com\r\nBcc: x@y.z"}, "email is invalid"},
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
		{"valid", user.Profile{ID: "p1", UserID: "u1", Name: "Main"}, ""},
		{"no ID", user.Profile{UserID: "u1", Name: "Main"}, "profile ID is required"},
		{"no user ID", user.Profile{ID: "p1", UserID: "", Name: "Main"}, "profile user ID is required"},
		{"no name", user.Profile{ID: "p1", UserID: "u1", Name: ""}, "profile name is required"},
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
		{"valid", user.InviteLink{ID: "inv-1", CreatedBy: "u1", Token: "tok", MaxUses: 5}, ""},
		{"no ID", user.InviteLink{CreatedBy: "u1", Token: "tok"}, "invite link ID is required"},
		{"no created_by", user.InviteLink{ID: "inv-1", CreatedBy: "", Token: "tok"}, "invite link created_by is required"},
		{"no token", user.InviteLink{ID: "inv-1", CreatedBy: "u1", Token: ""}, "invite link token is required"},
		{"negative max uses", user.InviteLink{ID: "inv-1", CreatedBy: "u1", Token: "tok", MaxUses: -1}, "invite link max uses must not be negative"},
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
		{"valid", user.APIKeyModel{ID: "ak-1", UserID: "u1", Name: "CI", KeyHash: "h", Permissions: []string{"media:read"}}, ""},
		{"no ID", user.APIKeyModel{UserID: "u1", Name: "CI", KeyHash: "h", Permissions: []string{"media:read"}}, "API key ID is required"},
		{"no user ID", user.APIKeyModel{ID: "ak-1", UserID: "", Name: "CI", KeyHash: "h", Permissions: []string{"media:read"}}, "API key user ID is required"},
		{"no name", user.APIKeyModel{ID: "ak-1", UserID: "u1", Name: "", KeyHash: "h", Permissions: []string{"media:read"}}, "API key name is required"},
		{"no hash", user.APIKeyModel{ID: "ak-1", UserID: "u1", Name: "CI", KeyHash: "", Permissions: []string{"media:read"}}, "API key hash is required"},
		{"no scopes", user.APIKeyModel{ID: "ak-1", UserID: "u1", Name: "CI", KeyHash: "h"}, "API key requires at least one scope (an unscoped key would be unrestricted)"},
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

func TestNewProfile(t *testing.T) {
	t.Parallel()

	t.Run("happy path with options", func(t *testing.T) {
		t.Parallel()
		prefs := map[string]string{"theme": "dark"}
		p, err := user.NewProfile("prf-1", "u1", "Alice",
			user.WithProfilePreferences(prefs),
			user.WithProfileIsKids(false),
			user.WithProfileLocale("fr"),
			user.WithProfileMaxBitrate(8000),
		)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if p.ID != "prf-1" || p.UserID != "u1" || p.Name != "Alice" {
			t.Errorf("core fields wrong: %+v", p)
		}
		if p.Preferences["theme"] != "dark" || p.Locale != "fr" {
			t.Errorf("options not applied: %+v", p)
		}
		if p.MaxBitrateKbps == nil || *p.MaxBitrateKbps != 8000 {
			t.Errorf("max bitrate option not applied")
		}
		if p.CreatedAt.IsZero() {
			t.Error("CreatedAt not stamped")
		}
	})

	t.Run("missing fields", func(t *testing.T) {
		t.Parallel()
		if _, err := user.NewProfile("", "u1", "A"); err == nil {
			t.Fatal("expected error on empty ID")
		}
		if _, err := user.NewProfile("p1", "", "A"); err == nil {
			t.Fatal("expected error on empty userID")
		}
		if _, err := user.NewProfile("p1", "u1", ""); err == nil {
			t.Fatal("expected error on empty name")
		}
	})
}

func TestNewInviteLink(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		expires := time.Now().Add(24 * time.Hour)
		l, err := user.NewInviteLink("inv-1", "admin-1", "tok-xyz",
			user.WithInviteLabel("beta"),
			user.WithInviteMaxUses(5),
			user.WithInviteAdmin(true),
			user.WithInviteLibraries([]shared.LibraryID{"lib-1", "lib-2"}),
			user.WithInviteExpiresAt(&expires),
		)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if l.ID != "inv-1" || l.CreatedBy != "admin-1" || l.Token != "tok-xyz" {
			t.Errorf("core fields wrong: %+v", l)
		}
		if l.Label == nil || *l.Label != "beta" {
			t.Errorf("label option not applied")
		}
		if l.MaxUses != 5 || !l.IsAdmin || len(l.LibraryIDs) != 2 {
			t.Errorf("options not applied: %+v", l)
		}
		if l.CreatedAt.IsZero() {
			t.Error("CreatedAt not stamped")
		}
	})

	t.Run("negative max uses rejected", func(t *testing.T) {
		t.Parallel()
		_, err := user.NewInviteLink("inv-1", "admin-1", "tok",
			user.WithInviteMaxUses(-5),
		)
		if err == nil {
			t.Fatal("expected error on negative max uses")
		}
	})

	t.Run("missing ID", func(t *testing.T) {
		t.Parallel()
		_, err := user.NewInviteLink("", "admin-1", "tok")
		if err == nil {
			t.Fatal("expected error on empty ID")
		}
	})
}

func TestNewAPIKey(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		k, err := user.NewAPIKey("ak-1", "u1", "CI", "hash",
			user.WithAPIKeyPermissions([]string{"read:library"}),
		)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if k.ID != "ak-1" || k.UserID != "u1" || k.KeyHash != "hash" {
			t.Errorf("core fields wrong: %+v", k)
		}
		if len(k.Permissions) != 1 || k.Permissions[0] != "read:library" {
			t.Errorf("permissions option not applied")
		}
		if k.CreatedAt.IsZero() {
			t.Error("CreatedAt not stamped")
		}
	})

	t.Run("missing hash", func(t *testing.T) {
		t.Parallel()
		_, err := user.NewAPIKey("ak-1", "u1", "CI", "")
		if err == nil {
			t.Fatal("expected error on empty hash")
		}
	})

	t.Run("empty scopes rejected", func(t *testing.T) {
		t.Parallel()
		// A key with no scopes would be an unrestricted credential - refuse it.
		_, err := user.NewAPIKey("ak-1", "u1", "CI", "hash")
		if err == nil {
			t.Fatal("expected error: an unscoped API key must be refused")
		}
	})
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

func TestProfile_EffectiveParental(t *testing.T) {
	t.Parallel()

	t.Run("non-kids without parental → nil", func(t *testing.T) {
		t.Parallel()
		p := &user.Profile{ID: "p1", IsKids: false}
		if got := p.EffectiveParental(); got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("non-kids with parental disabled → nil", func(t *testing.T) {
		t.Parallel()
		p := &user.Profile{
			ID:       "p1",
			IsKids:   false,
			Parental: &user.ParentalSettings{Enabled: false, MaxAge: intPtr(7)},
		}
		if got := p.EffectiveParental(); got != nil {
			t.Errorf("disabled parental should yield nil, got %+v", got)
		}
	})

	t.Run("non-kids with parental EXPLICITLY enabled and MaxAge → enforced (shared family profile)", func(t *testing.T) {
		t.Parallel()
		p := &user.Profile{
			ID:       "p1",
			IsKids:   false,
			Parental: &user.ParentalSettings{Enabled: true, MaxAge: new(15), BlockedTags: []string{"gore"}, PINHash: "h"},
		}
		got := p.EffectiveParental()
		if got == nil {
			t.Fatalf("an admin explicitly enabled parental controls on this profile: the server must enforce them, not treat the profile as unrestricted")
		}
		if got.MaxAge == nil || *got.MaxAge != 15 {
			t.Errorf("MaxAge = %v, want 15", got.MaxAge)
		}
		if len(got.BlockedTags) != 1 || got.BlockedTags[0] != "gore" {
			t.Errorf("BlockedTags = %v, want [gore]", got.BlockedTags)
		}
		if got.PINHash != "h" {
			t.Errorf("PINHash must carry over")
		}
	})

	t.Run("non-kids enabled with blocked tags only → enforced without an age cap", func(t *testing.T) {
		t.Parallel()
		p := &user.Profile{
			ID:       "p1",
			IsKids:   false,
			Parental: &user.ParentalSettings{Enabled: true, BlockedTags: []string{"horror"}},
		}
		got := p.EffectiveParental()
		if got == nil {
			t.Fatalf("blocked tags on an enabled profile must be enforced")
		}
		if got.MaxAge != nil {
			t.Errorf("non-kids profiles never get a DEFAULT age cap (kids-only fallback), got %v", *got.MaxAge)
		}
	})

	t.Run("non-kids enabled but nothing configured → nil (nothing to enforce)", func(t *testing.T) {
		t.Parallel()
		p := &user.Profile{
			ID:       "p1",
			IsKids:   false,
			Parental: &user.ParentalSettings{Enabled: true},
		}
		if got := p.EffectiveParental(); got != nil {
			t.Errorf("enabled with no MaxAge and no tags has nothing to enforce, got %+v", got)
		}
	})

	t.Run("kids without parental → enforced with default age", func(t *testing.T) {
		t.Parallel()
		p := &user.Profile{ID: "p1", IsKids: true}
		got := p.EffectiveParental()
		if got == nil {
			t.Fatal("kids profile must always have effective parental")
		}
		if !got.Enabled {
			t.Error("kids effective parental must be Enabled")
		}
		if got.MaxAge == nil || *got.MaxAge != user.DefaultKidsMaxAge {
			t.Errorf("kids without MaxAge should fall back to DefaultKidsMaxAge=%d, got %+v",
				user.DefaultKidsMaxAge, got.MaxAge)
		}
	})

	t.Run("kids with parental disabled but no MaxAge → enforced with default", func(t *testing.T) {
		t.Parallel()
		p := &user.Profile{
			ID:       "p1",
			IsKids:   true,
			Parental: &user.ParentalSettings{Enabled: false},
		}
		got := p.EffectiveParental()
		if got == nil {
			t.Fatal("kids profile must always have effective parental")
		}
		if !got.Enabled {
			t.Error("kids effective parental must be Enabled regardless of stored flag")
		}
		if got.MaxAge == nil || *got.MaxAge != user.DefaultKidsMaxAge {
			t.Errorf("expected default %d, got %+v", user.DefaultKidsMaxAge, got.MaxAge)
		}
	})

	t.Run("kids with explicit MaxAge → explicit wins", func(t *testing.T) {
		t.Parallel()
		p := &user.Profile{
			ID:       "p1",
			IsKids:   true,
			Parental: &user.ParentalSettings{MaxAge: intPtr(7), BlockedTags: []string{"violence"}},
		}
		got := p.EffectiveParental()
		if got == nil || got.MaxAge == nil || *got.MaxAge != 7 {
			t.Errorf("expected explicit MaxAge=7, got %+v", got)
		}
		if len(got.BlockedTags) != 1 || got.BlockedTags[0] != "violence" {
			t.Errorf("BlockedTags should propagate, got %v", got.BlockedTags)
		}
	})
}

// InviteLink.Validate had no test of its own, and mutation testing showed why
// that matters: flipping `MaxUses < 0` to `<= 0` survived the whole suite.
//
// That flip is not cosmetic. MaxUses == 0 means UNLIMITED - IsUsable skips the
// exhaustion check entirely when it is zero (model.go: `MaxUses > 0 && ...`) -
// so the mutant makes every unlimited invite link fail validation, i.e. you can
// no longer create one, and nothing would have told us.
func TestInviteLinkValidate_Boundaries(t *testing.T) {
	valid := func() user.InviteLink {
		return user.InviteLink{ID: "inv-1", CreatedBy: "usr-1", Token: "tok-1", MaxUses: 5}
	}

	t.Run("unlimited (MaxUses == 0) is valid", func(t *testing.T) {
		l := valid()
		l.MaxUses = 0
		if err := l.Validate(); err != nil {
			t.Fatalf("MaxUses 0 means unlimited and must validate, got %v", err)
		}
	})

	t.Run("a positive cap is valid", func(t *testing.T) {
		l := valid()
		if err := l.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("a negative cap is rejected", func(t *testing.T) {
		l := valid()
		l.MaxUses = -1
		if err := l.Validate(); err == nil {
			t.Fatal("a negative max-uses must be rejected")
		}
	})

	for _, tt := range []struct {
		name  string
		mutate func(*user.InviteLink)
	}{
		{"missing id", func(l *user.InviteLink) { l.ID = "" }},
		{"missing creator", func(l *user.InviteLink) { l.CreatedBy = "" }},
		{"missing token", func(l *user.InviteLink) { l.Token = "" }},
	} {
		t.Run(tt.name+" is rejected", func(t *testing.T) {
			l := valid()
			tt.mutate(&l)
			if err := l.Validate(); err == nil {
				t.Fatalf("%s must be rejected", tt.name)
			}
		})
	}
}
