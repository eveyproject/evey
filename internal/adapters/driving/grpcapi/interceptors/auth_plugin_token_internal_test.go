package interceptors

import (
	"context"
	"strings"
	"testing"
	"time"

	authpkg "github.com/evey-project/evey/internal/pkg/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// These tests close two mutation-testing survivors in the plugin-token path of
// the AuthInterceptor (gremlins, 2026-05-25): the Stream() plugin-token branch
// was not covered (auth.go:80) and the admin-floor condition in
// authorizePluginToken survived negation (auth.go:105). Both are security
// boundaries — a plugin token (mintable by any authenticated user) must never
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

// Kills the NOT-COVERED mutant at auth.go:80 — the Stream() plugin-token branch.
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

// Kills the LIVED mutant at auth.go:105 — the admin floor in
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
