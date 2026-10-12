package activity

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/eveyproject/evey/internal/domain/activity"
)

// The stats query groups by the genre string stored on the fiche, so a library
// tagged by two providers in two languages produced two slices for one genre.
// These pin the merge.

func TestFoldGenreBreakdown_MergesTheSpellingsOfOneGenre(t *testing.T) {
	t.Parallel()
	in := map[string]activity.GenreStats{
		"Drama": {TotalTimeMs: 1000, PlayCount: 2},
		"Drame": {TotalTimeMs: 500, PlayCount: 1},
	}

	got := foldGenreBreakdown(in, "fr")

	assert.Len(t, got, 1, "Drama and Drame are one genre")
	assert.Equal(t, int64(1500), got["Drame"].TotalTimeMs)
	assert.Equal(t, 3, got["Drame"].PlayCount)
}

// Without a configured server language nothing is translated - but the merge
// still has to happen, because the duplicate slices are the actual defect.
func TestFoldGenreBreakdown_MergesWithoutTranslating(t *testing.T) {
	t.Parallel()
	got := foldGenreBreakdown(map[string]activity.GenreStats{
		"Drama": {TotalTimeMs: 1000, PlayCount: 2},
		"Drame": {TotalTimeMs: 500, PlayCount: 1},
	}, "")

	assert.Len(t, got, 1)
	for label, gs := range got {
		assert.Contains(t, []string{"Drama", "Drame"}, label)
		assert.Equal(t, int64(1500), gs.TotalTimeMs)
	}
}

// A genre no provider table knows must survive: dropping it would quietly
// remove watch time from the totals the percentages are computed against.
func TestFoldGenreBreakdown_KeepsUnknownGenres(t *testing.T) {
	t.Parallel()
	got := foldGenreBreakdown(map[string]activity.GenreStats{
		"Mecha":  {TotalTimeMs: 700, PlayCount: 1},
		"Action": {TotalTimeMs: 300, PlayCount: 1},
	}, "fr")

	assert.Len(t, got, 2)
	assert.Equal(t, int64(700), got["Mecha"].TotalTimeMs)
	assert.Equal(t, int64(300), got["Action"].TotalTimeMs)
}

// Map iteration is unordered; the label a merged genre ends up under must not
// be, or the same profile would see "Drama" on one load and "Drame" on the next.
func TestFoldGenreBreakdown_PicksTheSameWinnerEveryTime(t *testing.T) {
	t.Parallel()
	in := map[string]activity.GenreStats{
		"Drama": {TotalTimeMs: 1000, PlayCount: 2},
		"Drame": {TotalTimeMs: 500, PlayCount: 1},
	}

	first := foldGenreBreakdown(in, "")
	for range 20 {
		assert.Equal(t, first, foldGenreBreakdown(in, ""))
	}
}

func TestFoldGenreBreakdown_EmptyStaysEmpty(t *testing.T) {
	t.Parallel()
	assert.Empty(t, foldGenreBreakdown(map[string]activity.GenreStats{}, "fr"))
	assert.Empty(t, foldGenreBreakdown(nil, "fr"))
}
