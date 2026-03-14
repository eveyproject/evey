package collection_test

import (
	"context"
	"testing"

	appcol "github.com/gravity-zero/evey/internal/app/collection"
	"github.com/gravity-zero/evey/internal/domain/library"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSvc() (*appcol.Service, *libfakes.FakeCollectionRepository, *libfakes.FakeMediaItemRepository) {
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	colRepo := libfakes.NewFakeCollectionRepository(mediaRepo)
	svc := appcol.NewService(colRepo.CommandRepo(), colRepo.QueryRepo())
	return svc, colRepo, mediaRepo
}

func TestUpsert(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	col, err := svc.Upsert(context.Background(), 131292, "The Dark Knight Collection", "Christopher Nolan's Batman saga", "", "")
	require.NoError(t, err)
	assert.NotEmpty(t, col.ID)
	assert.Equal(t, "The Dark Knight Collection", col.Name)
	assert.Equal(t, 131292, *col.TmdbID)
}

func TestUpsert_UpdatesExisting(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	ctx := context.Background()

	_, err := svc.Upsert(ctx, 131292, "Batman Collection", "old", "old_poster", "")
	require.NoError(t, err)

	_, err = svc.Upsert(ctx, 131292, "The Dark Knight Collection", "updated", "new_poster", "")
	require.NoError(t, err)

	cols, err := svc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, cols, 1, "upsert should not create a duplicate")
	assert.Equal(t, "The Dark Knight Collection", cols[0].Name)
}

func TestListCollections(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	ctx := context.Background()
	svc.Upsert(ctx, 1, "Saga 1", "", "", "")
	svc.Upsert(ctx, 2, "Saga 2", "", "", "")

	cols, err := svc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, cols, 2)
}

func TestDeleteCollection(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	ctx := context.Background()
	col, err := svc.Upsert(ctx, 1, "To Delete", "", "", "")
	require.NoError(t, err)

	err = svc.Delete(ctx, col.ID)
	require.NoError(t, err)

	_, err = svc.GetByID(ctx, col.ID)
	require.Error(t, err)
}

func TestGetItems(t *testing.T) {
	t.Parallel()
	svc, colRepo, mediaRepo := newSvc()
	ctx := context.Background()

	mediaRepo.Create(ctx, &library.MediaItem{ID: "m1", Title: "Film A"})
	mediaRepo.Create(ctx, &library.MediaItem{ID: "m2", Title: "Film B"})

	col, err := svc.Upsert(ctx, 42, "Saga", "", "", "")
	require.NoError(t, err)

	colRepo.LinkItem(ctx, col.ID, "m1", 0)
	colRepo.LinkItem(ctx, col.ID, "m2", 1)

	items, total, err := svc.GetItems(ctx, col.ID, 50, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, items, 2)
}

func TestGetItems_LimitCap(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	ctx := context.Background()
	col, err := svc.Upsert(ctx, 99, "Empty Saga", "", "", "")
	require.NoError(t, err)

	items, total, err := svc.GetItems(ctx, col.ID, 1000, 0)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, items)
}
