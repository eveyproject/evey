package user

import (
	"context"
	"errors"
	"testing"

	"github.com/eveyproject/evey/internal/domain/shared/fakes"
	ufakes "github.com/eveyproject/evey/internal/domain/user/fakes"
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

// GetLibraryAccessByUserID returns a non-nil empty slice when a user has
// zero grants. Handlers rely on the nil-vs-empty distinction: nil means
// "admin / unrestricted" and MUST NOT be confused with "zero grants".
func TestAccessService_GetLibraryAccessByUserID_NoGrants_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	svc, _ := newAccessSvcWithLibraryAccess()
	ids, err := svc.GetLibraryAccessByUserID(context.Background(), "nobody")
	require.NoError(t, err)
	require.NotNil(t, ids, "nil would promote a zero-grants user to unrestricted access in filter callers")
	assert.Empty(t, ids)
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

// ReplaceLibraryAccess swaps the whole grant set in one call: old grants not
// present in the new set are gone, new ones are present.
func TestAccessService_ReplaceLibraryAccess_SwapsSet(t *testing.T) {
	t.Parallel()
	svc, _ := newAccessSvcWithLibraryAccess()
	ctx := context.Background()

	require.NoError(t, svc.GrantLibraryAccess(ctx, "u4", "lib-old"))

	require.NoError(t, svc.ReplaceLibraryAccess(ctx, "u4", []string{"lib-1", "lib-2"}))

	ids, err := svc.GetLibraryAccessByUserID(ctx, "u4")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"lib-1", "lib-2"}, ids)
}

// ReplaceLibraryAccess with an empty list clears every grant, restoring
// unrestricted access (same convention as zero grants elsewhere).
func TestAccessService_ReplaceLibraryAccess_Empty_ClearsGrants(t *testing.T) {
	t.Parallel()
	svc, _ := newAccessSvcWithLibraryAccess()
	ctx := context.Background()

	require.NoError(t, svc.GrantLibraryAccess(ctx, "u5", "lib-1"))
	require.NoError(t, svc.ReplaceLibraryAccess(ctx, "u5", []string{}))

	ok, err := svc.HasLibraryAccess(ctx, "u5", "any-lib")
	require.NoError(t, err)
	assert.True(t, ok, "clearing all grants restores unrestricted access")
}

// A repo error during ReplaceLibraryAccess must not mutate the existing
// grant set: the whole point of the atomic replace is that a failure leaves
// the user with their PRIOR access, never zero. FakeLibraryAccessRepository
// checks Err before touching its map, exercising exactly that fail-closed
// contract at the layer the fake can prove.
func TestAccessService_ReplaceLibraryAccess_RepoError_LeavesExistingAccessIntact(t *testing.T) {
	t.Parallel()
	svc, la := newAccessSvcWithLibraryAccess()
	ctx := context.Background()

	require.NoError(t, svc.GrantLibraryAccess(ctx, "u6", "lib-old"))

	la.Err = errors.New("db down")
	err := svc.ReplaceLibraryAccess(ctx, "u6", []string{"lib-new"})
	require.Error(t, err)

	la.Err = nil
	ids, err := svc.GetLibraryAccessByUserID(ctx, "u6")
	require.NoError(t, err)
	assert.Equal(t, []string{"lib-old"}, ids, "failed replace must not have revoked the old grant")
}

// RevokeAllLibraryAccess clears every grant for the user in one call.
func TestAccessService_RevokeAllLibraryAccess(t *testing.T) {
	t.Parallel()
	svc, _ := newAccessSvcWithLibraryAccess()
	ctx := context.Background()

	require.NoError(t, svc.GrantLibraryAccess(ctx, "u7", "lib-1"))
	require.NoError(t, svc.GrantLibraryAccess(ctx, "u7", "lib-2"))

	require.NoError(t, svc.RevokeAllLibraryAccess(ctx, "u7"))

	ok, err := svc.HasLibraryAccess(ctx, "u7", "any-lib")
	require.NoError(t, err)
	assert.True(t, ok, "clearing all grants restores unrestricted access")
}

// Repo error on RevokeAllLibraryAccess propagates.
func TestAccessService_RevokeAllLibraryAccess_RepoError(t *testing.T) {
	t.Parallel()
	svc, la := newAccessSvcWithLibraryAccess()
	la.Err = errors.New("db down")
	err := svc.RevokeAllLibraryAccess(context.Background(), "u8")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}
