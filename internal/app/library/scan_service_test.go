package library_test

import (
	"context"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanLibrary_ZeroFiles_NoError(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, bus := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"},
	})
	scanner.Results["/movies"] = nil

	added, updated, err := svc.ScanLibrary(context.Background(), "lib-1")

	require.NoError(t, err)
	assert.Equal(t, 0, added)
	assert.Equal(t, 0, updated)
	assert.Empty(t, mediaRepo.All())

	var scanStarted, scanCompleted int
	for _, e := range bus.PublishedEvents() {
		switch e.EventName() {
		case "library.scan.started":
			scanStarted++
		case "library.scan.completed":
			scanCompleted++
		}
	}
	assert.Equal(t, 1, scanStarted)
	assert.Equal(t, 1, scanCompleted)
}

func TestScanLibrary_NewFile_CreatesMediaItem(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, bus := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"},
	})
	scanner.Results["/movies"] = []library.ScannedFile{
		{
			Path:        "/movies/test.mkv",
			Size:        1024,
			ModTime:     time.Now(),
			Fingerprint: "fp-1",
			Title:       "Test Movie",
			Type:        library.MovieItem,
		},
	}

	added, _, err := svc.ScanLibrary(context.Background(), "lib-1")

	require.NoError(t, err)
	assert.Equal(t, 1, added)

	items := mediaRepo.All()
	require.Len(t, items, 1)
	assert.Equal(t, "Test Movie", items[0].Title)
	assert.Equal(t, "/movies/test.mkv", items[0].FilePath)
	assert.Equal(t, "lib-1", items[0].LibraryID)
	assert.Equal(t, library.MediaItemType("movie"), items[0].Type)
	assert.Equal(t, int64(1024), items[0].FileSize)
	assert.Equal(t, "fp-1", items[0].FileFingerprint)

	var addedEvts int
	for _, e := range bus.PublishedEvents() {
		if e.EventName() == "media.item.added" {
			addedEvts++
		}
	}
	assert.Equal(t, 1, addedEvts)
}

func TestScanLibrary_ExistingFile_NoDuplicate(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"},
	})

	now := time.Now().Truncate(time.Second)
	scanner.Results["/movies"] = []library.ScannedFile{
		{
			Path:        "/movies/test.mkv",
			Size:        1024,
			ModTime:     now,
			Fingerprint: "fp-1",
			Title:       "Test Movie",
			Type:        library.MovieItem,
		},
	}

	// first scan
	added1, _, err := svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 1, added1)

	// second scan with same file
	added2, updated2, err := svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 0, added2)
	assert.Equal(t, 1, updated2) // detected as existing, counted as updated

	items := mediaRepo.All()
	assert.Len(t, items, 1, "should not create duplicate")
}

func TestScanLibrary_MultipleNewFiles(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/movies"},
	})
	scanner.Results["/movies"] = []library.ScannedFile{
		{Path: "/movies/a.mkv", Size: 100, Fingerprint: "fp-a", Title: "Movie A", Type: library.MovieItem},
		{Path: "/movies/b.mkv", Size: 200, Fingerprint: "fp-b", Title: "Movie B", Type: library.MovieItem},
		{Path: "/movies/c.mkv", Size: 300, Fingerprint: "fp-c", Title: "Movie C", Type: library.MovieItem},
	}

	added, _, err := svc.ScanLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, 3, added)
	assert.Len(t, mediaRepo.All(), 3)
}

func TestScanLibrary_NonexistentLibrary_Error(t *testing.T) {
	t.Parallel()
	svc, _, _, _, _, _ := newCmdSvc()

	_, _, err := svc.ScanLibrary(context.Background(), "does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get library")
}
