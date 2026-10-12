package postgres

import "github.com/eveyproject/evey/internal/domain/feed"

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

// broadGenres are the catch-alls nearly every title carries. Sharing one of
// them says nothing: Bonanza (Western, Action & Adventure, Drama, Family) was
// "similar" to Metropolis on Drama and to Caminandes on Family (demo,
// 2026-10-01). They still count in the ranking, never as the only link.
var broadGenres = map[string]bool{
	"Drama": true, "Comedy": true, "Family": true,
	"Drame": true, "Comédie": true, "Familial": true,
}

// linkingGenres are the genres a candidate must share at least one of: the
// target's specific ones, or all of them when it only has catch-alls.
func linkingGenres(genres []string) []string {
	out := make([]string, 0, len(genres))
	for _, g := range genres {
		if !broadGenres[g] {
			out = append(out, g)
		}
	}
	if len(out) == 0 {
		return genres
	}
	return out
}

// keepMostRelevant keeps the titles sharing at least two genres with the
// target (all of them when it has fewer): a war romance is not "similar" to
// Big Buck Bunny (2026-09-30). When nothing reaches that, the row is empty -
// no Similar row beats one that lies (user, 2026-10-01).
func keepMostRelevant(items []*feed.FeedItem, targetGenres int) []*feed.FeedItem {
	if len(items) == 0 {
		return items
	}
	cut := min(2, targetGenres)
	kept := items[:0]
	for _, it := range items {
		if it.Score >= cut {
			kept = append(kept, it)
		}
	}
	return kept
}
