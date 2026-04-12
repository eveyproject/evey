package auth

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// PluginTokenType marks tokens as scoped plugin tokens (vs user access tokens).
const PluginTokenType = "plugin"

// AllowedPluginScopes is the whitelist of scopes that may ever appear on a plugin token.
// Any scope not in this set is stripped during issuance (defence-in-depth).
var AllowedPluginScopes = map[string]bool{
	"media:read":        true,
	"media:write":       true,
	"library:read":      true,
	"user:read":         true,
	"playback:read":     true,
	"stats:read":        true,
	"notification:read": true,
}

// ForbiddenPluginScopePrefixes: scopes starting with any of these are rejected even
// if they slip into the manifest. admin:*/auth:*/system:*/user:write/library:write.
var forbiddenExact = map[string]bool{
	"user:write":    true,
	"library:write": true,
}

var forbiddenPrefix = []string{"admin:", "auth:", "system:"}

// PluginClaims are the claims carried by a scoped plugin iframe JWT.
type PluginClaims struct {
	UserID    string
	PluginID  string
	Scopes    []string
	TokenType string
	ExpiresAt time.Time
}

// FilterPluginScopes returns scopes that are both on the allow-list and not on the
// forbidden list. Order is preserved; duplicates removed.
func FilterPluginScopes(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		if forbiddenExact[s] {
			continue
		}
		bad := false
		for _, p := range forbiddenPrefix {
			if strings.HasPrefix(s, p) {
				bad = true
				break
			}
		}
		if bad {
			continue
		}
		if !AllowedPluginScopes[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// GeneratePluginToken signs a short-lived scoped JWT for a plugin iframe. Scopes are
// filtered through FilterPluginScopes before signing.
func (g *JWTGenerator) GeneratePluginToken(userID, pluginID string, scopes []string, ttl time.Duration) (string, time.Time, []string, error) {
	if userID == "" || pluginID == "" {
		return "", time.Time{}, nil, errors.New("userID and pluginID required")
	}
	if ttl <= 0 {
		return "", time.Time{}, nil, errors.New("ttl must be positive")
	}
	filtered := FilterPluginScopes(scopes)
	now := time.Now()
	exp := now.Add(ttl)
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub":        userID,
		"plugin_id":  pluginID,
		"scopes":     filtered,
		"token_type": PluginTokenType,
		"iat":        now.Unix(),
		"exp":        exp.Unix(),
	})
	signed, err := token.SignedString(g.privateKey)
	if err != nil {
		return "", time.Time{}, nil, err
	}
	return signed, exp, filtered, nil
}

// ParsePluginToken validates a plugin JWT signature/exp and returns its claims.
// Rejects tokens whose token_type != "plugin".
func (g *JWTGenerator) ParsePluginToken(raw string) (*PluginClaims, error) {
	parsed, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return g.publicKey, nil
	}, jwt.WithValidMethods([]string{"RS256"}))
	if err != nil {
		return nil, err
	}
	if !parsed.Valid {
		return nil, errors.New("invalid plugin token")
	}
	mc, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid claims")
	}
	tt, _ := mc["token_type"].(string)
	if tt != PluginTokenType {
		return nil, errors.New("not a plugin token")
	}
	sub, _ := mc["sub"].(string)
	pid, _ := mc["plugin_id"].(string)
	if sub == "" || pid == "" {
		return nil, errors.New("missing sub or plugin_id")
	}
	var scopes []string
	if raw, ok := mc["scopes"].([]any); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok {
				scopes = append(scopes, s)
			}
		}
	}
	var exp time.Time
	if v, ok := mc["exp"].(float64); ok {
		exp = time.Unix(int64(v), 0)
	}
	return &PluginClaims{
		UserID:    sub,
		PluginID:  pid,
		Scopes:    scopes,
		TokenType: tt,
		ExpiresAt: exp,
	}, nil
}

// HasScope reports whether the claims grant the named scope.
func (c *PluginClaims) HasScope(s string) bool {
	for _, sc := range c.Scopes {
		if sc == s {
			return true
		}
	}
	return false
}

