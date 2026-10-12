package library_test

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seriesID resolved through seasonParent tracking in pruneEmptyContainers.
// Tests in this file assert the returned ItemsRemoved count to kill
// INCREMENT_DECREMENT mutations on pruned++ / childCount--.

// Kills mutation at scan_service_tvshow.go:33:26 (ex.FileFingerprint != item.FileFingerprint).
// Pre-seed a series with stale fingerprint but matching title/type/parent;
// rescan must refresh the stored fingerprint via Update.
func TestScanTVShow_TitleMatch_UpdatesStaleFingerprint(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	})

	// Pre-existing series with the "wrong" fingerprint (simulating older scan)
	staleSeries := &library.MediaItem{
		ID:        "series-existing",
		LibraryID: "lib-tv",
		Type:      "series",
		Title:     "Show",
		File:      &library.MediaFile{Fingerprint: "series:lib-tv:Show:STALE"},
	}
	require.NoError(t, mediaRepo.Create(context.Background(), staleSeries))

	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/Show/S01/e01.mkv", Size: 100, Title: "Show", Season: intPtr(1), Episode: intPtr(1), Type: library.Episode},
	}
	_, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	// Still one series, same ID, but fingerprint refreshed to canonical one.
	series := itemsByType(mediaRepo, "series")
	require.Len(t, series, 1)
	assert.Equal(t, shared.MediaItemID("series-existing"), series[0].ID)
	require.NotNil(t, series[0].File)
	assert.Equal(t, "series:lib-tv:show", series[0].File.Fingerprint,
		"title-matched series must have fingerprint refreshed via Update")
}

// Kills mutation at scan_service_tvshow.go:22:27 (item.FileFingerprint != "").
// Seed a series matching the canonical fingerprint but with a DIFFERENT title.
// Correct behavior: byFingerprint hit reuses the existing series.
// Mutation (== ""): non-empty fingerprint items are excluded from the map, so
// fingerprint lookup misses, title lookup misses (different title), and a
// duplicate series is created.
func TestScanTVShow_FingerprintHit_ReusesExistingSeriesDespiteTitleDiff(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	})

	// canonical fingerprint for "Show" in lib-tv (title part lower-cased); we
	// store it under a renamed title.
	require.NoError(t, mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "series-existing", LibraryID: "lib-tv", Type: "series",
		Title: "OldTitle",
		File:  &library.MediaFile{Fingerprint: "series:lib-tv:show"},
	}))

	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/Show/S01/e01.mkv", Size: 100, Title: "Show", Season: intPtr(1), Episode: intPtr(1), Type: library.Episode},
	}
	_, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	series := itemsByType(mediaRepo, "series")
	require.Len(t, series, 1, "series must be looked up via fingerprint (non-empty) map")
	assert.Equal(t, shared.MediaItemID("series-existing"), series[0].ID)
}

// Kills mutation at scan_service_tvshow.go:160:9 (childCount[item.ParentID]--).
// When ALL seasons of a series are pruned, the series must also be pruned via
// the decrement cascade. Flipping the decrement to no-op leaves series orphaned.
func TestScanTVShow_PruneCascade_SeriesDeletedWhenAllSeasonsEmpty(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, bus := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	})

	// First scan creates TWO shows (series + season + episode each). A second
	// kept show keeps the next scan non-empty: an empty scan of a non-empty
	// library now trips the degraded-mount safety guard and refuses to prune (it
	// would otherwise wipe the catalog + cascade user data).
	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/Show/S01/e01.mkv", Size: 100, Title: "Show", Season: new(1), Episode: new(1), Type: library.Episode},
		{Path: "/tv/Kept/S01/e01.mkv", Size: 100, Title: "Kept", Season: new(1), Episode: new(1), Type: library.Episode},
	}
	_, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)
	require.Len(t, itemsByType(mediaRepo, "series"), 2)
	require.Len(t, itemsByType(mediaRepo, "season"), 2)
	require.Len(t, itemsByType(mediaRepo, "episode"), 2)

	// Second scan: only "Kept" survives, "Show"'s file is gone. Show's episode is
	// orphaned, then prune runs: its season becomes empty → deleted; series
	// childCount decrements to 0 → series also deleted. "Kept" is untouched.
	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/Kept/S01/e01.mkv", Size: 100, Title: "Kept", Season: new(1), Episode: new(1), Type: library.Episode},
	}
	_, _, err = svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	require.Len(t, itemsByType(mediaRepo, "episode"), 1, "Show episode pruned, Kept remains")
	require.Len(t, itemsByType(mediaRepo, "season"), 1, "Show empty season pruned, Kept remains")
	require.Len(t, itemsByType(mediaRepo, "series"), 1,
		"Show series pruned via decrement cascade when last season deleted, Kept remains")

	// Kills INCREMENT_DECREMENT mutation at scan_service_tvshow.go:160:9 (pruned++).
	// The scan-completed event carries ItemsRemoved = orphanEpisodes + prunedContainers.
	// Expected: 1 episode + 1 season + 1 series = 3. pruned-- would give 1-1-1 = -1.
	var removed int
	var found bool
	for _, e := range bus.PublishedEvents() {
		if sc, ok := e.(library.LibraryScanCompleted); ok {
			removed = sc.ItemsRemoved
			found = true
		}
	}
	require.True(t, found, "LibraryScanCompleted event must be published")
	assert.EqualValues(t, 3, removed, "removed count = orphan episode + pruned season + pruned series")
}
