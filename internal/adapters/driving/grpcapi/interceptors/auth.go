package interceptors

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"strings"
	"time"

	authpkg "github.com/evey-project/evey/internal/pkg/auth"
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
		if claims := PluginClaimsFromContext(ctx); claims != nil {
			if err := i.authorizePluginToken(ctx, claims, info.FullMethod); err != nil {
				return nil, err
			}
			return handler(pluginUserContext(ctx, claims), req)
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
		if claims := PluginClaimsFromContext(ss.Context()); claims != nil {
			if err := i.authorizePluginToken(ss.Context(), claims, info.FullMethod); err != nil {
				return err
			}
			wrapped := &wrappedStream{ServerStream: ss, ctx: pluginUserContext(ss.Context(), claims)}
			return handler(srv, wrapped)
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

// authorizePluginToken enforces what a scoped plugin token may call on the
// public API. It is DENY-BY-DEFAULT: a plugin token may only reach a method
// explicitly listed in PluginMethodScopes, and only if it carries the required
// scope. Admin methods are rejected up front as an explicit floor (defense in
// depth — they must never be in the allowlist either). A plugin token is
// mintable by any authenticated user, so this is the boundary that makes a
// granted plugin permission actually enforced rather than cosmetic.
func (i *AuthInterceptor) authorizePluginToken(ctx context.Context, claims *authpkg.PluginClaims, method string) error {
	if role, ok := i.rbac.MethodRoles[method]; ok && role == RoleAdmin {
		return i.translatedErr(ctx, codes.PermissionDenied, "errors.auth.admin_access_required")
	}
	scope, allowed := i.rbac.PluginMethodScopes[method]
	if !allowed {
		slog.Warn("plugin token denied: method not in plugin allowlist",
			slog.String("method", method), slog.String("plugin", claims.PluginID))
		return i.translatedErr(ctx, codes.PermissionDenied, "errors.auth.insufficient_permissions")
	}
	if scope != "" && !claims.HasScope(scope) {
		slog.Warn("plugin token denied: missing scope",
			slog.String("method", method), slog.String("required_scope", scope), slog.String("plugin", claims.PluginID))
		return i.translatedErr(ctx, codes.PermissionDenied, "errors.auth.insufficient_permissions")
	}
	return nil
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

	// Enforce API key scopes. Empty scopes = allow-all (legacy keys).
	// A key with scopes is authorized for a method if it holds either the coarse
	// scope (APIKeyScopeFor, colon notation) or the fine-grained legacy permission
	// (MethodPermissions, dot notation).
	if isAPIKey && len(claims.Permissions) > 0 {
		scope, hasScope := authpkg.APIKeyScopeFor[method]
		perm, hasPerm := authpkg.MethodPermissions[method]
		if hasScope || hasPerm {
			ok := (hasScope && claims.HasPermission(scope)) || (hasPerm && claims.HasPermission(perm))
			if !ok {
				slog.Warn("api key scope denied", slog.String("method", method), slog.String("required_scope", scope), slog.String("required_perm", perm))
				return i.translatedErr(ctx, codes.PermissionDenied, "errors.auth.insufficient_permissions")
			}
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
	u, err := i.users.GetByID(ctx, key.UserID)
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

// pluginUserContext injects a minimal NON-ADMIN user.Claims derived from the
// plugin token's minting user/profile, so the handlers a plugin may reach
// (the public media reads) enforce that user's library ACL and the active
// profile's parental filter. Without it those handlers see claims==nil and
// treat the caller as unrestricted/admin - a restricted user's plugin could
// list media from libraries they cannot access and above the profile's age
// limit. Deliberately IsAdmin=false: a plugin token must never confer admin.
func pluginUserContext(ctx context.Context, pc *authpkg.PluginClaims) context.Context {
	return ContextWithClaims(ctx, &user.Claims{
		UserID:    pc.UserID,
		ProfileID: pc.ProfileID,
		IsAdmin:   false,
	})
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
