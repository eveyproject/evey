package grpcapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/user"
)

// The no-library sentinel is a filter for the list handlers, not an id: the
// admin view never shows it, and a save that echoes it back never stores it.
func TestLibraryAccessHandler_NeverExposesOrStoresTheSentinel(t *testing.T) {
	h, accessRepo, _ := newTestLibraryAccessHandler()
	ctx := context.Background()
	_ = accessRepo.Grant(ctx, "user-1", "lib-gone")
	_ = accessRepo.Revoke(ctx, "user-1", "lib-gone")

	resp, err := h.GetUserLibraryAccess(ctx, &eveyv1.GetUserLibraryAccessRequest{UserId: "user-1"})
	require.NoError(t, err)
	assert.NotContains(t, resp.LibraryIds, user.NoLibraryAccess)

	_, err = h.SetUserLibraryAccess(ctx, &eveyv1.SetUserLibraryAccessRequest{UserId: "user-1", LibraryIds: []string{user.NoLibraryAccess}})
	require.NoError(t, err)
	restricted, err := accessRepo.IsRestricted(ctx, "user-1")
	require.NoError(t, err)
	assert.True(t, restricted, "echoing the sentinel alone must not lift the restriction")

	_, err = h.SetUserLibraryAccess(ctx, &eveyv1.SetUserLibraryAccessRequest{UserId: "user-1", LibraryIds: []string{user.NoLibraryAccess, "lib-1"}})
	require.NoError(t, err)
	ids, err := accessRepo.GetByUserID(ctx, "user-1")
	require.NoError(t, err)
	assert.Len(t, ids, 1)
	assert.EqualValues(t, "lib-1", ids[0])
}
