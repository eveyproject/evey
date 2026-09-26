// Package titlematch compare des noms d'artiste et de titres d'album venant de
// sources différentes (tags des fichiers, iTunes, MusicBrainz).
//
// Existe pour une raison précise : un fetcher d'artwork qui ne vérifie pas que
// le résultat correspond à ce qu'il a demandé sert la pochette d'un autre album.
// C'est arrivé en prod (2026-07-25) - la recherche plein-texte d'iTunes retombe
// sur l'album le plus populaire de l'artiste quand le titre demandé n'est pas
// indexé, et deux albums Neon Harbor se sont retrouvés avec la pochette de
// Mixed Signals. La comparaison doit donc être stricte sur le fond et tolérante
// sur la forme.
package titlematch

import (
	"strings"
	"unicode"
)

// Equivalent dit si deux libellés désignent la même chose. Accepte l'égalité
// après normalisation, ou l'inclusion d'un titre dans l'autre sur une frontière
// de mot - de quoi absorber les suffixes d'édition présents d'un seul côté
// ("Song Of The Valley" vs "Song Of The Valley (Remastered 2025)"), sans
// jamais rapprocher deux titres réellement différents ("From Nowhere" et "Hybrid
// Theory" ne se contiennent pas).
func Equivalent(a, b string) bool {
	na, nb := Normalize(a), Normalize(b)
	if na == "" || nb == "" {
		return false
	}
	if na == nb {
		return true
	}
	return strings.HasPrefix(na, nb+" ") || strings.HasPrefix(nb, na+" ")
}

// Normalize réduit un libellé à une forme comparable : minuscules, accents
// repliés, ponctuation transformée en séparateur de mot, espaces normalisés.
func Normalize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := true
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(foldAccent(r))
			prevSpace = false
		default:
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// Table explicite plutôt qu'un calcul d'index : les caractères accentués font
// plusieurs octets en UTF-8, donc parcourir en parallèle une chaîne "avec
// accents" et une chaîne "sans accents" est faux dès le premier caractère.
var accentFolds = map[rune]rune{
	'à': 'a', 'á': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a', 'å': 'a', 'æ': 'a',
	'ç': 'c',
	'è': 'e', 'é': 'e', 'ê': 'e', 'ë': 'e',
	'ì': 'i', 'í': 'i', 'î': 'i', 'ï': 'i',
	'ñ': 'n',
	'ò': 'o', 'ó': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o', 'ø': 'o',
	'ù': 'u', 'ú': 'u', 'û': 'u', 'ü': 'u',
	'ý': 'y', 'ÿ': 'y',
}

func foldAccent(r rune) rune {
	if plain, ok := accentFolds[r]; ok {
		return plain
	}
	return r
}
