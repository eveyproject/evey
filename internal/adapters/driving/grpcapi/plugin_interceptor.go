package grpcapi

import (
	"context"
	"log/slog"

	"github.com/evey-project/evey/internal/adapters/driving/pluginhost"
	appplugin "github.com/evey-project/evey/internal/app/plugin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var methodPermissions = map[string][]string{
	"/plugin.v1.HostService/GetMediaItem":       {"api.library", "media.read"},
	"/plugin.v1.HostService/ListMediaItems":     {"api.library", "media.read"},
	"/plugin.v1.HostService/UpdateMetadata":     {"api.metadata", "media.write"},
	"/plugin.v1.HostService/GetUserPreferences": {"api.user"},
	"/plugin.v1.HostService/SearchMedia":        {"api.library", "media.read"},
	"/plugin.v1.HostService/GetMediaStreams":    {"api.library", "media.read"},
	"/plugin.v1.HostService/GetChapters":        {"api.library", "media.read"},
	"/plugin.v1.HostService/GetSubtitles":       {"api.library", "media.read"},
	"/plugin.v1.HostService/SendNotification":   {"notification.sender"},
	"/plugin.v1.HostService/EmitEvent":          nil,
	"/plugin.v1.HostService/Log":                nil,
	"/plugin.v1.HostService/GetConfig":          nil,
	"/plugin.v1.HostService/SetConfig":          nil,
}

// PluginPermissionInterceptor returns a gRPC UnaryServerInterceptor that
// enforces plugin permissions on HostService RPCs. It MUST be chained
// after pluginhost's per-plugin SO_PEERCRED auth interceptor — that one
// injects the authenticated plugin ID into the context. Reading from the
// wire (e.g. an x-plugin-id header) would be trivially spoofable.
func PluginPermissionInterceptor(checker *appplugin.PermissionChecker) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		requiredPerms, known := methodPermissions[info.FullMethod]
		if !known {
			return handler(ctx, req)
		}
		if requiredPerms == nil {
			return handler(ctx, req)
		}

		pluginID := pluginhost.PluginIDFromCtx(ctx)
		if pluginID == "" {
			return nil, status.Error(codes.Unauthenticated, "plugin not authenticated")
		}

		if checkErr := checker.CheckAnyPermission(pluginID, requiredPerms...); checkErr != nil {
			slog.Warn("plugin RPC permission denied",
				slog.String("plugin_id", pluginID),
				slog.String("method", info.FullMethod))
			return nil, status.Errorf(codes.PermissionDenied,
				"plugin %s lacks permission for %s", pluginID, info.FullMethod)
		}

		return handler(ctx, req)
	}
}
