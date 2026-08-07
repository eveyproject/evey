package grpcapi

import (
	"context"
	"testing"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression: sort_title is empty on rows whose scan ran before that
// column was populated. The cursor must encode SortValue exactly the
// way the SQL keyset clause reads it (i.e. straight from SortTitle),
// otherwise the WHERE `(sort_title, mi.id) > (cursorSortValue, cursorID)`
// matches nothing and page 2 returns 0 items. We verified this against
// prod where 1030 movies were all rendered as ~40 (one page).
func TestMediaItemSortValue_TitleFallbackBreaksKeysetPagination(t *testing.T) {
	t.Parallel()

	t.Run("empty SortTitle returns empty (matches SQL column)", func(t *testing.T) {
		item := &library.MediaItem{
			ID:        shared.MediaItemID("abc"),
			Title:     "Some Movie Title",
			SortTitle: "",
		}
		got := mediaItemSortValue(item, "title")
		assert.Empty(t, got, "must not fall back to Title - that diverges from mi.sort_title")
	})

	t.Run("non-empty SortTitle is returned as-is", func(t *testing.T) {
		item := &library.MediaItem{
			ID:        shared.MediaItemID("abc"),
			Title:     "The Great Adventure",
			SortTitle: "great adventure, the",
		}
		got := mediaItemSortValue(item, "title")
		assert.Equal(t, "great adventure, the", got)
	})

	t.Run("sort_title sortKey aliases to the same column", func(t *testing.T) {
		item := &library.MediaItem{SortTitle: "x"}
		assert.Equal(t, "x", mediaItemSortValue(item, "sort_title"))
	})
}

// Sanity that the cursor builder yields an encodable cursor that round-trips
// through DecodeCursor with the same SortField/SortValue/LastID.
func TestNextMediaItemsCursor_RoundTrip(t *testing.T) {
	t.Parallel()

	items := []*library.MediaItem{
		{ID: shared.MediaItemID("first"), SortTitle: "alpha"},
		{ID: shared.MediaItemID("last"), SortTitle: "zulu"},
	}
	filter := library.ListMediaItemsFilter{Limit: 2, SortField: "title"}
	tok := nextMediaItemsCursor(items, filter)
	assert.NotEmpty(t, tok)

	got, err := shared.DecodeCursor(tok)
	require.NoError(t, err)
	assert.Equal(t, "title", got.SortField)
	assert.Equal(t, "zulu", got.SortValue)
	assert.Equal(t, "last", got.LastID)
}

// Empty page: cursor must be empty (no more pages).
func TestNextMediaItemsCursor_ShortPageReturnsEmpty(t *testing.T) {
	t.Parallel()

	items := []*library.MediaItem{
		{ID: shared.MediaItemID("only"), SortTitle: "alpha"},
	}
	filter := library.ListMediaItemsFilter{Limit: 10, SortField: "title"}
	assert.Empty(t, nextMediaItemsCursor(items, filter))
}

// The fake and the real repository have to agree on WHERE a missing value sits
// in the ordering, or handler tests validate a sequence production never emits.
// Postgres puts NULLs LAST on ASC and FIRST on DESC; a plain string comparison
// (an empty string sorts smallest) does the exact opposite in both directions.
//
// This is the defect shape that already cost a session once: a fake that did
// not sort the way the SQL did made a correct offset-to-cursor handover look
// broken. Here the mirror is asserted directly - the same walk runs against
// internal/adapters/driven/postgres in
// TestMediaItemRepo_ListCursor_SortByNullableYear.
func TestFakeMediaItemCursor_MissingYearKeepsItsPlaceInTheOrdering(t *testing.T) {
	t.Parallel()

	year1999, year2001 := 1999, 2001
	newItem := func(id string, y *int) *library.MediaItem {
		return &library.MediaItem{
			ID:        shared.MediaItemID(id),
			LibraryID: shared.LibraryID("lib-1"),
			Type:      library.MovieItem,
			Title:     "Some Movie " + id,
			SortTitle: "some movie " + id,
			Catalog:   &library.MediaCatalog{Year: y},
		}
	}
	want := []string{"a", "b", "c", "d"}

	for _, order := range []string{"ASC", "DESC"} {
		t.Run(order, func(t *testing.T) {
			repo := fakes.NewFakeMediaItemQueryRepo()
			repo.Add(newItem("a", &year1999))
			repo.Add(newItem("b", &year2001))
			repo.Add(newItem("c", nil)) // never matched by a provider
			repo.Add(newItem("d", nil))

			libID := shared.LibraryID("lib-1")
			var collected []string
			cursor := shared.Cursor{}
			// Iteration cap, not loop-until-empty: a seek that fails to advance
			// must fail fast instead of spinning.
			for range len(want) + 3 {
				page, err := repo.List(context.Background(), library.ListMediaItemsFilter{
					LibraryID: &libID,
					SortField: "year",
					SortOrder: order,
					Limit:     1,
					Cursor:    cursor,
				})
				require.NoError(t, err)
				if len(page) == 0 {
					break
				}
				for _, it := range page {
					collected = append(collected, string(it.ID))
				}
				last := page[len(page)-1]
				cursor = shared.Cursor{
					SortField: "year",
					SortValue: mediaItemSortValue(last, "year"),
					LastID:    string(last.ID),
				}
			}

			assert.ElementsMatch(t, want, collected,
				"an item with no year must still be reached - it must not vanish after the first page")
			assert.Len(t, collected, len(want), "no row may be skipped or repeated across a page boundary")
		})
	}
}
