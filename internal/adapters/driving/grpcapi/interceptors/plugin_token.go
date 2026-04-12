package interceptors

import (
	"context"
	"log/slog"
	"strings"

	authpkg "github.com/gravity-zero/evey/internal/pkg/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// pluginClaimsKey stores *authpkg.PluginClaims in context for scoped plugin tokens.
type pluginClaimsKey struct{}

// PluginTokenParser validates scoped plugin JWTs.
type PluginTokenParser interface {
	ParsePluginToken(raw string) (*authpkg.PluginClaims, error)
}

// PluginTokenInterceptor parses Authorization header; if it is a plugin token
// (token_type="plugin"), it populates ctx with *authpkg.PluginClaims and bypasses
// user-auth. If the token is not a plugin token, the request falls through to
// the next interceptor untouched.
type PluginTokenInterceptor struct {
	parser PluginTokenParser
}

func NewPluginTokenInterceptor(parser PluginTokenParser) *PluginTokenInterceptor {
	return &PluginTokenInterceptor{parser: parser}
}

func (i *PluginTokenInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if claims := i.maybeParse(ctx); claims != nil {
			ctx = context.WithValue(ctx, pluginClaimsKey{}, claims)
		}
		return handler(ctx, req)
	}
}

func (i *PluginTokenInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		if claims := i.maybeParse(ctx); claims != nil {
			ctx = context.WithValue(ctx, pluginClaimsKey{}, claims)
			return handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
		}
		return handler(srv, ss)
	}
}

func (i *PluginTokenInterceptor) maybeParse(ctx context.Context) *authpkg.PluginClaims {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil
	}
	vals := md.Get("authorization")
	if len(vals) == 0 {
		return nil
	}
	raw := strings.TrimPrefix(vals[0], "Bearer ")
	if raw == "" || strings.HasPrefix(raw, "evey_") {
		return nil
	}
	claims, err := i.parser.ParsePluginToken(raw)
	if err != nil {
		// Not a plugin token (or invalid) — silently skip; user auth interceptor handles it.
		return nil
	}
	slog.Debug("plugin token authenticated", slog.String("plugin_id", claims.PluginID), slog.String("user_id", claims.UserID))
	return claims
}

// PluginClaimsFromContext returns the scoped plugin claims stored in ctx, if any.
func PluginClaimsFromContext(ctx context.Context) *authpkg.PluginClaims {
	v, _ := ctx.Value(pluginClaimsKey{}).(*authpkg.PluginClaims)
	return v
}

// ScopeFromCtx returns the scopes granted by a scoped plugin token in ctx, if any.
func ScopeFromCtx(ctx context.Context) ([]string, bool) {
	c := PluginClaimsFromContext(ctx)
	if c == nil {
		return nil, false
	}
	return c.Scopes, true
}

// RequireScope returns a PermissionDenied error if the scoped plugin claims in
// ctx do not grant the named scope. If ctx has no plugin claims, returns nil
// (user auth applies instead).
func RequireScope(ctx context.Context, scope string) error {
	c := PluginClaimsFromContext(ctx)
	if c == nil {
		return nil
	}
	if !c.HasScope(scope) {
		return status.Errorf(codes.PermissionDenied, "plugin token lacks scope %q", scope)
	}
	return nil
}
