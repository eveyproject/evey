package grpcapi

import (
	"context"
	"testing"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

// Every sort key encodes a value the SQL keyset clause can compare against
// its column: fixed-width numbers so string order is numeric order, RFC3339
// times, and "" (never the item id) when the catalogue value is missing.
func TestMediaItemSortValue_EverySortKeyIsComparable(t *testing.T) {
	t.Parallel()
	added := time.Date(2026, 9, 16, 3, 0, 0, 500, time.UTC)
	premiere := time.Date(1999, 3, 31, 0, 0, 0, 0, time.UTC)
	full := &library.MediaItem{
		ID: "mi-1", SortTitle: "heat", AddedAt: added,
		Catalog:        &library.MediaCatalog{Year: new(1995), PremiereDate: &premiere},
		Classification: &library.MediaClassification{CommunityRating: new(8.25)},
	}
	bare := &library.MediaItem{ID: "mi-2", AddedAt: added}

	assert.Equal(t, "2026-09-16T03:00:00.0000005Z", mediaItemSortValue(full, "added_at"))
	assert.Equal(t, mediaItemSortValue(full, "added_at"), mediaItemSortValue(full, "date_added"))
	assert.Equal(t, "0000001995", mediaItemSortValue(full, "year"))
	assert.Equal(t, "1999-03-31T00:00:00Z", mediaItemSortValue(full, "premiere_date"))
	assert.Equal(t, "0000000000008.250000", mediaItemSortValue(full, "rating"))
	assert.Equal(t, mediaItemSortValue(full, "rating"), mediaItemSortValue(full, "community_rating"), "both spellings, the 2026-09-05 prod 500")
	// Fixed width keeps string order numeric: 10.0 sorts after 8.25.
	ten := &library.MediaItem{Classification: &library.MediaClassification{CommunityRating: new(10.0)}}
	assert.Greater(t, mediaItemSortValue(ten, "rating"), mediaItemSortValue(full, "rating"))

	for _, key := range []string{"year", "premiere_date", "rating", "community_rating", "title"} {
		assert.Empty(t, mediaItemSortValue(bare, key), "%s without a value encodes empty, never the id", key)
	}
}

// A page size beyond the ceiling is clamped, a page number below 2 starts at
// the top, and the offset is page-relative to the clamped size.
func TestClampPagination(t *testing.T) {
	t.Parallel()
	limit, offset := clampPagination(0, 0, 50)
	assert.Equal(t, 50, limit)
	assert.Zero(t, offset)
	limit, offset = clampPagination(20, 3, 50)
	assert.Equal(t, 20, limit)
	assert.Equal(t, 40, offset)
	limit, offset = clampPagination(100000, 2, 50)
	assert.Equal(t, maxPageSize, limit)
	assert.Equal(t, maxPageSize, offset)
	_, offset = clampPagination(20, -4, 50)
	assert.Zero(t, offset)
}

// The grid walks the library by following NextCursor: a cursor page must
// start right after the last row of the previous one (no repeat, no gap), win
// over any page/offset sent alongside it, and a mangled cursor is the
// caller's fault - InvalidArgument, not a silent first page again.
func TestLibraryHandler_ListMediaItems_CursorWalk(t *testing.T) {
	h, _, mediaQuery, _, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		mediaQuery.Seed(&library.MediaItem{ID: shared.MediaItemID("mi-" + id), LibraryID: "lib-1", Title: id, SortTitle: id, Type: "movie"})
	}

	first, err := h.ListMediaItems(ctx, &eveyv1.ListMediaItemsRequest{
		Pagination: &eveyv1.PaginationRequest{Page: 1, PageSize: 2},
	})
	require.NoError(t, err)
	require.Len(t, first.Items, 2)
	require.NotEmpty(t, first.Pagination.NextCursor)

	// Page 3 by offset would skip rows; with a cursor it is ignored.
	second, err := h.ListMediaItems(ctx, &eveyv1.ListMediaItemsRequest{
		Pagination: &eveyv1.PaginationRequest{Page: 3, PageSize: 2, Cursor: first.Pagination.NextCursor},
	})
	require.NoError(t, err)
	require.Len(t, second.Items, 2)
	assert.Equal(t, []string{"mi-c", "mi-d"}, []string{second.Items[0].Id, second.Items[1].Id})

	third, err := h.ListMediaItems(ctx, &eveyv1.ListMediaItemsRequest{
		Pagination: &eveyv1.PaginationRequest{PageSize: 2, Cursor: second.Pagination.NextCursor},
	})
	require.NoError(t, err)
	require.Len(t, third.Items, 1)
	assert.Equal(t, "mi-e", third.Items[0].Id)
	assert.Empty(t, third.Pagination.NextCursor, "a short page is the last one")

	_, err = h.ListMediaItems(ctx, &eveyv1.ListMediaItemsRequest{
		Pagination: &eveyv1.PaginationRequest{PageSize: 2, Cursor: "not-a-cursor!!"},
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// A kids profile walks a library by cursor. A title the SQL pre-filter lets
// through (no official rating, a foreign certification only) is dropped by the
// in-memory parental pass: the page comes back one short, and a cursor derived
// from the FILTERED page read it as the last one - every title after it was
// out of reach for the kids profile.
func TestLibraryHandler_ListMediaItems_CursorWalkSurvivesAParentalDrop(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, profiles, _ := newTestLibraryHandler()
	ctx := context.Background()
	pg := &library.MediaClassification{OfficialRating: "PG"}
	for _, id := range []string{"a", "c", "d", "e"} {
		mediaQuery.Seed(&library.MediaItem{ID: shared.MediaItemID("mi-" + id), LibraryID: "lib-1", Title: id, SortTitle: id, Type: "movie", Classification: pg})
	}
	mediaQuery.Seed(&library.MediaItem{ID: "mi-b", LibraryID: "lib-1", Title: "b", SortTitle: "b", Type: "movie",
		Classification: &library.MediaClassification{CertificationsByCountry: map[string]string{"GB": "18"}}})
	maxAge := 13
	profiles.Create(ctx, &user.Profile{ID: "profile-2", UserID: "user-2", Name: "Kid", IsKids: true,
		Parental: &user.ParentalSettings{MaxAge: &maxAge, Enabled: true}})
	libraryAccess.Grant(ctx, "user-2", "lib-1")
	kid := userCtx("user-2", "profile-2")

	var seen []string
	cursor := ""
	for range 10 {
		resp, err := h.ListMediaItems(kid, &eveyv1.ListMediaItemsRequest{
			LibraryId:  "lib-1",
			Pagination: &eveyv1.PaginationRequest{PageSize: 2, Cursor: cursor},
		})
		require.NoError(t, err)
		for _, it := range resp.Items {
			seen = append(seen, it.Id)
		}
		cursor = resp.Pagination.NextCursor
		if cursor == "" {
			break
		}
	}
	assert.Equal(t, []string{"mi-a", "mi-c", "mi-d", "mi-e"}, seen,
		"the kids profile must reach every title it may see, past the page that lost one")
}
