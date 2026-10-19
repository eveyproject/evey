package library

import "strings"

// ParentalRule is a profile's parental restriction as a list query applies it
// (ListMediaItemsFilter.Parental). The rule itself is ratings.Blocks; a
// repository applies it in its own terms (postgres reads the columns
// ratings/precomputed.go derives) and must hide exactly the items it hides.
type ParentalRule struct {
	// MaxAge is the age cap, -1 for none.
	MaxAge int
	// Region is region.Resolve's answer for the profile, never empty: the
	// country whose table reads the official rating.
	Region      string
	BlockedTags []string
}

// NormalizedBlockedTags is BlockedTags as ratings.HasBlockedTag compares
// them: trimmed and lower-cased.
func (p ParentalRule) NormalizedBlockedTags() []string {
	out := make([]string, 0, len(p.BlockedTags))
	for _, t := range p.BlockedTags {
		out = append(out, strings.ToLower(strings.TrimSpace(t)))
	}
	return out
}
