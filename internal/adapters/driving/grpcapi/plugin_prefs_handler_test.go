package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/domain/shared"
	authpkg "github.com/eveyproject/evey/internal/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// memPrefRepo is an in-memory PluginDataRepository for handler tests. The key
// includes the profile so per-profile rows are isolated, mirroring the real
// (plugin, profile, key) / (plugin, user, key) scoping.
type memPrefRepo struct{ m map[string]string }

func newMemPrefRepo() *memPrefRepo                   { return &memPrefRepo{m: map[string]string{}} }
func (r *memPrefRepo) key(p, u, pr, k string) string { return p + "|" + u + "|" + pr + "|" + k }

func (r *memPrefRepo) Get(_ context.Context, p, u, pr, k string) (*plugin.PluginDataEntry, error) {
	v, ok := r.m[r.key(p, u, pr, k)]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return &plugin.PluginDataEntry{PluginID: p, UserID: u, ProfileID: pr, Key: k, Value: v}, nil
}
func (r *memPrefRepo) Set(_ context.Context, p, u, pr, k, v string) error {
	r.m[r.key(p, u, pr, k)] = v
	return nil
}
func (r *memPrefRepo) Delete(_ context.Context, p, u, pr, k string) error {
	delete(r.m, r.key(p, u, pr, k))
	return nil
}
func (r *memPrefRepo) List(_ context.Context, p, u, pr string) ([]*plugin.PluginDataEntry, error) {
	var out []*plugin.PluginDataEntry
	for k, v := range r.m {
		out = append(out, &plugin.PluginDataEntry{PluginID: p, UserID: u, ProfileID: pr, Key: k, Value: v})
	}
	return out, nil
}

func pluginCtx(pluginID, userID string) context.Context {
	return interceptors.ContextWithPluginClaims(context.Background(),
		&authpkg.PluginClaims{PluginID: pluginID, UserID: userID, TokenType: "plugin"})
}

func pluginCtxProfile(pluginID, userID, profileID string) context.Context {
	return interceptors.ContextWithPluginClaims(context.Background(),
		&authpkg.PluginClaims{PluginID: pluginID, UserID: userID, ProfileID: profileID, TokenType: "plugin"})
}

func TestPluginPrefs_RequiresPluginToken(t *testing.T) {
	h := newPluginUserDataHandler(newMemPrefRepo())
	// No plugin claims in context → rejected (a user token must not reach this).
	_, err := h.GetPref(context.Background(), &eveyv1.GetPrefRequest{Key: "k"})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestPluginPrefs_ScopedToTokenPluginAndUser(t *testing.T) {
	repo := newMemPrefRepo()
	h := newPluginUserDataHandler(repo)

	_, err := h.SetPref(pluginCtx("plg-1", "u1"), &eveyv1.SetPrefRequest{Key: "theme", Value: "dark"})
	require.NoError(t, err)

	// Same key, same plugin, DIFFERENT user → independent (and absent here).
	got, err := h.GetPref(pluginCtx("plg-1", "u2"), &eveyv1.GetPrefRequest{Key: "theme"})
	require.NoError(t, err)
	assert.False(t, got.Found, "a plugin's pref for u1 must not leak to u2")

	got, err = h.GetPref(pluginCtx("plg-1", "u1"), &eveyv1.GetPrefRequest{Key: "theme"})
	require.NoError(t, err)
	assert.True(t, got.Found)
	assert.Equal(t, "dark", got.Value)

	// The value was stored under (plg-1, u1) - proving plugin+user come from the
	// token, not the request (the request only carries key/value).
	stored, _ := repo.Get(context.Background(), "plg-1", "u1", "", "theme")
	assert.Equal(t, "dark", stored.Value)
}

// With an active profile in the token, prefs are scoped per-profile: the same
// user's two profiles keep independent values, and neither collides with the
// user-scoped (no-profile) row.
func TestPluginPrefs_ScopedPerProfile(t *testing.T) {
	repo := newMemPrefRepo()
	h := newPluginUserDataHandler(repo)

	_, err := h.SetPref(pluginCtxProfile("plg-1", "u1", "prf-a"), &eveyv1.SetPrefRequest{Key: "city", Value: "Paris"})
	require.NoError(t, err)
	_, err = h.SetPref(pluginCtxProfile("plg-1", "u1", "prf-b"), &eveyv1.SetPrefRequest{Key: "city", Value: "Lyon"})
	require.NoError(t, err)

	a, err := h.GetPref(pluginCtxProfile("plg-1", "u1", "prf-a"), &eveyv1.GetPrefRequest{Key: "city"})
	require.NoError(t, err)
	assert.True(t, a.Found)
	assert.Equal(t, "Paris", a.Value)

	b, err := h.GetPref(pluginCtxProfile("plg-1", "u1", "prf-b"), &eveyv1.GetPrefRequest{Key: "city"})
	require.NoError(t, err)
	assert.Equal(t, "Lyon", b.Value, "each profile keeps its own value")

	// The user-scoped row (no active profile) is a different namespace entirely.
	none, err := h.GetPref(pluginCtx("plg-1", "u1"), &eveyv1.GetPrefRequest{Key: "city"})
	require.NoError(t, err)
	assert.False(t, none.Found, "per-profile rows must not leak into the per-user scope")
}
