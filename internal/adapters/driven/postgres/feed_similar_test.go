package postgres

import (
	"slices"
	"testing"

	"github.com/eveyproject/evey/internal/domain/feed"
)

func TestExpandGenreFamilies_SeriesAndFilmGenresMeet(t *testing.T) {
	got := expandGenreFamilies([]string{"Animation", "Sci-Fi & Fantasy"})
	for _, want := range []string{"Animation", "Sci-Fi & Fantasy", "Science Fiction", "Fantasy"} {
		if !slices.Contains(got, want) {
			t.Errorf("expand(series) = %v, missing %q", got, want)
		}
	}
	got = expandGenreFamilies([]string{"Science Fiction", "Drama"})
	if !slices.Contains(got, "Sci-Fi & Fantasy") {
		t.Errorf("expand(film) = %v, a film's half must reach the series' pair", got)
	}
	if n := len(expandGenreFamilies([]string{"Fantasy", "Sci-Fi & Fantasy"})); n != 3 {
		t.Errorf("duplicates kept: %d genres, want 3", n)
	}
}

func TestKeepMostRelevant_CutsOneGenreMatchesWhenBetterExist(t *testing.T) {
	scored := func(scores ...int) []*feed.FeedItem {
		out := make([]*feed.FeedItem, len(scores))
		for i, s := range scores {
			out[i] = &feed.FeedItem{Score: s}
		}
		return out
	}
	count := func(items []*feed.FeedItem) int { return len(items) }

	if n := count(keepMostRelevant(scored(3, 3, 2, 1, 1, 1), 3)); n != 3 {
		t.Errorf("three-genre target: kept %d, want the 3 titles sharing two genres or more", n)
	}
	if n := count(keepMostRelevant(scored(1, 1, 1), 3)); n != 0 {
		t.Errorf("best match is one genre of three: kept %d, want an empty row rather than a lie", n)
	}
	if n := count(keepMostRelevant(scored(1, 1), 1)); n != 2 {
		t.Errorf("one-genre target: kept %d, want 2", n)
	}
	if n := count(keepMostRelevant(nil, 2)); n != 0 {
		t.Errorf("empty list: kept %d", n)
	}
}
