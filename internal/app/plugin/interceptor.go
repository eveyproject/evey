package plugin

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const pluginIDHeader = "x-plugin-id"

// methodPermissions maps HostService RPC full methods to required permissions.
// A nil slice means always allowed (no permission check).
// Multiple entries mean the plugin needs at least one of them.
var methodPermissions = map[string][]string{
	"/plugin.v1.HostService/GetMediaItem":       {"api.library", "media.read"},
	"/plugin.v1.HostService/ListMediaItems":     {"api.library", "media.read"},
	"/plugin.v1.HostService/UpdateMetadata":     {"api.metadata", "media.write"},
	"/plugin.v1.HostService/GetUserPreferences": {"api.user"},
	"/plugin.v1.HostService/SearchMedia":        {"api.library", "media.read"},
	"/plugin.v1.HostService/GetMediaStreams":     {"api.library", "media.read"},
	"/plugin.v1.HostService/GetChapters":         {"api.library", "media.read"},
	"/plugin.v1.HostService/GetSubtitles":        {"api.library", "media.read"},
	"/plugin.v1.HostService/SendNotification":    {"notification.sender"},
	// Always allowed — nil means skip check
	"/plugin.v1.HostService/EmitEvent":  nil,
	"/plugin.v1.HostService/Log":        nil,
	"/plugin.v1.HostService/GetConfig":  nil,
	"/plugin.v1.HostService/SetConfig":  nil,
}

// PluginPermissionInterceptor returns a gRPC UnaryServerInterceptor that
// enforces plugin permissions on HostService RPCs.
func PluginPermissionInterceptor(checker *PermissionChecker) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		requiredPerms, known := methodPermissions[info.FullMethod]
		if !known {
			return handler(ctx, req)
		}
		// nil means always allowed
		if requiredPerms == nil {
			return handler(ctx, req)
		}

		pluginID, err := extractPluginID(ctx)
		if err != nil {
			return nil, err
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

func extractPluginID(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.InvalidArgument, "missing metadata")
	}
	vals := md.Get(pluginIDHeader)
	if len(vals) == 0 || vals[0] == "" {
		return "", status.Error(codes.InvalidArgument, "missing x-plugin-id header")
	}
	return vals[0], nil
}
