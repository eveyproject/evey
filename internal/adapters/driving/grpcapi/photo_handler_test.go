package grpcapi

import (
	"context"
	"errors"
	"testing"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appphoto "github.com/gravity-zero/evey/internal/app/photo"
	"github.com/gravity-zero/evey/internal/domain/photo"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	photofakes "github.com/gravity-zero/evey/internal/domain/photo/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestPhotoHandler() (*photoHandler, *photofakes.FakeRepository, *userfakes.FakeLibraryAccessRepository, *libfakes.FakeLibraryQueryRepo) {
	photoRepo := photofakes.NewFakeRepository()
	scanner := photofakes.NewFakePhotoScanner()
	bus := sharedfakes.NewFakeEventBus()
	libQueryRepo := libfakes.NewFakeLibraryQueryRepo()
	libMediaQuery := libfakes.NewFakeMediaItemQueryRepo()
	libStreamRepo := libfakes.NewFakeMediaStreamRepository()
	libSearchIdx := libfakes.NewFakeSearchIndex()
	libraryAccess := userfakes.NewFakeLibraryAccessRepository()

	svc := appphoto.NewService(photoRepo, bus, libQueryRepo, scanner)
	libQuery := applibrary.NewQueryService(libQueryRepo, libMediaQuery, libStreamRepo, libSearchIdx)

	h := &photoHandler{
		svc:           svc,
		libraryAccess: libraryAccess,
		libQuery:      libQuery,
	}
	return h, photoRepo, libraryAccess, libQueryRepo
}

func seedPhoto(repo *photofakes.FakeRepository, id, libraryID, title string) {
	repo.Upsert(context.Background(), photo.Photo{
		ID: id, LibraryID: libraryID, Title: title,
		FilePath: "/photos/" + id + ".jpg",
	})
}

func TestPhotoHandler_ListPhotos_Success(t *testing.T) {
	h, repo, _, _ := newTestPhotoHandler()
	seedPhoto(repo, "p-1", "lib-1", "Sunset")
	seedPhoto(repo, "p-2", "lib-1", "Mountain")

	resp, err := h.ListPhotos(adminCtx(), &eveyv1.ListPhotosRequest{LibraryId: "lib-1"})

	require.NoError(t, err)
	assert.Equal(t, int32(2), resp.Total)
	assert.Len(t, resp.Photos, 2)
}

func TestPhotoHandler_ListPhotos_Unauthenticated(t *testing.T) {
	h, _, _, _ := newTestPhotoHandler()

	_, err := h.ListPhotos(context.Background(), &eveyv1.ListPhotosRequest{LibraryId: "lib-1"})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestPhotoHandler_ListPhotos_MissingLibraryID(t *testing.T) {
	h, _, _, _ := newTestPhotoHandler()

	_, err := h.ListPhotos(adminCtx(), &eveyv1.ListPhotosRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestPhotoHandler_ListPhotos_NoAccess(t *testing.T) {
	h, repo, libraryAccess, _ := newTestPhotoHandler()
	seedPhoto(repo, "p-1", "lib-1", "Sunset")
	// grant access to different library to create a restriction
	libraryAccess.Grant(context.Background(), "user-2", "lib-other")

	ctx := userCtx("user-2", "profile-2")
	_, err := h.ListPhotos(ctx, &eveyv1.ListPhotosRequest{LibraryId: "lib-1"})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestPhotoHandler_ListPhotos_WithAccess(t *testing.T) {
	h, repo, libraryAccess, _ := newTestPhotoHandler()
	seedPhoto(repo, "p-1", "lib-1", "Sunset")
	libraryAccess.Grant(context.Background(), "user-2", "lib-1")

	ctx := userCtx("user-2", "profile-2")
	resp, err := h.ListPhotos(ctx, &eveyv1.ListPhotosRequest{LibraryId: "lib-1"})

	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.Total)
}

func TestPhotoHandler_ListPhotos_RepoError(t *testing.T) {
	h, repo, _, _ := newTestPhotoHandler()
	repo.Err = errors.New("db down")

	_, err := h.ListPhotos(adminCtx(), &eveyv1.ListPhotosRequest{LibraryId: "lib-1"})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestPhotoHandler_GetPhoto_Success(t *testing.T) {
	h, repo, _, _ := newTestPhotoHandler()
	seedPhoto(repo, "p-1", "lib-1", "Sunset")

	resp, err := h.GetPhoto(adminCtx(), &eveyv1.GetPhotoRequest{Id: "p-1"})

	require.NoError(t, err)
	assert.Equal(t, "p-1", resp.Id)
	assert.Equal(t, "Sunset", resp.Title)
}

func TestPhotoHandler_GetPhoto_NotFound(t *testing.T) {
	h, _, _, _ := newTestPhotoHandler()

	_, err := h.GetPhoto(adminCtx(), &eveyv1.GetPhotoRequest{Id: "nonexistent"})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestPhotoHandler_GetPhoto_Unauthenticated(t *testing.T) {
	h, _, _, _ := newTestPhotoHandler()

	_, err := h.GetPhoto(context.Background(), &eveyv1.GetPhotoRequest{Id: "p-1"})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestPhotoHandler_ListPhotoAlbums_Success(t *testing.T) {
	h, repo, _, _ := newTestPhotoHandler()
	repo.Upsert(context.Background(), photo.Photo{
		ID: "p-1", LibraryID: "lib-1", Title: "Beach", Album: "Vacation",
		FilePath: "/photos/p-1.jpg",
	})
	repo.Upsert(context.Background(), photo.Photo{
		ID: "p-2", LibraryID: "lib-1", Title: "Pool", Album: "Vacation",
		FilePath: "/photos/p-2.jpg",
	})

	resp, err := h.ListPhotoAlbums(adminCtx(), &eveyv1.ListPhotoAlbumsRequest{LibraryId: "lib-1"})

	require.NoError(t, err)
	assert.Len(t, resp.Albums, 1)
	assert.Equal(t, "Vacation", resp.Albums[0].Name)
	assert.Equal(t, int32(2), resp.Albums[0].PhotoCount)
}

func TestPhotoHandler_ListPhotoAlbums_MissingLibraryID(t *testing.T) {
	h, _, _, _ := newTestPhotoHandler()

	_, err := h.ListPhotoAlbums(adminCtx(), &eveyv1.ListPhotoAlbumsRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestPhotoHandler_SearchPhotos_Success(t *testing.T) {
	h, repo, _, _ := newTestPhotoHandler()
	seedPhoto(repo, "p-1", "lib-1", "Beach Sunset")
	seedPhoto(repo, "p-2", "lib-1", "Mountain View")

	resp, err := h.SearchPhotos(adminCtx(), &eveyv1.SearchPhotosRequest{
		LibraryId: "lib-1", Query: "Beach",
	})

	require.NoError(t, err)
	assert.Len(t, resp.Photos, 1)
	assert.Equal(t, "Beach Sunset", resp.Photos[0].Title)
}

func TestPhotoHandler_SearchPhotos_MissingArgs(t *testing.T) {
	h, _, _, _ := newTestPhotoHandler()

	_, err := h.SearchPhotos(adminCtx(), &eveyv1.SearchPhotosRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestPhotoHandler_DeletePhoto_Success(t *testing.T) {
	h, repo, _, _ := newTestPhotoHandler()
	seedPhoto(repo, "p-1", "lib-1", "To Delete")

	_, err := h.DeletePhoto(adminCtx(), &eveyv1.DeletePhotoRequest{Id: "p-1"})

	require.NoError(t, err)
}

func TestPhotoHandler_DeletePhoto_NotFound(t *testing.T) {
	h, _, _, _ := newTestPhotoHandler()

	_, err := h.DeletePhoto(adminCtx(), &eveyv1.DeletePhotoRequest{Id: "nonexistent"})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestPhotoHandler_DeletePhoto_Unauthenticated(t *testing.T) {
	h, _, _, _ := newTestPhotoHandler()

	_, err := h.DeletePhoto(context.Background(), &eveyv1.DeletePhotoRequest{Id: "p-1"})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestPhotoHandler_ScanPhotoDirectory_NonAdmin(t *testing.T) {
	h, _, _, _ := newTestPhotoHandler()
	ctx := userCtx("user-2", "profile-2")

	_, err := h.ScanPhotoDirectory(ctx, &eveyv1.ScanPhotoDirectoryRequest{
		LibraryId: "lib-1", Directory: "/media/photos",
	})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestPhotoHandler_ScanPhotoDirectory_MissingArgs(t *testing.T) {
	h, _, _, _ := newTestPhotoHandler()

	_, err := h.ScanPhotoDirectory(adminCtx(), &eveyv1.ScanPhotoDirectoryRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestPhotoHandler_ScanPhotoDirectory_RelativePath(t *testing.T) {
	h, _, _, _ := newTestPhotoHandler()

	_, err := h.ScanPhotoDirectory(adminCtx(), &eveyv1.ScanPhotoDirectoryRequest{
		LibraryId: "lib-1", Directory: "relative/path",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}
