package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	appplaylist "github.com/eveyproject/evey/internal/app/playlist"
	appuser "github.com/eveyproject/evey/internal/app/user"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/eveyproject/evey/internal/domain/music"
	"github.com/eveyproject/evey/internal/domain/playlist"
	playlistfakes "github.com/eveyproject/evey/internal/domain/playlist/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubPlaylistTracks map[string]*music.Track

func (s stubPlaylistTracks) GetTracksByIDs(_ context.Context, ids []string) ([]music.Track, error) {
	out := make([]music.Track, 0, len(ids))
	for _, id := range ids {
		if t, ok := s[id]; ok {
			out = append(out, *t)
		}
	}
	return out, nil
}

// A server built without the music module keeps a nil reader, not a nil
// service wrapped in a non-nil interface.
func TestPlaylistTracksFrom_NilServiceIsANilReader(t *testing.T) {
	assert.Nil(t, playlistTracksFrom(nil))
}

// newMusicPlaylist builds a handler wired like production, with one manual
// playlist holding a single music track, and returns the caller's context.
func newMusicPlaylist(t *testing.T, tracks stubPlaylistTracks, grantedLibrary shared.LibraryID) (*playlistHandler, context.Context, string) {
	t.Helper()
	itemRepo := playlistfakes.NewFakePlaylistItemRepository()
	mediaReader := libfakes.NewFakeMediaItemQueryRepo()
	svc := appplaylist.NewService(playlistfakes.NewFakePlaylistRepository(), itemRepo, playlistfakes.NewFakeSmartPlaylistQuerier(), mediaReader)
	libQuery := applibrary.NewQueryService(libfakes.NewFakeLibraryQueryRepo(), mediaReader, libfakes.NewFakeMediaStreamRepository(), libfakes.NewFakeSearchIndex())
	libAccess := userfakes.NewFakeLibraryAccessRepository()
	require.NoError(t, libAccess.Grant(context.Background(), "user-1", grantedLibrary))
	userAccess := appuser.NewAccessService(
		userfakes.NewFakeUserRepository(), userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(), userfakes.NewFakeQuickConnectRepository(),
		userfakes.NewFakePasswordHasher(), sharedfakes.NewFakeEventBus(),
		appuser.WithLibraryAccess(libAccess),
	)
	h := &playlistHandler{svc: svc, libQuery: libQuery, userAccess: userAccess, music: tracks}
	ctx := ctxWithClaims("user-1", "profile-1", false)
	created, err := h.CreatePlaylist(ctx, &eveyv1.CreatePlaylistRequest{Name: "Mixed", Type: eveyv1.PlaylistType_PLAYLIST_TYPE_MANUAL})
	require.NoError(t, err)
	require.NoError(t, itemRepo.Add(ctx, &playlist.PlaylistItem{PlaylistID: created.Id, MediaItemID: "t-1", MediaType: playlist.MediaTypeMusic}))
	return h, ctx, created.Id
}

// A music track is not a media item, so nothing named it: every client showed
// the bare track id on the row. The track's own title, artist, album, cover
// and length now ride the playlist answer.
func TestGetPlaylistItems_MusicTrackCarriesItsOwnMetadata(t *testing.T) {
	tracks := stubPlaylistTracks{"t-1": {
		ID: "t-1", LibraryID: "lib-music", Title: "A song", Artist: "An artist", Album: "An album",
		CoverArtURL: "/api/v1/music/tracks/t-1/art", Duration: 201.5,
	}}
	h, ctx, id := newMusicPlaylist(t, tracks, "lib-music")

	resp, err := h.GetPlaylistItems(ctx, &eveyv1.GetPlaylistItemsRequest{Id: id})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	it := resp.Items[0]
	assert.Equal(t, "music", it.MediaType)
	assert.Equal(t, "A song", it.Title)
	assert.Equal(t, "An artist", it.Artist)
	assert.Equal(t, "An album", it.Album)
	assert.Equal(t, "/api/v1/music/tracks/t-1/art", it.CoverArtUrl)
	assert.InDelta(t, 201.5, it.Duration, 0.001)
}

// The same library gate as a video row: a profile without access to the music
// library does not read the track's title back through a playlist.
func TestGetPlaylistItems_MusicTrackOutsideTheCallersLibrariesIsHidden(t *testing.T) {
	tracks := stubPlaylistTracks{"t-1": {ID: "t-1", LibraryID: "lib-music", Title: "A song"}}
	h, ctx, id := newMusicPlaylist(t, tracks, "lib-other")

	resp, err := h.GetPlaylistItems(ctx, &eveyv1.GetPlaylistItemsRequest{Id: id})
	require.NoError(t, err)
	for _, it := range resp.Items {
		assert.NotEqual(t, "A song", it.Title, "a track of a library the caller cannot see was named")
	}
	assert.Empty(t, resp.Items)
}
