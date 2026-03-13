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

func TestCreateCollection(t *testing.T) {
	svc, _, _ := newSvc()
	col := &library.Collection{Name: "Action Movies", Type: library.CollectionSmart}
	err := svc.Create(context.Background(), col)
	require.NoError(t, err)
	assert.NotEmpty(t, col.ID)
	assert.Equal(t, "sort_title", col.SortBy)

	got, err := svc.GetByID(context.Background(), col.ID)
	require.NoError(t, err)
	assert.Equal(t, "Action Movies", got.Name)
}

func TestListCollections(t *testing.T) {
	svc, _, _ := newSvc()
	svc.Create(context.Background(), &library.Collection{Name: "C1", Type: library.CollectionManual})
	svc.Create(context.Background(), &library.Collection{Name: "C2", Type: library.CollectionSmart})

	cols, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, cols, 2)
}

func TestUpdateCollection(t *testing.T) {
	svc, _, _ := newSvc()
	col := &library.Collection{Name: "Old", Type: library.CollectionSmart}
	svc.Create(context.Background(), col)

	err := svc.Update(context.Background(), &library.Collection{
		ID:   col.ID,
		Name: "New",
		Type: library.CollectionSmart,
		Rules: []library.CollectionRule{
			{Field: "genre", Operator: library.OpContains, Value: "Action"},
		},
	})
	require.NoError(t, err)

	got, _ := svc.GetByID(context.Background(), col.ID)
	assert.Equal(t, "New", got.Name)
	assert.Len(t, got.Rules, 1)
}

func TestDeleteCollection(t *testing.T) {
	svc, _, _ := newSvc()
	col := &library.Collection{Name: "Del", Type: library.CollectionManual}
	svc.Create(context.Background(), col)

	err := svc.Delete(context.Background(), col.ID)
	require.NoError(t, err)

	_, err = svc.GetByID(context.Background(), col.ID)
	require.Error(t, err)
}

func TestSmartCollectionItems(t *testing.T) {
	svc, _, mediaRepo := newSvc()
	ctx := context.Background()

	rating8 := 8.5
	rating5 := 5.0
	year2022 := 2022
	year2018 := 2018

	mediaRepo.Create(ctx, &library.MediaItem{
		ID: "m1", Type: "movie", Title: "Action Film",
		Genres: []string{"Action", "Drama"}, Year: &year2022, CommunityRating: &rating8,
	})
	mediaRepo.Create(ctx, &library.MediaItem{
		ID: "m2", Type: "movie", Title: "Comedy Film",
		Genres: []string{"Comedy"}, Year: &year2018, CommunityRating: &rating5,
	})
	mediaRepo.Create(ctx, &library.MediaItem{
		ID: "m3", Type: "movie", Title: "Action Old",
		Genres: []string{"Action"}, Year: &year2018, CommunityRating: &rating8,
	})

	col := &library.Collection{
		Name: "High Rated Action",
		Type: library.CollectionSmart,
		Rules: []library.CollectionRule{
			{Field: "genre", Operator: library.OpContains, Value: "Action"},
			{Field: "rating", Operator: library.OpGt, Value: "7.0"},
		},
	}
	svc.Create(ctx, col)

	items, total, err := svc.GetItems(ctx, col.ID, 50, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, items, 2)
}

func TestSmartCollectionYearFilter(t *testing.T) {
	svc, _, mediaRepo := newSvc()
	ctx := context.Background()

	y2023 := 2023
	y2019 := 2019
	mediaRepo.Create(ctx, &library.MediaItem{ID: "m1", Type: "movie", Year: &y2023})
	mediaRepo.Create(ctx, &library.MediaItem{ID: "m2", Type: "movie", Year: &y2019})

	col := &library.Collection{
		Name: "Recent",
		Type: library.CollectionSmart,
		Rules: []library.CollectionRule{
			{Field: "year", Operator: library.OpGt, Value: "2020"},
		},
	}
	svc.Create(ctx, col)

	items, total, err := svc.GetItems(ctx, col.ID, 50, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, "m1", items[0].ID)
}

func TestManualCollectionItems(t *testing.T) {
	svc, colRepo, mediaRepo := newSvc()
	ctx := context.Background()

	mediaRepo.Create(ctx, &library.MediaItem{ID: "m1", Title: "Film A"})
	mediaRepo.Create(ctx, &library.MediaItem{ID: "m2", Title: "Film B"})

	col := &library.Collection{Name: "My List", Type: library.CollectionManual}
	svc.Create(ctx, col)

	colRepo.AddItem(ctx, col.ID, "m1")
	colRepo.AddItem(ctx, col.ID, "m2")

	items, total, err := svc.GetItems(ctx, col.ID, 50, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, items, 2)
}

func TestUpdateCollection_NotFound(t *testing.T) {
	svc, _, _ := newSvc()
	err := svc.Update(context.Background(), &library.Collection{ID: "nonexistent", Name: "X"})
	require.Error(t, err)
}

func TestGetItems_LimitCap(t *testing.T) {
	svc, _, _ := newSvc()
	col := &library.Collection{Name: "Empty", Type: library.CollectionSmart}
	svc.Create(context.Background(), col)

	items, total, err := svc.GetItems(context.Background(), col.ID, 1000, 0)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, items)
}

func TestSmartCollectionTypeFilter(t *testing.T) {
	svc, _, mediaRepo := newSvc()
	ctx := context.Background()

	mediaRepo.Create(ctx, &library.MediaItem{ID: "m1", Type: "movie"})
	mediaRepo.Create(ctx, &library.MediaItem{ID: "m2", Type: "series"})

	col := &library.Collection{
		Name: "Movies Only",
		Type: library.CollectionSmart,
		Rules: []library.CollectionRule{
			{Field: "type", Operator: library.OpEq, Value: "movie"},
		},
	}
	svc.Create(ctx, col)

	items, total, err := svc.GetItems(ctx, col.ID, 50, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, "m1", items[0].ID)
}
