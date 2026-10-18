package postgres

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The trailers used to cost one INSERT per trailer, then one multi-row VALUES;
// they now ride in the item's single statement as one unnest() over per-column
// arrays, so any number of trailers is the same statement text and the same
// number of parameters.
func TestItemWriteQuery_TrailersAreOneUnnestNotOneParameterPerTrailer(t *testing.T) {
	mk := func(n int) *library.MediaItem {
		it := &library.MediaItem{ID: "mi1", Type: library.MovieItem}
		for range n {
			it.Trailers = append(it.Trailers, library.Trailer{Site: "YouTube", Key: "k"})
		}
		return it
	}
	q2, a2, err := itemWriteQuery(mk(2), false, "t", "t", nil)
	require.NoError(t, err)
	q9, a9, err := itemWriteQuery(mk(9), false, "t", "t", nil)
	require.NoError(t, err)

	assert.Equal(t, q2, q9, "the statement text must not grow with the trailer count")
	assert.Len(t, a9, len(a2), "nor the parameter count")
	assert.Contains(t, q2, "unnest(")
	assert.Contains(t, q2, "WITH ORDINALITY")
}

// No trailers: no INSERT at all (an empty unnest would be harmless, but the
// statement stays minimal), yet an update still deletes the old ones.
func TestItemWriteQuery_NoTrailers(t *testing.T) {
	item := &library.MediaItem{ID: "mi1", Type: library.MovieItem}

	upd, _, err := itemWriteQuery(item, false, "t", "t", nil)
	require.NoError(t, err)
	assert.NotContains(t, upd, "INSERT INTO media_trailers")
	assert.Contains(t, upd, "DELETE FROM media_trailers")

	// A create has nothing to erase: the item row was just inserted.
	crt, _, err := itemWriteQuery(item, true, "t", "t", nil)
	require.NoError(t, err)
	assert.NotContains(t, crt, "DELETE FROM")
}
