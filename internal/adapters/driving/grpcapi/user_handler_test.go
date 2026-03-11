package grpcapi

import (
	"context"
	"strings"
	"testing"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	activityfakes "github.com/gravity-zero/evey/internal/domain/activity/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/gravity-zero/evey/internal/domain/user"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type testUserEnv struct {
	handler      *userHandler
	userRepo     *userfakes.FakeUserRepository
	profileRepo  *userfakes.FakeProfileRepository
	svc          *appuser.Service
	events       *sharedfakes.FakeEventBus
	tokenGen     *userfakes.FakeTokenGenerator
	hasher       *userfakes.FakePasswordHasher
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

	svc := appuser.NewService(
		userRepo, profileRepo, refreshRepo, apiKeyRepo,
		inviteRepo, qcRepo, roleRepo, userRoleRepo,
		hasher, tokenGen, events,
	)

	actSvc := appactivity.NewService(
		activityfakes.NewFakeLoginHistoryRepository(),
		activityfakes.NewFakeServerMetricRepository(),
		activityfakes.NewFakeAuditLogRepository(),
		activityfakes.NewFakePlaySessionStatsReader(),
		activityfakes.NewFakePlayHistoryRepository(),
		events,
	)

	h := &userHandler{
		svc:         svc,
		users:       userRepo,
		activitySvc: actSvc,
	}

	return &testUserEnv{
		handler:     h,
		userRepo:    userRepo,
		profileRepo: profileRepo,
		svc:         svc,
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
	// seed a default profile so the login token gets a profile ID
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
	// need at least one admin in repo for delete checks
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
}

func TestCheckQuickConnect_InvalidCode(t *testing.T) {
	env := setupUserHandler()
	_, err := env.handler.CheckQuickConnect(context.Background(), &eveyv1.CheckQuickConnectRequest{Code: "bogus"})
	assertCode(t, err, codes.NotFound)
}
