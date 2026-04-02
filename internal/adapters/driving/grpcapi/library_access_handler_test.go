package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	"github.com/gravity-zero/evey/internal/domain/library"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
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

	h := &libraryAccessHandler{
		accessRepo: accessRepo,
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
