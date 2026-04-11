package plugin_test

import (
	"testing"

	appplugin "github.com/gravity-zero/evey/internal/app/plugin"
	plgfakes "github.com/gravity-zero/evey/internal/domain/plugin/fakes"
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
