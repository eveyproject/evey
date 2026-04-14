package plugin

import (
	"context"
	"errors"
	"time"

	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/pkg/auth"
)

// PluginTokenTTL is the fixed lifetime for iframe-scoped JWTs.
const PluginTokenTTL = 5 * time.Minute

// ErrPluginNotEnabled is returned when attempting to issue a token for a disabled plugin.
var ErrPluginNotEnabled = errors.New("plugin not enabled")

// PluginTokenIssuer signs scoped plugin tokens.
type PluginTokenIssuer interface {
	GeneratePluginToken(userID, pluginID string, scopes []string, ttl time.Duration) (string, time.Time, []string, error)
}

// TokenService issues short-lived scoped JWTs for plugin iframes.
type TokenService struct {
	plugins plugin.PluginRepository
	issuer  PluginTokenIssuer
	ttl     time.Duration
}

func NewTokenService(plugins plugin.PluginRepository, issuer PluginTokenIssuer) *TokenService {
	return &TokenService{plugins: plugins, issuer: issuer, ttl: PluginTokenTTL}
}

// IssuedToken bundles the results of IssueToken.
type IssuedToken struct {
	Token     string
	ExpiresAt time.Time
	Scopes    []string
}

// IssueToken returns a scoped JWT for (userID, pluginID). Scopes are the
// intersection of the plugin's admin-approved permissions (Plugin.Permissions)
// with the auth-level allow-list. Forbidden scopes (admin:*, auth:*, system:*,
// user:write, library:write) are stripped unconditionally.
func (s *TokenService) IssueToken(ctx context.Context, userID, pluginID string) (*IssuedToken, error) {
	if userID == "" {
		return nil, errors.New("userID required")
	}
	p, err := s.plugins.GetByID(ctx, pluginID)
	if err != nil || p == nil {
		return nil, ErrPluginNotFound
	}
	if !p.Enabled {
		return nil, ErrPluginNotEnabled
	}

	scopes := auth.FilterPluginScopes(permissionsToScopes(p.Permissions))

	tok, exp, final, err := s.issuer.GeneratePluginToken(userID, pluginID, scopes, s.ttl)
	if err != nil {
		return nil, err
	}
	return &IssuedToken{Token: tok, ExpiresAt: exp, Scopes: final}, nil
}

// permissionsToScopes maps legacy dot-style permissions (media.read) to scope
// form (media:read). Values already in scope form pass through unchanged.
func permissionsToScopes(perms []string) []string {
	out := make([]string, 0, len(perms))
	for _, p := range perms {
		out = append(out, normalizePermission(p))
	}
	return out
}

func normalizePermission(p string) string {
	// Replace first '.' with ':' for back-compat with api.* / media.* style.
	b := []byte(p)
	for i, c := range b {
		if c == '.' {
			b[i] = ':'
			break
		}
	}
	return string(b)
}
