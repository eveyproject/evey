package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestCollection(id string) *library.Collection {
	now := time.Now().UTC().Truncate(time.Millisecond)
	tmdbID := 12345
	return &library.Collection{
		ID:          id,
		Name:        "Collection " + id,
		Overview:    "Overview for " + id,
		TmdbID:      &tmdbID,
		PosterURL:   "https://image.tmdb.org/t/p/w500/poster.jpg",
		BackdropURL: "https://image.tmdb.org/t/p/original/backdrop.jpg",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func TestCollectionRepo_Create(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	col := newTestCollection("col1")
	col.TmdbID = nil // allow nil tmdb_id
	require.NoError(t, repo.Create(ctx, col))

	got, err := repo.GetByID(ctx, col.ID)
	require.NoError(t, err)
	assert.Equal(t, col.Name, got.Name)
	assert.Equal(t, col.Overview, got.Overview)
}

func TestCollectionRepo_Upsert(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	col := newTestCollection("col_upsert")
	require.NoError(t, repo.Upsert(ctx, col))

	// upsert same tmdb_id with updated name
	col2 := newTestCollection("col_upsert_2")
	col2.TmdbID = col.TmdbID
	col2.Name = "Updated Name"
	require.NoError(t, repo.Upsert(ctx, col2))

	list, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "Updated Name", list[0].Name)
}

func TestCollectionRepo_List(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	c1 := newTestCollection("col2a")
	c1.TmdbID = nil
	c2 := newTestCollection("col2b")
	c2.TmdbID = nil
	require.NoError(t, repo.Create(ctx, c1))
	require.NoError(t, repo.Create(ctx, c2))

	list, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestCollectionRepo_GetByID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	col := newTestCollection("col3")
	require.NoError(t, repo.Create(ctx, col))

	got, err := repo.GetByID(ctx, "col3")
	require.NoError(t, err)
	assert.Equal(t, col.Name, got.Name)
	assert.Equal(t, col.Overview, got.Overview)
	assert.NotNil(t, got.TmdbID)
	assert.Equal(t, *col.TmdbID, *got.TmdbID)

	_, err = repo.GetByID(ctx, "nonexistent")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestCollectionRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	col := newTestCollection("col5")
	require.NoError(t, repo.Create(ctx, col))
	require.NoError(t, repo.Delete(ctx, col.ID))

	_, err := repo.GetByID(ctx, col.ID)
	require.ErrorIs(t, err, shared.ErrNotFound)

	err = repo.Delete(ctx, "nonexistent")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestCollectionRepo_GetItems(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	colRepo := postgres.NewCollectionRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("collib1")
	require.NoError(t, libRepo.Create(ctx, lib))

	item1 := newTestMediaItem("colmi1", lib.ID)
	item2 := newTestMediaItem("colmi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item1))
	require.NoError(t, itemRepo.Create(ctx, item2))

	col := newTestCollection("colsaga1")
	col.TmdbID = nil
	require.NoError(t, colRepo.Create(ctx, col))

	require.NoError(t, colRepo.LinkItem(ctx, col.ID, item1.ID, 0))
	require.NoError(t, colRepo.LinkItem(ctx, col.ID, item2.ID, 1))

	items, err := colRepo.GetItems(ctx, col.ID, 100, 0)
	require.NoError(t, err)
	assert.Len(t, items, 2)

	count, err := colRepo.CountItems(ctx, col.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}
