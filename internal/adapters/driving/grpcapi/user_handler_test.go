package grpcapi

import (
	"context"
	"strings"
	"testing"
	"time"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	"github.com/gravity-zero/evey/internal/config"
	activityfakes "github.com/gravity-zero/evey/internal/domain/activity/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/gravity-zero/evey/internal/domain/user"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
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
	userSvc := appuser.NewUserService(userRepo, profileRepo, refreshRepo, hasher, events)
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
		users:       userRepo,
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

// --- CreateProfile tests ---

func TestCreateProfile_Success(t *testing.T) {
	env := setupUserHandler()
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
		Name: "my-key",
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

func TestListAPIKeys_Success(t *testing.T) {
	env := setupUserHandler()
	ctx := userCtx("usr-1", "prf-1")

	_, _ = env.handler.CreateAPIKey(ctx, &eveyv1.CreateAPIKeyRequest{Name: "key1"})
	_, _ = env.handler.CreateAPIKey(ctx, &eveyv1.CreateAPIKeyRequest{Name: "key2"})

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

	// Check before authorize — should not be authorized
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

	// Check after authorize — should be authorized
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
	resp, err := env.handler.DisableUser(ctx, &eveyv1.DisableUserRequest{Id: "usr-1"})
	if err != nil {
		t.Fatalf("DisableUser: %v", err)
	}
	if resp.Enabled {
		t.Fatal("expected user to be disabled")
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

// --- InviteLink tests ---

func TestCreateInviteLink_Success(t *testing.T) {
	env := setupUserHandler()
	ctx := userCtx("usr-1", "prf-1")

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

func TestListInviteLinks_Success(t *testing.T) {
	env := setupUserHandler()
	ctx := userCtx("usr-1", "prf-1")

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
	ctx := userCtx("usr-1", "prf-1")

	created, _ := env.handler.CreateInviteLink(ctx, &eveyv1.CreateInviteLinkRequest{MaxUses: 1})
	_, err := env.handler.RevokeInviteLink(ctx, &eveyv1.RevokeInviteLinkRequest{Id: created.Id})
	if err != nil {
		t.Fatalf("RevokeInviteLink: %v", err)
	}
}

func TestRevokeInviteLink_NonOwnerDenied(t *testing.T) {
	env := setupUserHandler()
	ctxOwner := userCtx("usr-1", "prf-1")
	created, _ := env.handler.CreateInviteLink(ctxOwner, &eveyv1.CreateInviteLinkRequest{MaxUses: 1})

	ctxOther := userCtx("usr-2", "prf-2")
	_, err := env.handler.RevokeInviteLink(ctxOther, &eveyv1.RevokeInviteLinkRequest{Id: created.Id})
	assertCode(t, err, codes.PermissionDenied)
}

// --- RegisterWithInvite tests ---

func TestRegisterWithInvite_Success(t *testing.T) {
	env := setupUserHandler()
	ctx := userCtx("usr-1", "prf-1")
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

	created, _ := env.handler.CreateAPIKey(ctx, &eveyv1.CreateAPIKeyRequest{Name: "my-key"})
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
	created, _ := env.handler.CreateAPIKey(ctxOwner, &eveyv1.CreateAPIKeyRequest{Name: "key1"})

	ctxOther := userCtx("usr-2", "prf-2")
	_, err := env.handler.RevokeAPIKey(ctxOther, &eveyv1.RevokeAPIKeyRequest{Id: created.Id})
	assertCode(t, err, codes.PermissionDenied)
}
