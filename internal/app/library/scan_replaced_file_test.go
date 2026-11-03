package library_test

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A quality upgrade by a download manager lands as a new file name AND a new
// size in the same folder: neither the path nor the size+mtime fingerprint
// finds the row, so the scan created a second item and the prune deleted the
// first - with every viewer's progress, favourites, playlists and metadata
// overrides cascading after it. The replaced row keeps its id.
func TestScan_ReplacedEpisodeUnderANewName_KeepsItsItem(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, bus := newCmdSvc()
	libQueryRepo.Seed(&library.Library{ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"}})
	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/MyShow/S01/MyShow.S01E01.720p.mkv", Size: 500, Fingerprint: "500-1", Title: "MyShow", Season: new(1), Episode: new(1), Type: library.Episode},
	}
	_, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)
	before := itemsByType(mediaRepo, "episode")
	require.Len(t, before, 1)
	id := before[0].ID

	// The upgrade: new name, new size, the old file gone.
	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/MyShow/S01/MyShow.S01E01.1080p.mkv", Size: 1500, Fingerprint: "1500-2", Title: "MyShow", Season: new(1), Episode: new(1), Type: library.Episode},
	}
	added, updated, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)
	assert.Equal(t, 0, added, "nothing new: the file stands in for the row it replaces")
	assert.Equal(t, 1, updated)
	after := itemsByType(mediaRepo, "episode")
	require.Len(t, after, 1, "one episode, the same row")
	assert.Equal(t, id, after[0].ID)
	assert.Equal(t, "/tv/MyShow/S01/MyShow.S01E01.1080p.mkv", after[0].File.Path)
	assert.Equal(t, int64(1500), after[0].File.Size)
	var replaced int
	for _, e := range bus.PublishedEvents() {
		if u, ok := e.(library.MediaItemUpdated); ok && u.MediaItemID == id && u.Reason == library.UpdateReasonFileReplaced {
			replaced++
		}
	}
	assert.Equal(t, 1, replaced, "announced as a replaced file, the subscribers remake what hangs off the bytes")
}

func TestScan_ReplacedMovieUnderANewName_KeepsItsItem(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()
	libQueryRepo.Seed(&library.Library{ID: "lib-m", Name: "Movies", Type: "movie", Paths: []string{"/movies"}})
	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/Film (2020)/Film.2020.720p.mkv", Size: 500, Fingerprint: "500-1", Title: "Film", Year: new(2020), Type: library.MovieItem},
	}
	_, _, err := svc.ScanLibrary(context.Background(), "lib-m")
	require.NoError(t, err)
	before := itemsByType(mediaRepo, "movie")
	require.Len(t, before, 1)
	id := before[0].ID

	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/Film (2020)/Film.2020.1080p.mkv", Size: 1500, Fingerprint: "1500-2", Title: "Film", Year: new(2020), Type: library.MovieItem},
	}
	added, _, err := svc.ScanLibrary(context.Background(), "lib-m")
	require.NoError(t, err)
	assert.Equal(t, 0, added)
	after := itemsByType(mediaRepo, "movie")
	require.Len(t, after, 1)
	assert.Equal(t, id, after[0].ID)
	assert.Equal(t, "/movies/Film (2020)/Film.2020.1080p.mkv", after[0].File.Path)
}

// Two files side by side are two items: a candidate whose own file is still
// on disk is not a replacement, and a same-titled film in another folder is
// another film.
func TestScan_ANewFileNextToAnExistingOne_IsANewItem(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()
	libQueryRepo.Seed(&library.Library{ID: "lib-m", Name: "Movies", Type: "movie", Paths: []string{"/movies"}})
	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/Film (2020)/Film.2020.720p.mkv", Size: 500, Fingerprint: "500-1", Title: "Film", Year: new(2020), Type: library.MovieItem},
	}
	_, _, err := svc.ScanLibrary(context.Background(), "lib-m")
	require.NoError(t, err)

	scanner.ExistingPaths = map[string]bool{"/movies/Film (2020)/Film.2020.720p.mkv": true}
	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/Film (2020)/Film.2020.720p.mkv", Size: 500, Fingerprint: "500-1", Title: "Film", Year: new(2020), Type: library.MovieItem},
		{Path: "/movies/Film (2020)/Film.2020.1080p.mkv", Size: 1500, Fingerprint: "1500-2", Title: "Film", Year: new(2020), Type: library.MovieItem},
		{Path: "/movies/Other/Film.2020.1080p.mkv", Size: 1600, Fingerprint: "1600-3", Title: "Film", Year: new(2020), Type: library.MovieItem},
	}
	added, _, err := svc.ScanLibrary(context.Background(), "lib-m")
	require.NoError(t, err)
	assert.Equal(t, 2, added, "the side-by-side upgrade and the other folder's film are new items")
	assert.Len(t, itemsByType(mediaRepo, "movie"), 3)
}
