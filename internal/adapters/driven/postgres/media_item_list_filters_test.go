package postgres_test

import (
	"context"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every dimension of the library filter panel against the real SQL: year
// (exact and range), rating floor, genre in any stored spelling, "missing
// provider ids" both ways, several libraries at once, and the id keyset.
// Count must agree with List for the same filter, or the grid's page count
// lies.
func TestMediaItemRepo_List_EveryFilterDimension(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)

	libA := newTestLibrary("flt-a")
	libB := newTestLibrary("flt-b")
	require.NoError(t, libRepo.Create(ctx, libA))
	require.NoError(t, libRepo.Create(ctx, libB))

	mk := func(id string, lib shared.LibraryID, year int, rating float64, genres []string, providerIDs map[string]string) {
		it := newTestMediaItem(id, lib)
		it.Catalog.Year = &year
		it.Catalog.Genres = genres
		it.Catalog.ProviderIDs = providerIDs
		if rating > 0 {
			it.Classification = &library.MediaClassification{CommunityRating: &rating}
		}
		require.NoError(t, itemRepo.Create(ctx, it))
	}
	mk("flt-1", libA.ID, 1999, 8.5, []string{"Drama"}, map[string]string{"tmdb": "1"})
	mk("flt-2", libA.ID, 2005, 6.0, []string{"Drame", "Action"}, map[string]string{})
	mk("flt-3", libB.ID, 2010, 7.2, []string{"Sci-Fi"}, map[string]string{"tmdb": "3"})
	mk("flt-4", libB.ID, 2020, 0, []string{"science-fiction"}, nil)

	ids := func(f library.ListMediaItemsFilter) []string {
		t.Helper()
		f.Limit = 50
		items, err := itemRepo.List(ctx, f)
		require.NoError(t, err)
		n, err := itemRepo.Count(ctx, f)
		require.NoError(t, err)
		assert.Equal(t, len(items), n, "Count must agree with List for %+v", f)
		out := make([]string, 0, len(items))
		for _, it := range items {
			out = append(out, string(it.ID))
		}
		return out
	}
	both := []shared.LibraryID{libA.ID, libB.ID}
	year := 2005

	assert.ElementsMatch(t, []string{"flt-2"}, ids(library.ListMediaItemsFilter{LibraryIDs: both, Year: &year}))
	assert.ElementsMatch(t, []string{"flt-2", "flt-3"}, ids(library.ListMediaItemsFilter{LibraryIDs: both, YearFrom: 2000, YearTo: 2019}))
	assert.ElementsMatch(t, []string{"flt-1", "flt-3"}, ids(library.ListMediaItemsFilter{LibraryIDs: both, MinRating: 7}), "an unrated title never passes a rating floor")
	assert.ElementsMatch(t, []string{"flt-1", "flt-2"}, ids(library.ListMediaItemsFilter{LibraryIDs: both, Genres: []string{"drama"}}), "Drama and Drame are one genre, case-insensitively")
	assert.ElementsMatch(t, []string{"flt-3", "flt-4"}, ids(library.ListMediaItemsFilter{LibraryIDs: both, Genres: []string{"Science-Fiction"}}), "IMDb's Sci-Fi and TMDB's Science-Fiction are one genre")
	assert.ElementsMatch(t, []string{"flt-2", "flt-3", "flt-4"}, ids(library.ListMediaItemsFilter{LibraryIDs: both, Genres: []string{"Action", "Sci-Fi"}}), "several genres OR together, each expanded to its spellings")

	missing, present := true, false
	assert.ElementsMatch(t, []string{"flt-2", "flt-4"}, ids(library.ListMediaItemsFilter{LibraryIDs: both, MissingProviderIDs: &missing}))
	assert.ElementsMatch(t, []string{"flt-1", "flt-3"}, ids(library.ListMediaItemsFilter{LibraryIDs: both, MissingProviderIDs: &present}))

	libID := libB.ID
	assert.ElementsMatch(t, []string{"flt-3", "flt-4"}, ids(library.ListMediaItemsFilter{LibraryID: &libID}))
	assert.ElementsMatch(t, []string{"flt-4"}, ids(library.ListMediaItemsFilter{LibraryIDs: both, AfterID: "flt-3"}), "the id keyset walks past the last seen id")
	assert.ElementsMatch(t, []string{"flt-3"}, ids(library.ListMediaItemsFilter{LibraryIDs: both, YearFrom: 2010, MinRating: 7, Genres: []string{"sci-fi"}}), "dimensions AND together")
}
