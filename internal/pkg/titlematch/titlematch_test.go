package titlematch_test

import (
	"testing"

	"github.com/evey-project/evey/internal/pkg/titlematch"
)

func TestEquivalent(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b string
		want bool
	}{
		{"identique", "From Nowhere", "From Nowhere", true},
		{"casse", "from nowhere", "FROM NOWHERE", true},
		{"ponctuation", "song of the valley!", "Song of the Valley", true},
		{"accents", "Cœur de pirate", "coeur de pirate", false}, // œ n'est pas replié, cf accentFolds
		{"accents simples", "Éxité", "exite", true},
		{"suffixe d'édition à droite", "Song Of The Valley", "Song Of The Valley (Remastered 2025)", true},
		{"suffixe d'édition à gauche", "Song Of The Valley (Remastered 2025)", "Song Of The Valley", true},
		{"tiret séparateur", "From Nowhere - Deluxe", "From Nowhere", true},

		// Le cas qui compte : deux albums réellement différents du même artiste
		// ne doivent JAMAIS être rapprochés, sinon on ressert la pochette de
		// l'album vedette (prod 2026-07-25).
		{"albums différents", "From Nowhere", "Mixed Signals", false},
		{"albums différents avec édition", "From Nowhere", "Mixed Signals (Deluxe Edition)", false},
		{"préfixe sans frontière de mot", "Meteora", "Meteorafoo", false},
		{"vide à gauche", "", "From Nowhere", false},
		{"vide à droite", "From Nowhere", "", false},
		{"ponctuation seule", "!!!", "???", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := titlematch.Equivalent(tc.a, tc.b); got != tc.want {
				t.Errorf("Equivalent(%q, %q) = %v, attendu %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestEquivalentIsSymmetric(t *testing.T) {
	pairs := [][2]string{
		{"From Nowhere", "From Nowhere (Deluxe Edition)"},
		{"From Nowhere", "Mixed Signals"},
		{"Meteora", "Meteora"},
	}
	for _, p := range pairs {
		if titlematch.Equivalent(p[0], p[1]) != titlematch.Equivalent(p[1], p[0]) {
			t.Errorf("asymétrie sur (%q, %q)", p[0], p[1])
		}
	}
}

func TestNormalize(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"  From   Nowhere  ", "from nowhere"},
		{"Song Of The Valley (Remastered 2025)", "song of the valley remastered 2025"},
		{"AC/DC", "ac dc"},
		{"", ""},
		{"...", ""},
	} {
		if got := titlematch.Normalize(tc.in); got != tc.want {
			t.Errorf("Normalize(%q) = %q, attendu %q", tc.in, got, tc.want)
		}
	}
}
