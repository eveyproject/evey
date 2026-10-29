package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLibraryAccessHandler() (*libraryAccessHandler, *userfakes.FakeLibraryAccessRepository, *libfakes.FakeLibraryQueryRepo) {
	accessRepo := userfakes.NewFakeLibraryAccessRepository()
	libQueryRepo := libfakes.NewFakeLibraryQueryRepo()
	mediaQueryRepo := libfakes.NewFakeMediaItemQueryRepo()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	searchIdx := libfakes.NewFakeSearchIndex()
	querySvc := applibrary.NewQueryService(libQueryRepo, mediaQueryRepo, streamRepo, searchIdx)

	userAccess := appuser.NewAccessService(
		userfakes.NewFakeUserRepository(),
		userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
		userfakes.NewFakePasswordHasher(),
		sharedfakes.NewFakeEventBus(),
		appuser.WithLibraryAccess(accessRepo),
	)

	h := &libraryAccessHandler{
		userAccess: userAccess,
		libQuery:   querySvc,
	}
	return h, accessRepo, libQueryRepo
}

func TestLibraryAccessHandler_GetUserLibraryAccess_Empty(t *testing.T) {
	h, _, _ := newTestLibraryAccessHandler()

	resp, err := h.GetUserLibraryAccess(context.Background(), &eveyv1.GetUserLibraryAccessRequest{UserId: "user-1"})

	require.NoError(t, err)
	assert.Equal(t, "user-1", resp.UserId)
	assert.Empty(t, resp.LibraryIds)
}

func TestLibraryAccessHandler_GetUserLibraryAccess_WithGrants(t *testing.T) {
	h, accessRepo, libQueryRepo := newTestLibraryAccessHandler()

	libQueryRepo.Seed(&library.Library{ID: "lib-1", Name: "Movies", Type: "movie"})
	libQueryRepo.Seed(&library.Library{ID: "lib-2", Name: "TV Shows", Type: "tvshow"})
	_ = accessRepo.Grant(context.Background(), "user-1", "lib-1")
	_ = accessRepo.Grant(context.Background(), "user-1", "lib-2")

	resp, err := h.GetUserLibraryAccess(context.Background(), &eveyv1.GetUserLibraryAccessRequest{UserId: "user-1"})

	require.NoError(t, err)
	assert.Equal(t, "user-1", resp.UserId)
	assert.Len(t, resp.LibraryIds, 2)
	assert.Len(t, resp.Libraries, 2)
}

func TestLibraryAccessHandler_SetUserLibraryAccess_ReplacesPrevious(t *testing.T) {
	h, accessRepo, _ := newTestLibraryAccessHandler()

	_ = accessRepo.Grant(context.Background(), "user-1", "lib-old")

	_, err := h.SetUserLibraryAccess(context.Background(), &eveyv1.SetUserLibraryAccessRequest{
		UserId:     "user-1",
		LibraryIds: []string{"lib-1", "lib-2"},
	})
	require.NoError(t, err)

	resp, err := h.GetUserLibraryAccess(context.Background(), &eveyv1.GetUserLibraryAccessRequest{UserId: "user-1"})
	require.NoError(t, err)
	assert.Len(t, resp.LibraryIds, 2)
	assert.NotContains(t, resp.LibraryIds, "lib-old")
}

func TestLibraryAccessHandler_SetUserLibraryAccess_Empty(t *testing.T) {
	h, accessRepo, _ := newTestLibraryAccessHandler()

	_ = accessRepo.Grant(context.Background(), "user-1", "lib-1")

	_, err := h.SetUserLibraryAccess(context.Background(), &eveyv1.SetUserLibraryAccessRequest{
		UserId:     "user-1",
		LibraryIds: []string{},
	})
	require.NoError(t, err)

	resp, err := h.GetUserLibraryAccess(context.Background(), &eveyv1.GetUserLibraryAccessRequest{UserId: "user-1"})
	require.NoError(t, err)
	assert.Empty(t, resp.LibraryIds)
	// An empty set confines the user to no library: it used to lift the
	// restriction, so unticking every box in the admin modal gave access to
	// the whole catalogue.
	assert.True(t, resp.Restricted, "an empty set means no library, not every library")
	has, err := accessRepo.HasAccess(context.Background(), "user-1", "lib-2")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestLibraryAccessHandler_RevokeAllUserLibraryAccess(t *testing.T) {
	h, accessRepo, _ := newTestLibraryAccessHandler()

	_ = accessRepo.Grant(context.Background(), "user-1", "lib-1")
	_ = accessRepo.Grant(context.Background(), "user-1", "lib-2")

	_, err := h.RevokeAllUserLibraryAccess(context.Background(), &eveyv1.RevokeAllUserLibraryAccessRequest{UserId: "user-1"})
	require.NoError(t, err)

	resp, err := h.GetUserLibraryAccess(context.Background(), &eveyv1.GetUserLibraryAccessRequest{UserId: "user-1"})
	require.NoError(t, err)
	assert.Empty(t, resp.LibraryIds)
}

// The core atomicity guarantee: a repo failure during SetUserLibraryAccess
// must not leave the user at zero access. The former implementation revoked
// every existing grant in one loop, then granted the new set in a second
// loop - an error between the two loops (or a process death) left the user
// revoked from everything with nothing re-granted. The atomic replace must
// fail closed onto the PRIOR set instead.
func TestLibraryAccessHandler_SetUserLibraryAccess_RepoError_DoesNotZeroOutAccess(t *testing.T) {
	h, accessRepo, _ := newTestLibraryAccessHandler()

	_ = accessRepo.Grant(context.Background(), "user-1", "lib-old")

	accessRepo.Err = assert.AnError
	_, err := h.SetUserLibraryAccess(context.Background(), &eveyv1.SetUserLibraryAccessRequest{
		UserId:     "user-1",
		LibraryIds: []string{"lib-1", "lib-2"},
	})
	require.Error(t, err)

	accessRepo.Err = nil
	resp, err := h.GetUserLibraryAccess(context.Background(), &eveyv1.GetUserLibraryAccessRequest{UserId: "user-1"})
	require.NoError(t, err)
	assert.Equal(t, []string{"lib-old"}, resp.LibraryIds, "a failed replace must leave the prior grants untouched, not zero them out")
}
