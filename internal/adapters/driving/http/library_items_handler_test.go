package httpapi

import (
	"context"
	"testing"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
)

func ptr[T any](v T) *T { return &v }

func TestLibraryItemsFilter_YearFrom(t *testing.T) {
	repo := libfakes.NewFakeMediaItemQueryRepo()
	repo.Seed(&domainlibrary.MediaItem{ID: "old", LibraryID: "lib-1", Type: "movie", Title: "Old Film", Catalog: &domainlibrary.MediaCatalog{Year: ptr(1990)}})
	repo.Seed(&domainlibrary.MediaItem{ID: "new", LibraryID: "lib-1", Type: "movie", Title: "New Film", Catalog: &domainlibrary.MediaCatalog{Year: ptr(2020)}})

	filter := domainlibrary.ListMediaItemsFilter{
		LibraryID: ptr(shared.LibraryID("lib-1")),
		YearFrom:  2000,
	}
	items, err := repo.List(context.Background(), filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "new" {
		t.Errorf("expected only 'new', got %v", items)
	}
}

func TestLibraryItemsFilter_YearTo(t *testing.T) {
	repo := libfakes.NewFakeMediaItemQueryRepo()
	repo.Seed(&domainlibrary.MediaItem{ID: "old", LibraryID: "lib-1", Type: "movie", Title: "Old Film", Catalog: &domainlibrary.MediaCatalog{Year: ptr(1990)}})
	repo.Seed(&domainlibrary.MediaItem{ID: "new", LibraryID: "lib-1", Type: "movie", Title: "New Film", Catalog: &domainlibrary.MediaCatalog{Year: ptr(2020)}})

	filter := domainlibrary.ListMediaItemsFilter{
		LibraryID: ptr(shared.LibraryID("lib-1")),
		YearTo:    1999,
	}
	items, err := repo.List(context.Background(), filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "old" {
		t.Errorf("expected only 'old', got %v", items)
	}
}

func TestLibraryItemsFilter_MinRating(t *testing.T) {
	repo := libfakes.NewFakeMediaItemQueryRepo()
	repo.Seed(&domainlibrary.MediaItem{ID: "low", LibraryID: "lib-1", Type: "movie", Title: "Low Rated", Classification: &domainlibrary.MediaClassification{CommunityRating: ptr(5.0)}})
	repo.Seed(&domainlibrary.MediaItem{ID: "high", LibraryID: "lib-1", Type: "movie", Title: "High Rated", Classification: &domainlibrary.MediaClassification{CommunityRating: ptr(8.5)}})

	filter := domainlibrary.ListMediaItemsFilter{
		LibraryID: ptr(shared.LibraryID("lib-1")),
		MinRating: 7.0,
	}
	items, err := repo.List(context.Background(), filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "high" {
		t.Errorf("expected only 'high', got %v", items)
	}
}

func TestLibraryItemsFilter_YearRange(t *testing.T) {
	repo := libfakes.NewFakeMediaItemQueryRepo()
	repo.Seed(&domainlibrary.MediaItem{ID: "early", LibraryID: "lib-1", Type: "movie", Title: "Early", Catalog: &domainlibrary.MediaCatalog{Year: ptr(1980)}})
	repo.Seed(&domainlibrary.MediaItem{ID: "mid", LibraryID: "lib-1", Type: "movie", Title: "Mid", Catalog: &domainlibrary.MediaCatalog{Year: ptr(2005)}})
	repo.Seed(&domainlibrary.MediaItem{ID: "late", LibraryID: "lib-1", Type: "movie", Title: "Late", Catalog: &domainlibrary.MediaCatalog{Year: ptr(2023)}})

	filter := domainlibrary.ListMediaItemsFilter{
		LibraryID: ptr(shared.LibraryID("lib-1")),
		YearFrom:  2000,
		YearTo:    2010,
	}
	items, err := repo.List(context.Background(), filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "mid" {
		t.Errorf("expected only 'mid', got %v", items)
	}
}

func TestLibraryItemsFilter_Count_WithFilters(t *testing.T) {
	repo := libfakes.NewFakeMediaItemQueryRepo()
	repo.Seed(&domainlibrary.MediaItem{ID: "a", LibraryID: "lib-1", Type: "movie", Title: "A", Catalog: &domainlibrary.MediaCatalog{Year: ptr(2020)}, Classification: &domainlibrary.MediaClassification{CommunityRating: ptr(9.0)}})
	repo.Seed(&domainlibrary.MediaItem{ID: "b", LibraryID: "lib-1", Type: "movie", Title: "B", Catalog: &domainlibrary.MediaCatalog{Year: ptr(2010)}, Classification: &domainlibrary.MediaClassification{CommunityRating: ptr(6.0)}})

	filter := domainlibrary.ListMediaItemsFilter{
		LibraryID: ptr(shared.LibraryID("lib-1")),
		YearFrom:  2015,
		MinRating: 8.0,
	}
	count, err := repo.Count(context.Background(), filter)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}
}

func TestLibraryItemsFilter_Cursor_TitlePagination(t *testing.T) {
	repo := libfakes.NewFakeMediaItemQueryRepo()
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		repo.Seed(&domainlibrary.MediaItem{
			ID: shared.MediaItemID(id), LibraryID: "lib-1", Type: "movie", Title: "Movie " + id, SortTitle: "Movie " + id,
		})
	}
	first := domainlibrary.ListMediaItemsFilter{
		LibraryID: ptr(shared.LibraryID("lib-1")),
		SortField: "title",
		Limit:     2,
	}
	page1, err := repo.List(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if len(page1) != 2 || string(page1[0].ID) != "a" || string(page1[1].ID) != "b" {
		t.Fatalf("page 1 ids = %+v, want [a b]", ids(page1))
	}

	second := first
	second.Cursor = shared.Cursor{SortField: "title", SortValue: page1[1].SortTitle, LastID: string(page1[1].ID)}
	page2, err := repo.List(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 2 || string(page2[0].ID) != "c" || string(page2[1].ID) != "d" {
		t.Fatalf("page 2 ids = %+v, want [c d]", ids(page2))
	}
}

func TestLibraryItemsFilter_Cursor_SortMismatchRejected(t *testing.T) {
	repo := libfakes.NewFakeMediaItemQueryRepo()
	repo.Seed(&domainlibrary.MediaItem{ID: "a", LibraryID: "lib-1", Type: "movie", Title: "A"})
	filter := domainlibrary.ListMediaItemsFilter{
		LibraryID: ptr(shared.LibraryID("lib-1")),
		SortField: "title",
		Cursor:    shared.Cursor{SortField: "year", SortValue: "2020", LastID: "x"},
	}
	if _, err := repo.List(context.Background(), filter); err == nil {
		t.Fatal("expected ErrCursorMismatch")
	}
}

func ids(items []*domainlibrary.MediaItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = string(it.ID)
	}
	return out
}
