package auth_test

import (
	"testing"

	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/stretchr/testify/assert"
)

func TestCheckPermission_ServerAdmin(t *testing.T) {
	perms := []string{auth.PermServerAdmin}

	tests := []string{
		auth.PermLibraryCreate,
		auth.PermUserDelete,
		auth.PermPluginInstall,
		auth.PermAuditRead,
		auth.PermMetadataUpdate,
	}
	for _, required := range tests {
		assert.True(t, auth.CheckPermission(perms, required),
			"server.admin should grant %s", required)
	}
}

func TestCheckPermission_ExactMatch(t *testing.T) {
	perms := []string{auth.PermLibraryRead, auth.PermMediaRead}

	assert.True(t, auth.CheckPermission(perms, auth.PermLibraryRead))
	assert.True(t, auth.CheckPermission(perms, auth.PermMediaRead))
}

func TestCheckPermission_NoMatch(t *testing.T) {
	perms := []string{auth.PermLibraryRead, auth.PermMediaRead}

	assert.False(t, auth.CheckPermission(perms, auth.PermLibraryDelete))
	assert.False(t, auth.CheckPermission(perms, auth.PermUserCreate))
	assert.False(t, auth.CheckPermission(nil, auth.PermLibraryRead))
	assert.False(t, auth.CheckPermission([]string{}, auth.PermLibraryRead))
}
