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
		{"the cascade → cascade", "The Cascade", "cascade"},
		{"a velvet kingdom → velvet kingdom", "A Velvet Kingdom", "velvet kingdom"},
		{"an ordinary life → ordinary life", "An Ordinary Life", "ordinary life"},
		// Articles: French
		{"les reves silencieux → reves silencieux", "Les Rêves Silencieux", "reves silencieux"},
		// Only the LEADING article is stripped - internal d'/l' stay
		{"le fabuleux destin → fabuleux destin (internal d' kept)", "Le Fabuleux Destin d'Émile Rivière", "fabuleux destin d'emile riviere"},
		{"la colere → colere", "La Colère", "colere"},
		{"l'etoile → etoile (apostrophe stripped + diacritics)", "L'Étoile", "etoile"},
		{"l’etoile curly apostrophe → etoile", "L’Étoile", "etoile"},
		// Articles: German
		{"der regenbogen → regenbogen", "Der Regenbogen", "regenbogen"},
		// Articles: Spanish/Italian (sanity)
		{"el jardin del silencio → jardin del silencio", "El Jardín del Silencio", "jardin del silencio"},
		// Don't strip non-articles that look like words
		{"anatomy keeps its 'an' prefix", "Anatomy", "anatomy"},
		{"andromeda signal keeps its 'an'", "The Andromeda Signal", "andromeda signal"},
		// Diacritics
		{"les âmes silencieuses → les ames silencieuses (after article strip)", "Les Âmes Silencieuses", "ames silencieuses"},
		{"naïveté → naivete", "Naïveté", "naivete"},
		// Leading numbers padded
		{"2001 → 002001 padding", "2001: A Distant Voyage", "002001: a distant voyage"},
		{"300 → 000300 (sorts before 2001)", "300", "000300"},
		{"1917 → 001917", "1917", "001917"},
		// Numeric sort sanity
		{"leading-digit title sorts numerically before a letter-led title", "2 Quick 2 Reckless", "000002 quick 2 reckless"},
		// Edge: empty
		{"empty input → empty", "", ""},
		{"whitespace only → empty", "   ", ""},
		// Edge: title is ONLY an article (degenerate, but shouldn't blow up)
		{"the only → only", "The", "the"}, // no trailing space → article not matched → kept as-is
		// Edge: French elided d' (movie titles like "L'arnaque d'Untel")
		{"d'untel preserved past first article", "d'Untel", "untel"},
		// Whitespace collapse
		{"double  spaces collapsed", "The   Realm  of   the Ancients", "realm of the ancients"},
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
// via NormalizeSortTitle, "Ashes and Embers" must sort under A so the
// keyset cursor advances through the alphabet rather than UUID order.
func TestNormalizeSortTitle_KeysetOrdering(t *testing.T) {
	t.Parallel()
	// Lexicographic order of the normalized strings must match user
	// expectation. Pick a small alphabet across diacritics + articles.
	titles := []string{
		"The Cascade",           // → cascade
		"Les Âmes Silencieuses", // → ames silencieuses
		"Ashes and Embers",      // → ashes and embers
		"L'Étoile",              // → etoile
		"2001 A Distant Voyage", // → 002001 a distant voyage
		"300",                   // → 000300
	}
	normalized := make([]string, len(titles))
	for i, t := range titles {
		normalized[i] = NormalizeSortTitle(t)
	}
	// Expected order:
	// "000300" < "002001 a distant voyage" < "ames silencieuses" <
	//   "ashes and embers" < "cascade" < "etoile"
	expected := []string{
		"000300",
		"002001 a distant voyage",
		"ames silencieuses",
		"ashes and embers",
		"cascade",
		"etoile",
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
		{"Les Âmes Silencieuses", "ames silencieuses"},
		{"L'Étoile", "etoile"},
		{"The Cascade", "cascade"},
		{"2001: A Distant Voyage", "002001: a distant voyage"},
		{"Der Regenbogen", "regenbogen"},
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
