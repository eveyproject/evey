package interceptors

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"strings"
	"time"

	authpkg "github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type claimsKey struct{}
type apiKeyAuthKey struct{}

type AuthInterceptor struct {
	tokenGen   user.TokenGenerator
	apiKeys    user.APIKeyRepository
	users      user.UserRepository
	rbac       RBACConfig
	translator *eveyi18n.Translator
}

func NewAuthInterceptor(tokenGen user.TokenGenerator) *AuthInterceptor {
	return &AuthInterceptor{tokenGen: tokenGen, rbac: DefaultRBACConfig()}
}


func (i *AuthInterceptor) WithTranslator(t *eveyi18n.Translator) *AuthInterceptor {
	i.translator = t
	return i
}

func (i *AuthInterceptor) WithRBAC(rbac RBACConfig) *AuthInterceptor {
	i.rbac = rbac
	return i
}

func (i *AuthInterceptor) WithAPIKeys(apiKeys user.APIKeyRepository, users user.UserRepository) *AuthInterceptor {
	i.apiKeys = apiKeys
	i.users = users
	return i
}

func (i *AuthInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if i.rbac.PublicMethods[info.FullMethod] {
			return handler(ctx, req)
		}
		if PluginClaimsFromContext(ctx) != nil {
			// Authenticated via scoped plugin token — skip user-auth. Scope enforcement is per-RPC.
			return handler(ctx, req)
		}
		newCtx, err := i.authenticate(ctx)
		if err != nil {
			return nil, err
		}
		if err := i.enforceRBAC(newCtx, info.FullMethod); err != nil {
			return nil, err
		}
		return handler(newCtx, req)
	}
}

func (i *AuthInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if i.rbac.PublicMethods[info.FullMethod] {
			return handler(srv, ss)
		}
		if PluginClaimsFromContext(ss.Context()) != nil {
			return handler(srv, ss)
		}
		newCtx, err := i.authenticate(ss.Context())
		if err != nil {
			return err
		}
		if err := i.enforceRBAC(newCtx, info.FullMethod); err != nil {
			return err
		}
		wrapped := &wrappedStream{ServerStream: ss, ctx: newCtx}
		return handler(srv, wrapped)
	}
}

func (i *AuthInterceptor) enforceRBAC(ctx context.Context, method string) error {
	claims := ClaimsFromContext(ctx)
	if claims == nil {
		return i.translatedErr(ctx, codes.PermissionDenied, "errors.auth.admin_access_required")
	}

	isAPIKey := IsAPIKeyAuth(ctx)

	// Block API keys from internal methods (infra/ops/auth — browser-only)
	if isAPIKey && authpkg.InternalMethods[method] {
		slog.Warn("api key blocked on internal method", slog.String("method", method), slog.String("user", claims.UserID))
		return i.translatedErr(ctx, codes.PermissionDenied, "errors.auth.internal_method")
	}

	role, exists := i.rbac.MethodRoles[method]
	if exists && role == RoleAdmin {
		if !claims.IsAdmin {
			return i.translatedErr(ctx, codes.PermissionDenied, "errors.auth.admin_access_required")
		}
	}

	// Enforce API key permissions: empty permissions = deny-all.
	if isAPIKey {
		if len(claims.Permissions) == 0 {
			slog.Warn("api key permission denied (no permissions)", slog.String("method", method))
			return i.translatedErr(ctx, codes.PermissionDenied, "errors.auth.insufficient_permissions")
		}
		if requiredPerm, hasMapping := authpkg.MethodPermissions[method]; hasMapping && !claims.HasPermission(requiredPerm) {
			slog.Warn("api key permission denied", slog.String("method", method), slog.String("required", requiredPerm))
			return i.translatedErr(ctx, codes.PermissionDenied, "errors.auth.insufficient_permissions")
		}
	}

	return nil
}

func (i *AuthInterceptor) authenticate(ctx context.Context) (context.Context, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, i.translatedErr(ctx, codes.Unauthenticated, "errors.auth.missing_metadata")
	}
	vals := md.Get("authorization")
	if len(vals) == 0 {
		return nil, i.translatedErr(ctx, codes.Unauthenticated, "errors.auth.missing_authorization")
	}
	raw := vals[0]
	// API key auth: "evey_..." (without "Bearer " prefix)
	trimmed := strings.TrimPrefix(raw, "Bearer ")
	if strings.HasPrefix(trimmed, "evey_") && i.apiKeys != nil && i.users != nil {
		return i.authenticateAPIKey(ctx, trimmed)
	}
	claims, err := i.tokenGen.ValidateAccessToken(trimmed)
	if err != nil {
		return nil, i.translatedErr(ctx, codes.Unauthenticated, "errors.auth.invalid_token")
	}
	return context.WithValue(ctx, claimsKey{}, claims), nil
}

func (i *AuthInterceptor) authenticateAPIKey(ctx context.Context, rawKey string) (context.Context, error) {
	h := sha256.Sum256([]byte(rawKey))
	hash := hex.EncodeToString(h[:])
	key, err := i.apiKeys.GetByKeyHash(ctx, hash)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid api key")
	}
	if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
		return nil, status.Error(codes.Unauthenticated, "api key expired")
	}
	u, err := i.users.GetByID(ctx, shared.UserID(key.UserID))
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid api key")
	}
	if !u.Enabled {
		return nil, status.Error(codes.Unauthenticated, "user disabled")
	}
	claims := &user.Claims{
		UserID:      string(u.ID),
		Username:    u.Username,
		IsAdmin:     u.IsAdmin,
		Permissions: key.Permissions,
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("recovered panic in api key last used update", "panic", r)
			}
		}()
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if upErr := i.apiKeys.UpdateLastUsed(bgCtx, key.ID, time.Now()); upErr != nil {
			slog.Warn("update api key last used", slog.String("keyID", key.ID), slog.Any("err", upErr))
		}
	}()
	ctx = context.WithValue(ctx, claimsKey{}, claims)
	ctx = context.WithValue(ctx, apiKeyAuthKey{}, true)
	return ctx, nil
}

// IsAPIKeyAuth returns true if the request was authenticated via API key.
func IsAPIKeyAuth(ctx context.Context) bool {
	v, _ := ctx.Value(apiKeyAuthKey{}).(bool)
	return v
}

func (i *AuthInterceptor) translatedErr(ctx context.Context, code codes.Code, key string) error {
	if i.translator != nil {
		return i.translator.TranslatedError(ctx, code, key)
	}
	return status.Error(code, key)
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
