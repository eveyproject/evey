package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
)

func TestSQLSearchFallback_IndexMediaItem_Noop(t *testing.T) {
	fb := postgres.NewSQLSearchFallback(nil)
	err := fb.IndexMediaItem(context.Background(), &library.MediaItem{})
	assert.NoError(t, err)
}

func TestSQLSearchFallback_RemoveMediaItem_Noop(t *testing.T) {
	fb := postgres.NewSQLSearchFallback(nil)
	err := fb.RemoveMediaItem(context.Background(), "any-id")
	assert.NoError(t, err)
}

func TestSQLSearchFallback_Search_EmptyQuery(t *testing.T) {
	fb := postgres.NewSQLSearchFallback(nil)

	hits, total, err := fb.Search(context.Background(), "", nil, nil, 20)
	require.NoError(t, err)
	assert.Nil(t, hits)
	assert.Equal(t, int64(0), total)
}

func TestSQLSearchFallback_Search_WhitespaceQuery(t *testing.T) {
	fb := postgres.NewSQLSearchFallback(nil)

	hits, total, err := fb.Search(context.Background(), "   ", nil, nil, 20)
	require.NoError(t, err)
	assert.Nil(t, hits)
	assert.Equal(t, int64(0), total)
}

func TestSQLSearchFallback_ImplementsSearchIndex(t *testing.T) {
	var _ library.SearchIndex = (*postgres.SQLSearchFallback)(nil)
}

func TestSQLSearchFallback_Search_Integration(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)

	lib := newTestLibrary("search-lib-1")
	require.NoError(t, libRepo.Create(ctx, lib))

	now := time.Now().UTC().Truncate(time.Millisecond)
	year := 1999
	items := []*library.MediaItem{
		{ID: "s1", LibraryID: lib.ID, Type: "movie", Title: "Punch Club", OriginalTitle: "Punch Club", Catalog: &library.MediaCatalog{Year: &year, Genres: []string{"Drama"}, ProviderIDs: map[string]string{}, Studios: []string{}, Tags: []string{}}, AddedAt: now, UpdatedAt: now, Classification: &library.MediaClassification{OfficialRating: "R"}},
		{ID: "s2", LibraryID: lib.ID, Type: "movie", Title: "The Cascade", OriginalTitle: "The Cascade", Catalog: &library.MediaCatalog{Year: &year, Genres: []string{"Sci-Fi"}, ProviderIDs: map[string]string{}, Studios: []string{}, Tags: []string{}}, AddedAt: now, UpdatedAt: now},
		{ID: "s3", LibraryID: lib.ID, Type: "movie", Title: "Meridian Voyage", OriginalTitle: "Meridian Voyage", Catalog: &library.MediaCatalog{Genres: []string{"Sci-Fi"}, ProviderIDs: map[string]string{}, Studios: []string{}, Tags: []string{}}, AddedAt: now, UpdatedAt: now},
	}
	for _, item := range items {
		require.NoError(t, itemRepo.Create(ctx, item))
	}

	fb := postgres.NewSQLSearchFallback(db)

	t.Run("match title", func(t *testing.T) {
		hits, total, err := fb.Search(ctx, "punch", nil, nil, 20)
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, hits, 1)
		assert.Equal(t, "s1", hits[0].ID)
		assert.Equal(t, "Punch Club", hits[0].Title)
		assert.Equal(t, &year, hits[0].Year)
		assert.Equal(t, "movie", hits[0].Type)
		assert.Equal(t, []string{"Drama"}, hits[0].Genres)
		assert.Equal(t, "R", hits[0].OfficialRating)
	})

	t.Run("case insensitive", func(t *testing.T) {
		hits, _, err := fb.Search(ctx, "CASCADE", nil, nil, 20)
		require.NoError(t, err)
		require.Len(t, hits, 1)
		assert.Equal(t, "s2", hits[0].ID)
	})

	t.Run("filter by type", func(t *testing.T) {
		hits, total, err := fb.Search(ctx, "meri", map[string]string{"type": "movie"}, nil, 20)
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, hits, 1)
		assert.Equal(t, "s3", hits[0].ID)
	})

	t.Run("filter by library_id", func(t *testing.T) {
		hits, _, err := fb.Search(ctx, "punch", map[string]string{"library_id": "nonexistent"}, nil, 20)
		require.NoError(t, err)
		assert.Empty(t, hits)
	})

	t.Run("limit", func(t *testing.T) {
		hits, total, err := fb.Search(ctx, "the", nil, nil, 1)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(hits), 1)
		_ = total
	})

	t.Run("no match", func(t *testing.T) {
		hits, total, err := fb.Search(ctx, "zzzznotfound", nil, nil, 20)
		require.NoError(t, err)
		assert.Empty(t, hits)
		assert.Equal(t, int64(0), total)
	})
}

// The SQL fallback used to bail out on an empty query REGARDLESS of filters.
// Meilisearch handles "no text, just a genre filter" fine, so nobody noticed
// until the TV gained a genre-first browse screen: that screen is the first
// caller with filters and no text, and it came back empty whenever Meili was
// down. Filters alone are a legitimate search.
func TestSearchWhere_EmptyQueryWithFilters_StillFilters(t *testing.T) {
	where, args, ok := postgres.SearchWhereForTest("", map[string]string{"genres": "Comedy"})
	require.True(t, ok, "a genre filter alone must still produce a query")
	assert.NotContains(t, where, "ILIKE", "no text clause when there is no text")
	assert.Contains(t, where, "mc.genres @>")
	assert.Equal(t, []any{`["Comedy"]`}, args)
}

func TestSearchWhere_EmptyQueryNoFilters_NoQueryAtAll(t *testing.T) {
	_, _, ok := postgres.SearchWhereForTest("   ", nil)
	assert.False(t, ok, "no text and no filter is not a search, and must not dump the catalog")
}

// Placeholders must stay dense and 1-based whatever the combination, or the
// args line up against the wrong $n.
func TestSearchWhere_PlaceholdersAreDenseWithoutText(t *testing.T) {
	where, args, ok := postgres.SearchWhereForTest("", map[string]string{
		"type": "movie", "library_id": "lib-1", "year_min": "2000",
	})
	require.True(t, ok)
	assert.Contains(t, where, "$1")
	assert.Contains(t, where, "$2")
	assert.Contains(t, where, "$3")
	assert.NotContains(t, where, "$4")
	assert.Equal(t, []any{"movie", "lib-1", 2000}, args)
}

func TestSearchWhere_TextAndFiltersKeepTextFirst(t *testing.T) {
	where, args, ok := postgres.SearchWhereForTest("matrix", map[string]string{"type": "movie"})
	require.True(t, ok)
	assert.Contains(t, where, "ILIKE $1")
	assert.Contains(t, where, "mi.type = $3", "filters start after the two text patterns")
	assert.Len(t, args, 3)
	assert.Equal(t, "movie", args[2])
}

// The default order of a search must be the default order of every other
// listing. "title" alone is a valid identifier here (it resolves to the output
// alias), so the divergence was silent: results simply came back in a different
// order than the same items browsed from the library.
func TestSortClause_UnknownAndEmptySortsFallBackToTheSharedColumn(t *testing.T) {
	shared := postgres.SortColumnForTest()
	for _, sort := range [][]string{nil, {}, {"nonexistent_field:desc"}} {
		assert.Equal(t, shared, postgres.SortClauseForTest(sort),
			"sort %v must fall back to the same column as every other listing path", sort)
	}
}
