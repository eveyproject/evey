package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appmusic "github.com/evey-project/evey/internal/app/music"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/music"
	musicfakes "github.com/evey-project/evey/internal/domain/music/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
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
		ID: id, LibraryID: shared.LibraryID(libraryID), Title: title, Artist: artist,
		FilePath: "/music/" + id + ".mp3",
	})
}

// fakeMusicSearch returns its seeded tracks for any query, optionally scoped by
// library, so handler-level access filtering can be exercised without Meili.
type fakeMusicSearch struct{ tracks []music.Track }

func (f *fakeMusicSearch) IndexTrack(context.Context, music.Track) error         { return nil }
func (f *fakeMusicSearch) IndexTracksBatch(context.Context, []music.Track) error { return nil }
func (f *fakeMusicSearch) RemoveTrack(context.Context, string) error             { return nil }
func (f *fakeMusicSearch) Search(_ context.Context, libraryID shared.LibraryID, _ string, limit int) ([]music.Track, error) {
	var out []music.Track
	for _, t := range f.tracks {
		if libraryID != "" && t.LibraryID != libraryID {
			continue
		}
		out = append(out, t)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
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

// Shuffle draws from the WHOLE library, not from a first page: with a page
// of 50, a draw of 50 still reaches past the first 50 tracks, and it never
// leaves the library.
func TestMusicHandler_ListTracks_ShuffleDrawsFromTheWholeLibrary(t *testing.T) {
	h, repo, _, _ := newTestMusicHandler()
	for i := range 120 {
		seedTrack(repo, fmt.Sprintf("t-%03d", i), "lib-1", "Some Track", "Some Artist")
	}
	seedTrack(repo, "other", "lib-2", "Some Track", "Some Artist")

	seen := map[string]bool{}
	for range 10 {
		resp, err := h.ListTracks(adminCtx(), &eveyv1.ListTracksRequest{LibraryId: "lib-1", Shuffle: true, Limit: 50})
		require.NoError(t, err)
		require.Len(t, resp.Tracks, 50)
		for _, tr := range resp.Tracks {
			require.NotEqual(t, "other", tr.Id, "a draw never leaves the library")
			seen[tr.Id] = true
		}
	}
	// Ten draws of 50 from 120: a shuffle of a first page would never see
	// more than 50 distinct tracks.
	assert.Greater(t, len(seen), 50)
}

func TestMusicHandler_ListTracks_ByAlbumID(t *testing.T) {
	h, repo, _, _ := newTestMusicHandler()
	// Two albums in distinct folders (folder = album identity).
	repo.Upsert(context.Background(), music.Track{ID: "a1", LibraryID: "lib-1", Title: "A1", Album: "X", FilePath: "/m/AlbumX/01.mp3"})
	repo.Upsert(context.Background(), music.Track{ID: "a2", LibraryID: "lib-1", Title: "A2", Album: "X", FilePath: "/m/AlbumX/02.mp3"})
	repo.Upsert(context.Background(), music.Track{ID: "b1", LibraryID: "lib-1", Title: "B1", Album: "Y", FilePath: "/m/AlbumY/01.mp3"})

	resp, err := h.ListTracks(adminCtx(), &eveyv1.ListTracksRequest{
		LibraryId: "lib-1",
		AlbumId:   music.AlbumID("/m/AlbumX"),
	})

	require.NoError(t, err)
	assert.Equal(t, int32(2), resp.Total)
	assert.Len(t, resp.Tracks, 2)
	for _, tr := range resp.Tracks {
		assert.Equal(t, "X", tr.Album)
	}
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

func TestMusicHandler_GetTrack_Success(t *testing.T) {
	h, repo, _, _ := newTestMusicHandler()
	seedTrack(repo, "t-1", "lib-1", "Song A", "Artist A")

	resp, err := h.GetTrack(adminCtx(), &eveyv1.GetTrackRequest{Id: "t-1"})

	require.NoError(t, err)
	require.NotNil(t, resp.Track)
	assert.Equal(t, "t-1", resp.Track.Id)
	assert.Equal(t, "Song A", resp.Track.Title)
}

func TestMusicHandler_GetTrack_NotFound(t *testing.T) {
	h, _, _, _ := newTestMusicHandler()

	_, err := h.GetTrack(adminCtx(), &eveyv1.GetTrackRequest{Id: "missing"})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestMusicHandler_GetTrack_Unauthenticated(t *testing.T) {
	h, repo, _, _ := newTestMusicHandler()
	seedTrack(repo, "t-1", "lib-1", "Song A", "Artist A")

	_, err := h.GetTrack(context.Background(), &eveyv1.GetTrackRequest{Id: "t-1"})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// A user restricted to another library must not retrieve a track from lib-1,
// even with its id (the access check uses the track's own library).
func TestMusicHandler_GetTrack_NoAccess(t *testing.T) {
	h, repo, libraryAccess, _ := newTestMusicHandler()
	seedTrack(repo, "t-1", "lib-1", "Song A", "Artist A")
	libraryAccess.Grant(context.Background(), "user-2", "lib-other")

	_, err := h.GetTrack(userCtx("user-2", "profile-2"), &eveyv1.GetTrackRequest{Id: "t-1"})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestMusicHandler_GetTrack_WithAccess(t *testing.T) {
	h, repo, libraryAccess, _ := newTestMusicHandler()
	seedTrack(repo, "t-1", "lib-1", "Song A", "Artist A")
	libraryAccess.Grant(context.Background(), "user-2", "lib-1")

	resp, err := h.GetTrack(userCtx("user-2", "profile-2"), &eveyv1.GetTrackRequest{Id: "t-1"})

	require.NoError(t, err)
	require.NotNil(t, resp.Track)
	assert.Equal(t, "t-1", resp.Track.Id)
}

func TestMusicHandler_ListTracks_RepoError(t *testing.T) {
	h, repo, _, _ := newTestMusicHandler()
	repo.Err = errors.New("db down")

	_, err := h.ListTracks(adminCtx(), &eveyv1.ListTracksRequest{LibraryId: "lib-1"})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func seedTrackWithAlbumArtist(repo *musicfakes.FakeMusicTrackRepository, id, libraryID, albumArtist string) {
	repo.Upsert(context.Background(), music.Track{
		ID: id, LibraryID: shared.LibraryID(libraryID), AlbumArtist: albumArtist,
		FilePath: "/music/" + id + ".mp3",
	})
}

func TestMusicHandler_ListTracks_Cursor_KeysetPage(t *testing.T) {
	h, repo, _, _ := newTestMusicHandler()
	seedTrackWithAlbumArtist(repo, "t-1", "lib-1", "AA")
	seedTrackWithAlbumArtist(repo, "t-2", "lib-1", "AB")
	seedTrackWithAlbumArtist(repo, "t-3", "lib-1", "AC")

	// Cursor pointing just past t-1 (album_artist ASC order: t-1, t-2, t-3).
	cursor := encodeTrackCursor(music.Track{ID: "t-1", AlbumArtist: "AA"})

	resp, err := h.ListTracks(adminCtx(), &eveyv1.ListTracksRequest{LibraryId: "lib-1", Limit: 1, Cursor: cursor})

	require.NoError(t, err)
	assert.Equal(t, int32(0), resp.Total, "keyset path must not pay the COUNT(*) cost")
	require.Len(t, resp.Tracks, 1)
	assert.Equal(t, "t-2", resp.Tracks[0].Id)
	assert.NotEmpty(t, resp.NextCursor, "full page must carry a cursor for the following page")
}

func TestMusicHandler_ListTracks_Cursor_LastPage(t *testing.T) {
	h, repo, _, _ := newTestMusicHandler()
	seedTrackWithAlbumArtist(repo, "t-1", "lib-1", "AA")
	seedTrackWithAlbumArtist(repo, "t-2", "lib-1", "AB")

	cursor := encodeTrackCursor(music.Track{ID: "t-1", AlbumArtist: "AA"})

	resp, err := h.ListTracks(adminCtx(), &eveyv1.ListTracksRequest{LibraryId: "lib-1", Limit: 5, Cursor: cursor})

	require.NoError(t, err)
	require.Len(t, resp.Tracks, 1)
	assert.Equal(t, "t-2", resp.Tracks[0].Id)
	assert.Empty(t, resp.NextCursor, "an empty next_cursor is the only end-of-list signal on the keyset path")
}

func TestMusicHandler_ListTracks_Cursor_Invalid(t *testing.T) {
	h, repo, _, _ := newTestMusicHandler()
	seedTrack(repo, "t-1", "lib-1", "Song A", "Artist A")

	_, err := h.ListTracks(adminCtx(), &eveyv1.ListTracksRequest{LibraryId: "lib-1", Cursor: "not-a-valid-cursor"})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestMusicHandler_SearchMusic_CrossLibrary_FiltersByAccess(t *testing.T) {
	h, _, libraryAccess, _ := newTestMusicHandler()
	// Index has tracks in two libraries; the user may only access lib-1.
	h.svc.QueryService().WithSearchIndex(&fakeMusicSearch{tracks: []music.Track{
		{ID: "t1", LibraryID: "lib-1", Title: "Allowed", Artist: "A"},
		{ID: "t2", LibraryID: "lib-2", Title: "Forbidden", Artist: "B"},
	}})
	libraryAccess.Grant(context.Background(), "user-2", "lib-1")

	ctx := userCtx("user-2", "profile-2")
	resp, err := h.SearchMusic(ctx, &eveyv1.SearchMusicRequest{Query: "anything"}) // empty LibraryId = global

	require.NoError(t, err)
	require.Len(t, resp.Tracks, 1)
	assert.Equal(t, "Allowed", resp.Tracks[0].Title)
	assert.Equal(t, "lib-1", resp.Tracks[0].LibraryId)
}

func TestMusicHandler_SearchMusic_SpecificLibrary_DeniesUnauthorized(t *testing.T) {
	h, _, libraryAccess, _ := newTestMusicHandler()
	libraryAccess.Grant(context.Background(), "user-2", "lib-1")

	ctx := userCtx("user-2", "profile-2")
	_, err := h.SearchMusic(ctx, &eveyv1.SearchMusicRequest{LibraryId: "lib-2", Query: "x"})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// With no Meili index wired (simulating Meilisearch down), the global search
// must still return results via the SQL fallback's cross-library search - not an
// empty list - while still filtering by the user's accessible libraries.
func TestMusicHandler_SearchMusic_CrossLibrary_SQLFallbackNotEmpty(t *testing.T) {
	h, repo, libraryAccess, _ := newTestMusicHandler() // no search index → repo SQL fallback
	repo.Upsert(context.Background(), music.Track{ID: "t1", LibraryID: "lib-1", Title: "Numb", Artist: "Neon Harbor", FilePath: "/m/a/1.mp3"})
	repo.Upsert(context.Background(), music.Track{ID: "t2", LibraryID: "lib-2", Title: "Numbers", Artist: "Other", FilePath: "/m/b/1.mp3"})
	libraryAccess.Grant(context.Background(), "user-2", "lib-1")

	ctx := userCtx("user-2", "profile-2")
	resp, err := h.SearchMusic(ctx, &eveyv1.SearchMusicRequest{Query: "num"}) // global, Meili "down"

	require.NoError(t, err)
	require.Len(t, resp.Tracks, 1)
	assert.Equal(t, "Numb", resp.Tracks[0].Title)
	assert.Equal(t, "lib-1", resp.Tracks[0].LibraryId)
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

// The scan target must sit INSIDE one of the library's roots - a sibling
// whose name merely starts with the root ("/music2" under "/music") is
// outside - and an unknown library is NotFound; a valid target is accepted
// and the scan detached (count -1 = started).
func TestMusicHandler_ScanMusicDirectory_ConfinedToLibraryPaths(t *testing.T) {
	h, _, _, libQueryRepo := newTestMusicHandler()
	libQueryRepo.Seed(&library.Library{ID: "lib-m", Name: "Music", Type: library.Music, Paths: []string{"/music"}})

	for _, tc := range []struct {
		dir  string
		want codes.Code
	}{
		{"/music", codes.OK},
		{"/music/albums/2024", codes.OK},
		{"/music/../etc", codes.InvalidArgument},
		{"/music2", codes.InvalidArgument},
		{"/photos", codes.InvalidArgument},
	} {
		resp, err := h.ScanMusicDirectory(adminCtx(), &eveyv1.ScanMusicDirectoryRequest{LibraryId: "lib-m", Directory: tc.dir})
		assert.Equal(t, tc.want, status.Code(err), tc.dir)
		if tc.want == codes.OK {
			assert.EqualValues(t, -1, resp.GetCount(), "started, counted later")
		}
	}
	_, err := h.ScanMusicDirectory(adminCtx(), &eveyv1.ScanMusicDirectoryRequest{LibraryId: "lib-ghost", Directory: "/music"})
	assert.Equal(t, codes.NotFound, status.Code(err))
}
