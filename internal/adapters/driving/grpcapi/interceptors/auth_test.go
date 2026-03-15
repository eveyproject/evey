package interceptors_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/domain/user/fakes"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func setupInterceptor() (*interceptors.AuthInterceptor, *fakes.FakeTokenGenerator) {
	tokenGen := fakes.NewFakeTokenGenerator()
	return interceptors.NewAuthInterceptor(tokenGen), tokenGen
}

func apiKeyHash(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func ctxWithAPIKey(key string) context.Context {
	md := metadata.Pairs("authorization", key)
	return metadata.NewIncomingContext(context.Background(), md)
}

func ctxWithToken(token string) context.Context {
	md := metadata.Pairs("authorization", "Bearer "+token)
	return metadata.NewIncomingContext(context.Background(), md)
}

var nopHandler = func(ctx context.Context, req any) (any, error) {
	return ctx, nil
}

func TestAuthInterceptor_PublicMethod(t *testing.T) {
	ai, _ := setupInterceptor()
	interceptor := ai.Unary()

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.UserService/Login"}
	resp, err := interceptor(context.Background(), nil, info, nopHandler)

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestAuthInterceptor_RequiresToken(t *testing.T) {
	ai, _ := setupInterceptor()
	interceptor := ai.Unary()

	tests := []struct {
		name string
		ctx  context.Context
	}{
		{"no metadata", context.Background()},
		{"empty authorization", metadata.NewIncomingContext(
			context.Background(), metadata.Pairs("other", "val"),
		)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.LibraryService/ListLibraries"}
			_, err := interceptor(tt.ctx, nil, info, nopHandler)

			require.Error(t, err)
			assert.Equal(t, codes.Unauthenticated, status.Code(err))
		})
	}
}

func TestAuthInterceptor_ExpiredToken(t *testing.T) {
	ai, _ := setupInterceptor()
	interceptor := ai.Unary()

	ctx := ctxWithToken("invalid-token-that-doesnt-exist")
	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.LibraryService/ListLibraries"}

	_, err := interceptor(ctx, nil, info, nopHandler)
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestAuthInterceptor_AdminOnly_NonAdmin(t *testing.T) {
	ai, tokenGen := setupInterceptor()
	interceptor := ai.Unary()

	token, _ := tokenGen.GenerateAccessToken(user.Claims{
		UserID:      "user-1",
		Username:    "viewer",
		IsAdmin:     false,
		Permissions: []string{auth.PermMediaRead},
	})

	adminHandler := func(ctx context.Context, req any) (any, error) {
		claims := interceptors.ClaimsFromContext(ctx)
		if claims == nil || !auth.CheckPermission(claims.Permissions, auth.PermServerAdmin) {
			return nil, status.Error(codes.PermissionDenied, "admin only")
		}
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.AdminService/GetAuditLog"}
	_, err := interceptor(ctxWithToken(token), nil, info, adminHandler)

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestAuthInterceptor_AdminOnly_Admin(t *testing.T) {
	ai, tokenGen := setupInterceptor()
	interceptor := ai.Unary()

	token, _ := tokenGen.GenerateAccessToken(user.Claims{
		UserID:      "admin-1",
		Username:    "admin",
		IsAdmin:     true,
		Permissions: []string{auth.PermServerAdmin},
	})

	adminHandler := func(ctx context.Context, req any) (any, error) {
		claims := interceptors.ClaimsFromContext(ctx)
		if claims == nil || !auth.CheckPermission(claims.Permissions, auth.PermServerAdmin) {
			return nil, status.Error(codes.PermissionDenied, "admin only")
		}
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.AdminService/GetAuditLog"}
	resp, err := interceptor(ctxWithToken(token), nil, info, adminHandler)

	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
}

func TestAuthInterceptor_APIKey_Success(t *testing.T) {
	tokenGen := fakes.NewFakeTokenGenerator()
	apiKeyRepo := fakes.NewFakeAPIKeyRepository()
	userRepo := fakes.NewFakeUserRepository()

	rawKey := "evey_testkey123"
	apiKeyRepo.Create(context.Background(), &user.APIKeyModel{
		ID:      "ak-1",
		UserID:  "usr-1",
		KeyHash: apiKeyHash(rawKey),
		Name:    "test",
	})
	userRepo.Create(context.Background(), &user.User{
		ID:       "usr-1",
		Username: "alice",
		IsAdmin:  false,
		Enabled:  true,
	})

	ai := interceptors.NewAuthInterceptor(tokenGen).WithAPIKeys(apiKeyRepo, userRepo)
	interceptor := ai.Unary()

	extractHandler := func(ctx context.Context, req any) (any, error) {
		return interceptors.ClaimsFromContext(ctx), nil
	}
	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.LibraryService/ListLibraries"}
	resp, err := interceptor(ctxWithAPIKey(rawKey), nil, info, extractHandler)

	require.NoError(t, err)
	claims := resp.(*user.Claims)
	assert.Equal(t, "usr-1", claims.UserID)
	assert.Equal(t, "alice", claims.Username)
}

func TestAuthInterceptor_APIKey_Invalid(t *testing.T) {
	tokenGen := fakes.NewFakeTokenGenerator()
	apiKeyRepo := fakes.NewFakeAPIKeyRepository()
	userRepo := fakes.NewFakeUserRepository()

	ai := interceptors.NewAuthInterceptor(tokenGen).WithAPIKeys(apiKeyRepo, userRepo)
	interceptor := ai.Unary()

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.LibraryService/ListLibraries"}
	_, err := interceptor(ctxWithAPIKey("evey_nonexistent"), nil, info, nopHandler)

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestAuthInterceptor_APIKey_Expired(t *testing.T) {
	tokenGen := fakes.NewFakeTokenGenerator()
	apiKeyRepo := fakes.NewFakeAPIKeyRepository()
	userRepo := fakes.NewFakeUserRepository()

	rawKey := "evey_expiredkey"
	past := time.Now().Add(-time.Hour)
	apiKeyRepo.Create(context.Background(), &user.APIKeyModel{
		ID:        "ak-exp",
		UserID:    "usr-1",
		KeyHash:   apiKeyHash(rawKey),
		Name:      "expired",
		ExpiresAt: &past,
	})
	userRepo.Create(context.Background(), &user.User{
		ID:      "usr-1",
		Enabled: true,
	})

	ai := interceptors.NewAuthInterceptor(tokenGen).WithAPIKeys(apiKeyRepo, userRepo)
	interceptor := ai.Unary()

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.LibraryService/ListLibraries"}
	_, err := interceptor(ctxWithAPIKey(rawKey), nil, info, nopHandler)

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestAuthInterceptor_CustomRBACConfig(t *testing.T) {
	tokenGen := fakes.NewFakeTokenGenerator()
	rbac := interceptors.RBACConfig{
		PublicMethods: map[string]bool{"/test/Public": true},
		MethodRoles:   map[string]interceptors.Role{"/test/Admin": interceptors.RoleAdmin},
	}
	ai := interceptors.NewAuthInterceptorWithRBAC(tokenGen, rbac)
	unary := ai.Unary()

	t.Run("custom public method skips auth", func(t *testing.T) {
		info := &grpc.UnaryServerInfo{FullMethod: "/test/Public"}
		resp, err := unary(context.Background(), nil, info, nopHandler)
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("custom admin method blocks non-admin", func(t *testing.T) {
		token, _ := tokenGen.GenerateAccessToken(user.Claims{
			UserID: "u1", Username: "user", IsAdmin: false,
		})
		info := &grpc.UnaryServerInfo{FullMethod: "/test/Admin"}
		_, err := unary(ctxWithToken(token), nil, info, nopHandler)
		require.Error(t, err)
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("custom admin method allows admin", func(t *testing.T) {
		token, _ := tokenGen.GenerateAccessToken(user.Claims{
			UserID: "a1", Username: "admin", IsAdmin: true,
		})
		info := &grpc.UnaryServerInfo{FullMethod: "/test/Admin"}
		_, err := unary(ctxWithToken(token), nil, info, nopHandler)
		require.NoError(t, err)
	})

	t.Run("default admin method NOT blocked with custom config", func(t *testing.T) {
		token, _ := tokenGen.GenerateAccessToken(user.Claims{
			UserID: "u2", Username: "user", IsAdmin: false,
		})
		info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.AdminService/GetAuditLog"}
		_, err := unary(ctxWithToken(token), nil, info, nopHandler)
		require.NoError(t, err)
	})
}

func TestEnforceRBAC_ContextClaims(t *testing.T) {
	ai, tokenGen := setupInterceptor()
	interceptor := ai.Unary()

	claims := user.Claims{
		UserID:      "user-42",
		Username:    "testuser",
		IsAdmin:     true,
		ProfileID:   "profile-7",
		Permissions: []string{auth.PermLibraryRead, auth.PermMediaRead},
	}
	token, _ := tokenGen.GenerateAccessToken(claims)

	extractHandler := func(ctx context.Context, req any) (any, error) {
		return interceptors.ClaimsFromContext(ctx), nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.LibraryService/ListLibraries"}
	resp, err := interceptor(ctxWithToken(token), nil, info, extractHandler)

	require.NoError(t, err)
	got := resp.(*user.Claims)
	assert.Equal(t, "user-42", got.UserID)
	assert.Equal(t, "testuser", got.Username)
	assert.True(t, got.IsAdmin)
	assert.Equal(t, "profile-7", got.ProfileID)
	assert.ElementsMatch(t, []string{auth.PermLibraryRead, auth.PermMediaRead}, got.Permissions)
}
