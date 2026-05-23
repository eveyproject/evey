package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
	authpkg "github.com/evey-project/evey/internal/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// memPrefRepo is an in-memory PluginDataRepository for handler tests.
type memPrefRepo struct{ m map[string]string }

func newMemPrefRepo() *memPrefRepo               { return &memPrefRepo{m: map[string]string{}} }
func (r *memPrefRepo) key(p, u, k string) string { return p + "|" + u + "|" + k }

func (r *memPrefRepo) Get(_ context.Context, p, u, k string) (*plugin.PluginDataEntry, error) {
	v, ok := r.m[r.key(p, u, k)]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return &plugin.PluginDataEntry{PluginID: p, UserID: u, Key: k, Value: v}, nil
}
func (r *memPrefRepo) Set(_ context.Context, p, u, k, v string) error {
	r.m[r.key(p, u, k)] = v
	return nil
}
func (r *memPrefRepo) Delete(_ context.Context, p, u, k string) error {
	delete(r.m, r.key(p, u, k))
	return nil
}
func (r *memPrefRepo) List(_ context.Context, p, u string) ([]*plugin.PluginDataEntry, error) {
	var out []*plugin.PluginDataEntry
	for k, v := range r.m {
		out = append(out, &plugin.PluginDataEntry{PluginID: p, UserID: u, Key: k, Value: v})
	}
	return out, nil
}

func pluginCtx(pluginID, userID string) context.Context {
	return interceptors.ContextWithPluginClaims(context.Background(),
		&authpkg.PluginClaims{PluginID: pluginID, UserID: userID, TokenType: "plugin"})
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

	// The value was stored under (plg-1, u1) — proving plugin+user come from the
	// token, not the request (the request only carries key/value).
	stored, _ := repo.Get(context.Background(), "plg-1", "u1", "theme")
	assert.Equal(t, "dark", stored.Value)
}
