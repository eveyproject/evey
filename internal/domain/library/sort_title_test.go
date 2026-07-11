package library

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeSortTitle(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
		want  string
	}{
		// Articles: English
		{"the matrix → matrix", "The Matrix", "matrix"},
		{"a clockwork orange → clockwork orange", "A Clockwork Orange", "clockwork orange"},
		{"an inconvenient truth → inconvenient truth", "An Inconvenient Truth", "inconvenient truth"},
		// Articles: French
		{"les amours imaginaires → amours imaginaires", "Les Amours Imaginaires", "amours imaginaires"},
		// Only the LEADING article is stripped — internal d'/l' stay
		{"le fabuleux destin → fabuleux destin (internal d' kept)", "Le Fabuleux Destin d'Amélie Poulain", "fabuleux destin d'amelie poulain"},
		{"la haine → haine", "La Haine", "haine"},
		{"l'odyssée → odyssee (apostrophe stripped + diacritics)", "L'Odyssée", "odyssee"},
		{"l’odyssée curly apostrophe → odyssee", "L’Odyssée", "odyssee"},
		// Articles: German
		{"der untergang → untergang", "Der Untergang", "untergang"},
		// Articles: Spanish/Italian (sanity)
		{"el laberinto del fauno → laberinto del fauno", "El Laberinto del Fauno", "laberinto del fauno"},
		// Don't strip non-articles that look like words
		{"anatomy keeps its 'an' prefix", "Anatomy", "anatomy"},
		{"andromeda strain keeps its 'an'", "The Andromeda Strain", "andromeda strain"},
		// Diacritics
		{"les âmes vagabondes → les ames vagabondes (after article strip)", "Les Âmes Vagabondes", "ames vagabondes"},
		{"naïveté → naivete", "Naïveté", "naivete"},
		// Leading numbers padded
		{"2001 → 002001 padding", "2001: A Space Odyssey", "002001: a space odyssey"},
		{"300 → 000300 (sorts before 2001)", "300", "000300"},
		{"1917 → 001917", "1917", "001917"},
		// Numeric sort sanity
		{"2 Fast 2 Furious < 20th Century Fox", "2 Fast 2 Furious", "000002 fast 2 furious"},
		// Edge: empty
		{"empty input → empty", "", ""},
		{"whitespace only → empty", "   ", ""},
		// Edge: title is ONLY an article (degenerate, but shouldn't blow up)
		{"the only → only", "The", "the"}, // no trailing space → article not matched → kept as-is
		// Edge: French elided d' (movie titles like "L'arnaque d'Untel")
		{"d'untel preserved past first article", "d'Untel", "untel"},
		// Whitespace collapse
		{"double  spaces collapsed", "The   Lord  of   the Rings", "lord of the rings"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NormalizeSortTitle(c.input)
			assert.Equal(t, c.want, got)
		})
	}
}

// Concrete regression: pagination on prod broke because sort_title was
// empty for items inserted before this function existed. Once backfilled
// via NormalizeSortTitle, "Angels and Demons" must sort under A so the
// keyset cursor advances through the alphabet rather than UUID order.
func TestNormalizeSortTitle_KeysetOrdering(t *testing.T) {
	t.Parallel()
	// Lexicographic order of the normalized strings must match user
	// expectation. Pick a small alphabet across diacritics + articles.
	titles := []string{
		"The Matrix",           // → matrix
		"Les Âmes Vagabondes",  // → ames vagabondes
		"Angels and Demons",    // → angels and demons
		"L'Odyssée",            // → odyssee
		"2001 A Space Odyssey", // → 002001 a space odyssey
		"300",                  // → 000300
	}
	normalized := make([]string, len(titles))
	for i, t := range titles {
		normalized[i] = NormalizeSortTitle(t)
	}
	// Expected order:
	// "000300" < "002001 a space odyssey" < "ames vagabondes" <
	//   "angels and demons" < "matrix" < "odyssee"
	expected := []string{
		"000300",
		"002001 a space odyssey",
		"ames vagabondes",
		"angels and demons",
		"matrix",
		"odyssee",
	}
	sorted := append([]string(nil), normalized...)
	for i := range sorted {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] < sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	assert.Equal(t, expected, sorted)
}

// Concurrent callers must produce identical, correct results. Regression
// guard for the data race that landed in CI when `transform.Chain` was
// stored at package scope (NFD/NFC chain holds mutable buffers). Run
// with `-race` to fail loudly if the stripDiacritics chain ever leaks
// back to a shared instance.
func TestNormalizeSortTitle_ConcurrentSafe(t *testing.T) {
	t.Parallel()

	type pair struct{ in, want string }
	cases := []pair{
		{"Les Âmes Vagabondes", "ames vagabondes"},
		{"L'Odyssée", "odyssee"},
		{"The Matrix", "matrix"},
		{"2001: A Space Odyssey", "002001: a space odyssey"},
		{"Der Untergang", "untergang"},
		{"Naïveté", "naivete"},
	}

	const workers = 32
	const iters = 200
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for i := range iters {
				c := cases[i%len(cases)]
				if got := NormalizeSortTitle(c.in); got != c.want {
					t.Errorf("NormalizeSortTitle(%q) = %q, want %q", c.in, got, c.want)
					return
				}
			}
		}()
	}
	wg.Wait()
}
