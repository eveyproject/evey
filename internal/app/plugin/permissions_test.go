package plugin_test

import (
	"testing"

	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	plugindomain "github.com/eveyproject/evey/internal/domain/plugin"
	plgfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckPermission_Allowed(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)
	checker.LoadPermissions("plg-1", []string{"api.library", "media.read"})

	err := checker.CheckPermission("plg-1", "api.library")
	assert.NoError(t, err)
}

func TestCheckPermission_Denied(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)
	checker.LoadPermissions("plg-1", []string{"api.library"})

	err := checker.CheckPermission("plg-1", "media.write")
	assert.ErrorIs(t, err, appplugin.ErrPermissionDenied)
}

func TestCheckPermission_UnknownPlugin(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)

	err := checker.CheckPermission("plg-unknown", "api.library")
	assert.ErrorIs(t, err, appplugin.ErrPermissionDenied)
}

// A plugin loaded with an empty permission list must still deny - distinct
// from "unknown plugin". Anti-regression for the cache zero-value mutation:
// if `perms[requiredPerm]` were flipped to `!perms[requiredPerm]`, an
// empty cache entry would erroneously allow every permission.
func TestCheckPermission_EmptyPermsList(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)
	checker.LoadPermissions("plg-1", []string{})

	err := checker.CheckPermission("plg-1", "api.library")
	assert.ErrorIs(t, err, appplugin.ErrPermissionDenied)
}

func TestCheckAnyPermission_OneMatch(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)
	checker.LoadPermissions("plg-1", []string{"media.read"})

	err := checker.CheckAnyPermission("plg-1", "api.library", "media.read")
	assert.NoError(t, err)
}

func TestCheckAnyPermission_NoneMatch(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)
	checker.LoadPermissions("plg-1", []string{"api.user"})

	err := checker.CheckAnyPermission("plg-1", "api.library", "media.read")
	assert.ErrorIs(t, err, appplugin.ErrPermissionDenied)
}

func TestInvalidatePermissions(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	checker := appplugin.NewPermissionChecker(repo)
	checker.LoadPermissions("plg-1", []string{"api.library"})

	require.NoError(t, checker.CheckPermission("plg-1", "api.library"))

	checker.InvalidatePermissions("plg-1")
	err := checker.CheckPermission("plg-1", "api.library")
	assert.ErrorIs(t, err, appplugin.ErrPermissionDenied)
}

// --- DiffPermissions ---

func TestDiffPermissions_NoChange(t *testing.T) {
	diff := appplugin.DiffPermissions([]string{"a", "b"}, []string{"a", "b"})
	assert.Empty(t, diff.Added)
	assert.Empty(t, diff.Removed)
	assert.True(t, diff.IsEmpty())
}

func TestDiffPermissions_Added(t *testing.T) {
	diff := appplugin.DiffPermissions([]string{"a"}, []string{"a", "b"})
	assert.Equal(t, []string{"b"}, diff.Added)
	assert.Empty(t, diff.Removed)
	assert.False(t, diff.IsEmpty())
}

func TestDiffPermissions_Removed(t *testing.T) {
	diff := appplugin.DiffPermissions([]string{"a", "b"}, []string{"a"})
	assert.Empty(t, diff.Added)
	assert.Equal(t, []string{"b"}, diff.Removed)
}

func TestDiffPermissions_Both(t *testing.T) {
	diff := appplugin.DiffPermissions([]string{"a", "b"}, []string{"b", "c"})
	assert.Equal(t, []string{"c"}, diff.Added)
	assert.Equal(t, []string{"a"}, diff.Removed)
}

func TestDiffPermissions_Empty(t *testing.T) {
	diff := appplugin.DiffPermissions(nil, nil)
	assert.Empty(t, diff.Added)
	assert.Empty(t, diff.Removed)
}

func TestDiffSlots_Added(t *testing.T) {
	current := []plugindomain.FrontModule{{Slot: "header"}}
	candidate := []plugindomain.FrontModule{{Slot: "header"}, {Slot: "footer"}}
	diff := appplugin.DiffSlots(current, candidate)
	assert.Equal(t, []string{"footer"}, diff.Added)
	assert.Empty(t, diff.Removed)
}

func TestDiffExternalFetch_Added(t *testing.T) {
	diff := appplugin.DiffExternalFetch([]string{"api.example.com"}, []string{"api.example.com", "cdn.evil.com"})
	assert.Equal(t, []string{"cdn.evil.com"}, diff.Added)
	assert.Empty(t, diff.Removed)
}
