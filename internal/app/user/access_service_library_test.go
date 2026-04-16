package user

import (
	"context"
	"errors"
	"testing"

	"github.com/evey-project/evey/internal/domain/shared/fakes"
	ufakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newAccessSvcWithLibraryAccess() (*AccessService, *ufakes.FakeLibraryAccessRepository) {
	users := ufakes.NewFakeUserRepository()
	apiKeys := ufakes.NewFakeAPIKeyRepository()
	invites := ufakes.NewFakeInviteLinkRepository()
	quickConnect := ufakes.NewFakeQuickConnectRepository()
	hasher := ufakes.NewFakePasswordHasher()
	events := fakes.NewFakeEventBus()
	la := ufakes.NewFakeLibraryAccessRepository()
	svc := NewAccessService(users, apiKeys, invites, quickConnect, hasher, events, WithLibraryAccess(la))
	return svc, la
}

// GrantLibraryAccess + HasLibraryAccess cycle.
func TestAccessService_GrantThenHas(t *testing.T) {
	t.Parallel()
	svc, _ := newAccessSvcWithLibraryAccess()
	ctx := context.Background()

	require.NoError(t, svc.GrantLibraryAccess(ctx, "u1", "lib-1"))

	ok, err := svc.HasLibraryAccess(ctx, "u1", "lib-1")
	require.NoError(t, err)
	assert.True(t, ok)

	// Different lib for same user without grant → denied (user has restrictions).
	ok2, err := svc.HasLibraryAccess(ctx, "u1", "lib-2")
	require.NoError(t, err)
	assert.False(t, ok2)
}

// Without any grants, the user has full access (no restrictions configured).
func TestAccessService_HasLibraryAccess_NoRestrictions_AllowsAll(t *testing.T) {
	t.Parallel()
	svc, _ := newAccessSvcWithLibraryAccess()
	ok, err := svc.HasLibraryAccess(context.Background(), "unrestricted-user", "any-lib")
	require.NoError(t, err)
	assert.True(t, ok)
}

// RevokeLibraryAccess removes the grant; HasLibraryAccess denies afterwards.
func TestAccessService_Revoke(t *testing.T) {
	t.Parallel()
	svc, _ := newAccessSvcWithLibraryAccess()
	ctx := context.Background()

	require.NoError(t, svc.GrantLibraryAccess(ctx, "u2", "lib-A"))
	require.NoError(t, svc.GrantLibraryAccess(ctx, "u2", "lib-B"))

	require.NoError(t, svc.RevokeLibraryAccess(ctx, "u2", "lib-A"))

	ok, err := svc.HasLibraryAccess(ctx, "u2", "lib-A")
	require.NoError(t, err)
	assert.False(t, ok, "after revoke, A is not accessible")

	ok, err = svc.HasLibraryAccess(ctx, "u2", "lib-B")
	require.NoError(t, err)
	assert.True(t, ok, "B still accessible")
}

// GetLibraryAccessByUserID returns a string slice of granted library IDs.
func TestAccessService_GetLibraryAccessByUserID(t *testing.T) {
	t.Parallel()
	svc, _ := newAccessSvcWithLibraryAccess()
	ctx := context.Background()

	require.NoError(t, svc.GrantLibraryAccess(ctx, "u3", "lib-1"))
	require.NoError(t, svc.GrantLibraryAccess(ctx, "u3", "lib-2"))

	ids, err := svc.GetLibraryAccessByUserID(ctx, "u3")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"lib-1", "lib-2"}, ids)
}

// GetLibraryAccessByUserID returns nil for a user with no grants.
func TestAccessService_GetLibraryAccessByUserID_NoGrants_ReturnsNil(t *testing.T) {
	t.Parallel()
	svc, _ := newAccessSvcWithLibraryAccess()
	ids, err := svc.GetLibraryAccessByUserID(context.Background(), "nobody")
	require.NoError(t, err)
	assert.Nil(t, ids)
}

// Repo error on GetByUserID propagates.
func TestAccessService_GetLibraryAccessByUserID_RepoError(t *testing.T) {
	t.Parallel()
	svc, la := newAccessSvcWithLibraryAccess()
	la.Err = errors.New("db down")
	_, err := svc.GetLibraryAccessByUserID(context.Background(), "u")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}
