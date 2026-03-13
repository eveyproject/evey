package ratings

import "strings"

// crossMap maps TV ratings to equivalent MPAA severity for cross-system comparison
var crossMap = map[string]int{
	"G": 0, "TV-Y": 0, "TV-Y7": 0, "TV-G": 0,
	"PG": 1, "TV-PG": 1,
	"PG-13": 2, "TV-14": 2,
	"R": 3, "TV-MA": 3,
	"NC-17": 4,
}

func normalize(r string) string {
	return strings.TrimSpace(strings.ToUpper(r))
}

// IsAllowed returns true if content with contentRating is allowed under maxRating.
// Unknown ratings are allowed (permissive by default).
func IsAllowed(contentRating, maxRating string) bool {
	if maxRating == "" || contentRating == "" {
		return true
	}
	content := normalize(contentRating)
	max := normalize(maxRating)

	contentLevel, cOK := crossMap[content]
	maxLevel, mOK := crossMap[max]
	if !cOK || !mOK {
		return true
	}
	return contentLevel <= maxLevel
}

// AllowedRatingsFor returns all known ratings with severity <= maxRating.
// Returns nil if maxRating is empty or unknown (no filter should be applied).
func AllowedRatingsFor(maxRating string) []string {
	if maxRating == "" {
		return nil
	}
	max := normalize(maxRating)
	maxLevel, ok := crossMap[max]
	if !ok {
		return nil
	}
	var allowed []string
	for r, level := range crossMap {
		if level <= maxLevel {
			allowed = append(allowed, r)
		}
	}
	return allowed
}

// HasBlockedTag returns true if any of the item's tags match blocked tags (case-insensitive).
func HasBlockedTag(itemTags, blockedTags []string) bool {
	if len(blockedTags) == 0 {
		return false
	}
	blocked := make(map[string]bool, len(blockedTags))
	for _, t := range blockedTags {
		blocked[strings.ToLower(strings.TrimSpace(t))] = true
	}
	for _, t := range itemTags {
		if blocked[strings.ToLower(strings.TrimSpace(t))] {
			return true
		}
	}
	return false
}
