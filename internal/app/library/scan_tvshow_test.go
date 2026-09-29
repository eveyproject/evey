package library_test

import (
	"context"
	"testing"

	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(v int) *int { return &v }

func itemsByType(repo *libfakes.FakeMediaItemCommandRepo, t string) []*library.MediaItem {
	var out []*library.MediaItem
	for _, item := range repo.All() {
		if string(item.Type) == t {
			out = append(out, item)
		}
	}
	return out
}

func TestScanTVShow_SingleEpisode_CreatesHierarchy(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, bus := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	})

	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/MyShow/S01/e01.mkv", Size: 500, Title: "MyShow", Season: intPtr(1), Episode: intPtr(1), Type: library.Episode},
	}

	added, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	series := itemsByType(mediaRepo, "series")
	require.Len(t, series, 1)
	assert.Equal(t, "MyShow", series[0].Title)

	seasons := itemsByType(mediaRepo, "season")
	require.Len(t, seasons, 1)
	assert.Equal(t, "Season 1", seasons[0].Title)
	assert.Equal(t, string(series[0].ID), seasons[0].ParentID)

	episodes := itemsByType(mediaRepo, "episode")
	require.Len(t, episodes, 1)
	assert.Equal(t, "S01E01", episodes[0].Title)
	assert.Equal(t, string(seasons[0].ID), episodes[0].ParentID)
	require.NotNil(t, episodes[0].File)
	assert.Equal(t, "/tv/MyShow/S01/e01.mkv", episodes[0].File.Path)

	// series + season + episode = 3 added
	assert.Equal(t, 3, added)

	var addedEvents int
	for _, e := range bus.PublishedEvents() {
		if e.EventName() == "media.item.added" {
			addedEvents++
		}
	}
	assert.GreaterOrEqual(t, addedEvents, 1)
}

func TestScanTVShow_MultipleSeasons(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	})

	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/Show/S01/e01.mkv", Size: 100, Title: "Show", Season: intPtr(1), Episode: intPtr(1), Type: library.Episode},
		{Path: "/tv/Show/S01/e02.mkv", Size: 200, Title: "Show", Season: intPtr(1), Episode: intPtr(2), Type: library.Episode},
		{Path: "/tv/Show/S02/e01.mkv", Size: 300, Title: "Show", Season: intPtr(2), Episode: intPtr(1), Type: library.Episode},
	}

	added, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	series := itemsByType(mediaRepo, "series")
	require.Len(t, series, 1, "one series")

	seasons := itemsByType(mediaRepo, "season")
	require.Len(t, seasons, 2, "two seasons")

	episodes := itemsByType(mediaRepo, "episode")
	require.Len(t, episodes, 3, "three episodes")

	// 1 series + 2 seasons + 3 episodes = 6
	assert.Equal(t, 6, added)
}

func TestScanTVShow_MultipleShows(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	})

	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/ShowA/S01/e01.mkv", Size: 100, Title: "ShowA", Season: intPtr(1), Episode: intPtr(1), Type: library.Episode},
		{Path: "/tv/ShowB/S01/e01.mkv", Size: 200, Title: "ShowB", Season: intPtr(1), Episode: intPtr(1), Type: library.Episode},
	}

	added, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	series := itemsByType(mediaRepo, "series")
	require.Len(t, series, 2, "two distinct series")

	titles := map[string]bool{}
	for _, s := range series {
		titles[s.Title] = true
	}
	assert.True(t, titles["ShowA"])
	assert.True(t, titles["ShowB"])

	// 2 series + 2 seasons + 2 episodes = 6
	assert.Equal(t, 6, added)
}

func TestScanTVShow_NilSeason_DefaultsToZero(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	})

	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/Show/special.mkv", Size: 100, Title: "Show", Season: nil, Episode: intPtr(1), Type: library.Episode},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	seasons := itemsByType(mediaRepo, "season")
	require.Len(t, seasons, 1)
	assert.Equal(t, "Season 0", seasons[0].Title)
	assert.NotNil(t, seasons[0].IndexNumber)
	assert.Equal(t, 0, *seasons[0].IndexNumber)

	episodes := itemsByType(mediaRepo, "episode")
	require.Len(t, episodes, 1)
	assert.Equal(t, "S00E01", episodes[0].Title)
}

func TestScanTVShow_NilEpisode_UsesOriginalTitle(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	})

	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/Show/bonus.mkv", Size: 100, Title: "Show", Season: intPtr(1), Episode: nil, Type: library.Episode},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	episodes := itemsByType(mediaRepo, "episode")
	require.Len(t, episodes, 1)
	assert.Equal(t, "Show", episodes[0].Title, "without episode number, original title preserved")
}

func TestScanTVShow_EmptyDirectory(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	})

	scanner.Results["/tv"] = []library.ScannedFile{}

	added, updated, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)
	assert.Equal(t, 0, added)
	assert.Equal(t, 0, updated)
	assert.Empty(t, mediaRepo.All())
}

func TestScanTVShow_SpecialCharactersInTitle(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	})

	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/L'Étrange Show (2020)/S01/e01.mkv", Size: 100, Title: "L'Étrange Show (2020)", Season: intPtr(1), Episode: intPtr(1), Type: library.Episode},
	}

	added, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)
	assert.Equal(t, 3, added)

	series := itemsByType(mediaRepo, "series")
	require.Len(t, series, 1)
	assert.Equal(t, "L'Étrange Show (2020)", series[0].Title)
}

func TestScanTVShow_RescanExisting_UpdatesEpisode(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, _, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	})

	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/Show/S01/e01.mkv", Size: 100, Title: "Show", Season: intPtr(1), Episode: intPtr(1), Type: library.Episode},
	}

	added1, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)
	assert.Equal(t, 3, added1)

	// Rescan same file with different size (file replaced)
	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/Show/S01/e01.mkv", Size: 999, Title: "Show", Season: intPtr(1), Episode: intPtr(1), Type: library.Episode},
	}

	added2, updated2, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)
	assert.Equal(t, 0, added2, "no new items on rescan")
	assert.Equal(t, 1, updated2, "episode updated")
}

func TestScanTVShow_RescanUnchanged_NoUpdates(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	})

	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/Show/S01/e01.mkv", Size: 100, Fingerprint: "fp1", Title: "Show", Season: intPtr(1), Episode: intPtr(1), Type: library.Episode},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	epsBefore := itemsByType(mediaRepo, "episode")
	require.Len(t, epsBefore, 1)

	// Rescan with identical size + fingerprint
	added2, updated2, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)
	assert.Equal(t, 0, added2)
	assert.Equal(t, 1, updated2, "unchanged file still counted as updated (upsert path)")
}

func TestScanTVShow_EpisodeTitleFormatPadding(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	})

	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/Show/S03/e12.mkv", Size: 100, Title: "Show", Season: intPtr(3), Episode: intPtr(12), Type: library.Episode},
		{Path: "/tv/Show/S10/e05.mkv", Size: 100, Title: "Show", Season: intPtr(10), Episode: intPtr(5), Type: library.Episode},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	episodes := itemsByType(mediaRepo, "episode")
	titles := map[string]bool{}
	for _, ep := range episodes {
		titles[ep.Title] = true
	}
	assert.True(t, titles["S03E12"], "season 3 episode 12")
	assert.True(t, titles["S10E05"], "season 10 episode 5")
}

func TestScanTVShow_SeriesFingerprint(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	})

	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/Show/S01/e01.mkv", Size: 100, Title: "Show", Season: intPtr(1), Episode: intPtr(1), Type: library.Episode},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	series := itemsByType(mediaRepo, "series")
	require.Len(t, series, 1)
	require.NotNil(t, series[0].File)
	// Fingerprint title part is lower-cased so mixed-casing filenames of the
	// same show share one fingerprint (see TestScanTVShow_MixedCaseTitle).
	assert.Equal(t, "series:lib-tv:show", series[0].File.Fingerprint)

	seasons := itemsByType(mediaRepo, "season")
	require.Len(t, seasons, 1)
	require.NotNil(t, seasons[0].File)
	assert.Equal(t, "season:lib-tv:show:1", seasons[0].File.Fingerprint)
}

// Regression for the duplicate-series bug: episode filenames of the same show
// with inconsistent casing ("Some Series" vs "Some series") must group under ONE
// series, not split into two fiches with seasons scattered between them.
func TestScanTVShow_MixedCaseTitle_SingleSeries(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	})

	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/media2/series/Some Series/S02/Some Series S02E01.mkv", Size: 100, Title: "Some Series", Season: new(2), Episode: new(1), Type: library.Episode},
		{Path: "/media2/series/Some Series/S05/Some series S05E08.mkv", Size: 200, Title: "Some series", Season: new(5), Episode: new(8), Type: library.Episode},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	series := itemsByType(mediaRepo, "series")
	require.Len(t, series, 1, "Some Series / Some series must be one series, not two")

	seasons := itemsByType(mediaRepo, "season")
	require.Len(t, seasons, 2, "both seasons hang off the single series")
	for _, s := range seasons {
		assert.Equal(t, string(series[0].ID), s.ParentID, "every season parented to the one series")
	}

	episodes := itemsByType(mediaRepo, "episode")
	require.Len(t, episodes, 2)
}

func TestScanTVShow_DuplicateSeriesNotCreated(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	})

	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/Show/S01/e01.mkv", Size: 100, Title: "Show", Season: intPtr(1), Episode: intPtr(1), Type: library.Episode},
		{Path: "/tv/Show/S01/e02.mkv", Size: 200, Title: "Show", Season: intPtr(1), Episode: intPtr(2), Type: library.Episode},
		{Path: "/tv/Show/S01/e03.mkv", Size: 300, Title: "Show", Season: intPtr(1), Episode: intPtr(3), Type: library.Episode},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	series := itemsByType(mediaRepo, "series")
	assert.Len(t, series, 1, "only one series despite 3 episodes")

	seasons := itemsByType(mediaRepo, "season")
	assert.Len(t, seasons, 1, "only one season despite 3 episodes")

	episodes := itemsByType(mediaRepo, "episode")
	assert.Len(t, episodes, 3)
}

func TestScanTVShow_SeasonIndexNumber(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	})

	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/Show/S03/e01.mkv", Size: 100, Title: "Show", Season: intPtr(3), Episode: intPtr(1), Type: library.Episode},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	seasons := itemsByType(mediaRepo, "season")
	require.Len(t, seasons, 1)
	require.NotNil(t, seasons[0].IndexNumber)
	assert.Equal(t, 3, *seasons[0].IndexNumber)
}

// Regression: scanTVShowFiles used to build the series/season MediaItem via a
// bare struct literal, bypassing the NewMediaItem invariants entirely. A show
// parsed with a blank title (e.g. a malformed filename) would silently
// persist as an unnamed series row instead of being rejected. Routing through
// NewMediaItem enforces ErrEmptyTitle like every other item type, and per the
// scan's per-item error policy the file is skipped rather than aborting the
// whole library scan.
func TestScanTVShow_BlankSeriesTitle_SkippedNotPersisted(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	})

	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/blank/S01/e01.mkv", Size: 100, Title: "   ", Season: new(1), Episode: new(1), Type: library.Episode},
	}

	added, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err, "an invalid item must be skipped, not fail the whole scan")
	assert.Equal(t, 0, added)
	assert.Empty(t, mediaRepo.All(), "blank-titled series/season/episode must not be persisted")
}

// Regression: the bare struct literal set Title/ScanTitle to the raw,
// untrimmed filename-derived value. NewMediaItem trims the title as part of
// its invariants; the scan path must not bypass that.
func TestScanTVShow_SeriesTitleTrimmed(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	})

	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/Show/S01/e01.mkv", Size: 100, Title: "  Show  ", Season: new(1), Episode: new(1), Type: library.Episode},
	}

	added, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)
	assert.Equal(t, 3, added)

	series := itemsByType(mediaRepo, "series")
	require.Len(t, series, 1)
	assert.Equal(t, "Show", series[0].Title)
	assert.Equal(t, "Show", series[0].ScanTitle)
}

func TestScanTVShow_EpisodeIndexNumber(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	})

	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/Show/S01/e07.mkv", Size: 100, Title: "Show", Season: intPtr(1), Episode: intPtr(7), Type: library.Episode},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	episodes := itemsByType(mediaRepo, "episode")
	require.Len(t, episodes, 1)
	require.NotNil(t, episodes[0].IndexNumber)
	assert.Equal(t, 7, *episodes[0].IndexNumber)
}

// The year on the show folder is the one hint that tells a remake from the
// original at enrichment time, so the series created from its episodes must
// carry it; a show without a year stays without one.
func TestScanTVShow_SeriesKeepsFolderYear(t *testing.T) {
	t.Parallel()
	svc, _, libQueryRepo, mediaRepo, scanner, _ := newCmdSvc()

	libQueryRepo.Seed(&library.Library{
		ID: "lib-tv", Name: "TV", Type: "tvshow", Paths: []string{"/tv"},
	})

	scanner.Results["/tv"] = []library.ScannedFile{
		{Path: "/tv/Old Show (1954)/S01/e01.mkv", Size: 500, Title: "Old Show", Year: new(1954), Season: new(1), Episode: new(1), Type: library.Episode},
		{Path: "/tv/Undated/S01/e01.mkv", Size: 600, Title: "Undated", Season: new(1), Episode: new(1), Type: library.Episode},
	}

	_, _, err := svc.ScanLibrary(context.Background(), "lib-tv")
	require.NoError(t, err)

	byTitle := map[string]*library.MediaItem{}
	for _, s := range itemsByType(mediaRepo, "series") {
		byTitle[s.Title] = s
	}
	require.Len(t, byTitle, 2)
	require.NotNil(t, byTitle["Old Show"].Catalog, "the dated series carries a catalog")
	require.NotNil(t, byTitle["Old Show"].Catalog.Year)
	assert.Equal(t, 1954, *byTitle["Old Show"].Catalog.Year)
	if c := byTitle["Undated"].Catalog; c != nil {
		assert.Nil(t, c.Year, "no year invented for an undated show")
	}
}
