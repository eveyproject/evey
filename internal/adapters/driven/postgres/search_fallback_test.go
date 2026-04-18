package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

func TestSQLSearchFallback_IndexMediaItem_Noop(t *testing.T) {
	fb := postgres.NewSQLSearchFallback(nil)
	err := fb.IndexMediaItem(context.Background(), &library.MediaItem{}, nil)
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
		{ID: "s1", LibraryID: shared.LibraryID(lib.ID), Type: "movie", Title: "Fight Club", OriginalTitle: "Fight Club", Catalog: &library.MediaCatalog{Year: &year, Genres: []string{"Drama"}, ProviderIDs: map[string]string{}, Studios: []string{}, Tags: []string{}}, AddedAt: now, UpdatedAt: now, Classification: &library.MediaClassification{OfficialRating: "R"}},
		{ID: "s2", LibraryID: shared.LibraryID(lib.ID), Type: "movie", Title: "The Matrix", OriginalTitle: "The Matrix", Catalog: &library.MediaCatalog{Year: &year, Genres: []string{"Sci-Fi"}, ProviderIDs: map[string]string{}, Studios: []string{}, Tags: []string{}}, AddedAt: now, UpdatedAt: now},
		{ID: "s3", LibraryID: shared.LibraryID(lib.ID), Type: "movie", Title: "Interstellar", OriginalTitle: "Interstellar", Catalog: &library.MediaCatalog{Genres: []string{"Sci-Fi"}, ProviderIDs: map[string]string{}, Studios: []string{}, Tags: []string{}}, AddedAt: now, UpdatedAt: now},
	}
	for _, item := range items {
		require.NoError(t, itemRepo.Create(ctx, item))
	}

	fb := postgres.NewSQLSearchFallback(db)

	t.Run("match title", func(t *testing.T) {
		hits, total, err := fb.Search(ctx, "fight", nil, nil, 20)
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, hits, 1)
		assert.Equal(t, "s1", hits[0].ID)
		assert.Equal(t, "Fight Club", hits[0].Title)
		assert.Equal(t, &year, hits[0].Year)
		assert.Equal(t, "movie", hits[0].Type)
		assert.Equal(t, []string{"Drama"}, hits[0].Genres)
		assert.Equal(t, "R", hits[0].OfficialRating)
	})

	t.Run("case insensitive", func(t *testing.T) {
		hits, _, err := fb.Search(ctx, "MATRIX", nil, nil, 20)
		require.NoError(t, err)
		require.Len(t, hits, 1)
		assert.Equal(t, "s2", hits[0].ID)
	})

	t.Run("filter by type", func(t *testing.T) {
		hits, total, err := fb.Search(ctx, "inter", map[string]string{"type": "movie"}, nil, 20)
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, hits, 1)
		assert.Equal(t, "s3", hits[0].ID)
	})

	t.Run("filter by library_id", func(t *testing.T) {
		hits, _, err := fb.Search(ctx, "fight", map[string]string{"library_id": "nonexistent"}, nil, 20)
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
