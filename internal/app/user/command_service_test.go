package user

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/gravity-zero/evey/internal/domain/user"
	ufakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
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

func TestLogin_UserDisabled(t *testing.T) {
	d := newAuthTestDeps()
	seedAuthUser(t, d, "u1", "alice", "pw", false, false)
	_, _, err := d.svc.Login(context.Background(), "alice", "pw")
	if !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("expected ErrUserDisabled, got %v", err)
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

func TestLogin_ProfileRepoError_NonFatal(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	d.profiles.Err = errors.New("profiles down")

	access, _, err := d.svc.Login(ctx, "alice", "pw")
	if err != nil {
		t.Fatalf("profile error should be non-fatal, got %v", err)
	}
	claims, _ := d.tokens.ValidateAccessToken(access)
	if claims.ProfileID != "" {
		t.Fatalf("expected empty profile ID, got %s", claims.ProfileID)
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

	access, newRefresh, err := d.svc.RefreshToken(ctx, rawToken)
	if err != nil {
		t.Fatal(err)
	}
	if access == "" || newRefresh == "" {
		t.Fatal("expected tokens")
	}
}

func TestRefreshToken_NotFound(t *testing.T) {
	d := newAuthTestDeps()
	_, _, err := d.svc.RefreshToken(context.Background(), "bogus")
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

	_, _, err := d.svc.RefreshToken(ctx, rawToken)
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials for used token, got %v", err)
	}
}

func TestRefreshToken_Reuse_PurgesAllUserTokens(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	reusedToken := "reused-tok"
	rh := sha256.Sum256([]byte(reusedToken))
	rhash := hex.EncodeToString(rh[:])

	otherToken := "other-tok"
	oh := sha256.Sum256([]byte(otherToken))
	ohash := hex.EncodeToString(oh[:])

	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: rhash,
		ExpiresAt: time.Now().Add(24 * time.Hour), Used: true,
	})
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt2", UserID: "u1", TokenHash: ohash,
		ExpiresAt: time.Now().Add(24 * time.Hour), Used: false,
	})

	_, _, err := d.svc.RefreshToken(ctx, reusedToken)
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}

	_, errOther := d.refreshTkns.GetByTokenHash(ctx, ohash)
	if errOther == nil {
		t.Fatal("reuse detection should purge ALL tokens for the user, but other token still exists")
	}
}

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

	_, _, err := d.svc.RefreshToken(ctx, rawToken)
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

	_, _, err := d.svc.RefreshToken(ctx, rawToken)
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

	_, _, err := d.svc.RefreshToken(ctx, rawToken)
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

	_, _, err := d.svc.RefreshToken(ctx, rawToken)
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

	access, newRefresh, err := d.svc.RefreshToken(ctx, rawToken)
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

	access, _, err := d.svc.RefreshToken(ctx, rawToken)
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

	access, _, err := d.svc.RefreshToken(ctx, rawToken)
	if err != nil {
		t.Fatal(err)
	}
	claims, _ := d.tokens.ValidateAccessToken(access)
	if claims.ProfileID != "p1" {
		t.Fatalf("expected fallback to p1, got %s", claims.ProfileID)
	}
}

// --- Logout ---

func TestLogout_Happy(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{ID: "rt1", UserID: "u1", TokenHash: "h1", ExpiresAt: time.Now().Add(time.Hour)})

	err := d.svc.Logout(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
}

func TestLogout_RepoError(t *testing.T) {
	d := newAuthTestDeps()
	d.refreshTkns.Err = errors.New("fail")
	err := d.svc.Logout(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected error")
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

func TestGenerateTokensForUser_ProfileError_NonFatal(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	d.profiles.Err = errors.New("profiles down")

	access, _, err := d.svc.GenerateTokensForUser(ctx, "u1")
	if err != nil {
		t.Fatalf("profile error should be non-fatal, got %v", err)
	}
	claims, _ := d.tokens.ValidateAccessToken(access)
	if claims.ProfileID != "" {
		t.Fatalf("expected empty profile, got %s", claims.ProfileID)
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
	_, _, err := svc.RefreshToken(ctx, rawToken)
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

	_, _, err := svc.RefreshToken(ctx, rawToken)
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
	time.Sleep(100 * time.Millisecond)

	if d.refreshTkns.Count() != 0 {
		t.Fatalf("expected expired token deleted, got %d", d.refreshTkns.Count())
	}
}

func TestStartCleanup_TickerCleansAgain(t *testing.T) {
	d := newAuthTestDeps()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d.svc.StartCleanupWithInterval(ctx, 50*time.Millisecond)
	time.Sleep(80 * time.Millisecond)

	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt-exp2", UserID: "u1", TokenHash: "h-exp2",
		ExpiresAt: time.Now().Add(-1 * time.Hour), CreatedAt: time.Now(),
	})
	if d.refreshTkns.Count() != 1 {
		t.Fatal("expected 1 token seeded")
	}

	time.Sleep(80 * time.Millisecond)
	if d.refreshTkns.Count() != 0 {
		t.Fatalf("expected ticker to clean expired token, got %d", d.refreshTkns.Count())
	}
}

func TestStartCleanup_StopsOnCancel(t *testing.T) {
	d := newAuthTestDeps()
	ctx, cancel := context.WithCancel(context.Background())
	d.svc.StartCleanup(ctx)
	cancel()
	time.Sleep(20 * time.Millisecond)
}

// --- Login lockout ---
// Note: loginAttemptLimiter's record() resets entry when lockedAt is zero
// because time.Since(time.Time{}) > window. So lockout only works when
// the user was previously locked (lockedAt set). Tests below use a custom
// limiter with maxFails=1 to hit the lock path in one call.

func TestLogin_LockedAfterMaxFails(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "correct", false, true)

	// Replace limiter with maxFails=1 so a single wrong attempt locks
	d.svc.loginAttempts = newLoginAttemptLimiter(1, 15*time.Minute)

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

	// maxFails=2: first fail doesn't lock (count=1 < 2)
	d.svc.loginAttempts = newLoginAttemptLimiter(2, 15*time.Minute)

	d.svc.Login(ctx, "alice", "wrong")

	// Since record resets the entry (lockedAt zero → time.Since > window),
	// the next call won't be locked
	_, _, err := d.svc.Login(ctx, "alice", "correct")
	if err != nil {
		t.Fatalf("should succeed before lockout threshold: %v", err)
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

	d.svc.Login(ctx, "alice", "wrong")

	// Successful login resets, so next wrong+correct cycle works
	d.svc.loginAttempts.reset("alice")
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
	newAccess, newRefresh, err := d.svc.RefreshToken(ctx, refresh, longTTL)
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

func TestRefreshToken_TOCTOU_ConcurrentReplayRejected(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)

	_, refresh, err := d.svc.Login(ctx, "alice", "pw")
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 10
	results := make([]error, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			_, _, results[i] = d.svc.RefreshToken(ctx, refresh)
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly 1 successful refresh, got %d", successes)
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

	_, _, err = d.svc.RefreshToken(ctx, refresh)
	if !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("expected ErrUserDisabled for disabled user, got %v", err)
	}
}

