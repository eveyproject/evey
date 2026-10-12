package plugin

import (
	"context"
	"errors"
	"time"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/pkg/auth"
)

// PluginTokenTTL is the fixed lifetime for iframe-scoped JWTs.
const PluginTokenTTL = 5 * time.Minute

// ErrPluginNotEnabled is returned when attempting to issue a token for a disabled plugin.
var ErrPluginNotEnabled = errors.New("plugin not enabled")

// PluginTokenIssuer signs scoped plugin tokens.
type PluginTokenIssuer interface {
	GeneratePluginToken(userID, pluginID, profileID string, scopes []string, ttl time.Duration) (string, time.Time, []string, error)
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
// with the auth-level allow-list. Forbidden scopes (admin.*, auth.*, system.*,
// user.write, library.write) are stripped unconditionally.
func (s *TokenService) IssueToken(ctx context.Context, userID, profileID, pluginID string) (*IssuedToken, error) {
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

	scopes := auth.FilterPluginScopes(p.Permissions)

	tok, exp, final, err := s.issuer.GeneratePluginToken(userID, pluginID, profileID, scopes, s.ttl)
	if err != nil {
		return nil, err
	}
	return &IssuedToken{Token: tok, ExpiresAt: exp, Scopes: final}, nil
}
