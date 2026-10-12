package interceptors_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/domain/user/fakes"
	"github.com/eveyproject/evey/internal/pkg/auth"
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
		if claims == nil || !claims.IsAdmin {
			return nil, status.Error(codes.PermissionDenied, "admin only")
		}
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.AdminSessionService/GetAuditLog"}
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
		if claims == nil || !claims.IsAdmin {
			return nil, status.Error(codes.PermissionDenied, "admin only")
		}
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.AdminSessionService/GetAuditLog"}
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
		ID:          "ak-1",
		UserID:      "usr-1",
		KeyHash:     apiKeyHash(rawKey),
		Name:        "test",
		Permissions: []string{auth.PermLibraryRead},
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
	ai := interceptors.NewAuthInterceptor(tokenGen).WithRBAC(rbac)
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
		info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.AdminSessionService/GetAuditLog"}
		_, err := unary(ctxWithToken(token), nil, info, nopHandler)
		require.NoError(t, err)
	})
}

func TestContextWithClaims(t *testing.T) {
	claims := &user.Claims{UserID: "u1", Username: "alice", IsAdmin: true}
	ctx := interceptors.ContextWithClaims(context.Background(), claims)

	got := interceptors.ClaimsFromContext(ctx)
	require.NotNil(t, got)
	assert.Equal(t, "u1", got.UserID)
	assert.Equal(t, "alice", got.Username)
	assert.True(t, got.IsAdmin)
}

func TestClaimsFromContext_NilWhenEmpty(t *testing.T) {
	got := interceptors.ClaimsFromContext(context.Background())
	assert.Nil(t, got)
}

func TestAuthInterceptor_Stream_PublicMethod(t *testing.T) {
	ai, _ := setupInterceptor()
	interceptor := ai.Stream()

	info := &grpc.StreamServerInfo{FullMethod: "/evey.v1.UserService/Login"}
	err := interceptor(nil, &fakeServerStream{ctx: context.Background()}, info, func(srv any, stream grpc.ServerStream) error {
		return nil
	})

	require.NoError(t, err)
}

func TestAuthInterceptor_Stream_RequiresToken(t *testing.T) {
	ai, _ := setupInterceptor()
	interceptor := ai.Stream()

	info := &grpc.StreamServerInfo{FullMethod: "/evey.v1.LibraryService/ListLibraries"}
	err := interceptor(nil, &fakeServerStream{ctx: context.Background()}, info, func(srv any, stream grpc.ServerStream) error {
		return nil
	})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestAuthInterceptor_Stream_ValidToken(t *testing.T) {
	ai, tokenGen := setupInterceptor()
	interceptor := ai.Stream()

	token, _ := tokenGen.GenerateAccessToken(user.Claims{
		UserID: "u1", Username: "alice", IsAdmin: false,
	})

	md := metadata.Pairs("authorization", "Bearer "+token)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var captured context.Context
	info := &grpc.StreamServerInfo{FullMethod: "/evey.v1.LibraryService/ListLibraries"}
	err := interceptor(nil, &fakeServerStream{ctx: ctx}, info, func(srv any, stream grpc.ServerStream) error {
		captured = stream.Context()
		return nil
	})

	require.NoError(t, err)
	claims := interceptors.ClaimsFromContext(captured)
	require.NotNil(t, claims)
	assert.Equal(t, "u1", claims.UserID)
}

func TestAuthInterceptor_Stream_AdminBlocked(t *testing.T) {
	ai, tokenGen := setupInterceptor()
	interceptor := ai.Stream()

	token, _ := tokenGen.GenerateAccessToken(user.Claims{
		UserID: "u1", Username: "viewer", IsAdmin: false,
	})

	md := metadata.Pairs("authorization", "Bearer "+token)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	info := &grpc.StreamServerInfo{FullMethod: "/evey.v1.AdminSessionService/GetAuditLog"}
	err := interceptor(nil, &fakeServerStream{ctx: ctx}, info, func(srv any, stream grpc.ServerStream) error {
		return nil
	})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestAuthInterceptor_APIKey_DisabledUser(t *testing.T) {
	tokenGen := fakes.NewFakeTokenGenerator()
	apiKeyRepo := fakes.NewFakeAPIKeyRepository()
	userRepo := fakes.NewFakeUserRepository()

	rawKey := "evey_disabledkey"
	apiKeyRepo.Create(context.Background(), &user.APIKeyModel{
		ID:      "ak-dis",
		UserID:  "usr-dis",
		KeyHash: apiKeyHash(rawKey),
		Name:    "disabled",
	})
	userRepo.Create(context.Background(), &user.User{
		ID:       "usr-dis",
		Username: "disabled-user",
		Enabled:  false,
	})

	ai := interceptors.NewAuthInterceptor(tokenGen).WithAPIKeys(apiKeyRepo, userRepo)
	interceptor := ai.Unary()

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.LibraryService/ListLibraries"}
	_, err := interceptor(ctxWithAPIKey(rawKey), nil, info, nopHandler)

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestAuthInterceptor_WithTranslator(t *testing.T) {
	tokenGen := fakes.NewFakeTokenGenerator()
	ai := interceptors.NewAuthInterceptor(tokenGen).WithTranslator(nil)
	interceptor := ai.Unary()

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.LibraryService/ListLibraries"}
	_, err := interceptor(context.Background(), nil, info, nopHandler)

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestAuthInterceptor_APIKey_WithBearerPrefix(t *testing.T) {
	tokenGen := fakes.NewFakeTokenGenerator()
	apiKeyRepo := fakes.NewFakeAPIKeyRepository()
	userRepo := fakes.NewFakeUserRepository()

	rawKey := "evey_bearertest"
	apiKeyRepo.Create(context.Background(), &user.APIKeyModel{
		ID:          "ak-b",
		UserID:      "usr-b",
		KeyHash:     apiKeyHash(rawKey),
		Name:        "bearer-prefix",
		Permissions: []string{auth.PermLibraryRead},
	})
	userRepo.Create(context.Background(), &user.User{
		ID:       "usr-b",
		Username: "bob",
		IsAdmin:  false,
		Enabled:  true,
	})

	ai := interceptors.NewAuthInterceptor(tokenGen).WithAPIKeys(apiKeyRepo, userRepo)
	interceptor := ai.Unary()

	md := metadata.Pairs("authorization", "Bearer "+rawKey)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	extractHandler := func(ctx context.Context, req any) (any, error) {
		return interceptors.ClaimsFromContext(ctx), nil
	}
	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.LibraryService/ListLibraries"}
	resp, err := interceptor(ctx, nil, info, extractHandler)

	require.NoError(t, err)
	claims := resp.(*user.Claims)
	assert.Equal(t, "usr-b", claims.UserID)
}

// TestAPIKeyScope_EmptyAllowsAll: legacy API keys with no scopes must be allowed.
func TestAPIKeyScope_EmptyAllowsAll(t *testing.T) {
	tokenGen := fakes.NewFakeTokenGenerator()
	apiKeyRepo := fakes.NewFakeAPIKeyRepository()
	userRepo := fakes.NewFakeUserRepository()

	rawKey := "evey_legacynoperm"
	apiKeyRepo.Create(context.Background(), &user.APIKeyModel{
		ID:          "ak-legacy",
		UserID:      "usr-1",
		KeyHash:     apiKeyHash(rawKey),
		Name:        "legacy",
		Permissions: nil,
	})
	userRepo.Create(context.Background(), &user.User{
		ID: "usr-1", Username: "alice", Enabled: true,
	})

	ai := interceptors.NewAuthInterceptor(tokenGen).WithAPIKeys(apiKeyRepo, userRepo)
	interceptor := ai.Unary()

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.LibraryService/ListLibraries"}
	_, err := interceptor(ctxWithAPIKey(rawKey), nil, info, nopHandler)
	require.NoError(t, err)
}

// TestAPIKeyScope_Granted: scope matching the APIKeyScopeFor map allows the call.
func TestAPIKeyScope_Granted(t *testing.T) {
	tokenGen := fakes.NewFakeTokenGenerator()
	apiKeyRepo := fakes.NewFakeAPIKeyRepository()
	userRepo := fakes.NewFakeUserRepository()

	rawKey := "evey_scopedkey"
	apiKeyRepo.Create(context.Background(), &user.APIKeyModel{
		ID:          "ak-scoped",
		UserID:      "usr-1",
		KeyHash:     apiKeyHash(rawKey),
		Name:        "scoped",
		Permissions: []string{auth.ScopeMediaRead},
	})
	userRepo.Create(context.Background(), &user.User{
		ID: "usr-1", Username: "alice", Enabled: true,
	})

	ai := interceptors.NewAuthInterceptor(tokenGen).WithAPIKeys(apiKeyRepo, userRepo)
	interceptor := ai.Unary()

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.LibraryService/ListLibraries"}
	_, err := interceptor(ctxWithAPIKey(rawKey), nil, info, nopHandler)
	require.NoError(t, err)
}

// TestAPIKeyScope_Denied: scope absent from key and no fine-grained fallback -> deny.
func TestAPIKeyScope_Denied(t *testing.T) {
	tokenGen := fakes.NewFakeTokenGenerator()
	apiKeyRepo := fakes.NewFakeAPIKeyRepository()
	userRepo := fakes.NewFakeUserRepository()

	rawKey := "evey_wrongscope"
	apiKeyRepo.Create(context.Background(), &user.APIKeyModel{
		ID:          "ak-wrong",
		UserID:      "usr-1",
		KeyHash:     apiKeyHash(rawKey),
		Name:        "wrong",
		Permissions: []string{auth.ScopePlaybackRead},
	})
	userRepo.Create(context.Background(), &user.User{
		ID: "usr-1", Username: "alice", Enabled: true,
	})

	ai := interceptors.NewAuthInterceptor(tokenGen).WithAPIKeys(apiKeyRepo, userRepo)
	interceptor := ai.Unary()

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.LibraryService/ListLibraries"}
	_, err := interceptor(ctxWithAPIKey(rawKey), nil, info, nopHandler)
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// A read-only playback key must not be able to WRITE watch state. Until
// 2026-08-01 it could: the write RPCs were listed under names that no longer
// existed (MarkWatched / MarkUnwatched / GetHistory), and a method absent from
// every table is RoleUser by default - so the scope check in auth.go step 3
// never ran and the key sailed through whatever it carried. This pins the
// three real names, plus the read that must stay reachable.
func TestAPIKey_PlaybackReadCannotWriteWatchState(t *testing.T) {
	keyWithScopes := func(perms []string) grpc.UnaryServerInterceptor {
		tokenGen := fakes.NewFakeTokenGenerator()
		apiKeyRepo := fakes.NewFakeAPIKeyRepository()
		userRepo := fakes.NewFakeUserRepository()
		apiKeyRepo.Create(context.Background(), &user.APIKeyModel{
			ID: "ak-ro", UserID: "usr-ro", KeyHash: apiKeyHash("evey_readonly"),
			Name: "read-only", Permissions: perms,
		})
		userRepo.Create(context.Background(), &user.User{
			ID: "usr-ro", Username: "alice", Enabled: true,
		})
		return interceptors.NewAuthInterceptor(tokenGen).WithAPIKeys(apiKeyRepo, userRepo).Unary()
	}

	writes := []string{
		"/evey.v1.PlaybackService/MarkAsWatched",
		"/evey.v1.PlaybackService/MarkAsUnwatched",
		"/evey.v1.PlaybackService/ClearWatchHistory",
	}
	ro := keyWithScopes([]string{auth.ScopePlaybackRead})
	for _, method := range writes {
		_, err := ro(ctxWithAPIKey("evey_readonly"), nil, &grpc.UnaryServerInfo{FullMethod: method}, nopHandler)
		require.Error(t, err, "a playback:read key must be denied on %s", method)
		assert.Equal(t, codes.PermissionDenied, status.Code(err), method)
	}

	// The same key keeps the read it is scoped for, and a write-scoped key gets
	// the writes - otherwise this guard would just be breaking the feature.
	_, err := ro(ctxWithAPIKey("evey_readonly"), nil,
		&grpc.UnaryServerInfo{FullMethod: "/evey.v1.PlaybackService/GetWatchHistory"}, nopHandler)
	require.NoError(t, err, "a playback:read key must still read its watch history")

	rw := keyWithScopes([]string{auth.ScopePlaybackWrite})
	for _, method := range writes {
		_, err := rw(ctxWithAPIKey("evey_readonly"), nil, &grpc.UnaryServerInfo{FullMethod: method}, nopHandler)
		require.NoError(t, err, "a playback:write key must reach %s", method)
	}
}

// apiKeyInterceptorWithPerms builds a fresh interceptor wired to a single
// non-admin API key carrying the given permissions.
func apiKeyInterceptorWithPerms(t *testing.T, rawKey string, perms []string) grpc.UnaryServerInterceptor {
	t.Helper()
	tokenGen := fakes.NewFakeTokenGenerator()
	apiKeyRepo := fakes.NewFakeAPIKeyRepository()
	userRepo := fakes.NewFakeUserRepository()
	userID := shared.UserID("usr-" + rawKey)
	apiKeyRepo.Create(context.Background(), &user.APIKeyModel{
		ID: "ak-" + rawKey, UserID: userID, KeyHash: apiKeyHash(rawKey),
		Name: rawKey, Permissions: perms,
	})
	userRepo.Create(context.Background(), &user.User{
		ID: userID, Username: "alice", Enabled: true,
	})
	return interceptors.NewAuthInterceptor(tokenGen).WithAPIKeys(apiKeyRepo, userRepo).Unary()
}

// TestAPIKey_UnclassifiedMethodDeniedByDefault is the direct proof for the
// 2026-08-01 fail-closed default (interceptors/auth.go enforceRBAC step 2): a
// method with no entry in auth.APIKeyScopeFor or auth.MethodPermissions must
// be refused to an API key regardless of what permissions it holds -
// including the pre-scope "legacy allow-all" empty-permissions key. Before
// this change, an unclassified non-admin method skipped straight through
// (the scope-check condition itself never triggered without a table entry),
// which is exactly how a "read only" key could reach
// ParentalService/VerifyPin. Reverting step 2 to only fail-close admin
// methods (isAdminMethod && !hasScope && !hasPerm, the pre-fix condition)
// turns every case below back into require.NoError - proven by hand: with
// that revert applied, this test fails red; with the fix in place, it is
// green.
func TestAPIKey_UnclassifiedMethodDeniedByDefault(t *testing.T) {
	// A method name that will never appear in APIKeyScopeFor or
	// MethodPermissions proves the default itself, independent of which real
	// RPCs happen to be classified today.
	const neverClassified = "/evey.v1.SomeFutureService/DoSomething"

	for _, perms := range [][]string{nil, {}, {auth.ScopeMediaRead}, {auth.ScopeAdminWrite}} {
		ic := apiKeyInterceptorWithPerms(t, "evey_unclassified", perms)
		_, err := ic(ctxWithAPIKey("evey_unclassified"), nil, &grpc.UnaryServerInfo{FullMethod: neverClassified}, nopHandler)
		require.Error(t, err, "perms=%v must be denied on an unclassified method", perms)
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	}
}

// TestAPIKey_FormerlyUnclassifiedMethodsNowEnforced spot-checks real RPCs that
// had zero entries in either method-keyed table before 2026-08-01 (see the
// doc comment on auth.APIKeyScopeFor): a key holding an unrelated scope must
// now be denied, and a key holding the assigned scope must still work.
func TestAPIKey_FormerlyUnclassifiedMethodsNowEnforced(t *testing.T) {
	cases := []struct {
		method string
		wrong  string
		right  string
	}{
		{"/evey.v1.ParentalService/VerifyPin", auth.ScopeMediaRead, auth.ScopeUsersWrite},
		{"/evey.v1.ParentalService/UpdateParentalSettings", auth.ScopePlaybackRead, auth.ScopeUsersWrite},
		{"/evey.v1.NotificationService/DeleteAllNotifications", auth.ScopeMediaRead, auth.ScopeUsersWrite},
		{"/evey.v1.MetadataService/UpdateMetadata", auth.ScopeUsersRead, auth.ScopeMediaWrite},
		{"/evey.v1.PhotoService/ScanPhotoDirectory", auth.ScopePlaybackWrite, auth.ScopeMediaWrite},
		{"/evey.v1.MediaRequestService/CreateRequest", auth.ScopeAdminRead, auth.ScopeMediaWrite},
	}
	for _, c := range cases {
		wrongIC := apiKeyInterceptorWithPerms(t, "evey_formerlyopen_wrong", []string{c.wrong})
		_, err := wrongIC(ctxWithAPIKey("evey_formerlyopen_wrong"), nil, &grpc.UnaryServerInfo{FullMethod: c.method}, nopHandler)
		require.Error(t, err, "%s: wrong scope %s must be denied", c.method, c.wrong)
		assert.Equal(t, codes.PermissionDenied, status.Code(err), c.method)

		rightIC := apiKeyInterceptorWithPerms(t, "evey_formerlyopen_right", []string{c.right})
		_, err = rightIC(ctxWithAPIKey("evey_formerlyopen_right"), nil, &grpc.UnaryServerInfo{FullMethod: c.method}, nopHandler)
		require.NoError(t, err, "%s: correct scope %s must be allowed", c.method, c.right)
	}
}

// TestAPIKey_DomainScopesGrantExpectedAccess is the "we didn't just close the
// hole, we didn't also break the feature" check for the classification work:
// one representative read and one write RPC per coarse scope domain must
// stay reachable to a key holding exactly that scope.
func TestAPIKey_DomainScopesGrantExpectedAccess(t *testing.T) {
	grants := map[string]string{
		"/evey.v1.CollectionService/ListCollections": auth.ScopeMediaRead,
		"/evey.v1.PlaylistService/CreatePlaylist":    auth.ScopeMediaWrite,
		"/evey.v1.FeedService/RecentlyAdded":         auth.ScopePlaybackRead,
		"/evey.v1.SyncPlayService/CreateRoom":        auth.ScopePlaybackWrite,
		"/evey.v1.UserService/GetCurrentUser":        auth.ScopeUsersRead,
		"/evey.v1.UserService/UpdateProfile":         auth.ScopeUsersWrite,
		"/evey.v1.ActivityService/GetAnalytics":      auth.ScopeAdminRead,
		"/evey.v1.LibraryService/SetLibrarySchedule": auth.ScopeAdminWrite,
	}
	for method, scope := range grants {
		ic := apiKeyInterceptorWithPerms(t, "evey_domain_"+scope, []string{scope})
		_, err := ic(ctxWithAPIKey("evey_domain_"+scope), nil, &grpc.UnaryServerInfo{FullMethod: method}, nopHandler)
		require.NoError(t, err, "%s: scope %s must grant access", method, scope)
	}
}

// An API key belonging to an admin user must NOT reach admin infra/ops RPCs
// unless the key explicitly carries the scope that RPC maps to. Admin methods
// are JWT-only by default (deny-by-default): forgetting to enumerate a new
// admin RPC can no longer expose it (RestartServer, arbitrary-file MKV
// attachment R/W, session kill, webhook CRUD, ...) to every admin API key, and
// an empty-scope "legacy allow-all" key gets NO admin reach at all.
func TestAPIKey_AdminMethodsDenyByDefault(t *testing.T) {
	adminKeyInterceptor := func(perms []string) grpc.UnaryServerInterceptor {
		tokenGen := fakes.NewFakeTokenGenerator()
		apiKeyRepo := fakes.NewFakeAPIKeyRepository()
		userRepo := fakes.NewFakeUserRepository()
		apiKeyRepo.Create(context.Background(), &user.APIKeyModel{
			ID: "ak-admin", UserID: "usr-admin", KeyHash: apiKeyHash("evey_adminkey"),
			Name: "admin-key", Permissions: perms,
		})
		userRepo.Create(context.Background(), &user.User{
			ID: "usr-admin", Username: "root", Enabled: true, IsAdmin: true,
		})
		return interceptors.NewAuthInterceptor(tokenGen).WithAPIKeys(apiKeyRepo, userRepo).Unary()
	}

	// Admin infra/ops RPCs with NO API-key scope mapping: blocked even for an
	// admin key, whatever its scopes.
	denied := []string{
		"/evey.v1.AdminSystemService/RestartServer",
		"/evey.v1.MKVService/ExtractAttachment",
		"/evey.v1.MKVService/ListAttachments",
		"/evey.v1.AdminSessionService/KillSession",
		"/evey.v1.AdminMediaService/DeleteMediaItem",
		"/evey.v1.WebhookService/CreateWebhook",
		"/evey.v1.AdminMediaService/RemuxAudioLibrary",
	}
	for _, scopes := range [][]string{nil, {auth.ScopeAdminRead}, {auth.ScopeMediaWrite}} {
		ic := adminKeyInterceptor(scopes)
		for _, method := range denied {
			_, err := ic(ctxWithAPIKey("evey_adminkey"), nil, &grpc.UnaryServerInfo{FullMethod: method}, nopHandler)
			require.Error(t, err, "admin key (scopes=%v) must be denied on %s", scopes, method)
			assert.Equal(t, codes.PermissionDenied, status.Code(err), method)
		}
	}

	// An admin RPC that DOES map to an API-key scope stays reachable - but only
	// with that scope, never via an empty-scope allow-all key.
	statsMethod := "/evey.v1.ActivityService/GetServerStats" // -> ScopeAdminRead
	_, err := adminKeyInterceptor([]string{auth.ScopeAdminRead})(ctxWithAPIKey("evey_adminkey"), nil, &grpc.UnaryServerInfo{FullMethod: statsMethod}, nopHandler)
	require.NoError(t, err, "an admin key holding admin:read may call an admin:read RPC")
	_, err = adminKeyInterceptor(nil)(ctxWithAPIKey("evey_adminkey"), nil, &grpc.UnaryServerInfo{FullMethod: statsMethod}, nopHandler)
	require.Error(t, err, "an empty-scope key must NOT allow-all onto an admin RPC")
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// The deny-by-default guard is scoped to API keys: a browser/app JWT for an
// admin user still reaches every admin RPC (that is the whole point of
// InternalMethods/deny-by-default - restrict KEYS, not the admin themselves).
func TestJWTAdmin_ReachesAdminInfraMethods(t *testing.T) {
	ai, tokenGen := setupInterceptor()
	token, _ := tokenGen.GenerateAccessToken(user.Claims{UserID: "a1", Username: "root", IsAdmin: true})
	for _, method := range []string{"/evey.v1.AdminSystemService/RestartServer", "/evey.v1.MKVService/ExtractAttachment"} {
		_, err := ai.Unary()(ctxWithToken(token), nil, &grpc.UnaryServerInfo{FullMethod: method}, nopHandler)
		require.NoError(t, err, "a JWT admin must still reach %s", method)
	}
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

type stubPluginParser struct{ claims *auth.PluginClaims }

func (s stubPluginParser) ParsePluginToken(string) (*auth.PluginClaims, error) {
	if s.claims == nil {
		return nil, errors.New("not a plugin token")
	}
	return s.claims, nil
}

// runPluginToken chains the plugin-token interceptor (which sets the claims)
// into the auth interceptor, exactly as the public server does.
func runPluginToken(rbac interceptors.RBACConfig, claims *auth.PluginClaims, method string) (any, error) {
	ai, _ := setupInterceptor()
	authUnary := ai.WithRBAC(rbac).Unary()
	ptUnary := interceptors.NewPluginTokenInterceptor(stubPluginParser{claims: claims}).Unary()
	info := &grpc.UnaryServerInfo{FullMethod: method}
	return ptUnary(ctxWithToken("any-plugin-token"), nil, info, func(c context.Context, r any) (any, error) {
		return authUnary(c, r, info, nopHandler)
	})
}

// TestAuthInterceptor_PluginTokenDenyByDefault proves the boundary for scoped
// plugin tokens (mintable by any authenticated user via IssuePluginToken):
// deny-by-default on the public API. They cannot reach admin methods, cannot
// reach any method absent from the plugin allowlist, and on allowlisted methods
// must carry the required scope.
func TestAuthInterceptor_PluginTokenDenyByDefault(t *testing.T) {
	claims := &auth.PluginClaims{PluginID: "p1", UserID: "u1", TokenType: "plugin", Scopes: []string{"media.read"}}

	t.Run("admin method denied", func(t *testing.T) {
		_, err := runPluginToken(interceptors.DefaultRBACConfig(), claims, "/evey.v1.AdminPluginService/UpdatePluginResourceLimits")
		require.Error(t, err)
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("non-allowlisted method denied (deny-by-default)", func(t *testing.T) {
		_, err := runPluginToken(interceptors.DefaultRBACConfig(), claims, "/evey.v1.LibraryService/ListLibraries")
		require.Error(t, err)
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("public media allowlisted with public scope (real config)", func(t *testing.T) {
		pub := &auth.PluginClaims{PluginID: "p1", UserID: "u1", TokenType: "plugin", Scopes: []string{"public"}}
		resp, err := runPluginToken(interceptors.DefaultRBACConfig(), pub, "/evey.v1.LibraryService/GetPublicMediaItem")
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("public media denied without public scope (real config)", func(t *testing.T) {
		_, err := runPluginToken(interceptors.DefaultRBACConfig(), claims, "/evey.v1.LibraryService/ListPublicMediaItems")
		require.Error(t, err)
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	rbac := interceptors.DefaultRBACConfig()
	rbac.PluginMethodScopes = map[string]string{
		"/test/PluginRead": "media.read", // requires a scope
		"/test/PluginOpen": "",           // allowlisted, any plugin token
	}

	t.Run("allowlisted method with required scope allowed", func(t *testing.T) {
		resp, err := runPluginToken(rbac, claims, "/test/PluginRead")
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("allowlisted method missing scope denied", func(t *testing.T) {
		noScope := &auth.PluginClaims{PluginID: "p1", UserID: "u1", TokenType: "plugin"}
		_, err := runPluginToken(rbac, noScope, "/test/PluginRead")
		require.Error(t, err)
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("allowlisted method without scope requirement allowed", func(t *testing.T) {
		resp, err := runPluginToken(rbac, claims, "/test/PluginOpen")
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})
}

// An impersonation token marks its requests, so their audit entries name the
// admin; an ordinary token does not.
func TestAuthInterceptor_MarksImpersonation(t *testing.T) {
	ai, tokenGen := setupInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.LibraryService/ListLibraries"}
	for _, impBy := range []string{"admin-1", ""} {
		token, err := tokenGen.GenerateAccessToken(user.Claims{UserID: "target", ImpersonatedBy: impBy})
		require.NoError(t, err)
		resp, err := ai.Unary()(ctxWithToken(token), nil, info, nopHandler)
		require.NoError(t, err)
		assert.Equal(t, impBy, activity.ImpersonatorFrom(resp.(context.Context)))
	}
}
