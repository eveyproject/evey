package ratings

import (
	"hash/fnv"
	"maps"
	"slices"
	"strconv"
)

// precomputed.go splits WorstCaseMinAge into the part a row can store and the
// part only the viewer's region can settle, so a database can apply the
// parental age cap without loading every item into Go.
//
// For a non-empty region:
//
//	WorstCaseMinAge(region, official, certs)
//	  == max(CertificationsMinAge(certs), minAgeForKey(region, RatingKey(official)))
//
// The region's own certification is one of the certs, so the first term does
// not depend on the region. The second is a lookup of a stored key in a table
// the region picks, which RatingKeysForRegion turns into two finite lists.
// TestPrecomputedMatchesWorstCaseMinAge pins the identity.

// CertificationsMinAge is the highest minimum age across every country
// certification that maps to one, or -1 when none does.
func CertificationsMinAge(certByCountry map[string]string) int {
	worst := -1
	for country, rating := range certByCountry {
		if rating == "" {
			continue
		}
		if age := MinAgeFor(country, rating); age > worst {
			worst = age
		}
	}
	return worst
}

// RatingKey is the official rating as the tables are keyed: upper-cased,
// trimmed, agency prefix removed. Empty for an empty rating.
func RatingKey(officialRating string) string {
	r := normalize(officialRating)
	if cleaned, _ := stripAgencyPrefix(r); cleaned != r {
		return cleaned
	}
	return r
}

// RatingKeysForRegion lists, for a viewer in region capped at maxAge, the
// rating keys whose age is known and within the cap (allowed) and those whose
// age is known and above it (blocked). Any other key has no known age there.
// Both lists are sorted. region must be non-empty, as region.Resolve returns.
func RatingKeysForRegion(region string, maxAge int) (allowed, blocked []string) {
	c := normalize(region)
	seen := make(map[string]bool)
	for _, table := range []map[string]int{byCountry[c], byCountry["US"]} {
		for key := range table {
			if seen[key] {
				continue
			}
			seen[key] = true
			switch age := minAgeForKey(c, key); {
			case age < 0:
			case age <= maxAge:
				allowed = append(allowed, key)
			default:
				blocked = append(blocked, key)
			}
		}
	}
	slices.Sort(allowed)
	slices.Sort(blocked)
	return allowed, blocked
}

// TablesRevision fingerprints the rating tables and agency prefixes. A stored
// CertificationsMinAge or RatingKey is only valid under the revision it was
// computed with: editing a table changes this value, and the rows are
// recomputed.
var TablesRevision = tablesRevision()

func tablesRevision() int64 {
	h := fnv.New64a()
	write := func(s string) { _, _ = h.Write([]byte(s)); _, _ = h.Write([]byte{0}) }
	for _, c := range slices.Sorted(maps.Keys(byCountry)) {
		write(c)
		for _, r := range slices.Sorted(maps.Keys(byCountry[c])) {
			write(r)
			write(strconv.Itoa(byCountry[c][r]))
		}
	}
	for _, p := range slices.Sorted(maps.Keys(agencyPrefixes)) {
		write(p)
		write(agencyPrefixes[p])
	}
	return int64(h.Sum64() >> 1)
}

// Rated is what the parental rule reads off one title: its official rating,
// its country certifications and its tags.
type Rated struct {
	OfficialRating string
	Certifications map[string]string
	Tags           []string
}

// Blocks is the parental rule for a profile capped at maxAge (-1 for none) in
// region and blocking blockedTags. series is what a season or an episode
// inherits from its series, nil for any other item: the age is the worst case
// across the item and its series, and a tag on either blocks both. Each age is
// the worst case across the region's reading of the official rating and every
// country certification, and an age no table knows is blocked as soon as a cap
// is set: a title rated only "AU: MA 15+" must not reach a kids profile in FR
// for want of a French rating, and an episode, which carries no rating of its
// own, is as old as its series. A database applying the rule must hide exactly
// what this hides (see the identity at the top of this file).
func Blocks(maxAge int, region string, blockedTags []string, item Rated, series *Rated) bool {
	if maxAge >= 0 {
		worst := WorstCaseMinAge(region, item.OfficialRating, item.Certifications)
		if series != nil {
			worst = max(worst, WorstCaseMinAge(region, series.OfficialRating, series.Certifications))
		}
		if worst < 0 || worst > maxAge {
			return true
		}
	}
	if series != nil && HasBlockedTag(series.Tags, blockedTags) {
		return true
	}
	return HasBlockedTag(item.Tags, blockedTags)
}
