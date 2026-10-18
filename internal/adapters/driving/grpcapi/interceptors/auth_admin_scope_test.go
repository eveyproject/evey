package interceptors_test

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
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

// adminOwnedKeyInterceptor wires one API key owned by an ADMIN account: the
// claims it produces carry IsAdmin=true, which is what the handler-side admin
// checks read.
func adminOwnedKeyInterceptor(t *testing.T, rawKey string, perms []string) grpc.UnaryServerInterceptor {
	t.Helper()
	apiKeyRepo := fakes.NewFakeAPIKeyRepository()
	userRepo := fakes.NewFakeUserRepository()
	require.NoError(t, apiKeyRepo.Create(context.Background(), &user.APIKeyModel{
		ID: "ak-" + rawKey, UserID: "usr-root", KeyHash: apiKeyHash(rawKey),
		Name: rawKey, Permissions: perms,
	}))
	require.NoError(t, userRepo.Create(context.Background(), &user.User{
		ID: "usr-root", Username: "root", IsAdmin: true, Enabled: true,
	}))
	return interceptors.NewAuthInterceptor(fakes.NewFakeTokenGenerator()).WithAPIKeys(apiKeyRepo, userRepo).Unary()
}

// These RPCs default to RoleUser and are admin-only inside their handler
// (claims.IsAdmin), with an admin:* API-key scope. A pre-scope key (empty
// permissions, still valid in api_keys whose column defaults to '[]') skipped
// enforceRBAC step 3 on them, because that step only ran for RoleAdmin methods
// or keys carrying permissions; the key's owner being admin, the handler check
// passed too. The same key IS refused on every RoleAdmin method.
var adminScopedUserMethods = []string{
	"/evey.v1.ActivityService/GetAnalytics",
	"/evey.v1.ActivityService/GetUserStats",
	"/evey.v1.LibraryService/GetLibrarySchedule",
	"/evey.v1.LibraryService/SetLibrarySchedule",
	"/evey.v1.LibraryService/DeleteLibrarySchedule",
	"/evey.v1.AdminPluginService/IssuePluginToken",
}

func TestAPIKey_UnscopedAdminKeyRefusedOnAdminScopedMethods(t *testing.T) {
	ic := adminOwnedKeyInterceptor(t, "evey_legacy_admin", nil)
	for _, method := range adminScopedUserMethods {
		reached := false
		_, err := ic(ctxWithAPIKey("evey_legacy_admin"), nil, &grpc.UnaryServerInfo{FullMethod: method},
			func(ctx context.Context, _ any) (any, error) {
				reached = true
				return nil, nil
			})
		require.Error(t, err, "an unscoped key must not reach %s", method)
		assert.Equal(t, codes.PermissionDenied, status.Code(err), method)
		assert.False(t, reached, "%s: the handler (whose only gate is claims.IsAdmin) must not run", method)
	}
}

// The scoped key keeps what its scope names, else the guard above would just
// be breaking admin automation.
func TestAPIKey_AdminScopedMethodsStillReachableWithTheirScope(t *testing.T) {
	read := adminOwnedKeyInterceptor(t, "evey_admin_read", []string{auth.ScopeAdminRead})
	for _, method := range []string{"/evey.v1.ActivityService/GetAnalytics", "/evey.v1.LibraryService/GetLibrarySchedule"} {
		_, err := read(ctxWithAPIKey("evey_admin_read"), nil, &grpc.UnaryServerInfo{FullMethod: method}, nopHandler)
		require.NoError(t, err, method)
	}
	write := adminOwnedKeyInterceptor(t, "evey_admin_write", []string{auth.ScopeAdminWrite})
	_, err := write(ctxWithAPIKey("evey_admin_write"), nil,
		&grpc.UnaryServerInfo{FullMethod: "/evey.v1.LibraryService/SetLibrarySchedule"}, nopHandler)
	require.NoError(t, err)

	// A legacy key keeps its allow-all on the owner's own, non-admin domain.
	legacy := adminOwnedKeyInterceptor(t, "evey_legacy_admin", nil)
	_, err = legacy(ctxWithAPIKey("evey_legacy_admin"), nil,
		&grpc.UnaryServerInfo{FullMethod: "/evey.v1.LibraryService/ListLibraries"}, nopHandler)
	require.NoError(t, err)
}

// AdminReach is what a handler asks before its admin-only branch. The symmetric
// cases matter as much as the refusals: an admin session keeps the role, and a
// key holding the named scope keeps it too.
func TestAdminReach(t *testing.T) {
	adminClaims := &user.Claims{UserID: "usr-root", IsAdmin: true}
	jwt := interceptors.ContextWithClaims(context.Background(), adminClaims)
	assert.True(t, interceptors.AdminReach(jwt, ""), "an admin session has the role")
	assert.True(t, interceptors.AdminReach(jwt, auth.ScopeMediaWrite))

	nonAdmin := interceptors.ContextWithClaims(context.Background(), &user.Claims{UserID: "usr-1"})
	assert.False(t, interceptors.AdminReach(nonAdmin, ""))
	assert.False(t, interceptors.AdminReach(context.Background(), ""), "no claims, no role")

	// Each key calls a method its own scope reaches, so the context handed to
	// the handler is the one a real call would carry.
	keyCtx := func(perms []string, method string) context.Context {
		var got context.Context
		ic := adminOwnedKeyInterceptor(t, "evey_reach", perms)
		_, err := ic(metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "evey_reach")), nil,
			&grpc.UnaryServerInfo{FullMethod: method},
			func(ctx context.Context, _ any) (any, error) { got = ctx; return nil, nil })
		require.NoError(t, err)
		return got
	}
	const read, write = "/evey.v1.LibraryService/ListLibraries", "/evey.v1.FavoriteService/AddFavorite"
	assert.False(t, interceptors.AdminReach(keyCtx(nil, read), auth.ScopeMediaWrite), "an unscoped key does not carry the role")
	assert.False(t, interceptors.AdminReach(keyCtx([]string{auth.ScopeMediaRead}, read), auth.ScopeMediaWrite))
	assert.True(t, interceptors.AdminReach(keyCtx([]string{auth.ScopeMediaWrite}, write), auth.ScopeMediaWrite))
	assert.False(t, interceptors.AdminReach(keyCtx([]string{"*", auth.ScopeUsersWrite}, write), ""),
		"an empty scope keeps the branch away from every key")
}
