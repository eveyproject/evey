package user

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/evey-project/evey/internal/domain/user"
	ufakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/require"
)

type authTestDeps struct {
	users       *ufakes.FakeUserRepository
	profiles    *ufakes.FakeProfileRepository
	refreshTkns *ufakes.FakeRefreshTokenRepository
	roles       *ufakes.FakeRoleRepository
	userRoles   *ufakes.FakeUserRoleRepository
	hasher      *ufakes.FakePasswordHasher
	tokens      *ufakes.FakeTokenGenerator
	events      *fakes.FakeEventBus
	svc         *CommandService
}

func newAuthTestDeps() *authTestDeps {
	users := ufakes.NewFakeUserRepository()
	profiles := ufakes.NewFakeProfileRepository()
	refreshTkns := ufakes.NewFakeRefreshTokenRepository()
	roles := ufakes.NewFakeRoleRepository()
	userRoles := ufakes.NewFakeUserRoleRepository(roles)
	hasher := ufakes.NewFakePasswordHasher()
	tokens := ufakes.NewFakeTokenGenerator()
	events := fakes.NewFakeEventBus()

	qc := ufakes.NewFakeQuickConnectRepository()
	svc := NewCommandService(users, profiles, refreshTkns, roles, userRoles, hasher, tokens, events, 7*24*time.Hour, qc)
	return &authTestDeps{users, profiles, refreshTkns, roles, userRoles, hasher, tokens, events, svc}
}

func seedAuthUser(t *testing.T, d *authTestDeps, id, username, password string, admin, enabled bool) *user.User {
	t.Helper()
	u := &user.User{
		ID:           shared.UserID(id),
		Username:     username,
		PasswordHash: "hashed:" + password,
		IsAdmin:      admin,
		Enabled:      enabled,
	}
	if err := d.users.Create(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	return u
}

// --- InitSetupState ---

func TestInitSetupState_SetsSetupDone(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "admin", "password", true, true)

	d.svc.InitSetupState(ctx)
	_, err := d.svc.Setup(ctx, "new", "longpassword", "e@e.com")
	if !errors.Is(err, ErrSetupDone) {
		t.Fatalf("expected ErrSetupDone after init, got %v", err)
	}
}

func TestInitSetupState_NoAdmins(t *testing.T) {
	d := newAuthTestDeps()
	d.svc.InitSetupState(context.Background())

	u, err := d.svc.Setup(context.Background(), "admin", "longpassword", "a@a.com")
	if err != nil {
		t.Fatalf("setup should succeed: %v", err)
	}
	if !u.IsAdmin {
		t.Fatal("expected admin user")
	}
}

func TestInitSetupState_RepoError(t *testing.T) {
	d := newAuthTestDeps()
	d.users.Err = errors.New("db down")
	d.svc.InitSetupState(context.Background())
	d.users.Err = nil

	u, err := d.svc.Setup(context.Background(), "admin", "longpassword", "a@a.com")
	if err != nil {
		t.Fatalf("expected setup to work: %v", err)
	}
	if u == nil {
		t.Fatal("expected user")
	}
}

// --- Setup ---

func TestSetup_Happy(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()

	u, err := d.svc.Setup(ctx, "admin", "longpassword", "a@a.com")
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "admin" || !u.IsAdmin || !u.Enabled {
		t.Fatal("wrong user fields")
	}
	if u.PasswordHash != "hashed:longpassword" {
		t.Fatalf("expected hashed password, got %s", u.PasswordHash)
	}

	evts := d.events.PublishedEvents()
	if len(evts) != 2 {
		t.Fatalf("expected 2 events (user.created + profile.created), got %d", len(evts))
	}
	if evts[0].EventName() != "user.created" {
		t.Fatalf("expected user.created, got %s", evts[0].EventName())
	}
	if evts[1].EventName() != "user.profile.created" {
		t.Fatalf("expected user.profile.created, got %s", evts[1].EventName())
	}
}

func TestSetup_AlreadyDone(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	_, _ = d.svc.Setup(ctx, "admin", "longpassword", "a@a.com")

	_, err := d.svc.Setup(ctx, "admin2", "longpassword", "b@b.com")
	if !errors.Is(err, ErrSetupDone) {
		t.Fatalf("expected ErrSetupDone, got %v", err)
	}
}

func TestSetup_WithLocaleAndRegion_SeedsProfile(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()

	u, err := d.svc.Setup(ctx, "admin", "longpassword", "a@a.com",
		WithSetupLocale("fr"),
		WithSetupRegion("FR"),
	)
	if err != nil {
		t.Fatal(err)
	}

	profs, err := d.profiles.GetByUserID(ctx, u.ID)
	if err != nil || len(profs) != 1 {
		t.Fatalf("expected 1 profile, got %d (err=%v)", len(profs), err)
	}
	p := profs[0]
	if p.Locale != "fr" {
		t.Errorf("Locale = %q, want %q", p.Locale, "fr")
	}
	if p.Region == nil || *p.Region != "FR" {
		got := "<nil>"
		if p.Region != nil {
			got = *p.Region
		}
		t.Errorf("Region = %q, want %q", got, "FR")
	}
	if !p.IsDefault {
		t.Error("profile should be default")
	}
}

func TestSetup_WithoutOptions_LeavesProfileLocaleEmpty(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()

	u, err := d.svc.Setup(ctx, "admin", "longpassword", "a@a.com")
	if err != nil {
		t.Fatal(err)
	}
	profs, err := d.profiles.GetByUserID(ctx, u.ID)
	if err != nil || len(profs) != 1 {
		t.Fatalf("expected 1 profile, got %d (err=%v)", len(profs), err)
	}
	if profs[0].Locale != "" {
		t.Errorf("Locale = %q, want empty (DB column default applies at persistence layer)", profs[0].Locale)
	}
	if profs[0].Region != nil {
		t.Errorf("Region = %v, want nil", *profs[0].Region)
	}
}

func TestSetup_ShortPassword(t *testing.T) {
	d := newAuthTestDeps()
	_, err := d.svc.Setup(context.Background(), "admin", "short", "a@a.com")
	if !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("expected ErrPasswordTooShort, got %v", err)
	}
}

func TestSetup_PasswordBoundary(t *testing.T) {
	t.Run("exactly 7 chars rejected", func(t *testing.T) {
		d := newAuthTestDeps()
		_, err := d.svc.Setup(context.Background(), "admin", "1234567", "a@a.com")
		if !errors.Is(err, ErrPasswordTooShort) {
			t.Fatalf("7-char password should be rejected, got %v", err)
		}
	})
	t.Run("exactly 8 chars accepted", func(t *testing.T) {
		d := newAuthTestDeps()
		_, err := d.svc.Setup(context.Background(), "admin", "12345678", "a@a.com")
		if errors.Is(err, ErrPasswordTooShort) {
			t.Fatal("8-char password should be accepted")
		}
	})
}

func TestSetup_CountAdminsError(t *testing.T) {
	d := newAuthTestDeps()
	d.users.Err = errors.New("db error")
	_, err := d.svc.Setup(context.Background(), "admin", "longpassword", "a@a.com")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSetup_AdminsAlreadyExist(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "existing", "pw", true, true)

	_, err := d.svc.Setup(ctx, "admin", "longpassword", "a@a.com")
	if !errors.Is(err, ErrSetupDone) {
		t.Fatalf("expected ErrSetupDone, got %v", err)
	}
}

func TestSetup_HashError(t *testing.T) {
	d := newAuthTestDeps()
	d.hasher.Err = errors.New("hash fail")
	_, err := d.svc.Setup(context.Background(), "admin", "longpassword", "a@a.com")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSetup_UserCreateError(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	_, err := d.svc.Setup(ctx, "admin", "longpassword", "a@a.com")
	if err != nil {
		t.Fatal(err)
	}
	d2 := newAuthTestDeps()
	d2.users.Err = errors.New("create fail")
	_, err = d2.svc.Setup(ctx, "admin", "longpassword", "a@a.com")
	if err == nil {
		t.Fatal("expected error from user create")
	}
}

func TestSetup_ProfileCreateError(t *testing.T) {
	d := newAuthTestDeps()
	d.profiles.Err = errors.New("profile fail")
	_, err := d.svc.Setup(context.Background(), "admin", "longpassword", "a@a.com")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSetup_EventPublishError_NonFatal(t *testing.T) {
	d := newAuthTestDeps()
	d.events.PublishErr = errors.New("bus down")
	u, err := d.svc.Setup(context.Background(), "admin", "longpassword", "a@a.com")
	if err != nil {
		t.Fatalf("event publish error should be non-fatal, got %v", err)
	}
	if u == nil {
		t.Fatal("expected user")
	}
}

// --- Login ---

func TestLogin_Happy(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "mypassword", false, true)

	access, refresh, err := d.svc.Login(ctx, "alice", "mypassword")
	if err != nil {
		t.Fatal(err)
	}
	if access == "" || refresh == "" {
		t.Fatal("expected tokens")
	}

	evts := d.events.PublishedEvents()
	found := false
	for _, e := range evts {
		if e.EventName() == "user.login.succeeded" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected login.succeeded event")
	}
}

// A refused login is announced (webhook user.login.failed) with what was
// typed and why - never the password.
func TestLogin_FailureIsAnnounced(t *testing.T) {
	cases := []struct {
		name, username, password, reason string
		seed                             bool
	}{
		{"unknown user", "nobody", "pw", "unknown_user", false},
		{"wrong password", "alice", "not-it", "bad_password", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newAuthTestDeps()
			if tc.seed {
				seedAuthUser(t, d, "u1", "alice", "mypassword", false, true)
			}
			if _, _, err := d.svc.Login(context.Background(), tc.username, tc.password); err == nil {
				t.Fatal("this login was supposed to fail")
			}
			var got *user.LoginFailed
			for _, e := range d.events.PublishedEvents() {
				if lf, ok := e.(user.LoginFailed); ok {
					got = &lf
				}
			}
			if got == nil {
				t.Fatal("expected a user.login.failed event")
			}
			if got.Username != tc.username || got.Reason != tc.reason {
				t.Fatalf("event = %+v, want username %q reason %q", *got, tc.username, tc.reason)
			}
			if strings.Contains(fmt.Sprintf("%+v", *got), tc.password) {
				t.Fatal("the password must never ride the event")
			}
		})
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	d := newAuthTestDeps()
	_, _, err := d.svc.Login(context.Background(), "nobody", "pw")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	d := newAuthTestDeps()
	seedAuthUser(t, d, "u1", "alice", "correct", false, true)
	_, _, err := d.svc.Login(context.Background(), "alice", "wrong")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_UnknownUser_RunsConstantTimeDummyVerify(t *testing.T) {
	d := newAuthTestDeps()
	// No user seeded → GetByUsername misses. The fix runs a dummy bcrypt verify so
	// a missing user costs the same ~100ms as a wrong password on an existing one,
	// closing the username-enumeration timing oracle. Pre-fix the miss path ran
	// zero verifies and returned in ~0ms.
	_, _, err := d.svc.Login(context.Background(), "ghost", "whatever")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
	if d.hasher.VerifyCalls != 1 {
		t.Fatalf("expected exactly 1 verify on unknown user (constant-time), got %d", d.hasher.VerifyCalls)
	}
	if len(d.hasher.VerifiedHashes) != 1 || d.hasher.VerifiedHashes[0] != dummyLoginHash {
		t.Fatalf("expected the dummy timing hash to be verified, got %v", d.hasher.VerifiedHashes)
	}
}

func TestLogin_UserDisabled(t *testing.T) {
	d := newAuthTestDeps()
	seedAuthUser(t, d, "u1", "alice", "pw", false, false)
	_, _, err := d.svc.Login(context.Background(), "alice", "pw")
	if !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("expected ErrUserDisabled, got %v", err)
	}
	// The password hash MUST be verified even for a disabled account, otherwise
	// the early ErrUserDisabled return is faster than a wrong-password attempt
	// and timing reveals that the username exists but is disabled.
	if d.hasher.VerifyCalls != 1 {
		t.Fatalf("disabled login must still run one bcrypt verify (timing-oracle guard), got %d", d.hasher.VerifyCalls)
	}
}

func TestLogin_HasherVerifyError(t *testing.T) {
	d := newAuthTestDeps()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	d.hasher.Err = errors.New("verify fail")
	_, _, err := d.svc.Login(context.Background(), "alice", "pw")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_AccessTokenError(t *testing.T) {
	d := newAuthTestDeps()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	d.tokens.Err = errors.New("token fail")
	_, _, err := d.svc.Login(context.Background(), "alice", "pw")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLogin_WithRolesAndPermissions(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	role := &user.Role{ID: "r1", Name: "viewer", Permissions: []string{"read", "watch"}}
	_ = d.roles.Create(ctx, role)
	_ = d.userRoles.AssignRole(ctx, "u1", "r1")

	access, _, err := d.svc.Login(ctx, "alice", "pw")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := d.tokens.ValidateAccessToken(access)
	if err != nil {
		t.Fatal(err)
	}
	if len(claims.Permissions) != 2 {
		t.Fatalf("expected 2 permissions, got %d", len(claims.Permissions))
	}
}

func TestLogin_WithDefaultProfile(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Main", IsDefault: true})

	access, _, err := d.svc.Login(ctx, "alice", "pw")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := d.tokens.ValidateAccessToken(access)
	if err != nil {
		t.Fatal(err)
	}
	if claims.ProfileID != "p1" {
		t.Fatalf("expected profile p1, got %s", claims.ProfileID)
	}
}

func TestLogin_FallbackToFirstProfile(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alt", IsDefault: false})

	access, _, err := d.svc.Login(ctx, "alice", "pw")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := d.tokens.ValidateAccessToken(access)
	if err != nil {
		t.Fatal(err)
	}
	if claims.ProfileID != "p1" {
		t.Fatalf("expected fallback to first profile p1, got %s", claims.ProfileID)
	}
}

// Was TestLogin_ProfileRepoError_NonFatal, which asserted the OPPOSITE: that a
// profile-repo error still issued a token, with an empty ProfileID. That
// expectation was written for availability, but an empty ProfileID is exactly
// what every parental gate reads as "no restriction" - so the tolerated
// degradation was a silent, complete parental bypass for the token's lifetime.
// A DB fault that blocks login is recoverable and visible; an unrestricted
// token is neither.
func TestLogin_ProfileRepoError_FailsClosed(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	d.profiles.Err = errors.New("profiles down")

	access, _, err := d.svc.Login(ctx, "alice", "pw")
	if err == nil {
		claims, _ := d.tokens.ValidateAccessToken(access)
		t.Fatalf("a profile lookup failure must refuse login, got a token with ProfileID=%q", claims.ProfileID)
	}
}

func TestLogin_UserRolesError_NonFatal(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	d.userRoles.Err = errors.New("roles down")

	_, _, err := d.svc.Login(ctx, "alice", "pw")
	if err != nil {
		t.Fatalf("roles error should be non-fatal, got %v", err)
	}
}

func TestLogin_EventPublishError_NonFatal(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	d.events.PublishErr = errors.New("bus down")

	_, _, err := d.svc.Login(ctx, "alice", "pw")
	if err != nil {
		t.Fatalf("event error should be non-fatal, got %v", err)
	}
}

// --- RefreshToken ---

func TestRefreshToken_Happy(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	rawToken := "my-refresh-token"
	h := sha256.Sum256([]byte(rawToken))
	hash := hex.EncodeToString(h[:])
	rt := &user.RefreshToken{
		ID:        "rt1",
		UserID:    "u1",
		TokenHash: hash,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	_ = d.refreshTkns.Create(ctx, rt)

	access, newRefresh, err := d.svc.RefreshToken(ctx, rawToken, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if access == "" || newRefresh == "" {
		t.Fatal("expected tokens")
	}
}

// A silent refresh (empty requestedProfileID) must keep the profile the token
// was bound to - not snap back to the default profile (the parental-escape bug).
func TestRefreshToken_PreservesBoundProfile(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-def", UserID: "u1", Name: "Main", IsDefault: true})
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-kid", UserID: "u1", Name: "Kid", IsKids: true})

	raw := "bound-token"
	h := sha256.Sum256([]byte(raw))
	kid := shared.ProfileID("p-kid")
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ProfileID: &kid, ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	access, _, err := d.svc.RefreshToken(ctx, raw, "", "")
	require.NoError(t, err)
	claims, err := d.tokens.ValidateAccessToken(access)
	require.NoError(t, err)
	require.Equal(t, "p-kid", claims.ProfileID, "silent refresh must preserve the bound profile, not fall back to default")
}

// The rotated refresh token must carry the binding forward: seeding it by hand
// (above) never exercised the code that stores it, so a rotation that dropped
// the profile let the SECOND silent refresh land a kids profile on the default.
func TestRefreshToken_BoundProfileSurvivesRotation(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-def", UserID: "u1", Name: "Main", IsDefault: true})
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-kid", UserID: "u1", Name: "Kid", IsKids: true})

	raw := "bound-token-rot"
	h := sha256.Sum256([]byte(raw))
	kid := shared.ProfileID("p-kid")
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ProfileID: &kid, ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	_, rotated, err := d.svc.RefreshToken(ctx, raw, "", "")
	require.NoError(t, err)
	access, _, err := d.svc.RefreshToken(ctx, rotated, "", "")
	require.NoError(t, err)
	claims, err := d.tokens.ValidateAccessToken(access)
	require.NoError(t, err)
	require.Equal(t, "p-kid", claims.ProfileID, "the second silent refresh must still be on the kids profile")
}

// An explicit switch to a profile that is not the caller's is refused outright,
// never quietly downgraded to the default profile. Uncovered until 2026-09-17
// (mutation run): the gate was fail-closed, but nothing pinned it.
func TestRefreshToken_ExplicitForeignProfile_Rejected(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	seedAuthUser(t, d, "u2", "bob", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-alice", UserID: "u1", Name: "Main", IsDefault: true})
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-bob", UserID: "u2", Name: "Main", IsDefault: true})

	raw := "alice-token"
	h := sha256.Sum256([]byte(raw))
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	_, _, err := d.svc.RefreshToken(ctx, raw, "p-bob", "")
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

// Switching OUT of a kids profile INTO a PIN-protected non-kids one is rejected
// without a valid PIN proof - even via a direct API call that bypasses the UI.
func TestRefreshToken_SwitchOutOfKids_RequiresPIN(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-adult", UserID: "u1", Name: "Adult", IsDefault: true, Parental: &user.ParentalSettings{PINHash: "h"}})
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-kid", UserID: "u1", Name: "Kid", IsKids: true})

	raw := "kid-token"
	h := sha256.Sum256([]byte(raw))
	kid := shared.ProfileID("p-kid")
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ProfileID: &kid, ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	_, _, err := d.svc.RefreshToken(ctx, raw, "p-adult", "") // no PIN proof
	require.ErrorIs(t, err, ErrPINRequired)
}

// A non-kids profile whose owner explicitly enabled parental controls with a
// PIN is restricted too: leaving it for an unrestricted PIN-protected profile
// must demand the same proof as leaving a kids profile. Without this, the
// server-side content filter on such a profile (see Profile.EffectiveParental)
// is trivially bypassed by switching profile - verified live against dev.
func TestRefreshToken_SwitchOutOfGatedNonKids_RequiresPIN(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-adult", UserID: "u1", Name: "Adult", IsDefault: true, Parental: &user.ParentalSettings{PINHash: "h"}})
	maxAge := 12
	_ = d.profiles.Create(ctx, &user.Profile{
		ID: "p-family", UserID: "u1", Name: "Family",
		Parental: &user.ParentalSettings{Enabled: true, MaxAge: &maxAge, PINHash: "h2"},
	})

	raw := "family-token"
	h := sha256.Sum256([]byte(raw))
	fam := shared.ProfileID("p-family")
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ProfileID: &fam, ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	_, _, err := d.svc.RefreshToken(ctx, raw, "p-adult", "") // no PIN proof
	require.ErrorIs(t, err, ErrPINRequired)
}

// An UNGATED non-kids profile (parental off, or on but with nothing configured)
// keeps switching freely: the gate must not appear for ordinary users.
func TestRefreshToken_SwitchOutOfUngatedNonKids_NoPIN(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-adult", UserID: "u1", Name: "Adult", IsDefault: true, Parental: &user.ParentalSettings{PINHash: "h"}})
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-other", UserID: "u1", Name: "Other"})

	raw := "other-token"
	h := sha256.Sum256([]byte(raw))
	other := shared.ProfileID("p-other")
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ProfileID: &other, ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	_, _, err := d.svc.RefreshToken(ctx, raw, "p-adult", "")
	require.NoError(t, err, "an unrestricted profile must switch without a PIN")
}

// TestRefreshToken_SwitchRevokesStreamTokensMintedUnderOldProfile proves the
// serve-time revalidation gap: a stream/cast token minted while profile A was
// active must not go on serving media once the session has moved to profile
// B - otherwise a kid profile (or any more-restrictive target) inherits every
// opaque token an unrestricted profile already had in flight, which is the
// exact parental bypass the PIN gate above is meant to close. Reusing the
// same TokenRevoker plumbing as DisableUser/ChangePassword purges those stale
// tokens (via media.StreamTokenStore.RevokeByUser) the instant the switch is
// authorised, so any request still riding the old token is cut. The target
// must actually be restricted (a PIN alone restricts nothing, see
// Profile.IsParentalRestricted): an age cap behind a PIN here.
func TestRefreshToken_SwitchRevokesStreamTokensMintedUnderOldProfile(t *testing.T) {
	d := newAuthTestDeps()
	rev := &recordingRevoker{}
	CommandWithTokenRevoker(rev)(d.svc)
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	maxAge := 12
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-adult", UserID: "u1", Name: "Teen", IsDefault: true, Parental: &user.ParentalSettings{PINHash: "h", Enabled: true, MaxAge: &maxAge}})
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-other", UserID: "u1", Name: "Other"})

	raw := "switch-revoke-token"
	h := sha256.Sum256([]byte(raw))
	other := shared.ProfileID("p-other")
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ProfileID: &other, ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	_, _, err := d.svc.RefreshToken(ctx, raw, "p-adult", "")
	require.NoError(t, err)

	require.Len(t, rev.revoked, 1, "profile switch must revoke the user's outstanding stream tokens")
	require.Equal(t, "u1", rev.revoked[0])
}

// A silent refresh (no profile change) must NOT revoke anything - it would
// otherwise cut every in-flight stream on each routine token renewal.
func TestRefreshToken_SilentRefreshDoesNotRevokeStreamTokens(t *testing.T) {
	d := newAuthTestDeps()
	rev := &recordingRevoker{}
	CommandWithTokenRevoker(rev)(d.svc)
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-other", UserID: "u1", Name: "Other", IsDefault: true})

	raw := "silent-refresh-token"
	h := sha256.Sum256([]byte(raw))
	other := shared.ProfileID("p-other")
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ProfileID: &other, ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	_, _, err := d.svc.RefreshToken(ctx, raw, "", "")
	require.NoError(t, err)

	require.Empty(t, rev.revoked, "a silent refresh on the same profile must not purge live streams")
}

// A switch to an UNRESTRICTED profile has nothing to protect: the tokens in
// flight were minted under a profile at least as restricted. Purging there
// cut the cast token of a TV the moment the phone picked its own profile
// after a login (the login binds the default profile, so the pick is a switch).
func TestRefreshToken_SwitchToUnrestrictedProfileKeepsStreamTokens(t *testing.T) {
	d := newAuthTestDeps()
	rev := &recordingRevoker{}
	CommandWithTokenRevoker(rev)(d.svc)
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-default", UserID: "u1", Name: "Default", IsDefault: true})
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-mine", UserID: "u1", Name: "Mine"})

	raw := "switch-keep-token"
	h := sha256.Sum256([]byte(raw))
	def := shared.ProfileID("p-default")
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ProfileID: &def, ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	_, _, err := d.svc.RefreshToken(ctx, raw, "p-mine", "")
	require.NoError(t, err)

	require.Empty(t, rev.revoked, "switching to an unrestricted profile must keep the user's live cast/stream tokens")
}

// The real-world bypass: the escape profile has NO PIN of its own, so the
// legacy "prove the target's PIN" rule could demand nothing. Leaving a gated
// profile must still be refused - its OWN PIN authorises the exit.
func TestRefreshToken_SwitchOutOfGatedNonKids_TargetWithoutPIN_StillRequiresPIN(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-free", UserID: "u1", Name: "Free", IsDefault: true})
	maxAge := 12
	_ = d.profiles.Create(ctx, &user.Profile{
		ID: "p-family", UserID: "u1", Name: "Family",
		Parental: &user.ParentalSettings{Enabled: true, MaxAge: &maxAge, PINHash: "h2"},
	})

	raw := "family-token"
	h := sha256.Sum256([]byte(raw))
	fam := shared.ProfileID("p-family")
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ProfileID: &fam, ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	_, _, err := d.svc.RefreshToken(ctx, raw, "p-free", "")
	require.ErrorIs(t, err, ErrPINRequired, "a PIN-less target must not be a free exit from a gated profile")

	// The gated profile's own PIN proof authorises the exit. A fresh refresh
	// token: the failed attempt above may have consumed the first one.
	raw2 := "family-token-2"
	h2 := sha256.Sum256([]byte(raw2))
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt2", UserID: "u1", TokenHash: hex.EncodeToString(h2[:]),
		ProfileID: &fam, ExpiresAt: time.Now().Add(24 * time.Hour),
	})
	proof, perr := d.tokens.GeneratePINToken("p-family", time.Minute)
	require.NoError(t, perr)
	_, _, err = d.svc.RefreshToken(ctx, raw2, "p-free", proof)
	require.NoError(t, err, "the gated profile's own PIN must authorise leaving it")
}

// A refused PIN gate must NOT consume the refresh token: the client fetches a
// PIN proof and retries with the same token. Burning it would log the user out,
// and the retry would trip the replay detector, purging every session they have.
func TestRefreshToken_PINRefusal_DoesNotBurnTheRefreshToken(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-adult", UserID: "u1", Name: "Adult", IsDefault: true, Parental: &user.ParentalSettings{PINHash: "h"}})
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-kid", UserID: "u1", Name: "Kid", IsKids: true})

	raw := "kid-token"
	h := sha256.Sum256([]byte(raw))
	kid := shared.ProfileID("p-kid")
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ProfileID: &kid, ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	_, _, err := d.svc.RefreshToken(ctx, raw, "p-adult", "")
	require.ErrorIs(t, err, ErrPINRequired)

	// Same token, now with the proof the user just fetched: must succeed.
	pinTok, gerr := d.tokens.GeneratePINToken("p-adult", time.Minute)
	require.NoError(t, gerr)
	access, _, err := d.svc.RefreshToken(ctx, raw, "p-adult", pinTok)
	require.NoError(t, err, "the refused attempt must have left the refresh token usable")
	claims, verr := d.tokens.ValidateAccessToken(access)
	require.NoError(t, verr)
	require.Equal(t, "p-adult", claims.ProfileID)
}

// Switching INTO a more restrictive profile is always free.
func TestRefreshToken_SwitchIntoKids_NoPIN(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	maxAge := 12
	_ = d.profiles.Create(ctx, &user.Profile{
		ID: "p-family", UserID: "u1", Name: "Family", IsDefault: true,
		Parental: &user.ParentalSettings{Enabled: true, MaxAge: &maxAge, PINHash: "h2"},
	})
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-kid", UserID: "u1", Name: "Kid", IsKids: true})

	raw := "family-token2"
	h := sha256.Sum256([]byte(raw))
	fam := shared.ProfileID("p-family")
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt2", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ProfileID: &fam, ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	_, _, err := d.svc.RefreshToken(ctx, raw, "p-kid", "")
	require.NoError(t, err, "entering a stricter profile needs no proof")
}

func TestRefreshToken_SwitchOutOfKids_WithValidPIN_OK(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-adult", UserID: "u1", Name: "Adult", IsDefault: true, Parental: &user.ParentalSettings{PINHash: "h"}})
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-kid", UserID: "u1", Name: "Kid", IsKids: true})

	raw := "kid-token-2"
	h := sha256.Sum256([]byte(raw))
	kid := shared.ProfileID("p-kid")
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt2", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ProfileID: &kid, ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	pinTok, err := d.tokens.GeneratePINToken("p-adult", time.Minute)
	require.NoError(t, err)
	access, _, err := d.svc.RefreshToken(ctx, raw, "p-adult", pinTok)
	require.NoError(t, err)
	claims, err := d.tokens.ValidateAccessToken(access)
	require.NoError(t, err)
	require.Equal(t, "p-adult", claims.ProfileID)
}

// A PIN proof for the WRONG profile must not unlock a different one.
func TestRefreshToken_SwitchOutOfKids_WrongProfilePIN_Rejected(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-adult", UserID: "u1", Name: "Adult", IsDefault: true, Parental: &user.ParentalSettings{PINHash: "h"}})
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-kid", UserID: "u1", Name: "Kid", IsKids: true})

	raw := "kid-token-3"
	h := sha256.Sum256([]byte(raw))
	kid := shared.ProfileID("p-kid")
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt3", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ProfileID: &kid, ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	wrongTok, _ := d.tokens.GeneratePINToken("p-kid", time.Minute) // proof for the wrong profile
	_, _, err := d.svc.RefreshToken(ctx, raw, "p-adult", wrongTok)
	require.ErrorIs(t, err, ErrPINRequired)
}

// Legacy tokens (no bound profile) keep the old behaviour: fall back to default.
func TestRefreshToken_LegacyTokenFallsBackToDefault(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-def", UserID: "u1", Name: "Main", IsDefault: true})
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-other", UserID: "u1", Name: "Other"})

	raw := "legacy-token"
	h := sha256.Sum256([]byte(raw))
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ProfileID: nil, ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	access, _, err := d.svc.RefreshToken(ctx, raw, "", "")
	require.NoError(t, err)
	claims, err := d.tokens.ValidateAccessToken(access)
	require.NoError(t, err)
	require.Equal(t, "p-def", claims.ProfileID, "legacy token (nil profile) falls back to default")
}

func TestRefreshToken_NotFound(t *testing.T) {
	d := newAuthTestDeps()
	_, _, err := d.svc.RefreshToken(context.Background(), "bogus", "", "")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestRefreshToken_AlreadyUsed(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	rawToken := "used-token"
	h := sha256.Sum256([]byte(rawToken))
	hash := hex.EncodeToString(h[:])
	rt := &user.RefreshToken{
		ID:        "rt1",
		UserID:    "u1",
		TokenHash: hash,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Used:      true,
	}
	_ = d.refreshTkns.Create(ctx, rt)

	_, _, err := d.svc.RefreshToken(ctx, rawToken, "", "")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials for used token, got %v", err)
	}
}

// The old TestRefreshToken_Reuse_PurgesAllUserTokens pinned the opposite
// contract (replay → purge every session of the user). That purge could only
// ever fire on multi-tab race losers or interrupted rotations - the Used row
// lives milliseconds, a real thief hits "not found" - and préprod logs showed
// it logging the whole account out of every device daily. The replacement
// contract is TestRefreshToken_ReplayedTokenDoesNotPurgeOtherSessions.

func TestRefreshToken_Expired(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	rawToken := "expired-token"
	h := sha256.Sum256([]byte(rawToken))
	hash := hex.EncodeToString(h[:])
	rt := &user.RefreshToken{
		ID:        "rt1",
		UserID:    "u1",
		TokenHash: hash,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	_ = d.refreshTkns.Create(ctx, rt)

	_, _, err := d.svc.RefreshToken(ctx, rawToken, "", "")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials for expired token, got %v", err)
	}
}

func TestRefreshToken_UserNotFound(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()

	rawToken := "orphan-token"
	h := sha256.Sum256([]byte(rawToken))
	hash := hex.EncodeToString(h[:])
	rt := &user.RefreshToken{
		ID:        "rt1",
		UserID:    "ghost",
		TokenHash: hash,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	_ = d.refreshTkns.Create(ctx, rt)

	_, _, err := d.svc.RefreshToken(ctx, rawToken, "", "")
	if err == nil {
		t.Fatal("expected error for missing user")
	}
}

func TestRefreshToken_AccessTokenGenError(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	rawToken := "tok"
	h := sha256.Sum256([]byte(rawToken))
	hash := hex.EncodeToString(h[:])
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hash, ExpiresAt: time.Now().Add(24 * time.Hour),
	})
	d.tokens.Err = errors.New("gen fail")

	_, _, err := d.svc.RefreshToken(ctx, rawToken, "", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRefreshToken_RefreshTokenGenError(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	rawToken := "tok2"
	h := sha256.Sum256([]byte(rawToken))
	hash := hex.EncodeToString(h[:])
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hash, ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	d.refreshTkns.Err = errors.New("store fail")

	_, _, err := d.svc.RefreshToken(ctx, rawToken, "", "")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials when repo fails, got %v", err)
	}
}

func TestRefreshToken_NewRefreshTokenStoreError(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	rawToken := "tok3"
	h := sha256.Sum256([]byte(rawToken))
	hash := hex.EncodeToString(h[:])
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hash, ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	access, newRefresh, err := d.svc.RefreshToken(ctx, rawToken, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if access == "" || newRefresh == "" {
		t.Fatal("expected tokens")
	}

	oldRT, _ := d.refreshTkns.GetByTokenHash(ctx, hash)
	if oldRT != nil && !oldRT.Used {
		t.Fatal("old token should be marked used")
	}
}

func TestRefreshToken_WithProfileAndRoles(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", true, true)

	_ = d.profiles.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Default", IsDefault: true})
	role := &user.Role{ID: "r1", Name: "admin", Permissions: []string{"all"}}
	_ = d.roles.Create(ctx, role)
	_ = d.userRoles.AssignRole(ctx, "u1", "r1")

	rawToken := "tok-roles"
	h := sha256.Sum256([]byte(rawToken))
	hash := hex.EncodeToString(h[:])
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hash, ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	access, _, err := d.svc.RefreshToken(ctx, rawToken, "", "")
	if err != nil {
		t.Fatal(err)
	}
	claims, _ := d.tokens.ValidateAccessToken(access)
	if claims.ProfileID != "p1" {
		t.Fatalf("expected profile p1, got %s", claims.ProfileID)
	}
	if len(claims.Permissions) != 1 || claims.Permissions[0] != "all" {
		t.Fatalf("expected [all] permissions, got %v", claims.Permissions)
	}
	if !claims.IsAdmin {
		t.Fatal("expected IsAdmin")
	}
}

func TestRefreshToken_FallbackFirstProfile(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alt", IsDefault: false})

	rawToken := "tok-fb"
	h := sha256.Sum256([]byte(rawToken))
	hash := hex.EncodeToString(h[:])
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hash, ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	access, _, err := d.svc.RefreshToken(ctx, rawToken, "", "")
	if err != nil {
		t.Fatal(err)
	}
	claims, _ := d.tokens.ValidateAccessToken(access)
	if claims.ProfileID != "p1" {
		t.Fatalf("expected fallback to p1, got %s", claims.ProfileID)
	}
}

// --- Logout ---

// A logout that names no session ends none. It used to end EVERY session of
// the account, which is how signing out of a phone signed out the television
// (production complaint, 2026-09-22) - and a client reaches this case without
// meaning to, whenever its stored token cannot be read.
func TestLogout_WithoutToken_EndsNothing(t *testing.T) {
	d := newAuthTestDeps()
	rev := &recordingRevoker{}
	CommandWithTokenRevoker(rev)(d.svc)
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_, alices, err := d.svc.Login(ctx, "alice", "pw")
	if err != nil {
		t.Fatal(err)
	}

	if err := d.svc.Logout(ctx, "u1", ""); err != nil {
		t.Fatal(err)
	}

	if _, _, err := d.svc.RefreshToken(ctx, alices, "", ""); err != nil {
		t.Errorf("the other devices of this account must keep their sessions: %v", err)
	}
	if len(rev.revoked) != 0 {
		t.Errorf("nothing was named, so nothing may be revoked, got %v", rev.revoked)
	}
}

// TestLogout_RevokesStreamTokens proves Logout cuts the user's outstanding
// stream/cast tokens (via the same TokenRevoker used by DisableUser /
// ChangePassword), not just their refresh token. Without this, a logged-out
// user's opaque stream token (up to 12h TTL) keeps serving media bytes long
// after the session ended.
// No logout revokes the ACCOUNT's tokens any more, whichever path it takes:
// the only revoker available works per user, so using it would stop the film
// playing on the television because a phone was signed out. Cutting an
// account's stream tokens is what DisableUser and ChangePassword do.
func TestLogout_NeverRevokesTheWholeAccount(t *testing.T) {
	d := newAuthTestDeps()
	rev := &recordingRevoker{}
	CommandWithTokenRevoker(rev)(d.svc)
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_, alices, err := d.svc.Login(ctx, "alice", "pw")
	if err != nil {
		t.Fatal(err)
	}

	if err := d.svc.Logout(ctx, "u1", alices); err != nil {
		t.Fatal(err)
	}
	if err := d.svc.Logout(ctx, "u1", ""); err != nil {
		t.Fatal(err)
	}

	if len(rev.revoked) != 0 {
		t.Errorf("no logout may revoke the account, got %v", rev.revoked)
	}
}

// A device that presents its refresh token ends its own session only: the
// account's other devices keep their tokens and no account-wide revoke fires.
func TestLogout_WithToken_EndsThatDeviceOnly(t *testing.T) {
	d := newAuthTestDeps()
	rev := &recordingRevoker{}
	CommandWithTokenRevoker(rev)(d.svc)
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_, phone, err := d.svc.Login(ctx, "alice", "pw")
	if err != nil {
		t.Fatal(err)
	}
	_, tv, err := d.svc.Login(ctx, "alice", "pw")
	if err != nil {
		t.Fatal(err)
	}

	if err := d.svc.Logout(ctx, "u1", phone); err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.svc.RefreshToken(ctx, phone, "", ""); err == nil {
		t.Error("the signed-out device's token must be dead")
	}
	if _, _, err := d.svc.RefreshToken(ctx, tv, "", ""); err != nil {
		t.Errorf("the other device must keep its session: %v", err)
	}
	if len(rev.revoked) != 0 {
		t.Errorf("a per-device logout must not revoke the account's tokens, got %v", rev.revoked)
	}
}

// A token that is not this user's (or unknown) is ignored and the logout
// falls back to the account-wide revoke rather than trusting the caller.
// A token this account does not own names no session of it: the presenter's
// own sessions stay, and the owner's too.
func TestLogout_ForeignToken_EndsNothing(t *testing.T) {
	d := newAuthTestDeps()
	rev := &recordingRevoker{}
	CommandWithTokenRevoker(rev)(d.svc)
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	seedAuthUser(t, d, "u2", "bob", "pw", false, true)
	_, bobs, err := d.svc.Login(ctx, "bob", "pw")
	if err != nil {
		t.Fatal(err)
	}

	if err := d.svc.Logout(ctx, "u1", bobs); err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.svc.RefreshToken(ctx, bobs, "", ""); err != nil {
		t.Errorf("bob's session must survive alice's logout: %v", err)
	}
	if len(rev.revoked) != 0 {
		t.Errorf("no session of alice's was named, so nothing may be revoked, got %v", rev.revoked)
	}
}

// --- generateAndStoreRefreshToken ---

func TestGenerateAndStoreRefreshToken_DuplicateHashRetries(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()

	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	access, refresh, err := d.svc.Login(ctx, "alice", "pw")
	if err != nil {
		t.Fatal(err)
	}
	if access == "" || refresh == "" {
		t.Fatal("expected tokens")
	}
}

func TestGenerateAndStoreRefreshToken_GenError(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	d.tokens.Err = errors.New("gen fail")

	_, _, err := d.svc.Login(ctx, "alice", "pw")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- GenerateTokensForUser ---

func TestGenerateTokensForUser_Happy(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	access, refresh, err := d.svc.GenerateTokensForUser(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if access == "" || refresh == "" {
		t.Fatal("expected tokens")
	}
}

func TestGenerateTokensForUser_UserNotFound(t *testing.T) {
	d := newAuthTestDeps()
	_, _, err := d.svc.GenerateTokensForUser(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGenerateTokensForUser_UserDisabled(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, false)

	_, _, err := d.svc.GenerateTokensForUser(ctx, "u1")
	if !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("expected ErrUserDisabled, got %v", err)
	}
}

func TestGenerateTokensForUser_WithRolesAndProfile(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", true, true)

	_ = d.profiles.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Default", IsDefault: true})
	role := &user.Role{ID: "r1", Name: "viewer", Permissions: []string{"read"}}
	_ = d.roles.Create(ctx, role)
	_ = d.userRoles.AssignRole(ctx, "u1", "r1")

	access, _, err := d.svc.GenerateTokensForUser(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	claims, _ := d.tokens.ValidateAccessToken(access)
	if claims.ProfileID != "p1" {
		t.Fatalf("expected profile p1, got %s", claims.ProfileID)
	}
	if len(claims.Permissions) != 1 || claims.Permissions[0] != "read" {
		t.Fatalf("expected [read], got %v", claims.Permissions)
	}
}

func TestGenerateTokensForUser_FallbackFirstProfile(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Alt", IsDefault: false})

	access, _, err := d.svc.GenerateTokensForUser(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	claims, _ := d.tokens.ValidateAccessToken(access)
	if claims.ProfileID != "p1" {
		t.Fatalf("expected fallback to p1, got %s", claims.ProfileID)
	}
}

func TestGenerateTokensForUser_AccessTokenError(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	d.tokens.Err = errors.New("token fail")

	_, _, err := d.svc.GenerateTokensForUser(ctx, "u1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// Same inversion as TestLogin_ProfileRepoError_FailsClosed: this path mints
// tokens too (QuickConnect, OIDC, impersonation), so a swallowed profile error
// here is the same parental bypass by another door.
func TestGenerateTokensForUser_ProfileError_FailsClosed(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	d.profiles.Err = errors.New("profiles down")

	access, _, err := d.svc.GenerateTokensForUser(ctx, "u1")
	if err == nil {
		claims, _ := d.tokens.ValidateAccessToken(access)
		t.Fatalf("a profile lookup failure must refuse token generation, got ProfileID=%q", claims.ProfileID)
	}
}

func TestGenerateTokensForUser_RolesError_NonFatal(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	d.userRoles.Err = errors.New("roles down")

	_, _, err := d.svc.GenerateTokensForUser(ctx, "u1")
	if err != nil {
		t.Fatalf("roles error should be non-fatal, got %v", err)
	}
}

// --- Setup empty username ---

func TestSetup_EmptyUsername(t *testing.T) {
	d := newAuthTestDeps()
	_, err := d.svc.Setup(context.Background(), "", "longpassword", "a@a.com")
	if err == nil {
		t.Fatal("expected error for empty username")
	}
}

// --- RefreshToken additional coverage ---

type failOnNewRefreshGen struct {
	inner *ufakes.FakeTokenGenerator
	fail  bool
}

func (f *failOnNewRefreshGen) GenerateAccessToken(c user.Claims) (string, error) {
	return f.inner.GenerateAccessToken(c)
}
func (f *failOnNewRefreshGen) GenerateRefreshToken() (string, error) {
	if f.fail {
		return "", errors.New("refresh gen boom")
	}
	return f.inner.GenerateRefreshToken()
}
func (f *failOnNewRefreshGen) ValidateAccessToken(t string) (*user.Claims, error) {
	return f.inner.ValidateAccessToken(t)
}
func (f *failOnNewRefreshGen) GeneratePINToken(p string, ttl time.Duration) (string, error) {
	return f.inner.GeneratePINToken(p, ttl)
}
func (f *failOnNewRefreshGen) ParsePINToken(raw string) (string, error) {
	return f.inner.ParsePINToken(raw)
}

func TestRefreshToken_NewRefreshGenError(t *testing.T) {
	users := ufakes.NewFakeUserRepository()
	profiles := ufakes.NewFakeProfileRepository()
	refreshTkns := ufakes.NewFakeRefreshTokenRepository()
	roles := ufakes.NewFakeRoleRepository()
	userRoles := ufakes.NewFakeUserRoleRepository(roles)
	hasher := ufakes.NewFakePasswordHasher()
	events := fakes.NewFakeEventBus()
	tokenGen := &failOnNewRefreshGen{inner: ufakes.NewFakeTokenGenerator()}

	svc := NewCommandService(users, profiles, refreshTkns, roles, userRoles, hasher, tokenGen, events, 7*24*time.Hour, ufakes.NewFakeQuickConnectRepository())
	ctx := context.Background()

	_ = users.Create(ctx, &user.User{ID: "u1", Username: "alice", PasswordHash: "hashed:pw", Enabled: true})

	rawToken := "tok-rg"
	h := sha256.Sum256([]byte(rawToken))
	hash := hex.EncodeToString(h[:])
	_ = refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hash, ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	// First call succeeds to mark used, then fail on new refresh gen
	tokenGen.fail = true
	_, _, err := svc.RefreshToken(ctx, rawToken, "", "")
	if err == nil {
		t.Fatal("expected error when new refresh token gen fails")
	}
}

type failOnSecondCreateRT struct {
	inner     *ufakes.FakeRefreshTokenRepository
	createCnt int
}

func (r *failOnSecondCreateRT) Create(ctx context.Context, t *user.RefreshToken) error {
	r.createCnt++
	if r.createCnt > 0 {
		return errors.New("store fail")
	}
	return r.inner.Create(ctx, t)
}
func (r *failOnSecondCreateRT) GetByTokenHash(ctx context.Context, hash string) (*user.RefreshToken, error) {
	return r.inner.GetByTokenHash(ctx, hash)
}
func (r *failOnSecondCreateRT) GetByID(ctx context.Context, id string) (*user.RefreshToken, error) {
	return r.inner.GetByID(ctx, id)
}
func (r *failOnSecondCreateRT) SetSuccessor(ctx context.Context, id, successorID string) error {
	return r.inner.SetSuccessor(ctx, id, successorID)
}
func (r *failOnSecondCreateRT) MarkUsedIfNotUsed(ctx context.Context, id string) (bool, error) {
	return r.inner.MarkUsedIfNotUsed(ctx, id)
}
func (r *failOnSecondCreateRT) DeleteByID(ctx context.Context, id string) error {
	return r.inner.DeleteByID(ctx, id)
}
func (r *failOnSecondCreateRT) DeleteByUserID(ctx context.Context, userID shared.UserID) error {
	return r.inner.DeleteByUserID(ctx, userID)
}
func (r *failOnSecondCreateRT) DeleteExpired(ctx context.Context) error {
	return r.inner.DeleteExpired(ctx)
}

func TestRefreshToken_NewRTStoreError(t *testing.T) {
	users := ufakes.NewFakeUserRepository()
	profiles := ufakes.NewFakeProfileRepository()
	innerRT := ufakes.NewFakeRefreshTokenRepository()
	rtRepo := &failOnSecondCreateRT{inner: innerRT}
	roles := ufakes.NewFakeRoleRepository()
	userRoles := ufakes.NewFakeUserRoleRepository(roles)
	hasher := ufakes.NewFakePasswordHasher()
	tokenGen := ufakes.NewFakeTokenGenerator()
	events := fakes.NewFakeEventBus()

	svc := NewCommandService(users, profiles, rtRepo, roles, userRoles, hasher, tokenGen, events, 7*24*time.Hour, ufakes.NewFakeQuickConnectRepository())
	ctx := context.Background()

	_ = users.Create(ctx, &user.User{ID: "u1", Username: "alice", PasswordHash: "hashed:pw", Enabled: true})

	rawToken := "tok-store"
	h := sha256.Sum256([]byte(rawToken))
	hash := hex.EncodeToString(h[:])
	_ = innerRT.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hash, ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	_, _, err := svc.RefreshToken(ctx, rawToken, "", "")
	if err == nil {
		t.Fatal("expected error when new RT store fails")
	}
}

// --- StartCleanup ---

func TestStartCleanup_RunsInitialCleanup(t *testing.T) {
	d := newAuthTestDeps()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt-exp", UserID: "u1", TokenHash: "h-exp",
		ExpiresAt: time.Now().Add(-1 * time.Hour), CreatedAt: time.Now(),
	})
	if d.refreshTkns.Count() != 1 {
		t.Fatal("expected 1 token seeded")
	}

	d.svc.StartCleanupWithInterval(ctx, 50*time.Millisecond)
	// Replaces a fragile time.Sleep that raced with the cleanup goroutine
	// on slow CI/WSL hosts. Eventually retries until the expired token is
	// purged or the timeout elapses.
	require.Eventually(t, func() bool {
		return d.refreshTkns.Count() == 0
	}, 2*time.Second, 5*time.Millisecond)
}

func TestStartCleanup_TickerCleansAgain(t *testing.T) {
	d := newAuthTestDeps()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d.svc.StartCleanupWithInterval(ctx, 20*time.Millisecond)

	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt-exp2", UserID: "u1", TokenHash: "h-exp2",
		ExpiresAt: time.Now().Add(-1 * time.Hour), CreatedAt: time.Now(),
	})
	// Even if we race the very first tick (which would observe 0 tokens
	// before our Create), a subsequent tick will catch the new expired
	// token. Eventually polls instead of sleeping a fixed duration.
	require.Eventually(t, func() bool {
		return d.refreshTkns.Count() == 0
	}, 2*time.Second, 5*time.Millisecond)
}

func TestStartCleanup_StopsOnCancel(t *testing.T) {
	d := newAuthTestDeps()
	ctx, cancel := context.WithCancel(context.Background())
	d.svc.StartCleanup(ctx)
	cancel()
	// Wait deterministically for the cancellation to propagate, then yield
	// the scheduler so the cleanup goroutine has a chance to observe
	// ctx.Done() and return. -race covers actual leak detection.
	<-ctx.Done()
	for i := 0; i < 100; i++ {
		runtime.Gosched()
	}
}

// --- Login lockout ---

func TestLogin_LockedAfterMaxFails(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "correct", false, true)

	d.svc.loginAttempts = newLoginAttemptLimiter(1, 15*time.Minute)
	d.svc.loginMaxFails = 1
	d.svc.loginWindow = 15 * time.Minute

	_, _, err := d.svc.Login(ctx, "alice", "wrong")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}

	_, _, err = d.svc.Login(ctx, "alice", "correct")
	if !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("expected ErrAccountLocked after 1 fail with maxFails=1, got %v", err)
	}
}

func TestLogin_NotLockedBeforeMaxFails(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "correct", false, true)

	d.svc.loginAttempts = newLoginAttemptLimiter(2, 15*time.Minute)
	d.svc.loginMaxFails = 2
	d.svc.loginWindow = 15 * time.Minute

	d.svc.Login(ctx, "alice", "wrong")

	_, _, err := d.svc.Login(ctx, "alice", "correct")
	if err != nil {
		t.Fatalf("should succeed before lockout threshold: %v", err)
	}
}

// Regression: isLocked used to delete non-locked entries because
// time.Since(zeroTime) > window was always true, so count never reached
// maxFails and lockout never triggered in production (maxFails=5).
func TestLogin_LocksAtProdThreshold(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "correct", false, true)

	d.svc.loginAttempts = newLoginAttemptLimiter(5, 15*time.Minute)
	d.svc.loginMaxFails = 5
	d.svc.loginWindow = 15 * time.Minute

	for i := 0; i < 5; i++ {
		_, _, err := d.svc.Login(ctx, "alice", "wrong")
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: expected ErrInvalidCredentials, got %v", i+1, err)
		}
	}

	_, _, err := d.svc.Login(ctx, "alice", "correct")
	if !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("expected ErrAccountLocked after 5 failed attempts, got %v", err)
	}
}

// --- IsPasswordRecoveryEnabled ---

func newAuthTestDepsWithReset() (*authTestDeps, *ufakes.FakePasswordResetTokenRepository, *fakes.FakeServerConfigRepository) {
	d := newAuthTestDeps()
	resetTokens := ufakes.NewFakePasswordResetTokenRepository()
	serverCfg := fakes.NewFakeServerConfigRepository()

	d.svc = NewCommandService(
		d.users, d.profiles, d.refreshTkns, d.roles, d.userRoles,
		d.hasher, d.tokens, d.events, 7*24*time.Hour, ufakes.NewFakeQuickConnectRepository(),
		WithPasswordReset(resetTokens, serverCfg, true),
	)
	return d, resetTokens, serverCfg
}

func TestIsPasswordRecoveryEnabled_True(t *testing.T) {
	d, _, serverCfg := newAuthTestDepsWithReset()
	ctx := context.Background()
	_ = serverCfg.Set(ctx, "auth.password_recovery_enabled", "true")

	if !d.svc.IsPasswordRecoveryEnabled(ctx) {
		t.Fatal("expected password recovery enabled")
	}
}

func TestIsPasswordRecoveryEnabled_False_NotConfigured(t *testing.T) {
	d, _, serverCfg := newAuthTestDepsWithReset()
	ctx := context.Background()
	_ = serverCfg.Set(ctx, "auth.password_recovery_enabled", "false")

	if d.svc.IsPasswordRecoveryEnabled(ctx) {
		t.Fatal("expected password recovery disabled when value is 'false'")
	}
}

func TestIsPasswordRecoveryEnabled_False_NoSMTP(t *testing.T) {
	d := newAuthTestDeps()
	resetTokens := ufakes.NewFakePasswordResetTokenRepository()
	serverCfg := fakes.NewFakeServerConfigRepository()
	d.svc = NewCommandService(
		d.users, d.profiles, d.refreshTkns, d.roles, d.userRoles,
		d.hasher, d.tokens, d.events, 7*24*time.Hour, ufakes.NewFakeQuickConnectRepository(),
		WithPasswordReset(resetTokens, serverCfg, false),
	)
	ctx := context.Background()
	_ = serverCfg.Set(ctx, "auth.password_recovery_enabled", "true")

	if d.svc.IsPasswordRecoveryEnabled(ctx) {
		t.Fatal("expected disabled when SMTP not configured")
	}
}

func TestIsPasswordRecoveryEnabled_False_NoServerCfg(t *testing.T) {
	d := newAuthTestDeps()
	if d.svc.IsPasswordRecoveryEnabled(context.Background()) {
		t.Fatal("expected disabled when serverCfg is nil")
	}
}

func TestIsPasswordRecoveryEnabled_False_ConfigError(t *testing.T) {
	d, _, serverCfg := newAuthTestDepsWithReset()
	serverCfg.Err = errors.New("db error")
	if d.svc.IsPasswordRecoveryEnabled(context.Background()) {
		t.Fatal("expected disabled when config repo errors")
	}
}

// --- RequestPasswordReset ---

func TestRequestPasswordReset_Happy(t *testing.T) {
	d, _, serverCfg := newAuthTestDepsWithReset()
	ctx := context.Background()
	_ = serverCfg.Set(ctx, "auth.password_recovery_enabled", "true")
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	u, _ := d.users.GetByID(ctx, "u1")
	u.Email = "alice@example.com"
	d.users.Update(ctx, u)

	err := d.svc.RequestPasswordReset(ctx, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}

	evts := d.events.PublishedEvents()
	found := false
	for _, e := range evts {
		if e.EventName() == "user.password.reset_requested" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected password_reset.requested event")
	}
}

func TestRequestPasswordReset_Disabled(t *testing.T) {
	d := newAuthTestDeps()
	err := d.svc.RequestPasswordReset(context.Background(), "alice@example.com")
	if !errors.Is(err, ErrPasswordRecoveryDisabled) {
		t.Fatalf("expected ErrPasswordRecoveryDisabled, got %v", err)
	}
}

func TestRequestPasswordReset_UserNotFound_NoLeak(t *testing.T) {
	d, _, serverCfg := newAuthTestDepsWithReset()
	ctx := context.Background()
	_ = serverCfg.Set(ctx, "auth.password_recovery_enabled", "true")

	err := d.svc.RequestPasswordReset(ctx, "nobody@example.com")
	if err != nil {
		t.Fatalf("should not leak user not found, got %v", err)
	}
}

func TestRequestPasswordReset_DisabledUser_NoLeak(t *testing.T) {
	d, _, serverCfg := newAuthTestDepsWithReset()
	ctx := context.Background()
	_ = serverCfg.Set(ctx, "auth.password_recovery_enabled", "true")
	seedAuthUser(t, d, "u1", "alice", "pw", false, false)
	u, _ := d.users.GetByID(ctx, "u1")
	u.Email = "alice@example.com"
	d.users.Update(ctx, u)

	err := d.svc.RequestPasswordReset(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("should not leak disabled user, got %v", err)
	}
}

// --- ResetPassword ---

func TestResetPassword_Happy(t *testing.T) {
	d, resetTokens, serverCfg := newAuthTestDepsWithReset()
	ctx := context.Background()
	_ = serverCfg.Set(ctx, "auth.password_recovery_enabled", "true")
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	// Create a valid reset token
	rawToken := "reset-token-hex"
	h := sha256Hash(rawToken)
	_ = resetTokens.Create(ctx, &user.PasswordResetToken{
		ID:        "prt-1",
		UserID:    "u1",
		TokenHash: h,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	})

	err := d.svc.ResetPassword(ctx, rawToken, "newpassword123")
	if err != nil {
		t.Fatal(err)
	}

	u, _ := d.users.GetByID(ctx, "u1")
	if u.PasswordHash != "hashed:newpassword123" {
		t.Fatalf("expected new hash, got %s", u.PasswordHash)
	}
}

func TestResetPassword_InvalidToken(t *testing.T) {
	d, _, serverCfg := newAuthTestDepsWithReset()
	ctx := context.Background()
	_ = serverCfg.Set(ctx, "auth.password_recovery_enabled", "true")

	err := d.svc.ResetPassword(ctx, "bogus-token", "newpassword123")
	if !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("expected ErrResetTokenInvalid, got %v", err)
	}
}

func TestResetPassword_ExpiredToken(t *testing.T) {
	d, resetTokens, serverCfg := newAuthTestDepsWithReset()
	ctx := context.Background()
	_ = serverCfg.Set(ctx, "auth.password_recovery_enabled", "true")
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	rawToken := "expired-reset"
	h := sha256Hash(rawToken)
	_ = resetTokens.Create(ctx, &user.PasswordResetToken{
		ID:        "prt-1",
		UserID:    "u1",
		TokenHash: h,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	})

	err := d.svc.ResetPassword(ctx, rawToken, "newpassword123")
	if !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("expected ErrResetTokenInvalid for expired token, got %v", err)
	}
}

func TestResetPassword_UsedToken(t *testing.T) {
	d, resetTokens, serverCfg := newAuthTestDepsWithReset()
	ctx := context.Background()
	_ = serverCfg.Set(ctx, "auth.password_recovery_enabled", "true")
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	rawToken := "used-reset"
	h := sha256Hash(rawToken)
	_ = resetTokens.Create(ctx, &user.PasswordResetToken{
		ID:        "prt-1",
		UserID:    "u1",
		TokenHash: h,
		ExpiresAt: time.Now().Add(30 * time.Minute),
		Used:      true,
	})

	err := d.svc.ResetPassword(ctx, rawToken, "newpassword123")
	if !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("expected ErrResetTokenInvalid for used token, got %v", err)
	}
}

func TestResetPassword_ShortPassword(t *testing.T) {
	d, resetTokens, serverCfg := newAuthTestDepsWithReset()
	ctx := context.Background()
	_ = serverCfg.Set(ctx, "auth.password_recovery_enabled", "true")
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	rawToken := "short-pw-reset"
	h := sha256Hash(rawToken)
	_ = resetTokens.Create(ctx, &user.PasswordResetToken{
		ID:        "prt-1",
		UserID:    "u1",
		TokenHash: h,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	})

	err := d.svc.ResetPassword(ctx, rawToken, "short")
	if !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("expected ErrPasswordTooShort, got %v", err)
	}
}

func TestResetPassword_NoResetTokens(t *testing.T) {
	d := newAuthTestDeps()
	err := d.svc.ResetPassword(context.Background(), "token", "newpassword123")
	if !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("expected ErrResetTokenInvalid when resetTokens nil, got %v", err)
	}
}

func TestResetPassword_PasswordBoundary(t *testing.T) {
	t.Run("7 chars rejected", func(t *testing.T) {
		d, resetTokens, serverCfg := newAuthTestDepsWithReset()
		ctx := context.Background()
		_ = serverCfg.Set(ctx, "auth.password_recovery_enabled", "true")
		seedAuthUser(t, d, "u1", "alice", "pw", false, true)
		rawToken := "boundary-7"
		h := sha256Hash(rawToken)
		_ = resetTokens.Create(ctx, &user.PasswordResetToken{
			ID: "prt-1", UserID: "u1", TokenHash: h,
			ExpiresAt: time.Now().Add(30 * time.Minute),
		})
		err := d.svc.ResetPassword(ctx, rawToken, "1234567")
		if !errors.Is(err, ErrPasswordTooShort) {
			t.Fatalf("7-char password should be rejected, got %v", err)
		}
	})
	t.Run("8 chars accepted", func(t *testing.T) {
		d, resetTokens, serverCfg := newAuthTestDepsWithReset()
		ctx := context.Background()
		_ = serverCfg.Set(ctx, "auth.password_recovery_enabled", "true")
		seedAuthUser(t, d, "u1", "alice", "pw", false, true)
		rawToken := "boundary-8"
		h := sha256Hash(rawToken)
		_ = resetTokens.Create(ctx, &user.PasswordResetToken{
			ID: "prt-1", UserID: "u1", TokenHash: h,
			ExpiresAt: time.Now().Add(30 * time.Minute),
		})
		err := d.svc.ResetPassword(ctx, rawToken, "12345678")
		if err != nil {
			t.Fatalf("8-char password should be accepted, got %v", err)
		}
	})
}

func sha256Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func TestLogin_ResetClearsLockout(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "correct", false, true)

	d.svc.loginAttempts = newLoginAttemptLimiter(1, 15*time.Minute)
	d.svc.loginMaxFails = 1
	d.svc.loginWindow = 15 * time.Minute

	d.svc.Login(ctx, "alice", "wrong")

	// Successful login resets, so next wrong+correct cycle works
	_ = d.svc.loginAttempts.Reset(context.Background(), "alice")
	_, _, err := d.svc.Login(ctx, "alice", "correct")
	if err != nil {
		t.Fatalf("login should succeed after reset: %v", err)
	}
}

func TestLogin_WithLongTTL(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	longTTL := 180 * 24 * time.Hour
	access, refresh, err := d.svc.Login(ctx, "alice", "pw", longTTL)
	if err != nil {
		t.Fatal(err)
	}
	if access == "" || refresh == "" {
		t.Fatal("expected non-empty tokens")
	}
}

func TestRefreshToken_WithLongTTL(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	_, refresh, err := d.svc.Login(ctx, "alice", "pw")
	if err != nil {
		t.Fatal(err)
	}

	longTTL := 180 * 24 * time.Hour
	newAccess, newRefresh, err := d.svc.RefreshToken(ctx, refresh, "", "", longTTL)
	if err != nil {
		t.Fatal(err)
	}
	if newAccess == "" || newRefresh == "" {
		t.Fatal("expected non-empty tokens")
	}
}

func TestGenerateTokensForUser_WithLongTTL(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	longTTL := 180 * 24 * time.Hour
	access, refresh, err := d.svc.GenerateTokensForUser(ctx, "u1", longTTL)
	if err != nil {
		t.Fatal(err)
	}
	if access == "" || refresh == "" {
		t.Fatal("expected non-empty tokens")
	}
}

// --- Security fixes ---

// Concurrent refreshes with the SAME token are the multi-tab race (N tabs,
// one shared HttpOnly cookie): under the rotation grace EVERY racer succeeds
// - one wins the claim, the others are recognised as grace-window losers and
// issued their own DISTINCT sibling session. What the old TOCTOU contract
// protected against (a replayed token minting sessions unboundedly over time)
// is now bounded by the grace window instead:
// TestRefreshToken_ReplayAfterGrace_Rejected pins that side.
func TestRefreshToken_ConcurrentMultiTabRefresh_AllGetDistinctSessions(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	_, refresh, err := d.svc.Login(ctx, "alice", "pw")
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 10
	results := make([]error, goroutines)
	tokens := make([]string, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			_, tokens[i], results[i] = d.svc.RefreshToken(ctx, refresh, "", "")
		}(i)
	}
	wg.Wait()

	seen := make(map[string]bool, goroutines)
	for i, err := range results {
		if err != nil {
			t.Fatalf("racer %d must get a session within the rotation grace, got: %v", i, err)
		}
		if tokens[i] == "" || seen[tokens[i]] {
			t.Fatalf("racer %d must get its own DISTINCT refresh token", i)
		}
		seen[tokens[i]] = true
	}
}

func TestRefreshToken_DisabledUserRejected(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	_, refresh, err := d.svc.Login(ctx, "alice", "pw")
	if err != nil {
		t.Fatal(err)
	}

	u, _ := d.users.GetByID(ctx, "u1")
	u.Enabled = false
	_ = d.users.Update(ctx, u)

	_, _, err = d.svc.RefreshToken(ctx, refresh, "", "")
	if !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("expected ErrUserDisabled for disabled user, got %v", err)
	}
}

// --- Impersonate ("connect as") ---

func TestImpersonate_MintsTargetClaimsWithAdminStamp(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "usr-admin", "root", "pw", true, true)
	seedAuthUser(t, d, "usr-bob", "bob", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "prf-bob-1", UserID: "usr-bob", Name: "Bob", IsDefault: true})
	_ = d.profiles.Create(ctx, &user.Profile{ID: "prf-bob-2", UserID: "usr-bob", Name: "Kids", IsDefault: false})

	access, err := d.svc.Impersonate(ctx, "usr-admin", "usr-bob", "prf-bob-2")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := d.tokens.ValidateAccessToken(access)
	if err != nil {
		t.Fatal(err)
	}
	// Full power AS the target: the claims are the target's, not the admin's.
	if claims.UserID != "usr-bob" {
		t.Fatalf("UserID = %q, want usr-bob", claims.UserID)
	}
	if claims.ProfileID != "prf-bob-2" {
		t.Fatalf("ProfileID = %q, want the requested prf-bob-2", claims.ProfileID)
	}
	if claims.ImpersonatedBy != "usr-admin" {
		t.Fatalf("ImpersonatedBy = %q, want usr-admin (audit stamp)", claims.ImpersonatedBy)
	}
	if claims.IsAdmin {
		t.Fatal("impersonating a non-admin must not carry admin power")
	}
}

func TestImpersonate_EmptyProfileUsesTargetDefault(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "usr-admin", "root", "pw", true, true)
	seedAuthUser(t, d, "usr-bob", "bob", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "prf-bob-1", UserID: "usr-bob", Name: "Bob", IsDefault: true})

	access, err := d.svc.Impersonate(ctx, "usr-admin", "usr-bob", "")
	if err != nil {
		t.Fatal(err)
	}
	claims, _ := d.tokens.ValidateAccessToken(access)
	if claims.ProfileID != "prf-bob-1" {
		t.Fatalf("ProfileID = %q, want the target's default prf-bob-1", claims.ProfileID)
	}
}

func TestImpersonate_ForeignProfileRejected(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "usr-admin", "root", "pw", true, true)
	seedAuthUser(t, d, "usr-bob", "bob", "pw", false, true)
	seedAuthUser(t, d, "usr-eve", "eve", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "prf-bob-1", UserID: "usr-bob", Name: "Bob", IsDefault: true})
	_ = d.profiles.Create(ctx, &user.Profile{ID: "prf-eve-1", UserID: "usr-eve", Name: "Eve", IsDefault: true})

	// Activating Eve's profile while impersonating Bob must be rejected: the
	// requested profile must belong to the target user.
	_, err := d.svc.Impersonate(ctx, "usr-admin", "usr-bob", "prf-eve-1")
	if !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}
}

func TestImpersonate_DisabledTargetRejected(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "usr-admin", "root", "pw", true, true)
	seedAuthUser(t, d, "usr-bob", "bob", "pw", false, false) // disabled
	_ = d.profiles.Create(ctx, &user.Profile{ID: "prf-bob-1", UserID: "usr-bob", Name: "Bob", IsDefault: true})

	_, err := d.svc.Impersonate(ctx, "usr-admin", "usr-bob", "")
	if !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("expected ErrUserDisabled, got %v", err)
	}
}

func TestImpersonate_UnknownTargetRejected(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "usr-admin", "root", "pw", true, true)

	_, err := d.svc.Impersonate(ctx, "usr-admin", "usr-ghost", "")
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
}

// A replayed (already-Used) refresh token must NOT nuke the user's other
// sessions. The Used state is only ever observable for the few milliseconds
// between the rotation's claim and its cleanup - a genuine thief replaying a
// stolen token later hits "not found" - so every purge this branch ever fired
// was a multi-tab race loser or an interrupted rotation, logging the whole
// account (mobile included) out of every device (préprod, daily since 07-06).
func TestRefreshToken_ReplayedTokenDoesNotPurgeOtherSessions(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	usedRaw := "already-rotated-token"
	usedHash := sha256.Sum256([]byte(usedRaw))
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID:        "rt-used",
		UserID:    "u1",
		TokenHash: hex.EncodeToString(usedHash[:]),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Used:      true,
	})
	// The same user's healthy mobile session.
	mobileRaw := "mobile-session-token"
	mobileHash := sha256.Sum256([]byte(mobileRaw))
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID:        "rt-mobile",
		UserID:    "u1",
		TokenHash: hex.EncodeToString(mobileHash[:]),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	_, _, err := d.svc.RefreshToken(ctx, usedRaw, "", "")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}

	if _, err := d.refreshTkns.GetByTokenHash(ctx, hex.EncodeToString(mobileHash[:])); err != nil {
		t.Fatalf("the mobile session must survive a replayed web token: %v", err)
	}
}

func (f *failOnNewRefreshGen) DescribeRefreshToken(raw string) string {
	return f.inner.DescribeRefreshToken(raw)
}
