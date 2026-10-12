package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ---------------------------------------------------------------------------
// DeletePref
// ---------------------------------------------------------------------------

func TestDeletePref_RequiresPluginToken(t *testing.T) {
	h := newPluginUserDataHandler(newMemPrefRepo())
	_, err := h.DeletePref(context.Background(), &eveyv1.DeletePrefRequest{Key: "k"})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestDeletePref_NilDataStore(t *testing.T) {
	h := newPluginUserDataHandler(nil)
	_, err := h.DeletePref(pluginCtx("plg-1", "u1"), &eveyv1.DeletePrefRequest{Key: "k"})
	require.Error(t, err)
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

func TestDeletePref_RemovesExistingKey(t *testing.T) {
	repo := newMemPrefRepo()
	h := newPluginUserDataHandler(repo)

	// Seed a value first.
	_, err := h.SetPref(pluginCtx("plg-1", "u1"), &eveyv1.SetPrefRequest{Key: "theme", Value: "dark"})
	require.NoError(t, err)

	// Confirm it exists.
	got, err := h.GetPref(pluginCtx("plg-1", "u1"), &eveyv1.GetPrefRequest{Key: "theme"})
	require.NoError(t, err)
	assert.True(t, got.Found)

	// Delete it.
	_, err = h.DeletePref(pluginCtx("plg-1", "u1"), &eveyv1.DeletePrefRequest{Key: "theme"})
	require.NoError(t, err)

	// Must be gone.
	got, err = h.GetPref(pluginCtx("plg-1", "u1"), &eveyv1.GetPrefRequest{Key: "theme"})
	require.NoError(t, err)
	assert.False(t, got.Found)
}

func TestDeletePref_Idempotent(t *testing.T) {
	h := newPluginUserDataHandler(newMemPrefRepo())
	ctx := pluginCtx("plg-1", "u1")

	// Delete a key that was never set - must not error.
	_, err := h.DeletePref(ctx, &eveyv1.DeletePrefRequest{Key: "nonexistent"})
	require.NoError(t, err)

	// Second delete also must not error.
	_, err = h.DeletePref(ctx, &eveyv1.DeletePrefRequest{Key: "nonexistent"})
	require.NoError(t, err)
}

func TestDeletePref_ScopedToCallerToken(t *testing.T) {
	repo := newMemPrefRepo()
	h := newPluginUserDataHandler(repo)

	// Set a pref for (plg-1, u1).
	_, err := h.SetPref(pluginCtx("plg-1", "u1"), &eveyv1.SetPrefRequest{Key: "col", Value: "red"})
	require.NoError(t, err)

	// Delete using a DIFFERENT user's token - must not affect u1's key because the
	// scope (plugin+user) comes from the token, not the request.
	_, err = h.DeletePref(pluginCtx("plg-1", "u2"), &eveyv1.DeletePrefRequest{Key: "col"})
	require.NoError(t, err)

	// u1's pref must still be present.
	got, err := h.GetPref(pluginCtx("plg-1", "u1"), &eveyv1.GetPrefRequest{Key: "col"})
	require.NoError(t, err)
	assert.True(t, got.Found, "delete by u2 must not touch u1's pref")
	assert.Equal(t, "red", got.Value)
}

// ---------------------------------------------------------------------------
// ListPrefs
// ---------------------------------------------------------------------------

func TestListPrefs_RequiresPluginToken(t *testing.T) {
	h := newPluginUserDataHandler(newMemPrefRepo())
	_, err := h.ListPrefs(context.Background(), &eveyv1.ListPrefsRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestListPrefs_NilDataStore(t *testing.T) {
	h := newPluginUserDataHandler(nil)
	_, err := h.ListPrefs(pluginCtx("plg-1", "u1"), &eveyv1.ListPrefsRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

func TestListPrefs_EmptyWhenNoneSet(t *testing.T) {
	h := newPluginUserDataHandler(newMemPrefRepo())
	resp, err := h.ListPrefs(pluginCtx("plg-1", "u1"), &eveyv1.ListPrefsRequest{})
	require.NoError(t, err)
	assert.Empty(t, resp.Prefs)
}

func TestListPrefs_ReturnsCallerPrefs(t *testing.T) {
	repo := newMemPrefRepo()
	h := newPluginUserDataHandler(repo)
	ctx := pluginCtx("plg-2", "u3")

	// Seed two prefs.
	_, err := h.SetPref(ctx, &eveyv1.SetPrefRequest{Key: "a", Value: "1"})
	require.NoError(t, err)
	_, err = h.SetPref(ctx, &eveyv1.SetPrefRequest{Key: "b", Value: "2"})
	require.NoError(t, err)

	resp, err := h.ListPrefs(ctx, &eveyv1.ListPrefsRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.Prefs, 2)

	// Values must be present; the fake stores the composite map key as Key so we
	// check values rather than exact key names.
	values := make([]string, 0, len(resp.Prefs))
	for _, p := range resp.Prefs {
		values = append(values, p.Value)
	}
	assert.ElementsMatch(t, []string{"1", "2"}, values)
}

func TestListPrefs_DeleteReducesCount(t *testing.T) {
	repo := newMemPrefRepo()
	h := newPluginUserDataHandler(repo)
	ctx := pluginCtx("plg-3", "u4")

	_, err := h.SetPref(ctx, &eveyv1.SetPrefRequest{Key: "x", Value: "hello"})
	require.NoError(t, err)
	_, err = h.SetPref(ctx, &eveyv1.SetPrefRequest{Key: "y", Value: "world"})
	require.NoError(t, err)

	resp, err := h.ListPrefs(ctx, &eveyv1.ListPrefsRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.Prefs, 2)

	_, err = h.DeletePref(ctx, &eveyv1.DeletePrefRequest{Key: "x"})
	require.NoError(t, err)

	resp, err = h.ListPrefs(ctx, &eveyv1.ListPrefsRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.Prefs, 1)
	assert.Equal(t, "world", resp.Prefs[0].Value)
}
