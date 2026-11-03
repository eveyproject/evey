package metadata

import (
	"strings"
	"unicode"
)

// SanitizeText is what a provider's text must pass before it reaches the
// catalogue. A JSON body may legally carry U+0000 (Postgres then refuses the
// whole row, so the enrichment fails on every pass), C0/C1 controls, the bidi
// overrides that make a title read one way and sort or search another, and
// the zero-width space and byte-order mark that only hide in a string. They
// are dropped; line and tab breaks in an overview stay, invalid UTF-8 goes,
// and the ends are trimmed. The joiners (U+200C, U+200D: Persian words,
// Indic conjuncts, emoji sequences), the LRM/RLM marks and the bidi isolates
// (U+2066-2069) are text a title is written with and are kept.
func SanitizeText(s string) string {
	if s == "" {
		return s
	}
	clean := strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case unicode.Is(unicode.Cc, r):
			return -1
		case r == 0x200B, r == 0xFEFF, r >= 0x202A && r <= 0x202E:
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, ""))
	return strings.TrimSpace(clean)
}

func sanitizeAll(ss []string) {
	for i := range ss {
		ss[i] = SanitizeText(ss[i])
	}
}

// Sanitize applies SanitizeText to every free-text field of the result,
// people and collection included. Identifiers, URLs and numbers are not text
// a person wrote and are left as they are.
func (r *MetadataResult) Sanitize() {
	if r == nil {
		return
	}
	r.Title = SanitizeText(r.Title)
	r.OriginalTitle = SanitizeText(r.OriginalTitle)
	r.Overview = SanitizeText(r.Overview)
	r.Tagline = SanitizeText(r.Tagline)
	r.OfficialRating = SanitizeText(r.OfficialRating)
	sanitizeAll(r.Genres)
	sanitizeAll(r.Studios)
	sanitizeAll(r.AlternativeTitles)
	for i := range r.People {
		r.People[i].Name = SanitizeText(r.People[i].Name)
		r.People[i].Role = SanitizeText(r.People[i].Role)
		r.People[i].Character = SanitizeText(r.People[i].Character)
	}
	if r.Collection != nil {
		r.Collection.Name = SanitizeText(r.Collection.Name)
	}
}
