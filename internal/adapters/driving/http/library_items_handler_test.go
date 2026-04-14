package httpapi

import (
	"context"
	"testing"

	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

func ptr[T any](v T) *T { return &v }

func TestLibraryItemsFilter_YearFrom(t *testing.T) {
	repo := libfakes.NewFakeMediaItemQueryRepo()
	repo.Seed(&domainlibrary.MediaItem{ID: "old", LibraryID: "lib-1", Type: "movie", Title: "Old Film", Year: ptr(1990)})
	repo.Seed(&domainlibrary.MediaItem{ID: "new", LibraryID: "lib-1", Type: "movie", Title: "New Film", Year: ptr(2020)})

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
	repo.Seed(&domainlibrary.MediaItem{ID: "old", LibraryID: "lib-1", Type: "movie", Title: "Old Film", Year: ptr(1990)})
	repo.Seed(&domainlibrary.MediaItem{ID: "new", LibraryID: "lib-1", Type: "movie", Title: "New Film", Year: ptr(2020)})

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
	repo.Seed(&domainlibrary.MediaItem{ID: "low", LibraryID: "lib-1", Type: "movie", Title: "Low Rated", CommunityRating: ptr(5.0)})
	repo.Seed(&domainlibrary.MediaItem{ID: "high", LibraryID: "lib-1", Type: "movie", Title: "High Rated", CommunityRating: ptr(8.5)})

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
	repo.Seed(&domainlibrary.MediaItem{ID: "early", LibraryID: "lib-1", Type: "movie", Title: "Early", Year: ptr(1980)})
	repo.Seed(&domainlibrary.MediaItem{ID: "mid", LibraryID: "lib-1", Type: "movie", Title: "Mid", Year: ptr(2005)})
	repo.Seed(&domainlibrary.MediaItem{ID: "late", LibraryID: "lib-1", Type: "movie", Title: "Late", Year: ptr(2023)})

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
	repo.Seed(&domainlibrary.MediaItem{ID: "a", LibraryID: "lib-1", Type: "movie", Title: "A", Year: ptr(2020), CommunityRating: ptr(9.0)})
	repo.Seed(&domainlibrary.MediaItem{ID: "b", LibraryID: "lib-1", Type: "movie", Title: "B", Year: ptr(2010), CommunityRating: ptr(6.0)})

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
