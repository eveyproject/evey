package plugin_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appplugin "github.com/gravity-zero/evey/internal/app/plugin"
	plugindomain "github.com/gravity-zero/evey/internal/domain/plugin"
	plgfakes "github.com/gravity-zero/evey/internal/domain/plugin/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeIssuer struct {
	lastUser    string
	lastPlugin  string
	lastScopes  []string
	lastTTL     time.Duration
	failWith    error
	returnToken string
}

func (f *fakeIssuer) GeneratePluginToken(userID, pluginID string, scopes []string, ttl time.Duration) (string, time.Time, []string, error) {
	f.lastUser = userID
	f.lastPlugin = pluginID
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
		Permissions: []string{"media.read", "library.read", "admin:users", "auth:login", "user:write"},
	}))
	res, err := svc.IssueToken(ctx, "user-1", "plg1")
	require.NoError(t, err)
	assert.NotEmpty(t, res.Token)
	// Forbidden scopes must have been stripped before signing.
	assert.Equal(t, []string{"media:read", "library:read"}, iss.lastScopes)
	assert.Equal(t, "user-1", iss.lastUser)
	assert.Equal(t, "plg1", iss.lastPlugin)
	assert.Equal(t, appplugin.PluginTokenTTL, iss.lastTTL)
}

func TestIssueToken_PluginNotFound(t *testing.T) {
	svc, _, _ := newTokenSvc(t)
	_, err := svc.IssueToken(context.Background(), "u", "nope")
	assert.ErrorIs(t, err, appplugin.ErrPluginNotFound)
}

func TestIssueToken_PluginDisabled(t *testing.T) {
	svc, repo, _ := newTokenSvc(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &plugindomain.Plugin{
		ID: "plgD", Name: "d", Enabled: false,
		Permissions: []string{"media.read"},
	}))
	_, err := svc.IssueToken(ctx, "u", "plgD")
	assert.ErrorIs(t, err, appplugin.ErrPluginNotEnabled)
}

func TestIssueToken_RequiresUserID(t *testing.T) {
	svc, repo, _ := newTokenSvc(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &plugindomain.Plugin{
		ID: "plg1", Name: "p", Enabled: true, Permissions: []string{"media.read"},
	}))
	_, err := svc.IssueToken(ctx, "", "plg1")
	require.Error(t, err)
}

func TestIssueToken_IssuerError(t *testing.T) {
	svc, repo, iss := newTokenSvc(t)
	iss.failWith = errors.New("signer down")
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &plugindomain.Plugin{
		ID: "plg1", Name: "p", Enabled: true, Permissions: []string{"media.read"},
	}))
	_, err := svc.IssueToken(ctx, "u", "plg1")
	require.Error(t, err)
}

func TestIssueToken_NoApprovedPermissionsYieldsEmptyScopes(t *testing.T) {
	svc, repo, iss := newTokenSvc(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &plugindomain.Plugin{
		ID: "plg1", Name: "p", Enabled: true, Permissions: nil,
	}))
	res, err := svc.IssueToken(ctx, "u", "plg1")
	require.NoError(t, err)
	assert.Empty(t, res.Scopes)
	assert.Empty(t, iss.lastScopes)
}
