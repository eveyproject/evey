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
		{"identique", "From Zero", "From Zero", true},
		{"casse", "from zero", "FROM ZERO", true},
		{"ponctuation", "crime of the century!", "Crime of the Century", true},
		{"accents", "Cœur de pirate", "coeur de pirate", false}, // œ n'est pas replié, cf accentFolds
		{"accents simples", "Éxité", "exite", true},
		{"suffixe d'édition à droite", "Crime Of The Century", "Crime Of The Century (Remastered 2025)", true},
		{"suffixe d'édition à gauche", "Crime Of The Century (Remastered 2025)", "Crime Of The Century", true},
		{"tiret séparateur", "From Zero - Deluxe", "From Zero", true},

		// Le cas qui compte : deux albums réellement différents du même artiste
		// ne doivent JAMAIS être rapprochés, sinon on ressert la pochette de
		// l'album vedette (prod 2026-07-25).
		{"albums différents", "From Zero", "Hybrid Theory", false},
		{"albums différents avec édition", "From Zero", "Hybrid Theory (Deluxe Edition)", false},
		{"préfixe sans frontière de mot", "Meteora", "Meteorafoo", false},
		{"vide à gauche", "", "From Zero", false},
		{"vide à droite", "From Zero", "", false},
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
		{"From Zero", "From Zero (Deluxe Edition)"},
		{"From Zero", "Hybrid Theory"},
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
		{"  From   Zero  ", "from zero"},
		{"Crime Of The Century (Remastered 2025)", "crime of the century remastered 2025"},
		{"AC/DC", "ac dc"},
		{"", ""},
		{"...", ""},
	} {
		if got := titlematch.Normalize(tc.in); got != tc.want {
			t.Errorf("Normalize(%q) = %q, attendu %q", tc.in, got, tc.want)
		}
	}
}
