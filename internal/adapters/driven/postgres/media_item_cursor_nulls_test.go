package postgres_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// Sorting a library by year is one click away in every client, and `year` is
// NULLABLE: an item the metadata provider has not matched yet - a fresh scan, a
// home video, a file TMDB cannot identify - simply has no year. The same holds
// for premiere_date and community_rating, the other two sortable nullable
// columns.
//
// The keyset cursor compares row-wise, `(mc.year, mi.id) < ($1, $2)`, and binds
// the cursor's sort value as TEXT. Two things go wrong once a NULL is involved:
//
//   - the handler has no value to encode and emits an EMPTY STRING, which
//     Postgres refuses to read as an integer (`invalid input syntax for type
//     integer: ""`). That surfaces as a 500. It needs no crafted input: it fires
//     whenever a year-less item happens to be the LAST row of a page;
//   - and a row comparison involving NULL evaluates to NULL rather than true, so
//     year-less rows are silently DROPPED from every page after the first - the
//     item vanishes from the library mid-scroll, with no error anywhere.
//
// Found 2026-08-08 while auditing the cursor path, which had just become the
// main pagination route on several screens.

// cursorSortValueForYear mirrors what grpcapi's mediaItemSortValue emits for
// the `year` sort: a zero-padded number, or the empty string when the item has
// no year. Duplicated here on purpose - this test pins what the REPOSITORY must
// do when handed that cursor shape, whatever the handler's encoding is called.
func cursorSortValueForYear(item *library.MediaItem) string {
	if item.Catalog != nil && item.Catalog.Year != nil {
		return fmt.Sprintf("%010d", *item.Catalog.Year)
	}
	return ""
}

func TestMediaItemRepo_ListCursor_SortByNullableYear(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mi-null-year-lib")
	require.NoError(t, libRepo.Create(ctx, lib))

	year1999, year2001 := 1999, 2001
	withYear := func(id string, y *int) *library.MediaItem {
		it := newTestMediaItem(id, lib.ID)
		it.Catalog.Year = y
		return it
	}

	// Two items carry a year, two do not.
	want := []string{"miny1", "miny2", "miny3", "miny4"}
	require.NoError(t, itemRepo.Create(ctx, withYear("miny1", &year1999)))
	require.NoError(t, itemRepo.Create(ctx, withYear("miny2", &year2001)))
	require.NoError(t, itemRepo.Create(ctx, withYear("miny3", nil)))
	require.NoError(t, itemRepo.Create(ctx, withYear("miny4", nil)))

	libID := lib.ID

	// Both directions, because the two defects hide in different ones: ASC puts
	// NULLs LAST, so the year-less rows are the ones that go missing; DESC puts
	// them FIRST, so a year-less row ends page 1 and its empty cursor value is
	// what Postgres refuses to read as an integer.
	for _, order := range []string{"ASC", "DESC"} {
		t.Run(order, func(t *testing.T) {
			var collected []string
			cursor := shared.Cursor{}
			// An iteration cap, not a loop-until-empty: a seek predicate that
			// fails to advance would otherwise spin forever against a live
			// connection instead of failing fast.
			for range len(want) + 3 {
				page, err := itemRepo.List(ctx, library.ListMediaItemsFilter{
					LibraryID: &libID,
					SortField: "year",
					SortOrder: order,
					Limit:     1,
					Cursor:    cursor,
				})
				require.NoError(t, err, "paging by year must not error once a year-less row becomes the cursor position")
				if len(page) == 0 {
					break
				}
				for _, it := range page {
					collected = append(collected, string(it.ID))
				}
				last := page[len(page)-1]
				cursor = shared.Cursor{
					SortField: "year",
					SortValue: cursorSortValueForYear(last),
					LastID:    string(last.ID),
				}
			}

			assert.ElementsMatch(t, want, collected,
				"every item must be reached exactly once - a NULL year must not make an item vanish mid-scroll")
			assert.Len(t, collected, len(want), "no row may be skipped or repeated across a page boundary")
		})
	}
}
