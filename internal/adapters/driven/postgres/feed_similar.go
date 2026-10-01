package postgres

import "github.com/evey-project/evey/internal/domain/feed"

// genreFamilies pairs the combined genres TMDB gives series with the single
// genres it gives films, in the two metadata languages the server ships. Left
// as they are, "Sci-Fi & Fantasy" never met "Science Fiction" and an animated
// series only ever matched films on "Animation".
var genreFamilies = map[string][]string{
	"Sci-Fi & Fantasy":              {"Science Fiction", "Fantasy"},
	"Action & Adventure":            {"Action", "Adventure"},
	"War & Politics":                {"War"},
	"Science-Fiction & Fantastique": {"Science-Fiction", "Fantastique"},
	"Action & Aventure":             {"Action", "Aventure"},
	"Guerre & Politique":            {"Guerre"},
}

// expandGenreFamilies returns the genres plus their family members, both ways:
// a combined genre brings its halves, a half brings the combined genres it
// belongs to. Order is kept, no duplicates.
func expandGenreFamilies(genres []string) []string {
	seen := make(map[string]bool, len(genres))
	out := make([]string, 0, len(genres))
	add := func(g string) {
		if !seen[g] {
			seen[g] = true
			out = append(out, g)
		}
	}
	for _, g := range genres {
		add(g)
		for _, half := range genreFamilies[g] {
			add(half)
		}
		for combined, halves := range genreFamilies {
			for _, half := range halves {
				if half == g {
					add(combined)
				}
			}
		}
	}
	return out
}

// keepMostRelevant cuts a score-ordered Similar list to the titles at the
// best level of match available, capped at two shared genres: a large library
// keeps everything sharing two genres, a small one no longer pads the row with
// titles that share one genre out of three (a war romance "similar" to Big
// Buck Bunny, 2026-09-30). When the best match is a single genre, those stay:
// a thin row beats a page ending on nothing.
func keepMostRelevant(items []*feed.FeedItem, targetGenres int) []*feed.FeedItem {
	if len(items) == 0 {
		return items
	}
	floor := min(2, targetGenres)
	best := 0
	for _, it := range items {
		best = max(best, it.Score)
	}
	cut := min(floor, best)
	kept := items[:0]
	for _, it := range items {
		if it.Score >= cut {
			kept = append(kept, it)
		}
	}
	return kept
}
