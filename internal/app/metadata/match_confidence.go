package metadata

import (
	"strings"

	domainmetadata "github.com/eveyproject/evey/internal/domain/metadata"
)

// Guard against a provider result that names a different work than the file.
//
// Why it exists: prod 2026-06-03, a film whose filename misspelled its title
// was stored as an unrelated "The ..." title. TMDB returns nothing for the
// misspelled query, the cascade retry in searchAndFetchBest trims trailing
// words down to "The", and every popular "The ..." candidate shares the
// stopword "the" with the query - so the coherence filter in pickBestMatch
// accepted the top result. Nothing compared the two titles before persisting.
//
// The thresholds below are calibrated on the owner's production catalogue
// (dumped 2026-08-23). Replayed on the 1329 movies carrying a TMDB id they
// reject 5, all confirmed wrong matches, and on the 207 matched series they
// reject none. Every legitimate pair is kept - French display titles over an
// English filename, filename typos, numbering and director's-cut variants.

// titleCoverageThreshold is the share of the query's content words that must
// be found in the candidate title. Bounded on both sides by production data:
// the tightest legitimate pair (a "chapter two ... directors cut" filename
// against the provider's "Part Two" title) is at 3/8 = 0.375, and the loosest
// wrong match shares one word of three, 1/3 = 0.333. 0.35 is the midpoint.
const titleCoverageThreshold = 0.35

// titleCompactSimilarity is the whole-string tolerance, applied after
// stripping separators. Catches titles the tokenizer splits differently
// ("Fathermachine" vs "Father/Machine") and single-word typos.
const titleCompactSimilarity = 0.85

// titleWordSimilarity is the per-word tolerance ("Silense" vs "Silence").
// Only applied from titleWordFuzzyMinLen runes up, so short words still have
// to match exactly and "Red 2" never absorbs "Red 3".
const (
	titleWordSimilarity  = 0.75
	titleWordFuzzyMinLen = 5
)

// titleStopwords are the words that carry no identity. A shared "the" or "de"
// is precisely what let the 2026-06-03 mismatch through.
var titleStopwords = map[string]bool{
	"the": true, "a": true, "an": true, "of": true, "and": true, "or": true,
	"in": true, "on": true, "to": true, "for": true, "with": true,
	"le": true, "la": true, "les": true, "l": true, "un": true, "une": true,
	"de": true, "des": true, "du": true, "d": true, "et": true,
	"en": true, "au": true, "aux": true, "sur": true, "il": true,
}

// titlePlausible reports whether any candidate title plausibly names the same
// work as query. Candidates are the provider's Title, OriginalTitle and any
// alternative titles - a legitimate localized title is almost always covered
// by one of them.
func titlePlausible(query string, candidates []string) bool {
	q := normalizeForMatch(query)
	if q == "" {
		return false
	}
	qWords := contentWords(q)
	for _, c := range candidates {
		n := normalizeForMatch(c)
		if n == "" {
			continue
		}
		if wordBoundaryContains(q, n) {
			return true
		}
		if compactRatio(q, n) >= titleCompactSimilarity {
			return true
		}
		if coverage(qWords, contentWords(n)) >= titleCoverageThreshold {
			return true
		}
	}
	return false
}

// candidateTitles lists every label a result may legitimately be known by.
func candidateTitles(r domainmetadata.MetadataResult) []string {
	out := make([]string, 0, len(r.AlternativeTitles)+2)
	out = append(out, r.Title, r.OriginalTitle)
	return append(out, r.AlternativeTitles...)
}

// contentWords splits a normalized label into identity-bearing words. A label
// made only of stopwords keeps them, otherwise it would compare as empty.
func contentWords(norm string) []string {
	all := strings.Fields(norm)
	kept := make([]string, 0, len(all))
	for _, w := range all {
		if !titleStopwords[w] {
			kept = append(kept, w)
		}
	}
	if len(kept) == 0 {
		return all
	}
	return kept
}

// wordBoundaryContains accepts the common "filename carries a marketing
// subtitle" case ("Night Train Presents Archipelago" vs "Archipelago"). The
// contained side must carry an identity word, otherwise a query reduced to
// "La" would match any title starting with "La".
func wordBoundaryContains(a, b string) bool {
	short, long := a, b
	if len(short) > len(long) {
		short, long = long, short
	}
	if !hasIdentityWord(short) {
		return false
	}
	return strings.Contains(" "+long+" ", " "+short+" ")
}

// hasIdentityWord ignores stopwords and bare numbers: a candidate reduced to
// "3001" (what normalization leaves of a CJK title ending in 3001) is
// contained in "3001 Le voyage des étoiles" without naming the same film.
func hasIdentityWord(norm string) bool {
	for w := range strings.FieldsSeq(norm) {
		if !titleStopwords[w] && strings.TrimLeft(w, "0123456789") != "" {
			return true
		}
	}
	return false
}

// coverage is the share of query words matched by a distinct candidate word.
func coverage(query, candidate []string) float64 {
	if len(query) == 0 || len(candidate) == 0 {
		return 0
	}
	used := make([]bool, len(candidate))
	hits := 0
	for _, q := range query {
		for i, c := range candidate {
			if used[i] || !wordsEquivalent(q, c) {
				continue
			}
			used[i] = true
			hits++
			break
		}
	}
	return float64(hits) / float64(len(query))
}

func wordsEquivalent(a, b string) bool {
	if a == b {
		return true
	}
	ra, rb := []rune(a), []rune(b)
	longest := max(len(ra), len(rb))
	if longest < titleWordFuzzyMinLen {
		return false
	}
	return 1-float64(levenshtein(ra, rb))/float64(longest) >= titleWordSimilarity
}

// compactRatio compares both labels with every separator removed.
func compactRatio(a, b string) float64 {
	ra := []rune(strings.ReplaceAll(a, " ", ""))
	rb := []rune(strings.ReplaceAll(b, " ", ""))
	longest := max(len(ra), len(rb))
	if longest == 0 {
		return 0
	}
	return 1 - float64(levenshtein(ra, rb))/float64(longest)
}

func levenshtein(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, min(cur[j-1]+1, prev[j-1]+cost))
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
