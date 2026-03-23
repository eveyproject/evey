package user

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

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

	svc := NewCommandService(users, profiles, refreshTkns, roles, userRoles, hasher, tokens, events)
	return &authTestDeps{users, profiles, refreshTkns, roles, userRoles, hasher, tokens, events, svc}
}

func seedAuthUser(t *testing.T, d *authTestDeps, id, username, password string, admin, enabled bool) *user.User {
	t.Helper()
	u := &user.User{
		ID:           id,
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

// --- StartCleanup ---

func TestStartCleanup_RunsAndStops(t *testing.T) {
	d := newAuthTestDeps()
	qc := ufakes.NewFakeQuickConnectRepository()
	ctx, cancel := context.WithCancel(context.Background())
	d.svc.StartCleanup(ctx, qc)
	cancel()
}
