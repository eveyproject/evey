package grpcapi

import (
	"context"
	"errors"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appmusic "github.com/evey-project/evey/internal/app/music"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/music"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	musicfakes "github.com/evey-project/evey/internal/domain/music/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestMusicHandler() (*musicHandler, *musicfakes.FakeMusicTrackRepository, *userfakes.FakeLibraryAccessRepository, *libfakes.FakeLibraryQueryRepo) {
	trackRepo := musicfakes.NewFakeMusicTrackRepository()
	scanner := musicfakes.NewFakeMusicScanner()
	bus := sharedfakes.NewFakeEventBus()
	libQueryRepo := libfakes.NewFakeLibraryQueryRepo()
	libMediaQuery := libfakes.NewFakeMediaItemQueryRepo()
	libStreamRepo := libfakes.NewFakeMediaStreamRepository()
	libSearchIdx := libfakes.NewFakeSearchIndex()
	libraryAccess := userfakes.NewFakeLibraryAccessRepository()

	svc := appmusic.NewService(trackRepo, bus, libQueryRepo, scanner)
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

	h := &musicHandler{
		svc:        svc,
		userAccess: userAccess,
		libQuery:   libQuery,
	}
	return h, trackRepo, libraryAccess, libQueryRepo
}

func seedTrack(repo *musicfakes.FakeMusicTrackRepository, id, libraryID, title, artist string) {
	repo.Upsert(context.Background(), music.Track{
		ID: id, LibraryID: libraryID, Title: title, Artist: artist,
		FilePath: "/music/" + id + ".mp3",
	})
}

func TestMusicHandler_ListTracks_Success(t *testing.T) {
	h, repo, _, _ := newTestMusicHandler()
	seedTrack(repo, "t-1", "lib-1", "Song A", "Artist A")
	seedTrack(repo, "t-2", "lib-1", "Song B", "Artist B")

	resp, err := h.ListTracks(adminCtx(), &eveyv1.ListTracksRequest{LibraryId: "lib-1"})

	require.NoError(t, err)
	assert.Equal(t, int32(2), resp.Total)
	assert.Len(t, resp.Tracks, 2)
}

func TestMusicHandler_ListTracks_Unauthenticated(t *testing.T) {
	h, _, _, _ := newTestMusicHandler()

	_, err := h.ListTracks(context.Background(), &eveyv1.ListTracksRequest{LibraryId: "lib-1"})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestMusicHandler_ListTracks_NoAccess(t *testing.T) {
	h, repo, libraryAccess, _ := newTestMusicHandler()
	seedTrack(repo, "t-1", "lib-1", "Song A", "Artist A")
	// grant access to different library to create a restriction
	libraryAccess.Grant(context.Background(), "user-2", "lib-other")

	ctx := userCtx("user-2", "profile-2")
	_, err := h.ListTracks(ctx, &eveyv1.ListTracksRequest{LibraryId: "lib-1"})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestMusicHandler_ListTracks_WithAccess(t *testing.T) {
	h, repo, libraryAccess, _ := newTestMusicHandler()
	seedTrack(repo, "t-1", "lib-1", "Song A", "Artist A")
	libraryAccess.Grant(context.Background(), "user-2", "lib-1")

	ctx := userCtx("user-2", "profile-2")
	resp, err := h.ListTracks(ctx, &eveyv1.ListTracksRequest{LibraryId: "lib-1"})

	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.Total)
}

func TestMusicHandler_ListTracks_RepoError(t *testing.T) {
	h, repo, _, _ := newTestMusicHandler()
	repo.Err = errors.New("db down")

	_, err := h.ListTracks(adminCtx(), &eveyv1.ListTracksRequest{LibraryId: "lib-1"})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestMusicHandler_GetAlbums_Success(t *testing.T) {
	h, repo, _, _ := newTestMusicHandler()
	seedTrack(repo, "t-1", "lib-1", "Track 1", "Artist A")

	resp, err := h.GetAlbums(adminCtx(), &eveyv1.GetAlbumsRequest{LibraryId: "lib-1"})

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestMusicHandler_GetArtists_Success(t *testing.T) {
	h, repo, _, _ := newTestMusicHandler()
	seedTrack(repo, "t-1", "lib-1", "Track 1", "Artist A")

	resp, err := h.GetArtists(adminCtx(), &eveyv1.GetArtistsRequest{LibraryId: "lib-1"})

	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Len(t, resp.Artists, 1)
}

func TestMusicHandler_GetArtists_Unauthenticated(t *testing.T) {
	h, _, _, _ := newTestMusicHandler()

	_, err := h.GetArtists(context.Background(), &eveyv1.GetArtistsRequest{LibraryId: "lib-1"})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestMusicHandler_SearchMusic_Success(t *testing.T) {
	h, repo, _, _ := newTestMusicHandler()
	seedTrack(repo, "t-1", "lib-1", "Hello World", "Artist A")
	seedTrack(repo, "t-2", "lib-1", "Goodbye", "Artist B")

	resp, err := h.SearchMusic(adminCtx(), &eveyv1.SearchMusicRequest{LibraryId: "lib-1", Query: "Hello"})

	require.NoError(t, err)
	assert.Len(t, resp.Tracks, 1)
	assert.Equal(t, "Hello World", resp.Tracks[0].Title)
}

func TestMusicHandler_DeleteTrack_Success(t *testing.T) {
	h, repo, _, _ := newTestMusicHandler()
	seedTrack(repo, "t-1", "lib-1", "To Delete", "Artist A")

	_, err := h.DeleteTrack(adminCtx(), &eveyv1.DeleteTrackRequest{Id: "t-1"})

	require.NoError(t, err)
}

func TestMusicHandler_DeleteTrack_NotFound(t *testing.T) {
	h, _, _, _ := newTestMusicHandler()

	_, err := h.DeleteTrack(adminCtx(), &eveyv1.DeleteTrackRequest{Id: "nonexistent"})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestMusicHandler_DeleteTrack_Unauthenticated(t *testing.T) {
	h, _, _, _ := newTestMusicHandler()

	_, err := h.DeleteTrack(context.Background(), &eveyv1.DeleteTrackRequest{Id: "t-1"})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestMusicHandler_ScanMusicDirectory_NonAdmin(t *testing.T) {
	h, _, _, _ := newTestMusicHandler()
	ctx := userCtx("user-2", "profile-2")

	_, err := h.ScanMusicDirectory(ctx, &eveyv1.ScanMusicDirectoryRequest{
		LibraryId: "lib-1", Directory: "/media/music",
	})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestMusicHandler_ScanMusicDirectory_MissingArgs(t *testing.T) {
	h, _, _, _ := newTestMusicHandler()

	_, err := h.ScanMusicDirectory(adminCtx(), &eveyv1.ScanMusicDirectoryRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestMusicHandler_ScanMusicDirectory_RelativePath(t *testing.T) {
	h, _, _, _ := newTestMusicHandler()

	_, err := h.ScanMusicDirectory(adminCtx(), &eveyv1.ScanMusicDirectoryRequest{
		LibraryId: "lib-1", Directory: "relative/path",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}
