package library_test

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/stretchr/testify/require"
)

// An upgrade that lands under a new name with a fresh mtime is skipped as
// still being written on the pass that sees the old file gone. That pass
// must not prune the old row: the rescan booked for when the file settles
// matches it against the row it replaces, and the item keeps its identity.
func TestScanService_UpgradeStillSettling_KeepsRowUntilMatched(t *testing.T) {
	h := newScanHarness()
	h.svc.WithFileSettle(time.Minute)
	lib := &library.Library{ID: "lib-m", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)
	ctx := context.Background()
	old := time.Now().Add(-2 * time.Hour)
	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/Film (2020)/Film.2020.720p.mkv", Size: 500, Fingerprint: "500-1", ModTime: old, Title: "Film", Year: intPtr(2020), Type: library.MovieItem},
	}
	_, _, err := h.svc.ScanLibrary(ctx, "lib-m")
	require.NoError(t, err)
	before := itemsByType(h.media, "movie")
	require.Len(t, before, 1)
	id := before[0].ID

	// The upgrade: new name, new size, written seconds ago; the old file gone.
	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/Film (2020)/Film.2020.1080p.mkv", Size: 1500, Fingerprint: "1500-2", ModTime: time.Now().Add(-5 * time.Second), Title: "Film", Year: intPtr(2020), Type: library.MovieItem},
	}
	_, _, err = h.svc.ScanLibrary(ctx, "lib-m")
	require.NoError(t, err)
	_, getErr := h.media.GetByID(ctx, id)
	require.NoError(t, getErr, "the old row survives the pass that skipped its replacement")

	// The settle rescan: the file is now old enough to be analysed.
	h.scanner.Results["/movies"][0].ModTime = old
	_, _, err = h.svc.ScanLibrary(ctx, "lib-m")
	require.NoError(t, err)
	final := itemsByType(h.media, "movie")
	require.Len(t, final, 1)
	require.Equal(t, id, final[0].ID, "the upgraded file keeps the row it replaces")
	require.Equal(t, "/movies/Film (2020)/Film.2020.1080p.mkv", final[0].File.Path)
}

// One series split over two roots: a second copy of an episode appearing
// under the first root must not take over the row of the file that still
// sits under the second root, whatever the file system answers about it.
// Every root is listed before any is processed, so the candidate's path is
// already among the paths seen this pass.
func TestScanService_TwoRoots_CandidateUnderLaterRoot_IsNotReplaced(t *testing.T) {
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()
	libQueryRepo.Seed(&library.Library{ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv1", "/tv2"}})
	scanner.Results["/tv1"] = nil
	scanner.Results["/tv2"] = []library.ScannedFile{
		{Path: "/tv2/Show/S01/Show.S01E01.mkv", Size: 500, Fingerprint: "500-1", Title: "Show", Season: intPtr(1), Episode: intPtr(1), Type: library.Episode},
	}
	_, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)
	before := itemsByType(mediaRepo, "episode")
	require.Len(t, before, 1)
	id := before[0].ID

	scanner.Results["/tv1"] = []library.ScannedFile{
		{Path: "/tv1/Show/S01/Show.S01E01.1080p.mkv", Size: 1500, Fingerprint: "1500-2", Title: "Show", Season: intPtr(1), Episode: intPtr(1), Type: library.Episode},
	}
	_, _, err = svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)
	require.Len(t, itemsByType(mediaRepo, "episode"), 2)
	got, err := mediaRepo.GetByID(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, "/tv2/Show/S01/Show.S01E01.mkv", got.File.Path, "the row under the second root keeps its file")
}

// A movie renamed to a file whose name carries no year still takes over the
// row of the same folder and title.
func TestScanService_ReplacedMovie_WithoutYear_StillMatches(t *testing.T) {
	h := newScanHarness()
	lib := &library.Library{ID: "lib-m", Name: "Movies", Type: "movie", Paths: []string{"/movies"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)
	ctx := context.Background()
	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/Film (2020)/Film.2020.720p.mkv", Size: 500, Fingerprint: "500-1", Title: "Film", Year: intPtr(2020), Type: library.MovieItem},
	}
	_, _, err := h.svc.ScanLibrary(ctx, "lib-m")
	require.NoError(t, err)
	before := itemsByType(h.media, "movie")
	require.Len(t, before, 1)
	id := before[0].ID

	h.scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/Film (2020)/Film.mkv", Size: 1500, Fingerprint: "1500-2", Title: "Film", Type: library.MovieItem},
	}
	_, _, err = h.svc.ScanLibrary(ctx, "lib-m")
	require.NoError(t, err)
	final := itemsByType(h.media, "movie")
	require.Len(t, final, 1)
	require.Equal(t, id, final[0].ID)
	require.Equal(t, "/movies/Film (2020)/Film.mkv", final[0].File.Path)
}
