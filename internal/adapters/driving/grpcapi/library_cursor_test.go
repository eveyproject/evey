package grpcapi

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/library"
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
