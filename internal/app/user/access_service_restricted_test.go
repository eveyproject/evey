package user

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/user"
	ufakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

// Every list caller reads an empty result as "unrestricted". A restricted user
// whose last grant is gone must get a list that matches no library instead.
func TestAccessService_RestrictedUserWithNoGrantLeftSeesNoLibrary(t *testing.T) {
	t.Parallel()
	svc, _ := newAccessSvcWithLibraryAccess()
	ctx := context.Background()
	require.NoError(t, svc.GrantLibraryAccess(ctx, "u-r", "lib-only"))
	require.NoError(t, svc.RevokeLibraryAccess(ctx, "u-r", "lib-only"))

	ids, err := svc.GetLibraryAccessByUserID(ctx, "u-r")
	require.NoError(t, err)
	assert.Equal(t, []string{user.NoLibraryAccess}, ids)
	ok, err := svc.HasLibraryAccess(ctx, "u-r", "lib-other")
	require.NoError(t, err)
	assert.False(t, ok)

	// RevokeAll is the admin's "unrestricted" action.
	require.NoError(t, svc.RevokeAllLibraryAccess(ctx, "u-r"))
	ids, err = svc.GetLibraryAccessByUserID(ctx, "u-r")
	require.NoError(t, err)
	assert.Empty(t, ids)
	ok, err = svc.HasLibraryAccess(ctx, "u-r", "lib-other")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestRegisterWithInvite_WithoutLibrariesStaysUnrestricted(t *testing.T) {
	ctx := context.Background()
	d := newAccessTestDeps()
	access := ufakes.NewFakeLibraryAccessRepository()
	d.svc = NewAccessService(d.users, d.apiKeys, d.invites, d.quickConnect, d.hasher, d.events, WithLibraryAccess(access))
	_ = d.invites.Create(ctx, &user.InviteLink{ID: "inv-o", Token: "open", MaxUses: 1})

	u, err := d.svc.RegisterWithInvite(ctx, "open", "ivy", "longpassword", "")
	require.NoError(t, err)
	restricted, err := access.IsRestricted(ctx, u.ID)
	require.NoError(t, err)
	assert.False(t, restricted)
	ids, err := d.svc.GetLibraryAccessByUserID(ctx, string(u.ID))
	require.NoError(t, err)
	assert.Empty(t, ids)
}
