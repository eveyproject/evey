package plugin_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	plugindomain "github.com/eveyproject/evey/internal/domain/plugin"
	plgfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	"github.com/eveyproject/evey/internal/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeIssuer struct {
	lastUser    string
	lastPlugin  string
	lastProfile string
	lastScopes  []string
	lastTTL     time.Duration
	failWith    error
	returnToken string
}

func (f *fakeIssuer) GeneratePluginToken(userID, pluginID, profileID string, scopes []string, ttl time.Duration) (string, time.Time, []string, error) {
	f.lastUser = userID
	f.lastPlugin = pluginID
	f.lastProfile = profileID
	f.lastScopes = append([]string(nil), scopes...)
	f.lastTTL = ttl
	if f.failWith != nil {
		return "", time.Time{}, nil, f.failWith
	}
	tok := f.returnToken
	if tok == "" {
		tok = "tok-" + pluginID
	}
	return tok, time.Now().Add(ttl), scopes, nil
}

func newTokenSvc(t *testing.T) (*appplugin.TokenService, *plgfakes.FakePluginRepository, *fakeIssuer) {
	t.Helper()
	repo := plgfakes.NewFakePluginRepository()
	iss := &fakeIssuer{}
	return appplugin.NewTokenService(repo, iss), repo, iss
}

func TestIssueToken_FiltersForbiddenAndIntersects(t *testing.T) {
	svc, repo, iss := newTokenSvc(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &plugindomain.Plugin{
		ID: "plg1", Name: "p", Enabled: true,
		// Permissions use dot format (canonical) as they appear in the manifest.
		Permissions: []string{"media.read", "library.read", "admin.users", "auth.login", "user.write"},
	}))
	res, err := svc.IssueToken(ctx, "user-1", "prf-1", "plg1")
	require.NoError(t, err)
	assert.NotEmpty(t, res.Token)
	// Forbidden scopes must have been stripped; allowed scopes stay in dot format.
	assert.Equal(t, []string{"media.read", "library.read"}, iss.lastScopes)
	assert.Equal(t, "user-1", iss.lastUser)
	assert.Equal(t, "prf-1", iss.lastProfile, "the caller's active profile must reach the issuer")
	assert.Equal(t, "plg1", iss.lastPlugin)
	assert.Equal(t, appplugin.PluginTokenTTL, iss.lastTTL)
}

func TestIssueToken_PluginNotFound(t *testing.T) {
	svc, _, _ := newTokenSvc(t)
	_, err := svc.IssueToken(context.Background(), "u", "", "nope")
	assert.ErrorIs(t, err, appplugin.ErrPluginNotFound)
}

func TestIssueToken_PluginDisabled(t *testing.T) {
	svc, repo, _ := newTokenSvc(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &plugindomain.Plugin{
		ID: "plgD", Name: "d", Enabled: false,
		Permissions: []string{"media.read"},
	}))
	_, err := svc.IssueToken(ctx, "u", "", "plgD")
	assert.ErrorIs(t, err, appplugin.ErrPluginNotEnabled)
}

func TestIssueToken_RequiresUserID(t *testing.T) {
	svc, repo, _ := newTokenSvc(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &plugindomain.Plugin{
		ID: "plg1", Name: "p", Enabled: true, Permissions: []string{"media.read"},
	}))
	_, err := svc.IssueToken(ctx, "", "", "plg1")
	require.Error(t, err)
}

func TestIssueToken_IssuerError(t *testing.T) {
	svc, repo, iss := newTokenSvc(t)
	iss.failWith = errors.New("signer down")
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &plugindomain.Plugin{
		ID: "plg1", Name: "p", Enabled: true, Permissions: []string{"media.read"},
	}))
	_, err := svc.IssueToken(ctx, "u", "", "plg1")
	require.Error(t, err)
}

func TestIssueToken_NoApprovedPermissionsYieldsEmptyScopes(t *testing.T) {
	svc, repo, iss := newTokenSvc(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &plugindomain.Plugin{
		ID: "plg1", Name: "p", Enabled: true, Permissions: nil,
	}))
	res, err := svc.IssueToken(ctx, "u", "", "plg1")
	require.NoError(t, err)
	assert.Empty(t, res.Scopes)
	assert.Empty(t, iss.lastScopes)
}

// TestScopeChain_DotFormat validates the full permission pipeline:
// manifest dot-format permission → IssueToken → JWT claims → PermissionChecker.
// This locks in that no colon-conversion step silently empties the scope list.
func TestScopeChain_DotFormat(t *testing.T) {
	t.Parallel()

	// Real RSA key + JWTGenerator so we exercise the actual signing/parsing path.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	gen := auth.NewJWTGenerator(key, time.Minute)

	repo := plgfakes.NewFakePluginRepository()
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &plugindomain.Plugin{
		ID:          "chain-plg",
		Name:        "chain",
		Enabled:     true,
		Permissions: []string{"media.read", "api.library"},
	}))

	svc := appplugin.NewTokenService(repo, gen)
	issued, err := svc.IssueToken(ctx, "u1", "", "chain-plg")
	require.NoError(t, err)
	assert.NotEmpty(t, issued.Token)

	// Scopes in the returned struct must be dot-format, non-empty.
	assert.Contains(t, issued.Scopes, "media.read", "media.read must survive issuance unchanged")
	assert.Contains(t, issued.Scopes, "api.library", "api.library must survive issuance unchanged")

	// Parse the raw JWT and verify claims carry dot-format scopes.
	claims, parseErr := gen.ParsePluginToken(issued.Token)
	require.NoError(t, parseErr)
	assert.True(t, claims.HasScope("media.read"), "JWT claims must include media.read")
	assert.True(t, claims.HasScope("api.library"), "JWT claims must include api.library")

	// PermissionChecker (used by gRPC interceptor) must accept the dot-format
	// permissions loaded from the manifest.
	checker := appplugin.NewPermissionChecker(repo)
	checker.LoadPermissions("chain-plg", []string{"media.read", "api.library"})
	assert.NoError(t, checker.CheckAnyPermission("chain-plg", "api.library", "media.read"),
		"interceptor must grant access when plugin has the required permission")
	assert.NoError(t, checker.CheckAnyPermission("chain-plg", "media.read"),
		"media.read alone must be sufficient for read-only RPCs")
}
