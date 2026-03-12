package interceptors

import (
	"context"
	"strings"

	"github.com/gravity-zero/evey/internal/domain/user"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type claimsKey struct{}

var publicMethods = map[string]bool{
	"/evey.v1.UserService/Login":              true,
	"/evey.v1.UserService/RefreshToken":       true,
	"/evey.v1.UserService/RegisterWithInvite": true,
	"/evey.v1.AdminService/SetupWizard":       true,
	"/evey.v1.AdminService/GetSystemInfo":     true,
	"/evey.v1.UserService/CheckQuickConnect":  true,
}

var adminOnlyMethods = map[string]bool{
	"/evey.v1.AdminService/GetServerConfig":                         true,
	"/evey.v1.AdminService/UpdateServerConfig":                      true,
	"/evey.v1.AdminService/GetDashboard":                            true,
	"/evey.v1.AdminService/ListScheduledTasks":                      true,
	"/evey.v1.AdminService/UpdateScheduledTask":                     true,
	"/evey.v1.AdminService/RunScheduledTask":                        true,
	"/evey.v1.AdminService/GetAuditLog":                             true,
	"/evey.v1.AdminService/ListPluginRuntimes":                      true,
	"/evey.v1.AdminService/AddPluginRuntime":                        true,
	"/evey.v1.AdminService/RemovePluginRuntime":                     true,
	"/evey.v1.AdminService/InstallPlugin":                           true,
	"/evey.v1.AdminService/UninstallPlugin":                         true,
	"/evey.v1.AdminService/EnablePlugin":                            true,
	"/evey.v1.AdminService/DisablePlugin":                           true,
	"/evey.v1.AdminService/ListPlugins":                             true,
	"/evey.v1.AdminService/GetPluginConfig":                         true,
	"/evey.v1.AdminService/UpdatePluginConfig":                      true,
	"/evey.v1.AdminService/GetPluginLogs":                           true,
	"/evey.v1.WebhookService/CreateWebhook":                         true,
	"/evey.v1.WebhookService/ListWebhooks":                          true,
	"/evey.v1.WebhookService/UpdateWebhook":                         true,
	"/evey.v1.WebhookService/DeleteWebhook":                         true,
	"/evey.v1.WebhookService/ListWebhookDeliveries":                 true,
	"/evey.v1.UserService/CreateUser":                               true,
	"/evey.v1.UserService/DeleteUser":                               true,
	"/evey.v1.UserService/DisableUser":                              true,
	"/evey.v1.UserService/ListUsers":                                true,
	"/evey.v1.LibraryService/CreateLibrary":                         true,
	"/evey.v1.LibraryService/DeleteLibrary":                         true,
	"/evey.v1.LibraryService/ScanLibrary":                           true,
	"/evey.v1.FeedService/GetAdminFeedConfigs":                      true,
	"/evey.v1.FeedService/UpdateAdminFeedConfig":                    true,
	"/evey.v1.NotificationService/GetGlobalNotificationSettings":    true,
	"/evey.v1.NotificationService/UpdateGlobalNotificationSettings": true,
	"/evey.v1.ActivityService/GetServerStats":                       true,
	"/evey.v1.ActivityService/ListServerMetrics":                    true,
	"/evey.v1.MetadataService/RefreshMetadata":                      true,
	"/evey.v1.MetadataService/SetMetadataOverride":                  true,
	"/evey.v1.MetadataService/RemoveMetadataOverride":               true,
	"/evey.v1.MKVService/MuxContainer":                              true,
	"/evey.v1.MKVService/DemuxContainer":                            true,
	"/evey.v1.MKVService/AddTrack":                                  true,
	"/evey.v1.MKVService/RemoveTrack":                               true,
	"/evey.v1.TranscodingService/StopTranscode":                     true,
	"/evey.v1.TranscodingService/ListActiveTranscodes":              true,
}

type AuthInterceptor struct {
	tokenGen user.TokenGenerator
}

func NewAuthInterceptor(tokenGen user.TokenGenerator) *AuthInterceptor {
	return &AuthInterceptor{tokenGen: tokenGen}
}

func (i *AuthInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if publicMethods[info.FullMethod] {
			return handler(ctx, req)
		}
		newCtx, err := i.authenticate(ctx)
		if err != nil {
			return nil, err
		}
		if err := enforceRBAC(newCtx, info.FullMethod); err != nil {
			return nil, err
		}
		return handler(newCtx, req)
	}
}

func (i *AuthInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if publicMethods[info.FullMethod] {
			return handler(srv, ss)
		}
		newCtx, err := i.authenticate(ss.Context())
		if err != nil {
			return err
		}
		if err := enforceRBAC(newCtx, info.FullMethod); err != nil {
			return err
		}
		wrapped := &wrappedStream{ServerStream: ss, ctx: newCtx}
		return handler(srv, wrapped)
	}
}

func enforceRBAC(ctx context.Context, method string) error {
	if !adminOnlyMethods[method] {
		return nil
	}
	claims := ClaimsFromContext(ctx)
	if claims == nil || !claims.IsAdmin {
		return status.Error(codes.PermissionDenied, "admin access required")
	}
	return nil
}

func (i *AuthInterceptor) authenticate(ctx context.Context) (context.Context, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}
	vals := md.Get("authorization")
	if len(vals) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing authorization")
	}
	token := strings.TrimPrefix(vals[0], "Bearer ")
	claims, err := i.tokenGen.ValidateAccessToken(token)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid token")
	}
	return context.WithValue(ctx, claimsKey{}, claims), nil
}

func ClaimsFromContext(ctx context.Context) *user.Claims {
	v, _ := ctx.Value(claimsKey{}).(*user.Claims)
	return v
}

func ContextWithClaims(ctx context.Context, claims *user.Claims) context.Context {
	return context.WithValue(ctx, claimsKey{}, claims)
}

type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context {
	return w.ctx
}
