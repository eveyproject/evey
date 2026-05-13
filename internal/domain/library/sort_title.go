package library

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// sortTitleArticles lists leading words stripped from the sort key. Order
// matters only for "l'" — it must be checked before "la " to avoid the
// space-anchored prefix scanning past it. Lowercased + already-trimmed
// before comparison, so "Les " in the title still matches "les ".
//
// French, English, German, Spanish, Italian — the languages currently
// supported by the i18n layer. Adding more is harmless as long as each
// entry ends with a space (or "'" for elided articles) so we never
// truncate a word like "an" out of "anatomy".
var sortTitleArticles = []string{
	"l'", "l’", // French elided "l'" (straight + curly apostrophe)
	"d'", "d’", // French "d'" (le film d'Untel)
	"les ", "le ", "la ", "un ", "une ", "des ",
	"the ", "a ", "an ",
	"der ", "die ", "das ", "ein ", "eine ",
	"el ", "los ",
	"il ", "lo ", "gli ",
}

// leadingNumberRe captures a run of digits at the start of the trimmed
// title so we can zero-pad it. Without padding, "2 Fast 2 Furious" sorts
// AFTER "20th Century Boys" because '2' < '2' is undefined → next char
// ' ' < '0' → "20…" wins. Padding to 6 digits is enough for any year or
// 5-figure release number.
var leadingNumberRe = regexp.MustCompile(`^(\d{1,6})`)

// whitespaceRe collapses any run of whitespace into a single space so the
// stripped result doesn't gain accidental double spaces from punctuation
// removal upstream.
var whitespaceRe = regexp.MustCompile(`\s+`)

// stripDiacritics removes combining marks via NFD decomposition, so
// "L'Odyssée" → "L'Odyssee". Unicode-aware; doesn't touch base letters.
var stripDiacritics = transform.Chain(
	norm.NFD,
	runes.Remove(runes.In(unicode.Mn)),
	norm.NFC,
)

// NormalizeSortTitle derives the sortable key from a display title. Aim:
// alphabetical browsing matches user expectations — "The Matrix" lands
// under M, "Les âmes vagabondes" under A, "2001" before "2010".
//
// Pipeline:
//
//  1. Trim + collapse whitespace
//  2. Strip Unicode combining marks (diacritics)
//  3. Lowercase
//  4. Strip a single leading article (les/le/la, the/a/an, der/die/das, …)
//  5. Zero-pad a leading number run so lexicographic sort matches numeric
//
// Returns an empty string for empty input so the SQL column can stay
// NOT NULL DEFAULT ''.
func NormalizeSortTitle(title string) string {
	t := strings.TrimSpace(title)
	if t == "" {
		return ""
	}
	if normalized, _, err := transform.String(stripDiacritics, t); err == nil {
		t = normalized
	}
	t = strings.ToLower(t)
	t = whitespaceRe.ReplaceAllString(t, " ")
	for _, art := range sortTitleArticles {
		if strings.HasPrefix(t, art) {
			t = strings.TrimSpace(t[len(art):])
			break
		}
	}
	if m := leadingNumberRe.FindStringSubmatch(t); m != nil {
		padded := strings.Repeat("0", 6-len(m[1])) + m[1]
		t = padded + t[len(m[1]):]
	}
	return t
}
