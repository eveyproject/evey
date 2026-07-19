package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/library"
)

// The media-request search must flag titles the server already holds, matched
// on provider id ONLY: a title match would flag remakes and same-name releases,
// silently blocking a legitimate request.
func TestLibraryLookupRepo_FindByProviderIDs(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	lookup := postgres.NewLibraryLookupRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("lookuplib")
	require.NoError(t, libRepo.Create(ctx, lib))

	movie := newTestMediaItem("lookupmovie", lib.ID)
	movie.Type = library.MovieItem
	movie.Catalog = &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "438631", "imdb": "tt1160419"}}
	require.NoError(t, itemRepo.Create(ctx, movie))

	got, err := lookup.FindByProviderIDs(ctx, "movie", []string{"tmdb:438631", "tmdb:999999"})
	require.NoError(t, err)
	assert.Equal(t, string(movie.ID), got["tmdb:438631"], "the held title resolves to its media item")
	assert.NotContains(t, got, "tmdb:999999", "an unheld id must not be flagged")

	// A second id on the SAME row must not credit a candidate nobody asked about.
	got, err = lookup.FindByProviderIDs(ctx, "movie", []string{"tmdb:438631"})
	require.NoError(t, err)
	assert.Len(t, got, 1)

	// Type-scoped: the same id under the series vocabulary is a different thing.
	got, err = lookup.FindByProviderIDs(ctx, "tv", []string{"tmdb:438631"})
	require.NoError(t, err)
	assert.Empty(t, got, "a movie must not answer a series lookup")

	// Music carries no provider id anywhere - never guess.
	got, err = lookup.FindByProviderIDs(ctx, "music", []string{"musicbrainz:abc"})
	require.NoError(t, err)
	assert.Empty(t, got)
}
