package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/music"
	"github.com/evey-project/evey/internal/domain/playlist"
	"github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
)

type fakePlaylistSource struct {
	pl         *playlist.Playlist
	plErr      error
	mediaItems []*domainlibrary.MediaItem // smart-playlist hydration slot
	plItems    []*playlist.PlaylistItem   // manual-playlist raw-items slot
	itemsErr   error
}

func (f *fakePlaylistSource) GetByID(_ context.Context, _ string, _ shared.ProfileID) (*playlist.Playlist, error) {
	return f.pl, f.plErr
}

func (f *fakePlaylistSource) GetItems(_ context.Context, _ string, _ shared.ProfileID) ([]*domainlibrary.MediaItem, []*playlist.PlaylistItem, error) {
	return f.mediaItems, f.plItems, f.itemsErr
}

type fakeMusicResolver struct{ tracks map[string]*music.Track }

func (f *fakeMusicResolver) GetTrack(_ context.Context, id string) (*music.Track, error) {
	return f.tracks[id], nil
}

func runPlaylistDownload(t *testing.T, h *playlistHandler, claims *domainuser.Claims) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/playlists/p1/download", nil)
	req.SetPathValue("id", "p1")
	req = withHTTPClaims(req, claims)
	w := httptest.NewRecorder()
	h.downloadPlaylist(w, req)
	return w
}

// A manual playlist's items come back in the PlaylistItem slot, with music
// tracks (resolved from music_tracks) and media items (movies/episodes) mixed.
func TestDownloadPlaylist_Manual_MediaAndMusic(t *testing.T) {
	dir := t.TempDir()
	movie := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", Type: domainlibrary.MovieItem, Title: "Some Movie", File: &domainlibrary.MediaFile{Path: archiveTempFile(t, dir, "some-movie.mkv", "movie")}}
	track := &music.Track{ID: "t1", LibraryID: "lib2", Title: "Song", FilePath: archiveTempFile(t, dir, "song.flac", "audio")}

	h := &playlistHandler{
		plSvc: &fakePlaylistSource{
			pl: &playlist.Playlist{Name: "Mine", ProfileID: "p1", Type: playlist.Manual},
			plItems: []*playlist.PlaylistItem{
				{MediaItemID: "m1", MediaType: playlist.MediaTypeVideo},
				{MediaItemID: "t1", MediaType: playlist.MediaTypeMusic},
			},
		},
		libSvc:     &fakeMediaItemGetter{item: movie},
		music:      &fakeMusicResolver{tracks: map[string]*music.Track{"t1": track}},
		libChecker: allowAll(),
		baseURL:    "http://localhost",
	}
	w := runPlaylistDownload(t, h, testClaims(false))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", ct)
	}
	got := zipEntryNames(t, w.Body.Bytes())
	want := []string{"Some Movie.mkv", "Song.flac"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("zip entries = %v, want %v", got, want)
	}
}

// A smart playlist hydrates to media items directly (the first GetItems slot).
func TestDownloadPlaylist_Smart_ResolvedSnapshot(t *testing.T) {
	dir := t.TempDir()
	movie := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", Type: domainlibrary.MovieItem, Title: "Rule Match", File: &domainlibrary.MediaFile{Path: archiveTempFile(t, dir, "a.mkv", "x")}}
	h := &playlistHandler{
		plSvc:      &fakePlaylistSource{pl: &playlist.Playlist{Name: "Auto", ProfileID: "p1", Type: playlist.Smart}, mediaItems: []*domainlibrary.MediaItem{movie}},
		libSvc:     &fakeMediaItemGetter{},
		libChecker: allowAll(),
		baseURL:    "http://localhost",
	}
	w := runPlaylistDownload(t, h, testClaims(false))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	got := zipEntryNames(t, w.Body.Bytes())
	want := []string{"Rule Match.mkv"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("smart playlist zip entries = %v, want %v", got, want)
	}
}

func TestDownloadPlaylist_NotOwner_NotFound(t *testing.T) {
	// The service returns an error when the caller isn't the owner; the handler
	// maps that to 404 so a non-owner can't even confirm the playlist exists.
	h := &playlistHandler{
		plSvc:      &fakePlaylistSource{pl: nil, plErr: errTest},
		libSvc:     &fakeMediaItemGetter{},
		libChecker: allowAll(),
		baseURL:    "http://localhost",
	}
	w := runPlaylistDownload(t, h, testClaims(false))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-owner/missing playlist, got %d", w.Code)
	}
}

func TestDownloadPlaylist_NoFiles_NotFound(t *testing.T) {
	h := &playlistHandler{
		plSvc:      &fakePlaylistSource{pl: &playlist.Playlist{Name: "Empty", ProfileID: "p1", Type: playlist.Manual}},
		libSvc:     &fakeMediaItemGetter{},
		libChecker: allowAll(),
		baseURL:    "http://localhost",
	}
	w := runPlaylistDownload(t, h, testClaims(false))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for playlist with no downloadable files, got %d", w.Code)
	}
}

// A download token is bound to the one playlist it was minted for; replayed on
// another it must be refused before the archive is built. Neutralising the
// check left the package green (sabotage sweep, 2026-09-28).
func TestDownloadPlaylist_TokenBoundElsewhere_Forbidden(t *testing.T) {
	movie := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", Type: domainlibrary.MovieItem, Title: "Some Movie", File: &domainlibrary.MediaFile{Path: archiveTempFile(t, t.TempDir(), "some-movie.mkv", "movie")}}
	h := &playlistHandler{
		plSvc: &fakePlaylistSource{
			pl:      &playlist.Playlist{Name: "Mine", ProfileID: "p1", Type: playlist.Manual},
			plItems: []*playlist.PlaylistItem{{MediaItemID: "m1", MediaType: playlist.MediaTypeVideo}},
		},
		libSvc:     &fakeMediaItemGetter{item: movie},
		music:      &fakeMusicResolver{},
		libChecker: allowAll(),
		baseURL:    "http://localhost",
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/playlists/p1/download", nil)
	req.SetPathValue("id", "p1")
	req = withHTTPClaims(req, testClaims(false))
	req = req.WithContext(context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{UserID: "user-1", MediaItemID: "p-other", Scope: shared.ScopeStream}))
	w := httptest.NewRecorder()
	h.downloadPlaylist(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("a token bound to another playlist must be refused, got %d", w.Code)
	}
}

// A playlist track whose file lies outside every library root is left out of
// the archive, next to a title that is kept: without it the playlist route
// read any file a track row pointed at (sabotage sweep, 2026-09-28).
func TestDownloadPlaylist_TrackOutsideRootsLeftOut(t *testing.T) {
	root := t.TempDir()
	movie := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", Type: domainlibrary.MovieItem, Title: "Some Movie", File: &domainlibrary.MediaFile{Path: archiveTempFile(t, root, "some-movie.mkv", "movie")}}
	track := &music.Track{ID: "t1", LibraryID: "lib1", Title: "Song", FilePath: archiveTempFile(t, t.TempDir(), "song.flac", "audio")}
	h := &playlistHandler{
		plSvc: &fakePlaylistSource{
			pl: &playlist.Playlist{Name: "Mine", ProfileID: "p1", Type: playlist.Manual},
			plItems: []*playlist.PlaylistItem{
				{MediaItemID: "m1", MediaType: playlist.MediaTypeVideo},
				{MediaItemID: "t1", MediaType: playlist.MediaTypeMusic},
			},
		},
		libSvc:     &fakeMediaItemGetter{item: movie, libraries: []*domainlibrary.Library{{ID: "lib1", Paths: []string{root}}}},
		music:      &fakeMusicResolver{tracks: map[string]*music.Track{"t1": track}},
		libChecker: allowAll(),
		baseURL:    "http://localhost",
	}
	w := runPlaylistDownload(t, h, testClaims(false))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	if got := zipEntryNames(t, w.Body.Bytes()); !reflect.DeepEqual(got, []string{"Some Movie.mkv"}) {
		t.Errorf("zip entries = %v, want only the in-roots title", got)
	}
}
