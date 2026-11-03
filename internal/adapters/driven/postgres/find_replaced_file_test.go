package postgres_test

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The row a file under a new name stands in for: a movie by folder, parsed
// title and year, an episode by season and number; never the row at f.Path,
// never a different folder, year or number.
func TestMediaItemRepo_FindReplacedFile(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	movies := newTestLibrary("rfmovies")
	require.NoError(t, libRepo.Create(ctx, movies))
	year := 2020
	film := &library.MediaItem{
		ID: "rf-film", LibraryID: movies.ID, Type: library.MovieItem, Title: "Film", ScanTitle: "Film", SortTitle: "film",
		Catalog: &library.MediaCatalog{Year: &year, ProviderIDs: map[string]string{}, Genres: []string{}, Studios: []string{}, Tags: []string{}},
		File:    &library.MediaFile{Path: "/m/Film (2020)/Film.2020.720p.mkv", Size: 500, Fingerprint: "500-1"},
	}
	require.NoError(t, itemRepo.Create(ctx, film))

	upgrade := library.ScannedFile{Path: "/m/Film (2020)/Film.2020.1080p.mkv", Size: 1500, Fingerprint: "1500-2", Title: "film", Year: &year, Type: library.MovieItem}
	got, err := itemRepo.FindReplacedFile(ctx, movies.ID, "", upgrade)
	require.NoError(t, err)
	assert.Equal(t, shared.MediaItemID("rf-film"), got.ID, "same folder, same title (case aside), same year")

	elsewhere := upgrade
	elsewhere.Path = "/m/Other/Film.2020.1080p.mkv"
	_, err = itemRepo.FindReplacedFile(ctx, movies.ID, "", elsewhere)
	assert.ErrorIs(t, err, shared.ErrNotFound, "another folder is another film")

	otherYear := 2021
	remake := upgrade
	remake.Year = &otherYear
	_, err = itemRepo.FindReplacedFile(ctx, movies.ID, "", remake)
	assert.ErrorIs(t, err, shared.ErrNotFound, "another year is another film")

	same := upgrade
	same.Path = film.File.Path
	_, err = itemRepo.FindReplacedFile(ctx, movies.ID, "", same)
	assert.ErrorIs(t, err, shared.ErrNotFound, "the row at the file's own path is the path lookup's, not a replacement")

	shows := newTestLibrary("rfshows")
	shows.Type = library.TVShow
	require.NoError(t, libRepo.Create(ctx, shows))
	require.NoError(t, itemRepo.Create(ctx, &library.MediaItem{ID: "rf-series", LibraryID: shows.ID, Type: library.Series, Title: "Show", SortTitle: "show"}))
	require.NoError(t, itemRepo.Create(ctx, &library.MediaItem{ID: "rf-season", LibraryID: shows.ID, Type: library.Season, ParentID: "rf-series", Title: "Season 1", SortTitle: "season 1"}))
	one := 1
	require.NoError(t, itemRepo.Create(ctx, &library.MediaItem{
		ID: "rf-ep", LibraryID: shows.ID, Type: library.Episode, ParentID: "rf-season", IndexNumber: &one, Title: "S01E01", SortTitle: "s01e01",
		File: &library.MediaFile{Path: "/tv/Show/S01/Show.S01E01.720p.mkv", Size: 500, Fingerprint: "500-3"},
	}))
	epUpgrade := library.ScannedFile{Path: "/tv/Show/S01/Show.S01E01.1080p.mkv", Size: 1500, Fingerprint: "1500-4", Title: "Show", Season: &one, Episode: &one, Type: library.Episode}
	got, err = itemRepo.FindReplacedFile(ctx, shows.ID, "rf-season", epUpgrade)
	require.NoError(t, err)
	assert.Equal(t, shared.MediaItemID("rf-ep"), got.ID, "same season, same number")

	two := 2
	next := epUpgrade
	next.Episode = &two
	_, err = itemRepo.FindReplacedFile(ctx, shows.ID, "rf-season", next)
	assert.ErrorIs(t, err, shared.ErrNotFound, "another number is another episode")
	_, err = itemRepo.FindReplacedFile(ctx, shows.ID, "rf-series", epUpgrade)
	assert.ErrorIs(t, err, shared.ErrNotFound, "another parent is another season")
}
