package music

import "strings"

// genreSeparators are the ways a tag packs several genres into one field:
// "Alternative/Pop/Rock", "Rock; Blues", "Jazz, Soul", "Electro|House".
const genreSeparators = "/;,|"

// SplitGenres turns a track's genre tag into its genres, trimmed, without
// empties or case-insensitive repeats, in tag order.
func SplitGenres(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool { return strings.ContainsRune(genreSeparators, r) })
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		g := strings.TrimSpace(p)
		if g == "" {
			continue
		}
		k := strings.ToLower(g)
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, g)
	}
	return out
}

// GenreCount is one genre of a library and how many tracks carry it.
type GenreCount struct {
	Name  string
	Count int
}
