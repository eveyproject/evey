package grpcapi

import (
	"context"
	"errors"
	"testing"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appphoto "github.com/evey-project/evey/internal/app/photo"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/photo"
	photofakes "github.com/evey-project/evey/internal/domain/photo/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
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
	userAccess := appuser.NewAccessService(
		userfakes.NewFakeUserRepository(),
		userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
		userfakes.NewFakePasswordHasher(),
		bus,
		appuser.WithLibraryAccess(libraryAccess),
	)

	h := &photoHandler{
		svc:        svc,
		userAccess: userAccess,
		libQuery:   libQuery,
	}
	return h, photoRepo, libraryAccess, libQueryRepo
}

func seedPhoto(repo *photofakes.FakeRepository, id, libraryID, title string) {
	repo.Upsert(context.Background(), photo.Photo{
		ID: id, LibraryID: shared.LibraryID(libraryID), Title: title,
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

func seedPhotoWithCreatedAt(repo *photofakes.FakeRepository, id, libraryID string, createdAt time.Time) {
	repo.Upsert(context.Background(), photo.Photo{
		ID: id, LibraryID: shared.LibraryID(libraryID), FilePath: "/photos/" + id + ".jpg", CreatedAt: createdAt,
	})
}

func TestPhotoHandler_ListPhotos_Cursor_KeysetPage(t *testing.T) {
	h, repo, _, _ := newTestPhotoHandler()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedPhotoWithCreatedAt(repo, "p-1", "lib-1", base)
	seedPhotoWithCreatedAt(repo, "p-2", "lib-1", base.Add(time.Hour))
	seedPhotoWithCreatedAt(repo, "p-3", "lib-1", base.Add(2*time.Hour))

	// Cursor pointing just past p-3 (created_at DESC order: p-3, p-2, p-1).
	cursor := encodePhotoCursor(photo.Photo{ID: "p-3", CreatedAt: base.Add(2 * time.Hour)})

	resp, err := h.ListPhotos(adminCtx(), &eveyv1.ListPhotosRequest{LibraryId: "lib-1", Limit: 1, Cursor: cursor})

	require.NoError(t, err)
	assert.Equal(t, int32(0), resp.Total, "keyset path must not pay the COUNT(*) cost")
	require.Len(t, resp.Photos, 1)
	assert.Equal(t, "p-2", resp.Photos[0].Id)
	assert.NotEmpty(t, resp.NextCursor, "full page must carry a cursor for the following page")
}

// The offset path must hand out the first cursor, otherwise keyset paging can
// never be entered: a client only ever sends back a cursor it was given. This
// walks the real bootstrap - page 1 without a cursor, page 2 with the one it
// returned - and pins that no row is skipped or repeated across the seam.
func TestPhotoHandler_ListPhotos_OffsetPageBootstrapsTheCursor(t *testing.T) {
	h, repo, _, _ := newTestPhotoHandler()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedPhotoWithCreatedAt(repo, "p-1", "lib-1", base)
	seedPhotoWithCreatedAt(repo, "p-2", "lib-1", base.Add(time.Hour))
	seedPhotoWithCreatedAt(repo, "p-3", "lib-1", base.Add(2*time.Hour))

	first, err := h.ListPhotos(adminCtx(), &eveyv1.ListPhotosRequest{LibraryId: "lib-1", Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, int32(3), first.Total, "page 1 keeps its exact total")
	require.Len(t, first.Photos, 2)
	require.NotEmpty(t, first.NextCursor, "page 1 must hand out a cursor or keyset is unreachable")

	second, err := h.ListPhotos(adminCtx(), &eveyv1.ListPhotosRequest{
		LibraryId: "lib-1", Limit: 2, Cursor: first.NextCursor,
	})
	require.NoError(t, err)
	assert.Equal(t, int32(0), second.Total, "page 2 is keyset-served, it pays no COUNT")

	var got []string
	for _, p := range append(first.Photos, second.Photos...) {
		got = append(got, p.Id)
	}
	assert.Equal(t, []string{"p-3", "p-2", "p-1"}, got, "no row skipped or repeated across the seam")
}

func TestPhotoHandler_ListPhotos_Cursor_LastPage(t *testing.T) {
	h, repo, _, _ := newTestPhotoHandler()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedPhotoWithCreatedAt(repo, "p-1", "lib-1", base)
	seedPhotoWithCreatedAt(repo, "p-2", "lib-1", base.Add(time.Hour))

	cursor := encodePhotoCursor(photo.Photo{ID: "p-2", CreatedAt: base.Add(time.Hour)})

	resp, err := h.ListPhotos(adminCtx(), &eveyv1.ListPhotosRequest{LibraryId: "lib-1", Limit: 5, Cursor: cursor})

	require.NoError(t, err)
	require.Len(t, resp.Photos, 1)
	assert.Equal(t, "p-1", resp.Photos[0].Id)
	assert.Empty(t, resp.NextCursor, "an empty next_cursor is the only end-of-list signal on the keyset path")
}

func TestPhotoHandler_ListPhotos_Cursor_Invalid(t *testing.T) {
	h, repo, _, _ := newTestPhotoHandler()
	seedPhoto(repo, "p-1", "lib-1", "Sunset")

	_, err := h.ListPhotos(adminCtx(), &eveyv1.ListPhotosRequest{LibraryId: "lib-1", Cursor: "not-a-valid-cursor"})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
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
		ID: "p-1", LibraryID: shared.LibraryID("lib-1"), Title: "Beach", Album: "Vacation",
		FilePath: "/photos/p-1.jpg",
	})
	repo.Upsert(context.Background(), photo.Photo{
		ID: "p-2", LibraryID: shared.LibraryID("lib-1"), Title: "Pool", Album: "Vacation",
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

// A member deleting a photo from a library they were not granted is refused and
// the photo stays.
func TestPhotoHandler_DeletePhoto_NonAdminOutsideTheirLibraryDenied(t *testing.T) {
	h, repo, libraryAccess, _ := newTestPhotoHandler()
	seedPhoto(repo, "p-1", "lib-1", "Keep Me")
	libraryAccess.Grant(context.Background(), "user-2", "lib-2")

	_, err := h.DeletePhoto(userCtx("user-2", "profile-2"), &eveyv1.DeletePhotoRequest{Id: "p-1"})

	require.Error(t, err)
	got, gErr := repo.GetByID(context.Background(), "p-1")
	require.NoError(t, gErr)
	assert.NotNil(t, got, "the photo must still be there")
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

func TestPhotoHandler_ScanPhotoDirectory_LibraryNotFound(t *testing.T) {
	h, _, _, _ := newTestPhotoHandler()

	_, err := h.ScanPhotoDirectory(adminCtx(), &eveyv1.ScanPhotoDirectoryRequest{
		LibraryId: "lib-missing", Directory: "/media/photos",
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestPhotoHandler_ScanPhotoDirectory_DirectoryOutsideLibrary(t *testing.T) {
	h, _, _, libQueryRepo := newTestPhotoHandler()
	libQueryRepo.Seed(&library.Library{
		ID: "lib-1", Name: "Photos", Type: "photo",
		Paths: []string{"/media/photos"},
	})

	_, err := h.ScanPhotoDirectory(adminCtx(), &eveyv1.ScanPhotoDirectoryRequest{
		LibraryId: "lib-1", Directory: "/elsewhere/random",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestPhotoHandler_ScanPhotoDirectory_Unauthenticated(t *testing.T) {
	h, _, _, _ := newTestPhotoHandler()

	_, err := h.ScanPhotoDirectory(context.Background(), &eveyv1.ScanPhotoDirectoryRequest{
		LibraryId: "lib-1", Directory: "/media/photos",
	})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestPhotoHandler_ListPhotos_WithAlbumFilter(t *testing.T) {
	h, repo, _, _ := newTestPhotoHandler()
	repo.Upsert(context.Background(), photo.Photo{
		ID: "p-1", LibraryID: shared.LibraryID("lib-1"), Title: "Beach", Album: "Vacation",
		FilePath: "/photos/p-1.jpg",
	})
	repo.Upsert(context.Background(), photo.Photo{
		ID: "p-2", LibraryID: shared.LibraryID("lib-1"), Title: "Pool", Album: "Vacation",
		FilePath: "/photos/p-2.jpg",
	})
	repo.Upsert(context.Background(), photo.Photo{
		ID: "p-3", LibraryID: shared.LibraryID("lib-1"), Title: "Snow", Album: "Winter",
		FilePath: "/photos/p-3.jpg",
	})

	resp, err := h.ListPhotos(adminCtx(), &eveyv1.ListPhotosRequest{
		LibraryId: "lib-1", Album: "Vacation",
	})

	require.NoError(t, err)
	assert.Equal(t, int32(2), resp.Total)
	assert.Len(t, resp.Photos, 2)
	for _, p := range resp.Photos {
		assert.Equal(t, "Vacation", p.Album)
	}
}

func TestPhotoHandler_GetPhoto_NoAccess(t *testing.T) {
	h, repo, libraryAccess, _ := newTestPhotoHandler()
	seedPhoto(repo, "p-1", "lib-1", "Sunset")
	libraryAccess.Grant(context.Background(), "user-2", "lib-other")

	ctx := userCtx("user-2", "profile-2")
	_, err := h.GetPhoto(ctx, &eveyv1.GetPhotoRequest{Id: "p-1"})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestPhotoHandler_ListPhotoAlbums_NoAccess(t *testing.T) {
	h, _, libraryAccess, _ := newTestPhotoHandler()
	libraryAccess.Grant(context.Background(), "user-2", "lib-other")

	ctx := userCtx("user-2", "profile-2")
	_, err := h.ListPhotoAlbums(ctx, &eveyv1.ListPhotoAlbumsRequest{LibraryId: "lib-1"})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestPhotoHandler_SearchPhotos_NoAccess(t *testing.T) {
	h, _, libraryAccess, _ := newTestPhotoHandler()
	libraryAccess.Grant(context.Background(), "user-2", "lib-other")

	ctx := userCtx("user-2", "profile-2")
	_, err := h.SearchPhotos(ctx, &eveyv1.SearchPhotosRequest{
		LibraryId: "lib-1", Query: "test",
	})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}
