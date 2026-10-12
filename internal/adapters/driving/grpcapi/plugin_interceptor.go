package grpcapi

import (
	"context"
	"log/slog"

	"github.com/eveyproject/evey/internal/adapters/driving/pluginhost"
	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// methodPermissions enumerates every HostService RPC the runtime
// accepts. Adding a new RPC to plugin.proto WITHOUT adding it here is a
// build-time omission the interceptor catches at runtime: unknown
// methods are denied (fail-closed). nil values mean "no permission
// required" - still authenticated, but any installed plugin can call.
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
	// Data store: a plugin's own namespace - no special permission required.
	"/plugin.v1.HostService/GetData":      nil,
	"/plugin.v1.HostService/SetData":      nil,
	"/plugin.v1.HostService/DeleteData":   nil,
	"/plugin.v1.HostService/ListDataKeys": nil,
	// Brokered outbound fetch: only plugins that declared (and were granted)
	// the "network" permission may reach the network, and only through the
	// host's SSRF-safe, allow-list-checked broker.
	"/plugin.v1.HostService/Fetch": {"network"},
}

// PluginPermissionInterceptor returns a gRPC UnaryServerInterceptor that
// enforces plugin permissions on HostService RPCs. It MUST be chained
// after pluginhost's per-plugin SO_PEERCRED auth interceptor - that one
// injects the authenticated plugin ID into the context. Reading from the
// wire (e.g. an x-plugin-id header) would be trivially spoofable.
//
// Fail-closed on unknown methods: if a new RPC ships in plugin.proto
// without an entry in methodPermissions, every call to it is denied
// rather than passed through unauthenticated.
func PluginPermissionInterceptor(checker *appplugin.PermissionChecker) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		requiredPerms, known := methodPermissions[info.FullMethod]
		if !known {
			slog.WarnContext(ctx, "plugin RPC denied: unknown method (missing methodPermissions entry)",
				slog.String("method", info.FullMethod))
			return nil, status.Errorf(codes.PermissionDenied, "method %s not permitted for plugins", info.FullMethod)
		}
		if requiredPerms == nil {
			return handler(ctx, req)
		}

		pluginID := pluginhost.PluginIDFromCtx(ctx)
		if pluginID == "" {
			return nil, status.Error(codes.Unauthenticated, "plugin not authenticated")
		}

		if checkErr := checker.CheckAnyPermission(pluginID, requiredPerms...); checkErr != nil {
			slog.WarnContext(ctx, "plugin RPC permission denied",
				slog.String("plugin_id", pluginID),
				slog.String("method", info.FullMethod))
			return nil, status.Errorf(codes.PermissionDenied,
				"plugin %s lacks permission for %s", pluginID, info.FullMethod)
		}

		return handler(ctx, req)
	}
}
