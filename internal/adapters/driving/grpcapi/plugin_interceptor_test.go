package grpcapi_test

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi"
	"github.com/eveyproject/evey/internal/adapters/driving/pluginhost"
	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	plgfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	grpclib "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPluginInterceptor_AllowedRPC(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)
	checker.LoadPermissions("plg-1", []string{"api.library"})

	interceptor := grpcapi.PluginPermissionInterceptor(checker)

	ctx := pluginhost.CtxWithPluginID(context.Background(), "plg-1")

	called := false
	handler := func(_ context.Context, _ any) (any, error) {
		called = true
		return "ok", nil
	}

	info := &grpclib.UnaryServerInfo{FullMethod: "/plugin.v1.HostService/GetMediaItem"}
	resp, err := interceptor(ctx, nil, info, handler)
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, "ok", resp)
}

func TestPluginInterceptor_DeniedRPC(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)
	checker.LoadPermissions("plg-1", []string{"api.user"})

	interceptor := grpcapi.PluginPermissionInterceptor(checker)

	ctx := pluginhost.CtxWithPluginID(context.Background(), "plg-1")

	called := false
	handler := func(_ context.Context, _ any) (any, error) {
		called = true
		return "ok", nil
	}

	info := &grpclib.UnaryServerInfo{FullMethod: "/plugin.v1.HostService/GetMediaItem"}
	_, err := interceptor(ctx, nil, info, handler)
	require.Error(t, err)
	assert.False(t, called)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestPluginInterceptor_AlwaysAllowedRPC(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)
	checker.LoadPermissions("plg-1", nil)

	interceptor := grpcapi.PluginPermissionInterceptor(checker)

	ctx := pluginhost.CtxWithPluginID(context.Background(), "plg-1")

	for _, method := range []string{
		"/plugin.v1.HostService/EmitEvent",
		"/plugin.v1.HostService/Log",
		"/plugin.v1.HostService/GetConfig",
		"/plugin.v1.HostService/SetConfig",
	} {
		called := false
		handler := func(_ context.Context, _ any) (any, error) {
			called = true
			return "ok", nil
		}

		info := &grpclib.UnaryServerInfo{FullMethod: method}
		_, err := interceptor(ctx, nil, info, handler)
		require.NoError(t, err, "method %s should be allowed", method)
		assert.True(t, called, "handler should be called for %s", method)
	}
}

func TestPluginInterceptor_FetchRequiresNetworkPermission(t *testing.T) {
	info := &grpclib.UnaryServerInfo{FullMethod: "/plugin.v1.HostService/Fetch"}
	handler := func(_ context.Context, _ any) (any, error) { return "ok", nil }

	// Plugin WITH the network permission → broker reachable.
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)
	checker.LoadPermissions("plg-net", []string{"network"})
	interceptor := grpcapi.PluginPermissionInterceptor(checker)
	ctx := pluginhost.CtxWithPluginID(context.Background(), "plg-net")
	called := false
	_, err := interceptor(ctx, nil, info, func(_ context.Context, _ any) (any, error) { called = true; return "ok", nil })
	require.NoError(t, err)
	assert.True(t, called)

	// Plugin WITHOUT network → denied (media-only perms don't grant egress).
	checker.LoadPermissions("plg-nonet", []string{"api.library", "media.read"})
	ctxNo := pluginhost.CtxWithPluginID(context.Background(), "plg-nonet")
	_, err = interceptor(ctxNo, nil, info, handler)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestPluginInterceptor_MissingPluginID(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)

	interceptor := grpcapi.PluginPermissionInterceptor(checker)

	ctx := context.Background()
	handler := func(_ context.Context, _ any) (any, error) { return "ok", nil }

	info := &grpclib.UnaryServerInfo{FullMethod: "/plugin.v1.HostService/GetMediaItem"}
	_, err := interceptor(ctx, nil, info, handler)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

// TestPluginInterceptor_UnknownMethod_Denied locks the fail-closed
// behavior in: any RPC missing from methodPermissions is denied. Adding
// a new HostService RPC without registering it must NOT silently bypass
// the permission gate.
func TestPluginInterceptor_UnknownMethod_Denied(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)

	interceptor := grpcapi.PluginPermissionInterceptor(checker)

	ctx := pluginhost.CtxWithPluginID(context.Background(), "plg-1")
	called := false
	handler := func(_ context.Context, _ any) (any, error) {
		called = true
		return "ok", nil
	}

	info := &grpclib.UnaryServerInfo{FullMethod: "/plugin.v1.HostService/NotARealMethod"}
	_, err := interceptor(ctx, nil, info, handler)
	require.Error(t, err)
	assert.False(t, called, "handler must not run for unknown method")

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestPluginInterceptor_UpdateMetadata_RequiresPermission(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)
	checker.LoadPermissions("plg-w", []string{"media.write"})

	interceptor := grpcapi.PluginPermissionInterceptor(checker)

	ctx := pluginhost.CtxWithPluginID(context.Background(), "plg-w")

	called := false
	handler := func(_ context.Context, _ any) (any, error) {
		called = true
		return "ok", nil
	}

	info := &grpclib.UnaryServerInfo{FullMethod: "/plugin.v1.HostService/UpdateMetadata"}
	_, err := interceptor(ctx, nil, info, handler)
	require.NoError(t, err)
	assert.True(t, called)
}

func TestPluginInterceptor_GetUserPreferences_RequiresApiUser(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)
	checker.LoadPermissions("plg-no", []string{"api.library"})

	interceptor := grpcapi.PluginPermissionInterceptor(checker)

	ctx := pluginhost.CtxWithPluginID(context.Background(), "plg-no")

	handler := func(_ context.Context, _ any) (any, error) { return "ok", nil }

	info := &grpclib.UnaryServerInfo{FullMethod: "/plugin.v1.HostService/GetUserPreferences"}
	_, err := interceptor(ctx, nil, info, handler)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}
