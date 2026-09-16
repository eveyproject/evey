package grpcapi

import (
	"context"
	"strings"
	"testing"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appactivity "github.com/evey-project/evey/internal/app/activity"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/config"
	activityfakes "github.com/evey-project/evey/internal/domain/activity/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type testUserEnv struct {
	handler     *userHandler
	userRepo    *userfakes.FakeUserRepository
	profileRepo *userfakes.FakeProfileRepository
	auth        *appuser.CommandService
	userSvc     *appuser.UserService
	access      *appuser.AccessService
	query       *appuser.QueryService
	events      *sharedfakes.FakeEventBus
	tokenGen    *userfakes.FakeTokenGenerator
	hasher      *userfakes.FakePasswordHasher
}

func setupUserHandler() *testUserEnv {
	userRepo := userfakes.NewFakeUserRepository()
	profileRepo := userfakes.NewFakeProfileRepository()
	refreshRepo := userfakes.NewFakeRefreshTokenRepository()
	apiKeyRepo := userfakes.NewFakeAPIKeyRepository()
	inviteRepo := userfakes.NewFakeInviteLinkRepository()
	qcRepo := userfakes.NewFakeQuickConnectRepository()
	roleRepo := userfakes.NewFakeRoleRepository()
	userRoleRepo := userfakes.NewFakeUserRoleRepository(roleRepo)
	hasher := userfakes.NewFakePasswordHasher()
	tokenGen := userfakes.NewFakeTokenGenerator()
	events := sharedfakes.NewFakeEventBus()

	auth := appuser.NewCommandService(
		userRepo, profileRepo, refreshRepo,
		roleRepo, userRoleRepo,
		hasher, tokenGen, events,
		7*24*time.Hour, userfakes.NewFakeQuickConnectRepository(),
	)
	userSvc := appuser.NewUserService(userRepo, profileRepo, refreshRepo, hasher, events, sharedfakes.NewFakeTransactionalUnit())
	access := appuser.NewAccessService(userRepo, apiKeyRepo, inviteRepo, qcRepo, hasher, events)
	query := appuser.NewQueryService(userRepo, profileRepo, apiKeyRepo, inviteRepo, qcRepo)

	actSvc := appactivity.NewCommandService(
		activityfakes.NewFakeLoginHistoryRepository(),
		activityfakes.NewFakeServerMetricRepository(),
		activityfakes.NewFakeAuditLogRepository(),
		events,
	)

	h := &userHandler{
		auth:        auth,
		userSvc:     userSvc,
		access:      access,
		query:       query,
		activitySvc: actSvc,
		cfg:         &config.Config{JWT: config.JWTConfig{RefreshTokenLongTTL: 180 * 24 * time.Hour}},
	}

	return &testUserEnv{
		handler:     h,
		userRepo:    userRepo,
		profileRepo: profileRepo,
		auth:        auth,
		userSvc:     userSvc,
		access:      access,
		query:       query,
		events:      events,
		tokenGen:    tokenGen,
		hasher:      hasher,
	}
}

func seedUser(t *testing.T, env *testUserEnv, u *user.User, password string) {
	t.Helper()
	hash, _ := env.hasher.Hash(password)
	u.PasswordHash = hash
	u.Enabled = true
	if err := env.userRepo.Create(context.Background(), u); err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func assertCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != want {
		t.Fatalf("got code %v, want %v", st.Code(), want)
	}
}

// --- Login tests ---

func TestLogin_Success(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com", IsAdmin: false}, "password123")
	env.profileRepo.Create(context.Background(), &user.Profile{ID: "prf-1", UserID: "usr-1", Name: "alice", IsDefault: true})

	resp, err := env.handler.Login(context.Background(), &eveyv1.LoginRequest{
		Username: "alice",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatal("expected tokens in response")
	}
	if resp.User == nil || resp.User.Username != "alice" {
		t.Fatal("expected user in response")
	}
}

func TestLogin_InvalidCredentials(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "password123")

	_, err := env.handler.Login(context.Background(), &eveyv1.LoginRequest{
		Username: "alice",
		Password: "wrong",
	})
	assertCode(t, err, codes.Unauthenticated)
}

// --- GetCurrentUser tests ---

func TestGetCurrentUser_Success(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "pass12345678")

	ctx := userCtx("usr-1", "prf-1")
	resp, err := env.handler.GetCurrentUser(ctx, &eveyv1.GetCurrentUserRequest{})
	if err != nil {
		t.Fatalf("GetCurrentUser: %v", err)
	}
	if resp.Username != "alice" {
		t.Fatalf("got username %q, want alice", resp.Username)
	}
}

func TestGetCurrentUser_Unauthenticated(t *testing.T) {
	env := setupUserHandler()
	_, err := env.handler.GetCurrentUser(context.Background(), &eveyv1.GetCurrentUserRequest{})
	assertCode(t, err, codes.Unauthenticated)
}

// --- CreateUser tests ---

func TestCreateUser_Success(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-admin", Username: "admin", Email: "admin@b.com", IsAdmin: true}, "adminpass1")

	ctx := adminCtx()
	resp, err := env.handler.CreateUser(ctx, &eveyv1.CreateUserRequest{
		Username: "bob",
		Email:    "bob@test.com",
		Password: "securepass",
		IsAdmin:  false,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if resp.Username != "bob" {
		t.Fatalf("got username %q, want bob", resp.Username)
	}
}

func TestCreateUser_NonAdminDenied(t *testing.T) {
	env := setupUserHandler()
	ctx := userCtx("usr-1", "prf-1")
	_, err := env.handler.CreateUser(ctx, &eveyv1.CreateUserRequest{
		Username: "bob",
		Password: "securepass",
	})
	assertCode(t, err, codes.PermissionDenied)
}

// --- UpdateUser tests ---

func TestUpdateUser_Self(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "pass12345678")

	ctx := userCtx("usr-1", "prf-1")
	newName := "alice_updated"
	resp, err := env.handler.UpdateUser(ctx, &eveyv1.UpdateUserRequest{
		Id:       "usr-1",
		Username: &newName,
	})
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if resp.Username != "alice_updated" {
		t.Fatalf("got %q, want alice_updated", resp.Username)
	}
}

// A rename must not store the space a phone keyboard appends: Login trims.
func TestUpdateUser_TrimsUsernameAndEmail(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "pass12345678")

	newName, newMail := " alice2 ", " a2@b.com "
	resp, err := env.handler.UpdateUser(userCtx("usr-1", "prf-1"), &eveyv1.UpdateUserRequest{
		Id: "usr-1", Username: &newName, Email: &newMail,
	})
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if resp.Username != "alice2" || resp.Email != "a2@b.com" {
		t.Fatalf("whitespace kept: %q / %q", resp.Username, resp.Email)
	}
}

func TestUpdateUser_OtherUserDenied(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "pass12345678")
	seedUser(t, env, &user.User{ID: "usr-2", Username: "bob", Email: "b@b.com"}, "pass12345678")

	ctx := userCtx("usr-1", "prf-1")
	newName := "hacked"
	_, err := env.handler.UpdateUser(ctx, &eveyv1.UpdateUserRequest{
		Id:       "usr-2",
		Username: &newName,
	})
	assertCode(t, err, codes.PermissionDenied)
}

func TestUpdateUser_NonAdminCannotSetCaps(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "pass12345678")

	ctx := userCtx("usr-1", "prf-1")
	big := int32(9999)
	cases := []struct {
		name string
		req  *eveyv1.UpdateUserRequest
	}{
		{"max_streams", &eveyv1.UpdateUserRequest{Id: "usr-1", MaxStreams: &big}},
		{"max_profiles", &eveyv1.UpdateUserRequest{Id: "usr-1", MaxProfiles: &big}},
		{"max_devices_per_profile", &eveyv1.UpdateUserRequest{Id: "usr-1", MaxDevicesPerProfile: &big}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := env.handler.UpdateUser(ctx, tc.req)
			assertCode(t, err, codes.PermissionDenied)
		})
	}
}

// A non-admin must not be able to grant themselves admin rights by setting
// IsAdmin=true on their own UpdateUser call.
func TestUpdateUser_NonAdminCannotEscalateAdmin(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "pass12345678")

	ctx := userCtx("usr-1", "prf-1")
	trueVal := true
	_, err := env.handler.UpdateUser(ctx, &eveyv1.UpdateUserRequest{
		Id:      "usr-1",
		IsAdmin: &trueVal,
	})
	assertCode(t, err, codes.PermissionDenied)

	stored, err := env.userRepo.GetByID(context.Background(), shared.UserID("usr-1"))
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored.IsAdmin {
		t.Fatal("user escalated to admin despite PermissionDenied response")
	}
}

func TestUpdateUser_AdminCanSetCaps(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-admin", Username: "root", Email: "r@b.com", IsAdmin: true}, "pass12345678")
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "pass12345678")

	admCtx := adminCtx()
	maxP := int32(5)
	resp, err := env.handler.UpdateUser(admCtx, &eveyv1.UpdateUserRequest{
		Id:          "usr-1",
		MaxProfiles: &maxP,
	})
	if err != nil {
		t.Fatalf("admin UpdateUser: %v", err)
	}
	if resp.MaxProfiles == nil || *resp.MaxProfiles != 5 {
		t.Fatalf("MaxProfiles = %v, want 5", resp.MaxProfiles)
	}
}

// Promoting a user who already owns a kids profile to admin is refused by the
// admin/kids invariant. That refusal is a business-rule precondition, not a
// server fault: it must map to FailedPrecondition with a clear reason, not a
// generic Internal (500) logged at ERROR. Regression for the F11 error-mapping
// nit (the security outcome - blocked, user stays non-admin - already held).
func TestUpdateUser_PromoteWithKidsProfile_FailedPrecondition(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-admin", Username: "root", Email: "r@b.com", IsAdmin: true}, "pass12345678")
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "pass12345678")
	// alice owns a kids profile - legal for a non-admin, forbidden once she is admin.
	if err := env.profileRepo.Create(context.Background(), &user.Profile{ID: "prf-kid", UserID: "usr-1", Name: "kiddo", IsKids: true}); err != nil {
		t.Fatalf("seed kids profile: %v", err)
	}

	trueVal := true
	_, err := env.handler.UpdateUser(adminCtx(), &eveyv1.UpdateUserRequest{
		Id:      "usr-1",
		IsAdmin: &trueVal,
	})
	assertCode(t, err, codes.FailedPrecondition)

	stored, gerr := env.userRepo.GetByID(context.Background(), shared.UserID("usr-1"))
	if gerr != nil {
		t.Fatalf("GetByID: %v", gerr)
	}
	if stored.IsAdmin {
		t.Fatal("user promoted to admin despite the kids-profile invariant refusal")
	}
}

// An admin changing another user's password via UpdateUser must actually
// persist the new hash - the field used to be silently ignored, so the user
// could never log in with the new password.
func TestUpdateUser_AdminChangesPassword(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-admin", Username: "root", Email: "r@b.com", IsAdmin: true}, "pass12345678")
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "oldpassword")

	newPass := "brandnewpass1"
	if _, err := env.handler.UpdateUser(adminCtx(), &eveyv1.UpdateUserRequest{
		Id:       "usr-1",
		Password: &newPass,
	}); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	u, err := env.userRepo.GetByID(context.Background(), "usr-1")
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	// FakePasswordHasher stores "hashed:<password>".
	if u.PasswordHash != "hashed:"+newPass {
		t.Fatalf("password hash not updated: got %q", u.PasswordHash)
	}
}

func TestUpdateUser_PasswordTooShort_InvalidArgument(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-admin", Username: "root", Email: "r@b.com", IsAdmin: true}, "pass12345678")
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "oldpassword")

	short := "abc"
	_, err := env.handler.UpdateUser(adminCtx(), &eveyv1.UpdateUserRequest{Id: "usr-1", Password: &short})
	assertCode(t, err, codes.InvalidArgument)
}

// An empty/omitted password leaves the existing hash intact ("leave blank to
// keep current").
func TestUpdateUser_EmptyPassword_KeepsCurrent(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "oldpassword")

	empty := ""
	newName := "alice2"
	if _, err := env.handler.UpdateUser(userCtx("usr-1", "prf-1"), &eveyv1.UpdateUserRequest{
		Id: "usr-1", Username: &newName, Password: &empty,
	}); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	u, _ := env.userRepo.GetByID(context.Background(), "usr-1")
	if u.PasswordHash != "hashed:oldpassword" {
		t.Fatalf("password should be unchanged, got %q", u.PasswordHash)
	}
}

// --- ListUsers test ---

func TestListUsers_Success(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com", IsAdmin: true}, "pass12345678")
	seedUser(t, env, &user.User{ID: "usr-2", Username: "bob", Email: "b@b.com"}, "pass12345678")

	resp, err := env.handler.ListUsers(context.Background(), &eveyv1.ListUsersRequest{})
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(resp.Users) != 2 {
		t.Fatalf("got %d users, want 2", len(resp.Users))
	}
}

// seedAdultProfile stores an unrestricted profile so handlers gated on the
// caller's active profile can resolve it.
func seedAdultProfile(t *testing.T, env *testUserEnv, profileID shared.ProfileID, userID shared.UserID) *user.Profile {
	t.Helper()
	p, err := user.NewProfile(profileID, userID, "Owner")
	if err != nil {
		t.Fatalf("NewProfile: %v", err)
	}
	if err := env.profileRepo.Create(context.Background(), p); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	return p
}

// --- CreateProfile tests ---

func TestCreateProfile_Success(t *testing.T) {
	env := setupUserHandler()
	// The caller's active profile must exist: the kids gate fails closed when it
	// cannot confirm the caller is on an unrestricted profile.
	seedAdultProfile(t, env, "prf-1", "usr-1")
	ctx := userCtx("usr-1", "prf-1")

	resp, err := env.handler.CreateProfile(ctx, &eveyv1.CreateProfileRequest{
		UserId: "usr-1",
		Name:   "Kids",
	})
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if resp.Name != "Kids" {
		t.Fatalf("got name %q, want Kids", resp.Name)
	}
	if resp.UserId != "usr-1" {
		t.Fatalf("got userId %q, want usr-1", resp.UserId)
	}
}

func TestCreateProfile_ForAnotherUserDenied(t *testing.T) {
	env := setupUserHandler()
	ctx := userCtx("usr-1", "prf-1")

	_, err := env.handler.CreateProfile(ctx, &eveyv1.CreateProfileRequest{
		UserId: "usr-other",
		Name:   "Hacked",
	})
	assertCode(t, err, codes.PermissionDenied)
}

// --- API Key tests ---

func TestCreateAPIKey_Success(t *testing.T) {
	env := setupUserHandler()
	ctx := userCtx("usr-1", "prf-1")

	resp, err := env.handler.CreateAPIKey(ctx, &eveyv1.CreateAPIKeyRequest{
		Name:   "my-key",
		Scopes: []string{"media:read"},
	})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if resp.Name != "my-key" {
		t.Fatalf("got name %q, want my-key", resp.Name)
	}
	if resp.UserId != "usr-1" {
		t.Fatalf("got userId %q, want usr-1", resp.UserId)
	}
	if resp.Key == "" || !strings.HasPrefix(resp.Key, "evey_") {
		t.Fatalf("expected raw key with evey_ prefix, got %q", resp.Key)
	}
}

func TestCreateAPIKey_EmptyScopesRejected(t *testing.T) {
	env := setupUserHandler()
	ctx := userCtx("usr-1", "prf-1")

	// A key with no scopes would be an unrestricted credential: refuse it at the
	// boundary rather than mint an allow-all key.
	_, err := env.handler.CreateAPIKey(ctx, &eveyv1.CreateAPIKeyRequest{Name: "no-scopes"})
	assertCode(t, err, codes.InvalidArgument)
}

func TestListAPIKeys_Success(t *testing.T) {
	env := setupUserHandler()
	ctx := userCtx("usr-1", "prf-1")

	_, _ = env.handler.CreateAPIKey(ctx, &eveyv1.CreateAPIKeyRequest{Name: "key1", Scopes: []string{"media:read"}})
	_, _ = env.handler.CreateAPIKey(ctx, &eveyv1.CreateAPIKeyRequest{Name: "key2", Scopes: []string{"media:read"}})

	resp, err := env.handler.ListAPIKeys(ctx, &eveyv1.ListAPIKeysRequest{})
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	if len(resp.ApiKeys) != 2 {
		t.Fatalf("got %d keys, want 2", len(resp.ApiKeys))
	}
	for _, k := range resp.ApiKeys {
		if k.Key != "" {
			t.Fatalf("ListAPIKeys should not return raw key, got %q", k.Key)
		}
	}
}

// --- QuickConnect tests ---

func TestQuickConnect_Flow(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-1", Username: "tester", IsAdmin: false}, "pass")

	// Initiate
	initResp, err := env.handler.InitiateQuickConnect(context.Background(), &eveyv1.InitiateQuickConnectRequest{})
	if err != nil {
		t.Fatalf("InitiateQuickConnect: %v", err)
	}
	code := initResp.Code
	if code == "" {
		t.Fatal("expected non-empty code")
	}

	// Check before authorize - should not be authorized
	checkResp, err := env.handler.CheckQuickConnect(context.Background(), &eveyv1.CheckQuickConnectRequest{Code: code})
	if err != nil {
		t.Fatalf("CheckQuickConnect: %v", err)
	}
	if checkResp.Authorized {
		t.Fatal("expected not authorized before AuthorizeQuickConnect")
	}

	// Authorize
	ctx := userCtx("usr-1", "prf-1")
	_, err = env.handler.AuthorizeQuickConnect(ctx, &eveyv1.AuthorizeQuickConnectRequest{Code: code})
	if err != nil {
		t.Fatalf("AuthorizeQuickConnect: %v", err)
	}

	// Check after authorize - should be authorized
	checkResp, err = env.handler.CheckQuickConnect(context.Background(), &eveyv1.CheckQuickConnectRequest{Code: code})
	if err != nil {
		t.Fatalf("CheckQuickConnect after authorize: %v", err)
	}
	if !checkResp.Authorized {
		t.Fatal("expected authorized after AuthorizeQuickConnect")
	}
	if checkResp.Auth == nil || checkResp.Auth.AccessToken == "" {
		t.Fatal("expected auth tokens in CheckQuickConnect response after authorize")
	}
}

func TestCheckQuickConnect_InvalidCode(t *testing.T) {
	env := setupUserHandler()
	_, err := env.handler.CheckQuickConnect(context.Background(), &eveyv1.CheckQuickConnectRequest{Code: "bogus"})
	assertCode(t, err, codes.NotFound)
}

// --- RefreshToken tests ---

func TestRefreshToken_Success(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "password123")
	env.profileRepo.Create(context.Background(), &user.Profile{ID: "prf-1", UserID: "usr-1", Name: "alice", IsDefault: true})

	loginResp, err := env.handler.Login(context.Background(), &eveyv1.LoginRequest{
		Username: "alice", Password: "password123",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	resp, err := env.handler.RefreshToken(context.Background(), &eveyv1.RefreshTokenRequest{
		RefreshToken: loginResp.RefreshToken,
	})
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatal("expected tokens in response")
	}
}

func TestRefreshToken_InvalidToken(t *testing.T) {
	env := setupUserHandler()
	_, err := env.handler.RefreshToken(context.Background(), &eveyv1.RefreshTokenRequest{
		RefreshToken: "invalid-token",
	})
	assertCode(t, err, codes.Unauthenticated)
}

// --- Logout tests ---

func TestLogout_Success(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "password123")

	ctx := userCtx("usr-1", "prf-1")
	resp, err := env.handler.Logout(ctx, &eveyv1.LogoutRequest{})
	if err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestLogout_Unauthenticated(t *testing.T) {
	env := setupUserHandler()
	_, err := env.handler.Logout(context.Background(), &eveyv1.LogoutRequest{})
	assertCode(t, err, codes.Unauthenticated)
}

// --- DeleteUser tests ---

func TestDeleteUser_Success(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "pass12345678")

	ctx := adminCtx()
	resp, err := env.handler.DeleteUser(ctx, &eveyv1.DeleteUserRequest{Id: "usr-1"})
	if err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// --- DisableUser tests ---

func TestDisableUser_Success(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "pass12345678")

	ctx := adminCtx()
	resp, err := env.handler.DisableUser(ctx, &eveyv1.DisableUserRequest{Id: "usr-1", Disabled: true})
	if err != nil {
		t.Fatalf("DisableUser: %v", err)
	}
	if resp.Enabled {
		t.Fatal("expected user to be disabled")
	}
}

func TestDisableUser_EnableToggle(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "pass12345678")

	ctx := adminCtx()
	// Disable.
	if _, err := env.handler.DisableUser(ctx, &eveyv1.DisableUserRequest{Id: "usr-1", Disabled: true}); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	// Re-enable via the same RPC with Disabled=false.
	resp, err := env.handler.DisableUser(ctx, &eveyv1.DisableUserRequest{Id: "usr-1", Disabled: false})
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if !resp.Enabled {
		t.Fatal("expected user to be re-enabled")
	}
}

// --- UpdateProfile tests ---

func TestUpdateProfile_Success(t *testing.T) {
	env := setupUserHandler()
	env.profileRepo.Create(context.Background(), &user.Profile{
		ID: "prf-1", UserID: "usr-1", Name: "Default",
	})

	ctx := userCtx("usr-1", "prf-1")
	newName := "Updated Profile"
	resp, err := env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{
		Id:   "prf-1",
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if resp.Name != "Updated Profile" {
		t.Fatalf("got name %q, want Updated Profile", resp.Name)
	}
}

// Sending `preferences` REPLACES the whole map - it does not merge key by key,
// even though the verb is PATCH and every other field of this request is a
// partial update. A caller that writes {"theme": "amoled"} alone therefore
// erases the viewer's audio and subtitle languages. That is why every client
// merges before writing (themePreferences in @evey/store), and this test is
// what tells the next reader - and stops someone "fixing" the semantics by
// accident: omitting a key is how a client REMOVES one (the theme-plugin id).
// Reproduced against the running server on 2026-08-05.
func TestUpdateProfile_PreferencesReplaceTheWholeMap(t *testing.T) {
	env := setupUserHandler()
	env.profileRepo.Create(context.Background(), &user.Profile{
		ID: "prf-1", UserID: "usr-1", Name: "Default",
		Preferences: map[string]string{"audio_languages": "fr", "subtitle_languages": "fr"},
	})

	resp, err := env.handler.UpdateProfile(userCtx("usr-1", "prf-1"), &eveyv1.UpdateProfileRequest{
		Id:          "prf-1",
		Preferences: map[string]string{"theme": "amoled"},
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if _, still := resp.Preferences["audio_languages"]; still {
		t.Error("audio_languages survived a partial write: the semantics changed to merge - " +
			"if that is deliberate, give clients another way to REMOVE a preference key")
	}
	if resp.Preferences["theme"] != "amoled" {
		t.Errorf("theme = %q, want amoled", resp.Preferences["theme"])
	}
}

func TestUpdateProfile_ParentalChangeFromKidsProfileDenied(t *testing.T) {
	env := setupUserHandler()
	env.profileRepo.Create(context.Background(), &user.Profile{
		ID: "prf-kid", UserID: "usr-1", Name: "Kid", IsKids: true,
	})
	disable := false
	// The caller is ON the kids profile and tries to clear its kids flag in
	// place → must switch to a non-kids profile (which needs the PIN) first.
	_, err := env.handler.UpdateProfile(userCtx("usr-1", "prf-kid"), &eveyv1.UpdateProfileRequest{
		Id: "prf-kid", IsKids: &disable,
	})
	assertCode(t, err, codes.PermissionDenied)
}

func TestUpdateProfile_ParentalChangeFromAdultProfileAllowed(t *testing.T) {
	env := setupUserHandler()
	// Adult profile must carry a PIN once a kids profile exists (domain rule).
	env.profileRepo.Create(context.Background(), &user.Profile{
		ID: "prf-adult", UserID: "usr-1", Name: "Adult", IsDefault: true,
		Parental: &user.ParentalSettings{PINHash: "hashed-pin"},
	})
	maxAge := 6
	env.profileRepo.Create(context.Background(), &user.Profile{
		ID: "prf-kid", UserID: "usr-1", Name: "Kid", IsKids: true,
		Parental: &user.ParentalSettings{MaxAge: &maxAge},
	})
	newAge := int32(12)
	// Parent operating from the (non-kids) adult profile edits the kid's age cap.
	if _, err := env.handler.UpdateProfile(userCtx("usr-1", "prf-adult"), &eveyv1.UpdateProfileRequest{
		Id: "prf-kid", MaxAge: &newAge,
	}); err != nil {
		t.Fatalf("a caller on a non-kids profile must be allowed to edit parental: %v", err)
	}
}

func TestUpdateProfile_NonParentalChangeFromKidsProfileAllowed(t *testing.T) {
	env := setupUserHandler()
	env.profileRepo.Create(context.Background(), &user.Profile{
		ID: "prf-kid", UserID: "usr-1", Name: "Kid", IsKids: true,
	})
	newName := "Renamed"
	// Renaming touches no parental field → allowed even from the kids profile.
	if _, err := env.handler.UpdateProfile(userCtx("usr-1", "prf-kid"), &eveyv1.UpdateProfileRequest{
		Id: "prf-kid", Name: &newName,
	}); err != nil {
		t.Fatalf("a non-parental change must be allowed: %v", err)
	}
}

func TestUpdateProfile_PreservesUnreferencedFields(t *testing.T) {
	env := setupUserHandler()
	ctx := context.Background()
	maxAge := 12
	// Seed a profile with every optional field populated.
	existing := &user.Profile{
		ID: "prf-1", UserID: "usr-1", Name: "Main", IsDefault: true, IsKids: false,
		AudioLanguages:    []string{"en", "fr"},
		SubtitleLanguages: []string{"en"},
		Parental: &user.ParentalSettings{
			MaxAge:      &maxAge,
			BlockedTags: []string{"violence"},
			PINHash:     "hashed-pin",
			Enabled:     true,
		},
	}
	bitrate := 8000
	existing.MaxBitrateKbps = &bitrate
	_ = env.profileRepo.Create(ctx, existing)

	// Edit only the name.
	reqCtx := userCtx("usr-1", "prf-1")
	newName := "Renamed"
	_, err := env.handler.UpdateProfile(reqCtx, &eveyv1.UpdateProfileRequest{
		Id:   "prf-1",
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	// Re-read and assert every other field is intact.
	got, err := env.profileRepo.GetByID(ctx, "prf-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Renamed" {
		t.Fatalf("name = %q, want Renamed", got.Name)
	}
	if !got.IsDefault {
		t.Fatal("is_default was wiped")
	}
	if len(got.AudioLanguages) != 2 {
		t.Fatalf("audio_languages wiped: %v", got.AudioLanguages)
	}
	if len(got.SubtitleLanguages) != 1 {
		t.Fatalf("subtitle_languages wiped: %v", got.SubtitleLanguages)
	}
	if got.Parental == nil || got.Parental.PINHash != "hashed-pin" {
		t.Fatal("parental.PINHash wiped")
	}
	if got.Parental.MaxAge == nil || *got.Parental.MaxAge != 12 {
		t.Fatal("parental.MaxAge wiped")
	}
	if !got.Parental.Enabled {
		t.Fatal("parental.Enabled wiped")
	}
	if len(got.Parental.BlockedTags) != 1 {
		t.Fatalf("parental.BlockedTags wiped: %v", got.Parental.BlockedTags)
	}
	if got.MaxBitrateKbps == nil || *got.MaxBitrateKbps != 8000 {
		t.Fatal("max_bitrate_kbps wiped")
	}
}

func TestUpdateProfile_OtherProfileDenied(t *testing.T) {
	env := setupUserHandler()
	ctx := userCtx("usr-1", "prf-1")
	newName := "Hacked"
	_, err := env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{
		Id:   "prf-other",
		Name: &newName,
	})
	assertCode(t, err, codes.PermissionDenied)
}

// --- DeleteProfile tests ---

func TestDeleteProfile_Success(t *testing.T) {
	env := setupUserHandler()
	env.profileRepo.Create(context.Background(), &user.Profile{
		ID: "prf-1", UserID: "usr-1", Name: "Default",
	})

	ctx := userCtx("usr-1", "prf-1")
	resp, err := env.handler.DeleteProfile(ctx, &eveyv1.DeleteProfileRequest{Id: "prf-1"})
	if err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestDeleteProfile_OtherProfileDenied(t *testing.T) {
	env := setupUserHandler()
	// The caller's active profile must resolve: the kids gate fails closed.
	seedAdultProfile(t, env, "prf-1", "usr-1")
	ctx := userCtx("usr-1", "prf-1")
	_, err := env.handler.DeleteProfile(ctx, &eveyv1.DeleteProfileRequest{Id: "prf-other"})
	assertCode(t, err, codes.PermissionDenied)
}

// --- ListProfiles tests ---

func TestListProfiles_Success(t *testing.T) {
	env := setupUserHandler()
	env.profileRepo.Create(context.Background(), &user.Profile{ID: "prf-1", UserID: "usr-1", Name: "Main"})
	env.profileRepo.Create(context.Background(), &user.Profile{ID: "prf-2", UserID: "usr-1", Name: "Kids"})

	ctx := userCtx("usr-1", "prf-1")
	resp, err := env.handler.ListProfiles(ctx, &eveyv1.ListProfilesRequest{UserId: "usr-1"})
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(resp.Profiles) != 2 {
		t.Fatalf("got %d profiles, want 2", len(resp.Profiles))
	}
}

func TestListProfiles_NonAdminForcesOwnID(t *testing.T) {
	env := setupUserHandler()
	env.profileRepo.Create(context.Background(), &user.Profile{ID: "prf-1", UserID: "usr-1", Name: "Main"})

	ctx := userCtx("usr-1", "prf-1")
	resp, err := env.handler.ListProfiles(ctx, &eveyv1.ListProfilesRequest{UserId: "usr-other"})
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	for _, p := range resp.Profiles {
		if p.UserId != "usr-1" {
			t.Fatalf("non-admin should only see own profiles, got userId %q", p.UserId)
		}
	}
}

// --- ListAllProfiles tests ---

func TestListAllProfiles_AdminSeesEveryHousehold(t *testing.T) {
	env := setupUserHandler()
	env.profileRepo.Create(context.Background(), &user.Profile{ID: "prf-1", UserID: "usr-1", Name: "Main"})
	env.profileRepo.Create(context.Background(), &user.Profile{ID: "prf-2", UserID: "usr-1", Name: "Kids"})
	env.profileRepo.Create(context.Background(), &user.Profile{ID: "prf-3", UserID: "usr-2", Name: "Bob"})

	resp, err := env.handler.ListAllProfiles(adminCtx(), &eveyv1.ListAllProfilesRequest{})
	if err != nil {
		t.Fatalf("ListAllProfiles: %v", err)
	}
	if len(resp.Profiles) != 3 {
		t.Fatalf("got %d profiles, want 3", len(resp.Profiles))
	}
	// The owner travels with each profile: that is what lets the admin panel
	// group profiles under their user without a second call.
	owners := map[string]string{}
	for _, p := range resp.Profiles {
		owners[p.Id] = p.UserId
	}
	if owners["prf-3"] != "usr-2" {
		t.Fatalf("prf-3 owner = %q, want usr-2", owners["prf-3"])
	}
}

func TestListAllProfiles_NonAdminDenied(t *testing.T) {
	env := setupUserHandler()
	env.profileRepo.Create(context.Background(), &user.Profile{ID: "prf-1", UserID: "usr-1", Name: "Main"})

	_, err := env.handler.ListAllProfiles(userCtx("usr-1", "prf-1"), &eveyv1.ListAllProfilesRequest{})
	assertCode(t, err, codes.PermissionDenied)
}

// --- InviteLink tests ---

func TestCreateInviteLink_Success(t *testing.T) {
	env := setupUserHandler()
	ctx := adminCtx()

	resp, err := env.handler.CreateInviteLink(ctx, &eveyv1.CreateInviteLinkRequest{MaxUses: 5})
	if err != nil {
		t.Fatalf("CreateInviteLink: %v", err)
	}
	if resp.Id == "" {
		t.Fatal("expected non-empty invite link id")
	}
	if resp.Code == "" {
		t.Fatal("expected non-empty invite code")
	}
	if resp.MaxUses != 5 {
		t.Fatalf("got max_uses %d, want 5", resp.MaxUses)
	}
}

func TestCreateInviteLink_Unauthenticated(t *testing.T) {
	env := setupUserHandler()
	_, err := env.handler.CreateInviteLink(context.Background(), &eveyv1.CreateInviteLinkRequest{})
	assertCode(t, err, codes.Unauthenticated)
}

func TestCreateInviteLink_NonAdminDenied(t *testing.T) {
	env := setupUserHandler()
	ctx := userCtx("usr-1", "prf-1")
	_, err := env.handler.CreateInviteLink(ctx, &eveyv1.CreateInviteLinkRequest{MaxUses: 1})
	assertCode(t, err, codes.PermissionDenied)
}

func TestListInviteLinks_Success(t *testing.T) {
	env := setupUserHandler()
	ctx := adminCtx()

	_, _ = env.handler.CreateInviteLink(ctx, &eveyv1.CreateInviteLinkRequest{MaxUses: 1})
	_, _ = env.handler.CreateInviteLink(ctx, &eveyv1.CreateInviteLinkRequest{MaxUses: 2})

	resp, err := env.handler.ListInviteLinks(ctx, &eveyv1.ListInviteLinksRequest{})
	if err != nil {
		t.Fatalf("ListInviteLinks: %v", err)
	}
	if len(resp.InviteLinks) != 2 {
		t.Fatalf("got %d invite links, want 2", len(resp.InviteLinks))
	}
}

func TestRevokeInviteLink_OwnerSuccess(t *testing.T) {
	env := setupUserHandler()
	ctx := adminCtx()

	created, _ := env.handler.CreateInviteLink(ctx, &eveyv1.CreateInviteLinkRequest{MaxUses: 1})
	_, err := env.handler.RevokeInviteLink(ctx, &eveyv1.RevokeInviteLinkRequest{Id: created.Id})
	if err != nil {
		t.Fatalf("RevokeInviteLink: %v", err)
	}
}

func TestRevokeInviteLink_NonOwnerDenied(t *testing.T) {
	env := setupUserHandler()
	ctxOwner := adminCtx()
	created, _ := env.handler.CreateInviteLink(ctxOwner, &eveyv1.CreateInviteLinkRequest{MaxUses: 1})

	ctxOther := userCtx("usr-2", "prf-2")
	_, err := env.handler.RevokeInviteLink(ctxOther, &eveyv1.RevokeInviteLinkRequest{Id: created.Id})
	assertCode(t, err, codes.PermissionDenied)
}

// --- RegisterWithInvite tests ---

func TestRegisterWithInvite_Success(t *testing.T) {
	env := setupUserHandler()
	ctx := adminCtx()
	created, _ := env.handler.CreateInviteLink(ctx, &eveyv1.CreateInviteLinkRequest{MaxUses: 1})

	resp, err := env.handler.RegisterWithInvite(context.Background(), &eveyv1.RegisterWithInviteRequest{
		InviteCode: created.Code,
		Username:   "newuser",
		Password:   "newpassword123",
		Email:      "new@test.com",
	})
	if err != nil {
		t.Fatalf("RegisterWithInvite: %v", err)
	}
	if resp.User == nil || resp.User.Username != "newuser" {
		t.Fatal("expected user in response")
	}
}

func TestRegisterWithInvite_InvalidCode(t *testing.T) {
	env := setupUserHandler()
	_, err := env.handler.RegisterWithInvite(context.Background(), &eveyv1.RegisterWithInviteRequest{
		InviteCode: "invalid-code",
		Username:   "newuser",
		Password:   "newpassword123",
	})
	assertCode(t, err, codes.InvalidArgument)
}

// --- RevokeAPIKey tests ---

func TestRevokeAPIKey_OwnerSuccess(t *testing.T) {
	env := setupUserHandler()
	ctx := userCtx("usr-1", "prf-1")

	created, _ := env.handler.CreateAPIKey(ctx, &eveyv1.CreateAPIKeyRequest{Name: "my-key", Scopes: []string{"media:read"}})
	_, err := env.handler.RevokeAPIKey(ctx, &eveyv1.RevokeAPIKeyRequest{Id: created.Id})
	if err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}

	resp, _ := env.handler.ListAPIKeys(ctx, &eveyv1.ListAPIKeysRequest{})
	if len(resp.ApiKeys) != 0 {
		t.Fatalf("expected 0 keys after revoke, got %d", len(resp.ApiKeys))
	}
}

func TestRevokeAPIKey_NonOwnerDenied(t *testing.T) {
	env := setupUserHandler()
	ctxOwner := userCtx("usr-1", "prf-1")
	created, _ := env.handler.CreateAPIKey(ctxOwner, &eveyv1.CreateAPIKeyRequest{Name: "key1", Scopes: []string{"media:read"}})

	ctxOther := userCtx("usr-2", "prf-2")
	_, err := env.handler.RevokeAPIKey(ctxOther, &eveyv1.RevokeAPIKeyRequest{Id: created.Id})
	assertCode(t, err, codes.PermissionDenied)
}

// A kids profile must not be able to create profiles. UpdateProfile already
// refuses parental changes from a kids profile (enforceParentalUpdateGate),
// and its comment states the intended escape hatch: "switch to a non-kids
// profile, which already requires the parental PIN". RefreshToken's switch
// gate rests on the same assumption - "a kid cannot know the adult profile's
// PIN".
//
// CreateProfile had no such gate, which broke both assumptions at once: the
// kid creates a fresh non-kids profile WITH A PIN IT CHOOSES, proves that PIN,
// and switches onto an unrestricted profile. Parental control is neutralised
// on the whole account without ever knowing the parent's PIN.
func TestCreateProfile_FromKidsProfileDenied(t *testing.T) {
	env := setupUserHandler()
	ctx := context.Background()

	kid, err := user.NewProfile("prf-kid", "usr-1", "Kid", user.WithProfileIsKids(true))
	if err != nil {
		t.Fatalf("NewProfile: %v", err)
	}
	if cErr := env.profileRepo.Create(ctx, kid); cErr != nil {
		t.Fatalf("seed kid profile: %v", cErr)
	}

	kidPIN := "0000"
	_, err = env.handler.CreateProfile(userCtx("usr-1", string(kid.ID)), &eveyv1.CreateProfileRequest{
		UserId: "usr-1",
		Name:   "Free",
		IsKids: false,
		Pin:    &kidPIN,
	})
	assertCode(t, err, codes.PermissionDenied)
}

func TestCreateProfile_FromAdultProfileStillAllowed(t *testing.T) {
	env := setupUserHandler()
	ctx := context.Background()

	// The sibling invariant requires a non-kids profile to carry a PIN once a
	// kids profile exists, so give the parent one up front.
	adult, err := user.NewProfile("prf-adult", "usr-1", "Parent",
		user.WithProfileParental(&user.ParentalSettings{PINHash: "hashed"}))
	if err != nil {
		t.Fatalf("NewProfile: %v", err)
	}
	if err := env.profileRepo.Create(ctx, adult); err != nil {
		t.Fatalf("seed adult profile: %v", err)
	}

	if _, err := env.handler.CreateProfile(userCtx("usr-1", string(adult.ID)), &eveyv1.CreateProfileRequest{
		UserId: "usr-1",
		Name:   "Kids",
		IsKids: true,
	}); err != nil {
		t.Fatalf("an unrestricted profile must still be able to create profiles: %v", err)
	}
}

// Same class as CreateProfile: profile management is an adult action. Deleting
// a sibling grants a kid no escape (the switch gate still holds), but letting a
// kids profile wipe the family's profiles is plainly not intended, and the
// ownership check alone allows it.
func TestDeleteProfile_FromKidsProfileDenied(t *testing.T) {
	env := setupUserHandler()
	ctx := context.Background()

	kid, err := user.NewProfile("prf-kid-del", "usr-1", "Kid", user.WithProfileIsKids(true))
	if err != nil {
		t.Fatalf("NewProfile: %v", err)
	}
	if cErr := env.profileRepo.Create(ctx, kid); cErr != nil {
		t.Fatalf("seed kid profile: %v", cErr)
	}
	sibling := seedAdultProfile(t, env, "prf-sibling", "usr-1")

	_, err = env.handler.DeleteProfile(userCtx("usr-1", string(kid.ID)), &eveyv1.DeleteProfileRequest{
		Id: string(sibling.ID),
	})
	assertCode(t, err, codes.PermissionDenied)
}

// The optional fields of a new profile land as given (avatar under avatars/,
// region upper-cased, PIN hashed and max age kept as parental settings), an
// escaping avatar path and a malformed PIN are refused before anything is
// written, and the account's profile cap answers FailedPrecondition.
func TestCreateProfile_OptionsAndRefusals(t *testing.T) {
	env := setupUserHandler()
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice"}, "pass12345678")
	seedAdultProfile(t, env, "prf-1", "usr-1")
	ctx := userCtx("usr-1", "prf-1")

	pin, region, maxAge := "1234", "fr", int32(12)
	resp, err := env.handler.CreateProfile(ctx, &eveyv1.CreateProfileRequest{
		UserId: "usr-1", Name: "Junior", AvatarPath: "avatars/kid.png", MaxRating: "PG", Region: &region, Pin: &pin, MaxAge: &maxAge, IsKids: false,
	})
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	stored, err := env.profileRepo.GetByID(context.Background(), shared.ProfileID(resp.Id))
	if err != nil {
		t.Fatal(err)
	}
	if stored.AvatarPath == nil || *stored.AvatarPath != "avatars/kid.png" || stored.Region == nil || *stored.Region != "FR" {
		t.Errorf("avatar/region: %+v", stored)
	}
	if stored.Parental == nil || stored.Parental.PINHash == "" || stored.Parental.PINHash == "1234" || stored.Parental.MaxAge == nil || *stored.Parental.MaxAge != 12 {
		t.Errorf("parental: %+v", stored.Parental)
	}

	_, err = env.handler.CreateProfile(ctx, &eveyv1.CreateProfileRequest{UserId: "usr-1", Name: "Sneaky", AvatarPath: "avatars/../../etc/passwd"})
	assertCode(t, err, codes.InvalidArgument)
	_, err = env.handler.CreateProfile(ctx, &eveyv1.CreateProfileRequest{UserId: "usr-1", Name: "Elsewhere", AvatarPath: "/tmp/x.png"})
	assertCode(t, err, codes.InvalidArgument)
	badPIN := "12"
	_, err = env.handler.CreateProfile(ctx, &eveyv1.CreateProfileRequest{UserId: "usr-1", Name: "BadPIN", Pin: &badPIN})
	assertCode(t, err, codes.InvalidArgument)
	_, err = env.handler.CreateProfile(ctx, &eveyv1.CreateProfileRequest{UserId: "usr-1", Name: ""})
	assertCode(t, err, codes.InvalidArgument)
	all, _ := env.profileRepo.GetByUserID(context.Background(), "usr-1")
	if len(all) != 2 {
		t.Errorf("refused creations must write nothing: %d profiles", len(all))
	}

	// The account's cap: usr-1 already has two profiles.
	cap := 2
	u, err := env.userRepo.GetByID(context.Background(), "usr-1")
	if err != nil {
		t.Fatal(err)
	}
	u.MaxProfiles = &cap
	if updErr := env.userRepo.Update(context.Background(), u); updErr != nil {
		t.Fatal(updErr)
	}
	_, err = env.handler.CreateProfile(ctx, &eveyv1.CreateProfileRequest{UserId: "usr-1", Name: "Third"})
	assertCode(t, err, codes.FailedPrecondition)
}

// The patch semantics of the remaining profile fields: an empty region
// clears it, a bitrate cap rides the preferences map and is lifted out of
// it (0 clears it), a negative max age clears the age cap, a new PIN is
// hashed, an escaping avatar or a malformed PIN is refused with nothing
// persisted, and removing the guardian PIN next to a kids profile is a
// FailedPrecondition.
func TestUpdateProfile_FieldSemantics(t *testing.T) {
	env := setupUserHandler()
	region := "fr"
	age := 12
	require.NoError(t, env.profileRepo.Create(context.Background(), &user.Profile{
		ID: "prf-1", UserID: "usr-1", Name: "Parent", Region: &region,
		Parental: &user.ParentalSettings{MaxAge: &age, PINHash: "hashed:1111"},
	}))
	ctx := userCtx("usr-1", "prf-1")
	get := func() *user.Profile {
		p, err := env.profileRepo.GetByID(context.Background(), "prf-1")
		require.NoError(t, err)
		return p
	}

	empty, pin, negAge := "", "4321", int32(-1)
	_, err := env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{
		Id: "prf-1", Region: &empty, Pin: &pin, MaxAge: &negAge,
		Preferences: map[string]string{"theme": "dark", "max_bitrate_kbps": "8000"},
	})
	require.NoError(t, err)
	p := get()
	assert.Nil(t, p.Region, "an empty region clears it")
	assert.Nil(t, p.Parental.MaxAge, "a negative max age clears the cap")
	assert.NotEqual(t, "hashed:1111", p.Parental.PINHash)
	assert.NotEqual(t, "4321", p.Parental.PINHash, "the PIN is hashed")
	require.NotNil(t, p.MaxBitrateKbps)
	assert.Equal(t, 8000, *p.MaxBitrateKbps)
	assert.NotContains(t, p.Preferences, "max_bitrate_kbps", "the cap is lifted out of the map")
	assert.Equal(t, "dark", p.Preferences["theme"])

	_, err = env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", Preferences: map[string]string{"max_bitrate_kbps": "0"}})
	require.NoError(t, err)
	assert.Nil(t, get().MaxBitrateKbps, "0 lifts the cap")

	bad := "avatars/../secret"
	_, err = env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", AvatarPath: &bad})
	assertCode(t, err, codes.InvalidArgument)
	badPIN := "12ab"
	_, err = env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", Pin: &badPIN})
	assertCode(t, err, codes.InvalidArgument)
	assert.Nil(t, get().AvatarPath, "refused patches write nothing")

	// A kids sibling appears: the guardian must keep a PIN.
	require.NoError(t, env.profileRepo.Create(context.Background(), &user.Profile{ID: "prf-kid", UserID: "usr-1", Name: "Kid", IsKids: true}))
	noPIN := ""
	_, err = env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-1", Pin: &noPIN})
	assertCode(t, err, codes.FailedPrecondition)
	assert.NotEmpty(t, get().Parental.PINHash, "the refused write left the PIN in place")
}

// Revoking a key or an invite that is already gone is NotFound for the admin
// (a viewer never learns whether another user's id exists: PermissionDenied
// comes first for them).
func TestRevokeAPIKeyAndInvite_UnknownIsNotFound(t *testing.T) {
	env := setupUserHandler()
	ctx := adminCtx()
	_, err := env.handler.RevokeAPIKey(ctx, &eveyv1.RevokeAPIKeyRequest{Id: "ghost"})
	assertCode(t, err, codes.NotFound)
	_, err = env.handler.RevokeInviteLink(ctx, &eveyv1.RevokeInviteLinkRequest{Id: "ghost"})
	assertCode(t, err, codes.NotFound)

	_, err = env.handler.RevokeAPIKey(userCtx("u-viewer", "p-viewer"), &eveyv1.RevokeAPIKeyRequest{Id: "ghost"})
	assertCode(t, err, codes.PermissionDenied)
	_, err = env.handler.RevokeInviteLink(userCtx("u-viewer", "p-viewer"), &eveyv1.RevokeInviteLinkRequest{Id: "ghost"})
	assertCode(t, err, codes.PermissionDenied)
}
