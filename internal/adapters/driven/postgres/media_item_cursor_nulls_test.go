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

// cursorSortValueForRating mirrors mediaItemSortValue's "rating" case.
func cursorSortValueForRating(it *library.MediaItem) string {
	if it.Classification != nil && it.Classification.CommunityRating != nil {
		return fmt.Sprintf("%020.6f", *it.Classification.CommunityRating)
	}
	return ""
}

// Sorting by rating, descending, is the one an operator reaches for to see the
// best films first. Two things made it useless on 2026-09-05, both live on prod:
//
//   - Postgres orders NULLs FIRST on DESC, so a library opened on the films
//     NOBODY has rated, above the 8.7s. "Trier par note ça marche pas";
//   - and scrolling died with a 500, because the cursor for a rating page
//     carried the item's UUID (mediaItemSortValue knew only "community_rating"
//     while the API emits "rating") and page 2 bound it against a numeric
//     column.
//
// The ordering is now NULLS LAST in both directions, which this pins against a
// real Postgres: the seek predicate and the ORDER BY must agree, or a page
// boundary skips rows or repeats forever.
func TestMediaItemRepo_ListCursor_SortByRating_UnratedComeLast(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mi-rating-lib")
	require.NoError(t, libRepo.Create(ctx, lib))

	withRating := func(id string, r *float64) *library.MediaItem {
		it := newTestMediaItem(id, lib.ID)
		// newTestMediaItem leaves Classification nil (unlike Catalog).
		it.Classification = &library.MediaClassification{CommunityRating: r}
		return it
	}
	high, low := 8.7, 4.2
	require.NoError(t, itemRepo.Create(ctx, withRating("mir-high", &high)))
	require.NoError(t, itemRepo.Create(ctx, withRating("mir-low", &low)))
	require.NoError(t, itemRepo.Create(ctx, withRating("mir-none1", nil)))
	require.NoError(t, itemRepo.Create(ctx, withRating("mir-none2", nil)))

	libID := lib.ID
	want := []string{"mir-high", "mir-low", "mir-none1", "mir-none2"}

	t.Run("descending opens on the best rated, not on the unrated", func(t *testing.T) {
		page, err := itemRepo.List(ctx, library.ListMediaItemsFilter{
			LibraryID: &libID, SortField: "rating", SortOrder: "DESC", Limit: 2,
		})
		require.NoError(t, err)
		require.Len(t, page, 2)
		assert.Equal(t, "mir-high", string(page[0].ID), "8.7 must come first - an unrated film is not the best-rated film")
		assert.Equal(t, "mir-low", string(page[1].ID))
	})

	for _, order := range []string{"ASC", "DESC"} {
		t.Run("paging holds every row - "+order, func(t *testing.T) {
			var collected []string
			cursor := shared.Cursor{}
			for range len(want) + 3 {
				page, err := itemRepo.List(ctx, library.ListMediaItemsFilter{
					LibraryID: &libID,
					SortField: "rating",
					SortOrder: order,
					Limit:     1,
					Cursor:    cursor,
				})
				require.NoError(t, err, "paging by rating must not error once an unrated row becomes the cursor position")
				if len(page) == 0 {
					break
				}
				for _, it := range page {
					collected = append(collected, string(it.ID))
				}
				last := page[len(page)-1]
				cursor = shared.Cursor{
					SortField: "rating",
					SortValue: cursorSortValueForRating(last),
					LastID:    string(last.ID),
				}
			}
			assert.ElementsMatch(t, want, collected, "every item reached exactly once")
			assert.Len(t, collected, len(want), "no row skipped or repeated across a page boundary")
			if order == "DESC" {
				assert.Equal(t, []string{"mir-high", "mir-low"}, collected[:2], "rated films first, in order")
			}
		})
	}
}
