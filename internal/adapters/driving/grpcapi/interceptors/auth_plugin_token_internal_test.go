package interceptors

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/user"
	authpkg "github.com/eveyproject/evey/internal/pkg/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// These tests close two mutation-testing survivors in the plugin-token path of
// the AuthInterceptor (gremlins, 2026-05-25): the Stream() plugin-token branch
// was not covered (auth.go:80) and the admin-floor condition in
// authorizePluginToken survived negation (auth.go:105). Both are security
// boundaries - a plugin token (mintable by any authenticated user) must never
// reach an admin or non-allowlisted method, on unary OR streaming RPCs.

func pluginAuthInterceptor() *AuthInterceptor {
	return NewAuthInterceptor(nil).WithRBAC(RBACConfig{
		MethodRoles:        map[string]Role{"/evey.v1.AdminSystemService/GetSystemInfo": RoleAdmin},
		PluginMethodScopes: map[string]string{"/evey.v1.PreferencesService/ListPrefs": ""},
	})
}

func ctxWithPluginClaims() context.Context {
	claims := &authpkg.PluginClaims{UserID: "u1", PluginID: "plg1", TokenType: "plugin", ExpiresAt: time.Now().Add(time.Minute)}
	return context.WithValue(context.Background(), pluginClaimsKey{}, claims)
}

// Kills the NOT-COVERED mutant at auth.go:80 - the Stream() plugin-token branch.
// A plugin token on a non-allowlisted streaming method must propagate the deny
// error and NOT invoke the handler (a mutant negating `err != nil` there would
// fall through and run the handler).
func TestAuthInterceptor_Stream_PluginToken_Denied(t *testing.T) {
	i := pluginAuthInterceptor()
	called := false
	handler := func(any, grpc.ServerStream) error { called = true; return nil }

	err := i.Stream()(nil, &streamStub{ctx: ctxWithPluginClaims()},
		&grpc.StreamServerInfo{FullMethod: "/evey.v1.LibraryService/ListLibraries"}, handler)

	if err == nil {
		t.Fatal("expected a deny error for a plugin token on a non-allowlisted streaming method")
	}
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}
	if called {
		t.Fatal("handler must not run when the plugin token is denied")
	}
}

// The allowed side of the same branch: a plugin token on an allowlisted method
// reaches the handler with no error.
func TestAuthInterceptor_Stream_PluginToken_Allowed(t *testing.T) {
	i := pluginAuthInterceptor()
	called := false
	handler := func(any, grpc.ServerStream) error { called = true; return nil }

	err := i.Stream()(nil, &streamStub{ctx: ctxWithPluginClaims()},
		&grpc.StreamServerInfo{FullMethod: "/evey.v1.PreferencesService/ListPrefs"}, handler)

	if err != nil {
		t.Fatalf("unexpected err for an allowlisted plugin method: %v", err)
	}
	if !called {
		t.Fatal("handler must run for an allowlisted plugin method")
	}
}

// Kills the LIVED mutant at auth.go:105 - the admin floor in
// authorizePluginToken. A plugin token targeting an admin method must be denied
// by the floor with the admin-specific reason, distinct from the generic
// allowlist-miss reason. A mutant negating `role == RoleAdmin` skips the floor
// and falls through to the allowlist-miss path, yielding a different reason key.
func TestAuthInterceptor_PluginToken_AdminFloor(t *testing.T) {
	i := pluginAuthInterceptor()
	claims := &authpkg.PluginClaims{UserID: "u1", PluginID: "plg1", TokenType: "plugin"}

	err := i.authorizePluginToken(context.Background(), claims, "/evey.v1.AdminSystemService/GetSystemInfo")
	if err == nil {
		t.Fatal("expected the admin floor to deny a plugin token on an admin method")
	}
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}
	if !strings.Contains(err.Error(), "admin_access_required") {
		t.Fatalf("reason = %q, want admin_access_required (the floor must fire before the allowlist check)", err.Error())
	}
}

// A plugin token on an allowlisted method must hand the handler a NON-ADMIN
// user.Claims derived from the minting user/profile, so downstream library
// ACL + parental filters actually apply (audit H1: without this the media
// reads a plugin can reach saw claims==nil and treated the caller as
// unrestricted, leaking other libraries and age-gated content).
func TestAuthInterceptor_Unary_PluginToken_InjectsRestrictedClaims(t *testing.T) {
	i := pluginAuthInterceptor()
	claims := &authpkg.PluginClaims{
		UserID: "u-restricted", ProfileID: "prf-kids", PluginID: "plg1",
		TokenType: "plugin", ExpiresAt: time.Now().Add(time.Minute),
		Scopes: []string{""},
	}
	ctx := context.WithValue(context.Background(), pluginClaimsKey{}, claims)

	var seen *user.Claims
	handler := func(hctx context.Context, _ any) (any, error) {
		seen = ClaimsFromContext(hctx)
		return nil, nil
	}
	_, err := i.Unary()(ctx, nil,
		&grpc.UnaryServerInfo{FullMethod: "/evey.v1.PreferencesService/ListPrefs"}, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seen == nil {
		t.Fatal("handler saw nil user.Claims - library/parental filters would treat the plugin as admin")
	}
	if seen.UserID != "u-restricted" || seen.ProfileID != "prf-kids" {
		t.Fatalf("claims = %+v, want UserID/ProfileID from the plugin token", seen)
	}
	if seen.IsAdmin {
		t.Fatal("a plugin token must never confer admin")
	}
}
