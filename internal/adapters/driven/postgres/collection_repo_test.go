package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestCollection(id string, colType library.CollectionType) *library.Collection {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return &library.Collection{
		ID:        id,
		Name:      "Collection " + id,
		Overview:  "Overview for " + id,
		Type:      colType,
		Rules:     []library.CollectionRule{},
		SortBy:    "sort_title",
		SortOrder: "asc",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestCollectionRepo_Create(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	col := newTestCollection("col1", library.CollectionManual)
	require.NoError(t, repo.Create(ctx, col))

	got, err := repo.GetByID(ctx, col.ID)
	require.NoError(t, err)
	assert.Equal(t, col.Name, got.Name)
	assert.Equal(t, col.Overview, got.Overview)
	assert.Equal(t, library.CollectionManual, got.Type)
	assert.Equal(t, "sort_title", got.SortBy)
	assert.Equal(t, "asc", got.SortOrder)
}

func TestCollectionRepo_List(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, newTestCollection("col2a", library.CollectionManual)))
	require.NoError(t, repo.Create(ctx, newTestCollection("col2b", library.CollectionSmart)))

	list, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestCollectionRepo_GetByID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	col := newTestCollection("col3", library.CollectionSmart)
	col.Rules = []library.CollectionRule{
		{Field: "genre", Operator: library.OpContains, Value: "Action"},
		{Field: "year", Operator: library.OpGt, Value: "2000"},
	}
	require.NoError(t, repo.Create(ctx, col))

	got, err := repo.GetByID(ctx, "col3")
	require.NoError(t, err)
	assert.Equal(t, col.Name, got.Name)
	assert.Equal(t, col.Overview, got.Overview)
	assert.Equal(t, library.CollectionSmart, got.Type)
	assert.Len(t, got.Rules, 2)
	assert.Equal(t, "genre", got.Rules[0].Field)
	assert.Equal(t, library.OpContains, got.Rules[0].Operator)

	_, err = repo.GetByID(ctx, "nonexistent")
	assert.ErrorIs(t, err, library.ErrNotFound)
}

func TestCollectionRepo_Update(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	col := newTestCollection("col4", library.CollectionManual)
	require.NoError(t, repo.Create(ctx, col))

	col.Name = "Updated Collection"
	col.Overview = "Updated Overview"
	col.SortBy = "year"
	col.SortOrder = "desc"
	col.UpdatedAt = time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, repo.Update(ctx, col))

	got, err := repo.GetByID(ctx, col.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Collection", got.Name)
	assert.Equal(t, "Updated Overview", got.Overview)
	assert.Equal(t, "year", got.SortBy)
	assert.Equal(t, "desc", got.SortOrder)
}

func TestCollectionRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	col := newTestCollection("col5", library.CollectionManual)
	require.NoError(t, repo.Create(ctx, col))
	require.NoError(t, repo.Delete(ctx, col.ID))

	_, err := repo.GetByID(ctx, col.ID)
	assert.ErrorIs(t, err, library.ErrNotFound)

	err = repo.Delete(ctx, "nonexistent")
	assert.ErrorIs(t, err, library.ErrNotFound)
}

func TestCollectionRepo_GetItems_Smart(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	colRepo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("collib1")
	require.NoError(t, libRepo.Create(ctx, lib))

	year2020 := 2020
	year2015 := 2015
	item1 := newTestMediaItem("colmi1", lib.ID)
	item1.Genres = []string{"Action", "Sci-Fi"}
	item1.Year = &year2020
	require.NoError(t, itemRepo.Create(ctx, item1))

	item2 := newTestMediaItem("colmi2", lib.ID)
	item2.Genres = []string{"Drama"}
	item2.Year = &year2015
	require.NoError(t, itemRepo.Create(ctx, item2))

	item3 := newTestMediaItem("colmi3", lib.ID)
	item3.Genres = []string{"Action", "Comedy"}
	item3.Year = &year2020
	require.NoError(t, itemRepo.Create(ctx, item3))

	col := newTestCollection("colsmart1", library.CollectionSmart)
	col.Rules = []library.CollectionRule{
		{Field: "genre", Operator: library.OpContains, Value: "Action"},
	}
	require.NoError(t, colRepo.Create(ctx, col))

	items, err := colRepo.GetSmartItems(ctx, col, 100, 0)
	require.NoError(t, err)
	assert.Len(t, items, 2)

	count, err := colRepo.CountSmartItems(ctx, col)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	col2 := newTestCollection("colsmart2", library.CollectionSmart)
	col2.Rules = []library.CollectionRule{
		{Field: "year", Operator: library.OpGt, Value: "2018"},
	}
	require.NoError(t, colRepo.Create(ctx, col2))

	items2, err := colRepo.GetSmartItems(ctx, col2, 100, 0)
	require.NoError(t, err)
	assert.Len(t, items2, 2)
	for _, it := range items2 {
		assert.NotNil(t, it.Year)
		assert.Greater(t, *it.Year, 2018)
	}
}
