package interceptors

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"log/slog"
	"strings"
	"time"

	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/domain/user"
	authpkg "github.com/eveyproject/evey/internal/pkg/auth"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"github.com/eveyproject/evey/internal/pkg/metrics"
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
// depth - they must never be in the allowlist either). A plugin token is
// mintable by any authenticated user, so this is the boundary that makes a
// granted plugin permission actually enforced rather than cosmetic.
func (i *AuthInterceptor) authorizePluginToken(ctx context.Context, claims *authpkg.PluginClaims, method string) error {
	if role, ok := i.rbac.MethodRoles[method]; ok && role == RoleAdmin {
		return i.translatedErr(ctx, codes.PermissionDenied, "errors.auth.admin_access_required")
	}
	scope, allowed := i.rbac.PluginMethodScopes[method]
	if !allowed {
		slog.WarnContext(ctx, "plugin token denied: method not in plugin allowlist",
			slog.String("method", method), slog.String("plugin", claims.PluginID))
		return i.translatedErr(ctx, codes.PermissionDenied, "errors.auth.insufficient_permissions")
	}
	if scope != "" && !claims.HasScope(scope) {
		slog.WarnContext(ctx, "plugin token denied: missing scope",
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

	role, exists := i.rbac.MethodRoles[method]
	isAdminMethod := exists && role == RoleAdmin
	if isAdminMethod && !claims.IsAdmin {
		// Refusing in silence is what made the 2026-07-29 lockout take two days:
		// a client that cannot get in and a server that says nothing. Every
		// neighbouring refusal in this file already logs; this one did not.
		metrics.AuthzDeniedTotal.WithLabelValues("grpc", "admin_required").Inc()
		slog.WarnContext(ctx, "admin access denied",
			slog.String("method", method),
			slog.String("user", claims.UserID))
		return i.translatedErr(ctx, codes.PermissionDenied, "errors.auth.admin_access_required")
	}

	if isAPIKey {
		// 1. Hard block list: auth / session / user-lifecycle / infra RPCs are
		//    never reachable by an API key, even one whose user is admin.
		if authpkg.InternalMethods[method] {
			slog.WarnContext(ctx, "api key blocked on internal method", slog.String("method", method), slog.String("user", claims.UserID))
			return i.translatedErr(ctx, codes.PermissionDenied, "errors.auth.internal_method")
		}

		scope, hasScope := authpkg.APIKeyScopeFor[method]
		perm, hasPerm := authpkg.MethodPermissions[method]

		// 2. Fail-closed default: a method with no entry in APIKeyScopeFor or
		//    MethodPermissions is REFUSED to every API key outright, whatever
		//    scopes/permissions it carries and whether or not it is admin-only.
		//    Until 2026-08-01 this only applied to admin methods (isAdminMethod
		//    && !hasScope && !hasPerm) - a method that defaulted to RoleUser and
		//    named in no table sailed straight through step 3 below, because
		//    that step's own guard condition (hasScope || hasPerm) was false and
		//    so it never ran. 100 real RPCs were in that state, including
		//    ParentalService/UpdateParentalSettings and VerifyPin: a "read only"
		//    key could disable parental controls. Classifying a method here is
		//    now mandatory, not opt-in - TestAPIKeyTables_NoUnclassifiedRealMethods
		//    (rbac_test.go) fails the build the moment a new RPC ships unclassified.
		if !hasScope && !hasPerm {
			slog.WarnContext(ctx, "api key blocked: method has no scope/permission entry (deny-by-default)",
				slog.String("method", method), slog.String("user", claims.UserID))
			return i.translatedErr(ctx, codes.PermissionDenied, "errors.auth.internal_method")
		}

		// 3. Enforce the scope. Admin methods ALWAYS enforce it, so an
		//    empty-scope "legacy allow-all" key gets no admin reach; non-admin
		//    methods keep the legacy empty-scope allow-all for the key owner's
		//    own domain resources (a classified method with no permissions on
		//    the key is not the same failure as an unclassified method - the
		//    former is a known pre-scope key, the latter is the gap step 2
		//    closes). A key holding either the coarse scope (APIKeyScopeFor,
		//    colon notation) or the fine-grained permission (MethodPermissions,
		//    dot notation) is authorized.
		//    An admin:* scope is enforced like an admin method: GetAnalytics,
		//    the library schedules and their kin default to RoleUser and are
		//    admin-only in the handler (claims.IsAdmin), so an admin-owned
		//    legacy key used to reach them with no scope at all.
		adminScope := scope == authpkg.ScopeAdminRead || scope == authpkg.ScopeAdminWrite
		if isAdminMethod || adminScope || len(claims.Permissions) > 0 {
			ok := (hasScope && claims.HasPermission(scope)) || (hasPerm && claims.HasPermission(perm))
			if !ok {
				slog.WarnContext(ctx, "api key scope denied", slog.String("method", method), slog.String("required_scope", scope), slog.String("required_perm", perm))
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
		// Same distinction as the HTTP middleware: a token that cannot be
		// CHECKED is not a token that is wrong. An unreadable epoch means the
		// database is not answering and every authenticated RPC is about to
		// fail identically - the client still gets the generic Unauthenticated
		// (it must learn nothing more), but the server says out loud where to
		// look, because "invalid token" sends the operator to the JWT key.
		if errors.Is(err, appuser.ErrEpochUnavailable) {
			metrics.AuthFailuresTotal.WithLabelValues("epoch_lookup_failed").Inc()
			slog.ErrorContext(ctx, "auth rejected: the token epoch could not be read, so every authenticated request is failing - check the database, not the JWT key",
				slog.Any("cause", err))
		}
		return nil, i.translatedErr(ctx, codes.Unauthenticated, "errors.auth.invalid_token")
	}
	ctx = activity.WithImpersonator(ctx, claims.ImpersonatedBy)
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
	// Throttle the last-used write: a plugin polling by API key would otherwise
	// write api_keys (and spawn a goroutine) on EVERY request. Minute-level
	// freshness is plenty for a "last used" column, so skip the write when it is
	// already recent - this cuts the write amplification to at most one per key
	// per window.
	if shouldWriteLastUsed(key.LastUsedAt, time.Now(), apiKeyLastUsedThrottle) {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.ErrorContext(ctx, "recovered panic in api key last used update", "panic", r)
				}
			}()
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if upErr := i.apiKeys.UpdateLastUsed(bgCtx, key.ID, time.Now()); upErr != nil {
				slog.WarnContext(bgCtx, "update api key last used", slog.String("keyID", key.ID), slog.Any("err", upErr))
			}
		}()
	}
	ctx = context.WithValue(ctx, claimsKey{}, claims)
	ctx = context.WithValue(ctx, apiKeyAuthKey{}, true)
	return ctx, nil
}

// apiKeyLastUsedThrottle bounds how often an api key's last_used column is
// rewritten - a "last used" timestamp does not need per-request precision.
const apiKeyLastUsedThrottle = 5 * time.Minute

// shouldWriteLastUsed reports whether the api-key last-used write should run:
// on first use (nil) or once the previous write is at least a window old.
func shouldWriteLastUsed(lastUsedAt *time.Time, now time.Time, window time.Duration) bool {
	return lastUsedAt == nil || now.Sub(*lastUsedAt) >= window
}

// IsAPIKeyAuth returns true if the request was authenticated via API key.
func IsAPIKeyAuth(ctx context.Context) bool {
	v, _ := ctx.Value(apiKeyAuthKey{}).(bool)
	return v
}

// AdminReach reports whether the caller may use its admin role on this request:
// the admin-only branch of a handler on a method the RBAC leaves to every user
// (another account's password, profiles or sessions, a host scan).
//
// A JWT carries the role itself. An API key does not: authenticateAPIKey copies
// its owner's IsAdmin, so a bare claims.IsAdmin test handed the whole admin role
// to every admin-owned key whatever its scopes - a "users:write" key could reset
// another account's password, the account lifecycle InternalMethods keeps away
// from every key. For an API key the role counts only when the key holds scope,
// and an empty scope means no key ever gets it (JWT-only, like ImpersonateUser).
func AdminReach(ctx context.Context, scope string) bool {
	claims := ClaimsFromContext(ctx)
	if claims == nil || !claims.IsAdmin {
		return false
	}
	if !IsAPIKeyAuth(ctx) {
		return true
	}
	return scope != "" && claims.HasPermission(scope)
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
