package plugin_test

import (
	"context"
	"testing"

	appplugin "github.com/gravity-zero/evey/internal/app/plugin"
	plgfakes "github.com/gravity-zero/evey/internal/domain/plugin/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestCheckPermission_Allowed(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)
	checker.LoadPermissions("plg-1", []string{"api.library", "media.read"})

	err := checker.CheckPermission("plg-1", "api.library")
	assert.NoError(t, err)
}

func TestCheckPermission_Denied(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)
	checker.LoadPermissions("plg-1", []string{"api.library"})

	err := checker.CheckPermission("plg-1", "media.write")
	assert.ErrorIs(t, err, appplugin.ErrPermissionDenied)
}

func TestCheckPermission_UnknownPlugin(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)

	err := checker.CheckPermission("plg-unknown", "api.library")
	assert.ErrorIs(t, err, appplugin.ErrPermissionDenied)
}

func TestCheckAnyPermission_OneMatch(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)
	checker.LoadPermissions("plg-1", []string{"media.read"})

	err := checker.CheckAnyPermission("plg-1", "api.library", "media.read")
	assert.NoError(t, err)
}

func TestCheckAnyPermission_NoneMatch(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)
	checker.LoadPermissions("plg-1", []string{"api.user"})

	err := checker.CheckAnyPermission("plg-1", "api.library", "media.read")
	assert.ErrorIs(t, err, appplugin.ErrPermissionDenied)
}

func TestInvalidatePermissions(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)
	checker.LoadPermissions("plg-1", []string{"api.library"})

	require.NoError(t, checker.CheckPermission("plg-1", "api.library"))

	checker.InvalidatePermissions("plg-1")
	err := checker.CheckPermission("plg-1", "api.library")
	assert.ErrorIs(t, err, appplugin.ErrPermissionDenied)
}

func TestInterceptor_AllowedRPC(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)
	checker.LoadPermissions("plg-1", []string{"api.library"})

	interceptor := appplugin.PluginPermissionInterceptor(checker)

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("x-plugin-id", "plg-1"))

	called := false
	handler := func(_ context.Context, _ any) (any, error) {
		called = true
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/plugin.v1.HostService/GetMediaItem"}
	resp, err := interceptor(ctx, nil, info, handler)
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, "ok", resp)
}

func TestInterceptor_DeniedRPC(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)
	checker.LoadPermissions("plg-1", []string{"api.user"})

	interceptor := appplugin.PluginPermissionInterceptor(checker)

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("x-plugin-id", "plg-1"))

	called := false
	handler := func(_ context.Context, _ any) (any, error) {
		called = true
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/plugin.v1.HostService/GetMediaItem"}
	_, err := interceptor(ctx, nil, info, handler)
	require.Error(t, err)
	assert.False(t, called)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestInterceptor_AlwaysAllowedRPC(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)
	// no permissions loaded at all
	checker.LoadPermissions("plg-1", nil)

	interceptor := appplugin.PluginPermissionInterceptor(checker)

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("x-plugin-id", "plg-1"))

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

		info := &grpc.UnaryServerInfo{FullMethod: method}
		_, err := interceptor(ctx, nil, info, handler)
		require.NoError(t, err, "method %s should be allowed", method)
		assert.True(t, called, "handler should be called for %s", method)
	}
}

func TestInterceptor_MissingPluginID(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)

	interceptor := appplugin.PluginPermissionInterceptor(checker)

	ctx := context.Background()
	handler := func(_ context.Context, _ any) (any, error) { return "ok", nil }

	info := &grpc.UnaryServerInfo{FullMethod: "/plugin.v1.HostService/GetMediaItem"}
	_, err := interceptor(ctx, nil, info, handler)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestInterceptor_UnknownMethod_PassesThrough(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)

	interceptor := appplugin.PluginPermissionInterceptor(checker)

	ctx := context.Background()
	called := false
	handler := func(_ context.Context, _ any) (any, error) {
		called = true
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/some.other.Service/Method"}
	_, err := interceptor(ctx, nil, info, handler)
	require.NoError(t, err)
	assert.True(t, called)
}

func TestInterceptor_UpdateMetadata_RequiresPermission(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)
	checker.LoadPermissions("plg-w", []string{"media.write"})

	interceptor := appplugin.PluginPermissionInterceptor(checker)

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("x-plugin-id", "plg-w"))

	called := false
	handler := func(_ context.Context, _ any) (any, error) {
		called = true
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/plugin.v1.HostService/UpdateMetadata"}
	_, err := interceptor(ctx, nil, info, handler)
	require.NoError(t, err)
	assert.True(t, called)
}

func TestInterceptor_GetUserPreferences_RequiresApiUser(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)
	checker.LoadPermissions("plg-no", []string{"api.library"})

	interceptor := appplugin.PluginPermissionInterceptor(checker)

	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("x-plugin-id", "plg-no"))

	handler := func(_ context.Context, _ any) (any, error) { return "ok", nil }

	info := &grpc.UnaryServerInfo{FullMethod: "/plugin.v1.HostService/GetUserPreferences"}
	_, err := interceptor(ctx, nil, info, handler)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}
