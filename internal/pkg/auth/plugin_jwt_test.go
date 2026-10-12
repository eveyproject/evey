package auth_test

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func userClaims() user.Claims {
	return user.Claims{UserID: "u1", Username: "bob", IsAdmin: false}
}

func newGen(t *testing.T, ttl time.Duration) *auth.JWTGenerator {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return auth.NewJWTGenerator(key, ttl)
}

func TestFilterPluginScopes_WhitelistAndForbidden(t *testing.T) {
	t.Parallel()
	in := []string{
		"media.read", "media.write", "library.read", "user.read",
		"playback.read", "stats.read", "notification.read",
		"admin.users", "auth.login", "user.write", "library.write",
		"system.shutdown", "unknown.scope", "media.read", // dup
	}
	out := auth.FilterPluginScopes(in)
	assert.Equal(t, []string{
		"media.read", "media.write", "library.read", "user.read",
		"playback.read", "stats.read", "notification.read",
	}, out)
}

// TestFilterPluginScopes_GrantableAndHardLocked pins the boundary for the
// capabilities added with the permission-model work: `public` and `prefs` are
// grantable; admin/auth/system and generic write scopes are stripped even if a
// manifest tries to declare them (non-bypassable hard lock at the token layer).
func TestFilterPluginScopes_GrantableAndHardLocked(t *testing.T) {
	t.Parallel()
	in := []string{
		"public", "prefs", // grantable
		"admin.fullaccess", "auth.session", "system.config", // hard-locked prefixes
		"user.write", "library.write", // hard-locked exact
		"prefs.admin", // not whitelisted (only bare "prefs" is)
	}
	out := auth.FilterPluginScopes(in)
	assert.Equal(t, []string{"public", "prefs"}, out)
}

func TestGenerateAndParsePluginToken_Roundtrip(t *testing.T) {
	t.Parallel()
	g := newGen(t, time.Minute)
	tok, exp, scopes, err := g.GeneratePluginToken("u1", "plg1", "prf-1",
		[]string{"media.read", "admin.x", "library.read"}, 5*time.Minute)
	require.NoError(t, err)
	assert.NotEmpty(t, tok)
	assert.WithinDuration(t, time.Now().Add(5*time.Minute), exp, 2*time.Second)
	assert.Equal(t, []string{"media.read", "library.read"}, scopes)

	claims, err := g.ParsePluginToken(tok)
	require.NoError(t, err)
	assert.Equal(t, "u1", claims.UserID)
	assert.Equal(t, "plg1", claims.PluginID)
	assert.Equal(t, "prf-1", claims.ProfileID, "the active profile must round-trip through the token")
	assert.Equal(t, auth.PluginTokenType, claims.TokenType)
	assert.True(t, claims.HasScope("media.read"))
	assert.False(t, claims.HasScope("admin.x"))
}

func TestParsePluginToken_RejectsUserAccessToken(t *testing.T) {
	t.Parallel()
	g := newGen(t, time.Minute)
	// A normal access token (not plugin type) must be rejected.
	accessTok, err := g.GenerateAccessToken(userClaims())
	require.NoError(t, err)
	_, err = g.ParsePluginToken(accessTok)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a plugin token")
}

func TestGeneratePluginToken_Expiry(t *testing.T) {
	t.Parallel()
	g := newGen(t, time.Minute)
	tok, _, _, err := g.GeneratePluginToken("u1", "plg1", "", []string{"media.read"}, -1*time.Second)
	// negative ttl rejected
	require.Error(t, err)
	assert.Empty(t, tok)
}

func TestGeneratePluginToken_ZeroTTL(t *testing.T) {
	t.Parallel()
	g := newGen(t, time.Minute)
	tok, _, _, err := g.GeneratePluginToken("u1", "plg1", "", []string{"media.read"}, 0)
	require.Error(t, err, "ttl=0 must be rejected (boundary)")
	assert.Empty(t, tok)
}

func TestParsePluginToken_Expired(t *testing.T) {
	t.Parallel()
	g := newGen(t, time.Minute)
	// generate with tiny positive ttl then wait
	tok, _, _, err := g.GeneratePluginToken("u1", "plg1", "", []string{"media.read"}, 1*time.Millisecond)
	require.NoError(t, err)
	time.Sleep(20 * time.Millisecond)
	_, err = g.ParsePluginToken(tok)
	require.Error(t, err)
}

func TestGeneratePluginToken_RequiresIDs(t *testing.T) {
	t.Parallel()
	g := newGen(t, time.Minute)
	_, _, _, err := g.GeneratePluginToken("", "p", "", []string{"media.read"}, time.Minute)
	require.Error(t, err)
	_, _, _, err = g.GeneratePluginToken("u", "", "", []string{"media.read"}, time.Minute)
	require.Error(t, err)
}

func TestGeneratePluginToken_ForbiddenScopesStripped(t *testing.T) {
	t.Parallel()
	g := newGen(t, time.Minute)
	_, _, scopes, err := g.GeneratePluginToken("u", "p", "",
		[]string{"admin.users", "auth.login", "system.x", "user.write", "library.write", "media.read"},
		time.Minute)
	require.NoError(t, err)
	assert.Equal(t, []string{"media.read"}, scopes, "only whitelisted scope remains")
}
