package postgres_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// A season's episodes are listed with ListMediaItems(parentId) and no sort, and
// every client re-sorts the page by episode number. That only works while the
// season fits in one page: the server answered by title, so a 600-episode
// season came back as the first 500 TITLES - episodes missing from the start,
// others from the end - and no client-side re-sort can recover them. The
// handler now asks for "index_number" when it lists a parent's children; this
// pins the repository side against a real Postgres, unnumbered episode included
// (index_number is the one nullable media_items sort column).

// cursorSortValueForIndex mirrors grpcapi's mediaItemSortValue "index_number" case.
func cursorSortValueForIndex(it *library.MediaItem) string {
	if it.IndexNumber != nil {
		return fmt.Sprintf("%010d", *it.IndexNumber)
	}
	return ""
}

func TestMediaItemRepo_ListChildren_ByIndexNumberAcrossPages(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mi-children-order-lib")
	require.NoError(t, libRepo.Create(ctx, lib))

	series := newTestMediaItem("mico-series", lib.ID)
	series.Type = library.Series
	require.NoError(t, itemRepo.Create(ctx, series))
	season := newTestMediaItem("mico-season", lib.ID)
	season.Type = library.Season
	season.ParentID = string(series.ID)
	require.NoError(t, itemRepo.Create(ctx, season))

	// Titles run AGAINST the numbers, so a title ordering cannot pass for this one.
	episode := func(id, title string, n *int) {
		it := newTestMediaItem(id, lib.ID)
		it.Type = library.Episode
		it.ParentID = string(season.ID)
		it.Title, it.SortTitle = title, title
		it.IndexNumber = n
		require.NoError(t, itemRepo.Create(ctx, it))
	}
	one, two, three, ten := 1, 2, 3, 10
	episode("mico-e10", "a pilot reprise", &ten)
	episode("mico-e3", "b", &three)
	episode("mico-e2", "c", &two)
	episode("mico-e1", "d", &one)
	episode("mico-sp", "0 special", nil)
	want := []string{"mico-e1", "mico-e2", "mico-e3", "mico-e10", "mico-sp"}

	parentID := string(season.ID)
	t.Run("first page without a cursor", func(t *testing.T) {
		page, err := itemRepo.List(ctx, library.ListMediaItemsFilter{
			ParentID: &parentID, SortField: "index_number", Limit: 3,
		})
		require.NoError(t, err)
		var got []string
		for _, it := range page {
			got = append(got, string(it.ID))
		}
		assert.Equal(t, want[:3], got, "a page of a season is its first episodes by number, not by title")
	})

	t.Run("cursor pages", func(t *testing.T) {
		var collected []string
		cursor := shared.Cursor{}
		for range len(want) + 3 {
			page, err := itemRepo.List(ctx, library.ListMediaItemsFilter{
				ParentID: &parentID, SortField: "index_number", Limit: 2, Cursor: cursor,
			})
			require.NoError(t, err, "an unnumbered episode as the cursor row must not error")
			if len(page) == 0 {
				break
			}
			for _, it := range page {
				collected = append(collected, string(it.ID))
			}
			last := page[len(page)-1]
			cursor = shared.Cursor{SortField: "index_number", SortValue: cursorSortValueForIndex(last), LastID: string(last.ID)}
		}
		assert.Equal(t, want, collected, "every episode once, by number, the unnumbered one last")
	})
}
