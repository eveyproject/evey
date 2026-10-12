package interceptors_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/eveyproject/evey/internal/pkg/demo"
)

// The demo interceptor is the one place the demo refuses an RPC. Both
// directions are pinned: what the policy opens must reach the handler and what
// it locks must not, for an admin RPC, a user RPC the policy names, and an
// admin RPC nobody classified (locked by default).
func TestDemoInterceptor_RefusesLockedAndPassesOpen(t *testing.T) {
	di := interceptors.NewDemoInterceptor(interceptors.DefaultRBACConfig(), nil)
	unary := di.Unary()

	cases := []struct {
		method string
		locked bool
	}{
		{"/evey.v1.AdminSystemService/RestartServer", true},              // LockedAdminRPCs
		{"/evey.v1.AdminSystemService/GetDashboard", false},              // OpenAdminRPCs
		{"/evey.v1.UserService/UpdateUser", true},                        // LockedUserRPCs (shared account)
		{"/evey.v1.PlaybackService/StartStream", false},                  // user RPC, the product
		{"/evey.v1.CollectionService/CreateCollection", true},            // LockedAdminRPCs (shared content)
		{"/evey.v1.FavoriteService/AddFavorite", true},                   // LockedUserRPCs (shared content)
		{"/evey.v1.UserService/CreateAPIKey", true},                      // locked 2026-10-01: a key outlived the visitor
		{"/evey.v1.UserService/ImpersonateUser", false},                  // admin RPC the demo opens
		{"/evey.v1.MetadataService/RefreshMetadata", true},               // LockedUserRPCs: rewrites a title for everyone
		{"/evey.v1.AdminSystemService/SomethingNobodyClassified", false}, // not in the RBAC map: not admin, not locked
	}
	for _, c := range cases {
		called := false
		_, err := unary(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: c.method},
			func(ctx context.Context, req any) (any, error) { called = true; return nil, nil })
		if c.locked {
			require.Error(t, err, c.method)
			assert.Equal(t, codes.PermissionDenied, status.Code(err), c.method)
			assert.Equal(t, demo.ErrorKey, status.Convert(err).Message(), "without a translator the key itself travels")
			assert.False(t, called, "%s: handler must not run", c.method)
		} else {
			require.NoError(t, err, c.method)
			assert.True(t, called, "%s: handler must run", c.method)
		}
	}
}

// An admin RPC that IS in the RBAC map but in neither demo list is the
// drift case the guard refuses at build time; at runtime it must be locked.
func TestDemoInterceptor_UnclassifiedAdminRPCIsLocked(t *testing.T) {
	rbac := interceptors.DefaultRBACConfig()
	rbac.MethodRoles["/evey.v1.AdminSystemService/BrandNew"] = interceptors.RoleAdmin
	di := interceptors.NewDemoInterceptor(rbac, nil)

	_, err := di.Unary()(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/evey.v1.AdminSystemService/BrandNew"}, nopHandler)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestDemoInterceptor_Stream(t *testing.T) {
	di := interceptors.NewDemoInterceptor(interceptors.DefaultRBACConfig(), nil)
	ss := &fakeServerStream{ctx: context.Background()}

	err := di.Stream()(nil, ss, &grpc.StreamServerInfo{FullMethod: "/evey.v1.AdminPluginService/GetPluginLogs"},
		func(srv any, stream grpc.ServerStream) error { return nil })
	require.NoError(t, err, "an open admin read streams")

	err = di.Stream()(nil, ss, &grpc.StreamServerInfo{FullMethod: "/evey.v1.AdminMediaService/RemuxAudioLibrary"},
		func(srv any, stream grpc.ServerStream) error { t.Fatal("handler must not run"); return nil })
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}
