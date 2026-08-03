package collection_test

import (
	"context"
	"fmt"
	"testing"

	appcol "github.com/evey-project/evey/internal/app/collection"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSvc() (*appcol.Service, *libfakes.FakeCollectionRepository, *libfakes.FakeMediaItemRepository) {
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	colRepo := libfakes.NewFakeCollectionRepository(mediaRepo)
	smartQ := libfakes.NewFakeCollectionSmartQuerier()
	svc := appcol.NewService(colRepo.CommandRepo(), colRepo.QueryRepo(), smartQ, mediaRepo)
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

func TestLinkMediaItem_ValidatesFK(t *testing.T) {
	t.Parallel()
	svc, _, mediaRepo := newSvc()
	ctx := context.Background()

	col, err := svc.Upsert(ctx, 1, "Saga", "", "", "")
	require.NoError(t, err)

	// non-existent media item → error
	err = svc.LinkMediaItem(ctx, col.ID, "ghost-id", 0)
	require.Error(t, err)

	// add the item → should succeed
	mediaRepo.Create(ctx, &library.MediaItem{ID: "m1", Title: "Film A"})
	err = svc.LinkMediaItem(ctx, col.ID, "m1", 0)
	require.NoError(t, err)
}

func TestLinkMediaItem_ValidatesCollectionExists(t *testing.T) {
	t.Parallel()
	svc, _, mediaRepo := newSvc()
	ctx := context.Background()

	mediaRepo.Create(ctx, &library.MediaItem{ID: "m1", Title: "Film A"})

	err := svc.LinkMediaItem(ctx, "ghost-col", "m1", 0)
	require.Error(t, err)
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

func TestGetByID(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	ctx := context.Background()

	col, err := svc.Upsert(ctx, 5, "Marvel Saga", "overview", "", "")
	require.NoError(t, err)

	got, err := svc.GetByID(ctx, col.ID)
	require.NoError(t, err)
	assert.Equal(t, col.ID, got.ID)
	assert.Equal(t, "Marvel Saga", got.Name)
}

func TestGetByID_NotFound(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	ctx := context.Background()

	_, err := svc.GetByID(ctx, "nonexistent-id")
	require.Error(t, err)
}

func TestCountItems(t *testing.T) {
	t.Parallel()
	svc, colRepo, mediaRepo := newSvc()
	ctx := context.Background()

	mediaRepo.Create(ctx, &library.MediaItem{ID: "m1", Title: "Film A"})
	mediaRepo.Create(ctx, &library.MediaItem{ID: "m2", Title: "Film B"})
	mediaRepo.Create(ctx, &library.MediaItem{ID: "m3", Title: "Film C"})

	col, err := svc.Upsert(ctx, 10, "Triple Saga", "", "", "")
	require.NoError(t, err)

	colRepo.LinkItem(ctx, col.ID, "m1", 0)
	colRepo.LinkItem(ctx, col.ID, "m2", 1)
	colRepo.LinkItem(ctx, col.ID, "m3", 2)

	count, err := svc.CountItems(ctx, col.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestLinkMediaItem(t *testing.T) {
	t.Parallel()
	svc, _, mediaRepo := newSvc()
	ctx := context.Background()

	mediaRepo.Create(ctx, &library.MediaItem{ID: "m1", Title: "Film A"})

	col, err := svc.Upsert(ctx, 7, "Saga", "", "", "")
	require.NoError(t, err)

	err = svc.LinkMediaItem(ctx, col.ID, "m1", 0)
	require.NoError(t, err)

	items, total, err := svc.GetItems(ctx, col.ID, 50, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, items, 1)
	assert.Equal(t, shared.MediaItemID("m1"), items[0].ID)
}

func TestGetItems_ZeroLimit_DefaultsTo50(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	ctx := context.Background()

	col, err := svc.Upsert(ctx, 8, "Saga", "", "", "")
	require.NoError(t, err)

	// limit=0 is clamped to 50 by the service
	items, total, err := svc.GetItems(ctx, col.ID, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, items)
}

func TestListCollections_Empty(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()

	cols, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, cols)
}

func TestGetItems_NegativeLimit_DefaultsTo50(t *testing.T) {
	t.Parallel()
	svc, colRepo, mediaRepo := newSvc()
	ctx := context.Background()

	// create 3 items and link them
	for i := range 3 {
		id := fmt.Sprintf("m%d", i)
		mediaRepo.Create(ctx, &library.MediaItem{ID: shared.MediaItemID(id), Title: id})
	}
	col, err := svc.Upsert(ctx, 80, "Saga", "", "", "")
	require.NoError(t, err)
	for i := range 3 {
		colRepo.LinkItem(ctx, col.ID, fmt.Sprintf("m%d", i), i)
	}

	// limit=-1 should be clamped to default 50, returning all 3 items
	items, total, err := svc.GetItems(ctx, col.ID, -1, 0)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, items, 3)
}

func TestGetItems_Limit500_NotCapped(t *testing.T) {
	t.Parallel()
	svc, colRepo, mediaRepo := newSvc()
	ctx := context.Background()

	mediaRepo.Create(ctx, &library.MediaItem{ID: "m1", Title: "A"})
	col, err := svc.Upsert(ctx, 81, "Saga", "", "", "")
	require.NoError(t, err)
	colRepo.LinkItem(ctx, col.ID, "m1", 0)

	// limit=500 is exactly the boundary - should NOT be capped further
	items, total, err := svc.GetItems(ctx, col.ID, 500, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, items, 1)
}

func TestGetItems_Limit501_CappedTo500(t *testing.T) {
	t.Parallel()
	svc, colRepo, mediaRepo := newSvc()
	ctx := context.Background()

	mediaRepo.Create(ctx, &library.MediaItem{ID: "m1", Title: "A"})
	col, err := svc.Upsert(ctx, 82, "Saga", "", "", "")
	require.NoError(t, err)
	colRepo.LinkItem(ctx, col.ID, "m1", 0)

	// limit=501 should be capped to 500
	items, total, err := svc.GetItems(ctx, col.ID, 501, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, items, 1)
}

func TestGetItems_SmartCollection_DefaultSortBy(t *testing.T) {
	t.Parallel()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	colRepo := libfakes.NewFakeCollectionRepository(mediaRepo)
	smartQ := libfakes.NewFakeCollectionSmartQuerier()
	ctx := context.Background()

	smartQ.Items = []*library.MediaItem{{ID: "m1", Title: "A", Catalog: &library.MediaCatalog{Genres: []string{"Action"}}}}
	svc := appcol.NewService(colRepo.CommandRepo(), colRepo.QueryRepo(), smartQ, mediaRepo)

	col, err := svc.Upsert(ctx, 90, "Smart", "", "", "")
	require.NoError(t, err)
	col.Rules = []library.CollectionRule{{Field: "genre", Operator: "eq", Value: "Action"}}
	col.SortBy = "" // empty → default "added_at"
	colRepo.CommandRepo().Upsert(ctx, col)

	_, _, err = svc.GetItems(ctx, col.ID, 50, 0)
	require.NoError(t, err)
	assert.Equal(t, "added_at", smartQ.LastSortBy)
}

func TestGetItems_SmartCollection_CustomSortBy(t *testing.T) {
	t.Parallel()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	colRepo := libfakes.NewFakeCollectionRepository(mediaRepo)
	smartQ := libfakes.NewFakeCollectionSmartQuerier()
	ctx := context.Background()

	smartQ.Items = []*library.MediaItem{{ID: "m1", Title: "A", Catalog: &library.MediaCatalog{Genres: []string{"Action"}}}}
	svc := appcol.NewService(colRepo.CommandRepo(), colRepo.QueryRepo(), smartQ, mediaRepo)

	col, err := svc.Upsert(ctx, 91, "Smart", "", "", "")
	require.NoError(t, err)
	col.Rules = []library.CollectionRule{{Field: "genre", Operator: "eq", Value: "Action"}}
	col.SortBy = "release_date"
	colRepo.CommandRepo().Upsert(ctx, col)

	_, _, err = svc.GetItems(ctx, col.ID, 50, 0)
	require.NoError(t, err)
	assert.Equal(t, "release_date", smartQ.LastSortBy)
}

func TestGetItems_SmartCollection_PaginatesAndCountsAll(t *testing.T) {
	t.Parallel()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	colRepo := libfakes.NewFakeCollectionRepository(mediaRepo)
	smartQ := libfakes.NewFakeCollectionSmartQuerier()
	ctx := context.Background()

	for _, id := range []string{"m1", "m2", "m3", "m4", "m5"} {
		smartQ.Items = append(smartQ.Items, &library.MediaItem{
			ID: shared.MediaItemID(id), Title: id,
			Catalog: &library.MediaCatalog{Genres: []string{"Action"}},
		})
	}
	svc := appcol.NewService(colRepo.CommandRepo(), colRepo.QueryRepo(), smartQ, mediaRepo)

	col, err := svc.Upsert(ctx, 92, "Smart", "", "", "")
	require.NoError(t, err)
	col.Rules = []library.CollectionRule{{Field: "genre", Operator: "eq", Value: "Action"}}
	colRepo.CommandRepo().Upsert(ctx, col)

	// Page (limit 2, offset 2) of 5 matches: the page holds 2 items but the total
	// must be the full match count, not the page size - the old version returned
	// len(page) (and capped at 500).
	items, total, err := svc.GetItems(ctx, col.ID, 2, 2)
	require.NoError(t, err)
	assert.Equal(t, 5, total, "total must be the full match count")
	assert.Len(t, items, 2, "page is bounded by limit")
}

func TestGetItems_SmartCollection_OffsetEqualsTotal_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	colRepo := libfakes.NewFakeCollectionRepository(mediaRepo)
	smartQ := libfakes.NewFakeCollectionSmartQuerier()
	ctx := context.Background()

	smartQ.Items = []*library.MediaItem{
		{ID: "m1", Title: "A", Catalog: &library.MediaCatalog{Genres: []string{"Action"}}},
		{ID: "m2", Title: "B", Catalog: &library.MediaCatalog{Genres: []string{"Action"}}},
	}
	svc := appcol.NewService(colRepo.CommandRepo(), colRepo.QueryRepo(), smartQ, mediaRepo)

	col, err := svc.Upsert(ctx, 92, "Smart", "", "", "")
	require.NoError(t, err)
	col.Rules = []library.CollectionRule{{Field: "genre", Operator: "eq", Value: "Action"}}
	colRepo.CommandRepo().Upsert(ctx, col)

	// offset == total(2) → empty result, total still reported
	items, total, err := svc.GetItems(ctx, col.ID, 50, 2)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Empty(t, items)
}

func TestGetItems_SmartCollection_OffsetLessThanTotal(t *testing.T) {
	t.Parallel()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	colRepo := libfakes.NewFakeCollectionRepository(mediaRepo)
	smartQ := libfakes.NewFakeCollectionSmartQuerier()
	ctx := context.Background()

	smartQ.Items = []*library.MediaItem{
		{ID: "m1", Title: "A", Catalog: &library.MediaCatalog{Genres: []string{"Action"}}},
		{ID: "m2", Title: "B", Catalog: &library.MediaCatalog{Genres: []string{"Action"}}},
		{ID: "m3", Title: "C", Catalog: &library.MediaCatalog{Genres: []string{"Action"}}},
	}
	svc := appcol.NewService(colRepo.CommandRepo(), colRepo.QueryRepo(), smartQ, mediaRepo)

	col, err := svc.Upsert(ctx, 93, "Smart", "", "", "")
	require.NoError(t, err)
	col.Rules = []library.CollectionRule{{Field: "genre", Operator: "eq", Value: "Action"}}
	colRepo.CommandRepo().Upsert(ctx, col)

	// offset=1 < total(3) → returns 2 items
	items, total, err := svc.GetItems(ctx, col.ID, 50, 1)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, items, 2)
}

func TestGetItems_SmartCollection_LimitTruncatesResults(t *testing.T) {
	t.Parallel()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	colRepo := libfakes.NewFakeCollectionRepository(mediaRepo)
	smartQ := libfakes.NewFakeCollectionSmartQuerier()
	ctx := context.Background()

	smartQ.Items = []*library.MediaItem{
		{ID: "m1", Title: "A", Catalog: &library.MediaCatalog{Genres: []string{"Action"}}},
		{ID: "m2", Title: "B", Catalog: &library.MediaCatalog{Genres: []string{"Action"}}},
		{ID: "m3", Title: "C", Catalog: &library.MediaCatalog{Genres: []string{"Action"}}},
	}
	svc := appcol.NewService(colRepo.CommandRepo(), colRepo.QueryRepo(), smartQ, mediaRepo)

	col, err := svc.Upsert(ctx, 94, "Smart", "", "", "")
	require.NoError(t, err)
	col.Rules = []library.CollectionRule{{Field: "genre", Operator: "eq", Value: "Action"}}
	colRepo.CommandRepo().Upsert(ctx, col)

	// limit=2 < len(3) → truncated to 2
	items, total, err := svc.GetItems(ctx, col.ID, 2, 0)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, items, 2)
	assert.Equal(t, shared.MediaItemID("m1"), items[0].ID)
	assert.Equal(t, shared.MediaItemID("m2"), items[1].ID)
}

func TestCreateSmart(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	ctx := context.Background()

	rules := []library.CollectionRule{{Field: "genre", Operator: "eq", Value: "Action"}}
	col, err := svc.CreateSmart(ctx, "Action Movies", rules, "year")
	require.NoError(t, err)
	assert.NotEmpty(t, col.ID)
	assert.Equal(t, "Action Movies", col.Name)
	assert.Equal(t, "year", col.SortBy)
	assert.Len(t, col.Rules, 1)
	assert.Nil(t, col.TmdbID)
}

func TestCreateSmart_EmptyName(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	rules := []library.CollectionRule{{Field: "genre", Operator: "eq", Value: "Action"}}
	_, err := svc.CreateSmart(context.Background(), "", rules, "year")
	require.Error(t, err)
}

func TestCreateSmart_NoRules(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	_, err := svc.CreateSmart(context.Background(), "Empty", nil, "year")
	require.Error(t, err)
}

func TestCreateSmart_DefaultSortBy(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	rules := []library.CollectionRule{{Field: "type", Operator: "eq", Value: "movie"}}
	col, err := svc.CreateSmart(context.Background(), "Movies", rules, "")
	require.NoError(t, err)
	assert.Equal(t, "added_at", col.SortBy)
}

func TestUpdateSmart(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	ctx := context.Background()

	rules := []library.CollectionRule{{Field: "genre", Operator: "eq", Value: "Action"}}
	col, err := svc.CreateSmart(ctx, "Action Movies", rules, "year")
	require.NoError(t, err)

	newRules := []library.CollectionRule{{Field: "genre", Operator: "eq", Value: "Drama"}}
	updated, err := svc.UpdateSmart(ctx, col.ID, "Drama Movies", newRules, "rating")
	require.NoError(t, err)
	assert.Equal(t, col.ID, updated.ID)
	assert.Equal(t, "Drama Movies", updated.Name)
	assert.Equal(t, "rating", updated.SortBy)
	assert.Len(t, updated.Rules, 1)
	assert.Equal(t, "Drama", updated.Rules[0].Value)
}

func TestUpdateSmart_NotFound(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	rules := []library.CollectionRule{{Field: "genre", Operator: "eq", Value: "Action"}}
	_, err := svc.UpdateSmart(context.Background(), "nonexistent", "Name", rules, "year")
	require.Error(t, err)
}

func TestUpdateSmart_EmptyName(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	ctx := context.Background()

	rules := []library.CollectionRule{{Field: "genre", Operator: "eq", Value: "Action"}}
	col, err := svc.CreateSmart(ctx, "Action Movies", rules, "year")
	require.NoError(t, err)

	_, err = svc.UpdateSmart(ctx, col.ID, "", rules, "year")
	require.Error(t, err)
}

func TestUpdateSmart_NoRules(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	ctx := context.Background()

	rules := []library.CollectionRule{{Field: "genre", Operator: "eq", Value: "Action"}}
	col, err := svc.CreateSmart(ctx, "Action", rules, "year")
	require.NoError(t, err)

	_, err = svc.UpdateSmart(ctx, col.ID, "Action", nil, "year")
	require.Error(t, err)
}

func TestGetItems_SmartCollection_LimitEqualLen_NoTruncation(t *testing.T) {
	t.Parallel()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	colRepo := libfakes.NewFakeCollectionRepository(mediaRepo)
	smartQ := libfakes.NewFakeCollectionSmartQuerier()
	ctx := context.Background()

	smartQ.Items = []*library.MediaItem{
		{ID: "m1", Title: "A", Catalog: &library.MediaCatalog{Genres: []string{"Action"}}},
		{ID: "m2", Title: "B", Catalog: &library.MediaCatalog{Genres: []string{"Action"}}},
	}
	svc := appcol.NewService(colRepo.CommandRepo(), colRepo.QueryRepo(), smartQ, mediaRepo)

	col, err := svc.Upsert(ctx, 95, "Smart", "", "", "")
	require.NoError(t, err)
	col.Rules = []library.CollectionRule{{Field: "genre", Operator: "eq", Value: "Action"}}
	colRepo.CommandRepo().Upsert(ctx, col)

	// limit=2 == len(2) → no truncation, all returned
	items, total, err := svc.GetItems(ctx, col.ID, 2, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, items, 2)
}

func TestCreate(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	ctx := context.Background()

	rules := []library.CollectionRule{{Field: "genre", Operator: "eq", Value: "Action"}}
	col, err := svc.Create(ctx, "Manual Collection", rules, "title")
	require.NoError(t, err)
	assert.NotEmpty(t, col.ID)
	assert.Equal(t, "Manual Collection", col.Name)
	assert.Equal(t, "title", col.SortBy)
	assert.Len(t, col.Rules, 1)
}

func TestCreate_RepoError(t *testing.T) {
	t.Parallel()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	colRepo := libfakes.NewFakeCollectionRepository(mediaRepo)
	smartQ := libfakes.NewFakeCollectionSmartQuerier()
	cmdRepo := colRepo.CommandRepo()
	cmdRepo.Err = fmt.Errorf("db down")
	svc := appcol.NewService(cmdRepo, colRepo.QueryRepo(), smartQ, mediaRepo)

	_, err := svc.Create(context.Background(), "Fail", nil, "title")
	require.Error(t, err)
}

func TestUpdate(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	ctx := context.Background()

	rules := []library.CollectionRule{{Field: "genre", Operator: "eq", Value: "Action"}}
	col, err := svc.Create(ctx, "Original", rules, "title")
	require.NoError(t, err)

	newRules := []library.CollectionRule{{Field: "genre", Operator: "eq", Value: "Drama"}}
	err = svc.Update(ctx, col.ID, "Updated", newRules, "year")
	require.NoError(t, err)

	got, err := svc.GetByID(ctx, col.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated", got.Name)
	assert.Equal(t, "year", got.SortBy)
}

func TestUpdate_NotFound(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	err := svc.Update(context.Background(), "nonexistent", "Name", nil, "title")
	require.Error(t, err)
}

func TestUpsert_RepoError(t *testing.T) {
	t.Parallel()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	colRepo := libfakes.NewFakeCollectionRepository(mediaRepo)
	smartQ := libfakes.NewFakeCollectionSmartQuerier()
	cmdRepo := colRepo.CommandRepo()
	cmdRepo.Err = fmt.Errorf("db down")
	svc := appcol.NewService(cmdRepo, colRepo.QueryRepo(), smartQ, mediaRepo)

	_, err := svc.Upsert(context.Background(), 1, "Fail", "", "", "")
	require.Error(t, err)
}

func TestCreateSmart_RepoError(t *testing.T) {
	t.Parallel()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	colRepo := libfakes.NewFakeCollectionRepository(mediaRepo)
	smartQ := libfakes.NewFakeCollectionSmartQuerier()
	cmdRepo := colRepo.CommandRepo()
	cmdRepo.Err = fmt.Errorf("db down")
	svc := appcol.NewService(cmdRepo, colRepo.QueryRepo(), smartQ, mediaRepo)

	rules := []library.CollectionRule{{Field: "genre", Operator: "eq", Value: "Action"}}
	_, err := svc.CreateSmart(context.Background(), "Fail", rules, "year")
	require.Error(t, err)
}

func TestUpdateSmart_RepoError(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	ctx := context.Background()

	rules := []library.CollectionRule{{Field: "genre", Operator: "eq", Value: "Action"}}
	col, err := svc.CreateSmart(ctx, "Action", rules, "year")
	require.NoError(t, err)

	// inject error after creation
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	colRepo := libfakes.NewFakeCollectionRepository(mediaRepo)
	smartQ := libfakes.NewFakeCollectionSmartQuerier()
	cmdRepo := colRepo.CommandRepo()
	// seed the collection in the new repo
	colRepo.CommandRepo().Create(ctx, col)
	cmdRepo.Err = fmt.Errorf("db down")
	svc2 := appcol.NewService(cmdRepo, colRepo.QueryRepo(), smartQ, mediaRepo)

	_, err = svc2.UpdateSmart(ctx, col.ID, "Updated", rules, "year")
	require.Error(t, err)
}

func TestUpdateSmart_DefaultSortBy(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	ctx := context.Background()

	rules := []library.CollectionRule{{Field: "genre", Operator: "eq", Value: "Action"}}
	col, err := svc.CreateSmart(ctx, "Action", rules, "year")
	require.NoError(t, err)

	updated, err := svc.UpdateSmart(ctx, col.ID, "Action v2", rules, "")
	require.NoError(t, err)
	assert.Equal(t, "added_at", updated.SortBy)
}

func TestGetItems_SmartCollection_QueryError(t *testing.T) {
	t.Parallel()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	colRepo := libfakes.NewFakeCollectionRepository(mediaRepo)
	smartQ := libfakes.NewFakeCollectionSmartQuerier()
	smartQ.Err = fmt.Errorf("query failed")
	ctx := context.Background()

	svc := appcol.NewService(colRepo.CommandRepo(), colRepo.QueryRepo(), smartQ, mediaRepo)

	col, err := svc.Upsert(ctx, 100, "Smart Fail", "", "", "")
	require.NoError(t, err)
	col.Rules = []library.CollectionRule{{Field: "genre", Operator: "eq", Value: "Action"}}
	colRepo.CommandRepo().Upsert(ctx, col)

	_, _, err = svc.GetItems(ctx, col.ID, 50, 0)
	require.Error(t, err)
}

func TestGetItems_NotFound(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()
	_, _, err := svc.GetItems(context.Background(), "nonexistent", 50, 0)
	require.Error(t, err)
}

// TestCollection_KillMutant_ZeroLimitClampedTo50_NonSmart ensures that limit=0
// is clamped to 50 on the non-smart path: with 51 linked items and limit=0,
// we must get exactly 50 results (mutant "limit < 0" would leave limit=0,
// causing the fake repo to return all 51).
func TestCollection_KillMutant_ZeroLimitClampedTo50_NonSmart(t *testing.T) {
	t.Parallel()
	svc, colRepo, mediaRepo := newSvc()
	ctx := context.Background()

	for i := range 51 {
		id := fmt.Sprintf("m%d", i)
		mediaRepo.Create(ctx, &library.MediaItem{ID: shared.MediaItemID(id), Title: id})
	}
	col, err := svc.Upsert(ctx, 150, "Big Saga", "", "", "")
	require.NoError(t, err)
	for i := range 51 {
		colRepo.LinkItem(ctx, col.ID, fmt.Sprintf("m%d", i), i)
	}

	items, total, err := svc.GetItems(ctx, col.ID, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 51, total)
	assert.Len(t, items, 50, "limit=0 must be clamped to 50")
}

func TestGetItems_SmartCollection_RulesEvaluated(t *testing.T) {
	t.Parallel()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	colRepo := libfakes.NewFakeCollectionRepository(mediaRepo)
	smartQ := libfakes.NewFakeCollectionSmartQuerier()

	ctx := context.Background()
	mediaRepo.Create(ctx, &library.MediaItem{ID: "m1", Title: "Action Film", Catalog: &library.MediaCatalog{Genres: []string{"Action"}}})
	mediaRepo.Create(ctx, &library.MediaItem{ID: "m2", Title: "Drama Film", Catalog: &library.MediaCatalog{Genres: []string{"Drama"}}})
	smartQ.Items = []*library.MediaItem{
		{ID: "m1", Title: "Action Film", Catalog: &library.MediaCatalog{Genres: []string{"Action"}}},
		{ID: "m2", Title: "Drama Film", Catalog: &library.MediaCatalog{Genres: []string{"Drama"}}},
	}

	svc := appcol.NewService(colRepo.CommandRepo(), colRepo.QueryRepo(), smartQ, mediaRepo)

	col, err := svc.Upsert(ctx, 99, "Smart Action", "", "", "")
	require.NoError(t, err)

	col.Rules = []library.CollectionRule{{Field: "genre", Operator: "eq", Value: "Action"}}
	colRepo.CommandRepo().Upsert(ctx, col)

	items, total, err := svc.GetItems(ctx, col.ID, 50, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, items, 1)
	assert.Equal(t, shared.MediaItemID("m1"), items[0].ID)
}

// The TMDB upsert takes its name from a third party's payload. Before the
// invariant was enforced on this path, an empty name produced a `name = ”`
// row - the column is NOT NULL, but the empty string satisfies that - and the
// library showed a collection nobody could identify, name or search for.
// Refusing costs one unlinked film and one log line.
func TestUpsert_RefusesANamelessCollection(t *testing.T) {
	t.Parallel()
	svc, _, _ := newSvc()

	col, err := svc.Upsert(context.Background(), 131292, "", "overview", "", "")

	require.Error(t, err, "a collection with no name must not be persisted")
	assert.Nil(t, col)

	cols, listErr := svc.List(context.Background())
	require.NoError(t, listErr)
	assert.Empty(t, cols, "nothing may have reached the store")
}
