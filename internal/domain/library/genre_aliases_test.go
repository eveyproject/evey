package library

import (
	"slices"
	"strings"
	"testing"
)

// The 28 genres one real library actually held on 2026-09-04. Eight of them are
// duplicates of another in the other language, and three are IMDb-only labels
// TMDB never emits. This is the fixture the fold has to survive.
var storedGenresFromRealLibrary = []string{
	"Action", "Adventure", "Animation", "Aventure", "Comedy", "Comédie", "Crime",
	"Documentaire", "Documentary", "Drama", "Drame", "Familial", "Fantastique",
	"Fantasy", "Guerre", "Histoire", "Horreur", "Horror", "Musique", "Mystery",
	"Mystère", "Romance", "Sci-Fi", "Science-Fiction", "Short", "Thriller",
	"Téléfilm", "Western",
}

func TestCanonicalGenres_FoldsTheRealLibrary(t *testing.T) {
	got := CanonicalGenres(storedGenresFromRealLibrary, "fr")

	// No duplicate label.
	seen := map[string]bool{}
	for _, g := range got {
		if seen[g] {
			t.Errorf("duplicate label %q survived the fold", g)
		}
		seen[g] = true
	}
	// The eight known pairs must have collapsed - and onto the FRENCH label,
	// because this deployment's server language is French.
	for _, pair := range [][2]string{
		{"Adventure", "Aventure"}, {"Comedy", "Comédie"}, {"Documentary", "Documentaire"},
		{"Drama", "Drame"}, {"Fantasy", "Fantastique"}, {"Horror", "Horreur"},
		{"Mystery", "Mystère"}, {"Sci-Fi", "Science-Fiction"},
	} {
		if slices.Contains(got, pair[0]) {
			t.Errorf("English label %q still listed alongside %q", pair[0], pair[1])
		}
		if !slices.Contains(got, pair[1]) {
			t.Errorf("French label %q missing after folding %q", pair[1], pair[0])
		}
	}
	if len(got) >= len(storedGenresFromRealLibrary) {
		t.Errorf("fold did not shorten the list: %d in, %d out", len(storedGenresFromRealLibrary), len(got))
	}
	if !slices.IsSorted(got) {
		t.Error("output must be sorted - it feeds a filter panel")
	}
}

// A genre we have never seen must pass through, not vanish: an allowlist here
// would silently drop content from the panel.
func TestCanonicalGenres_KeepsUnknownGenres(t *testing.T) {
	got := CanonicalGenres([]string{"Mecha", "Drame"}, "fr")
	if !slices.Contains(got, "Mecha") {
		t.Errorf("unknown genre dropped: %v", got)
	}
}

func TestCanonicalGenre_RendersInRequestedLanguage(t *testing.T) {
	if got := CanonicalGenre("Drama", "fr"); got != "Drame" {
		t.Errorf(`CanonicalGenre("Drama","fr") = %q, want "Drame"`, got)
	}
	if got := CanonicalGenre("Drame", "en"); got != "Drama" {
		t.Errorf(`CanonicalGenre("Drame","en") = %q, want "Drama"`, got)
	}
	// A language we hold no labels for keeps the STORED spelling. Substituting
	// French there would show French to a German server for no reason.
	if got := CanonicalGenre("Drama", "de"); got != "Drama" {
		t.Errorf(`CanonicalGenre("Drama","de") = %q, want the stored spelling back`, got)
	}
}

// No configured server language must still deduplicate, without translating -
// that is what a server whose operator never picked a language gets.
func TestCanonicalGenres_NoLanguageFoldsWithoutTranslating(t *testing.T) {
	got := CanonicalGenres([]string{"Drama", "Drame", "Action"}, "")

	if len(got) != 2 {
		t.Fatalf("duplicates not folded without a language: %v", got)
	}
	frenchOrEnglish := slices.Contains(got, "Drama") || slices.Contains(got, "Drame")
	if !frenchOrEnglish {
		t.Errorf("the surviving spelling should be one of the stored ones: %v", got)
	}
	// Deterministic: the same input must always yield the same winner, whatever
	// the row order the database happened to return.
	again := CanonicalGenres([]string{"Drame", "Action", "Drama"}, "")
	if !slices.Equal(got, again) {
		t.Errorf("result depends on input order: %v vs %v", got, again)
	}
}

// THE pairing test: whatever label the panel offers must select every stored
// spelling of that genre. If this fails, folding the list has actively broken
// the filter - the panel shows "Drame" and every "Drama" title disappears.
func TestExpandGenreAliases_MatchesEveryStoredSpelling(t *testing.T) {
	for _, label := range CanonicalGenres(storedGenresFromRealLibrary, "fr") {
		expanded := ExpandGenreAliases([]string{label})
		for _, stored := range storedGenresFromRealLibrary {
			if CanonicalGenre(stored, "fr") != label {
				continue
			}
			// The query compares lower(stored) against this list.
			if !slices.Contains(expanded, strings.ToLower(stored)) {
				t.Errorf("panel label %q would not select stored genre %q (expanded: %v)", label, stored, expanded)
			}
		}
	}
}

// The expansion feeds a query that lowercases the stored side, so everything it
// returns must already be lowercase - otherwise half the comparisons silently
// never match.
func TestExpandGenreAliases_IsAllLowercase(t *testing.T) {
	for _, g := range ExpandGenreAliases([]string{"Drame", "Sci-Fi", "Mecha"}) {
		if g != strings.ToLower(g) {
			t.Errorf("expansion returned non-lowercase %q; the SQL compares lower(eg)", g)
		}
	}
}

// An unknown genre must still filter on itself.
func TestExpandGenreAliases_KeepsUnknownRequest(t *testing.T) {
	if !slices.Contains(ExpandGenreAliases([]string{"Mecha"}), "mecha") {
		t.Error("an unknown genre must still filter on itself")
	}
}

// GenreIdentity is what the stats fold keys on: every spelling of one genre
// shares an identity, an unknown spelling is its own (case-insensitively), and
// it is never a label - callers pair it with CanonicalGenre for that.
func TestGenreIdentity_SharedAcrossSpellings(t *testing.T) {
	if GenreIdentity("Drama") != GenreIdentity("Drame") || GenreIdentity("Drama") != GenreIdentity(" drama ") {
		t.Error("the spellings of one genre must share an identity")
	}
	if GenreIdentity("Drama") == GenreIdentity("Comedy") {
		t.Error("two genres must not collide")
	}
	if GenreIdentity("Mecha") != "mecha" || GenreIdentity("MECHA") != "mecha" {
		t.Errorf("an unknown genre is its own lowercased identity, got %q / %q", GenreIdentity("Mecha"), GenreIdentity("MECHA"))
	}
}
