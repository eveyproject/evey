// Package ratings maps country-specific content classifications (CSA, FSK, MPAA,
// BBFC, ACB, …) onto a single numeric "minimum recommended age" axis so the
// parental filter can reason across systems without a giant cross-string table.
package ratings

import (
	"strings"
)

// agencyPrefixes maps a classification-agency code (sometimes glued to the
// rating string in TMDB output, e.g. "ACB MA 15+", "BBFC 15", "MPAA R") to
// the ISO country code its table lives under. We strip the prefix before
// lookup, and fall back to the agency's country when the caller didn't pass
// one (TMDB occasionally emits agency-prefixed strings as the OfficialRating
// without an accompanying country tag).
var agencyPrefixes = map[string]string{
	"ACB":   "AU",
	"OFLC":  "AU",
	"BBFC":  "GB",
	"MPAA":  "US",
	"MPA":   "US",
	"FSK":   "DE",
	"USK":   "DE",
	"CNC":   "FR",
	"CSA":   "FR",
	"DJCTQ": "BR",
	"ICAA":  "ES",
	"CHVRS": "CA",
	"SBP":   "PT",
	"EIRIN": "JP",
}

// stripAgencyPrefix peels a "<AGENCY> " or "<AGENCY>-" prefix off a rating
// string. Returns the cleaned rating plus the implied country (empty when
// no prefix matched).
func stripAgencyPrefix(rating string) (string, string) {
	for prefix, country := range agencyPrefixes {
		// Need the boundary char to avoid matching "ACBA…" etc.
		if strings.HasPrefix(rating, prefix+" ") || strings.HasPrefix(rating, prefix+"-") {
			stripped := strings.TrimSpace(rating[len(prefix):])
			stripped = strings.TrimPrefix(stripped, "-")
			stripped = strings.TrimSpace(stripped)
			if stripped != "" {
				return stripped, country
			}
		}
	}
	return rating, ""
}

// MinAgeFor returns the minimum recommended age for a rating in a given country,
// or -1 if the (country, rating) pair is unknown (caller decides the policy -
// typically "allow" to avoid blocking legitimate content due to missing data).
//
// Country is an ISO 3166-1 alpha-2 code (e.g. "FR", "US", "DE").
// Rating is the raw string TMDB returns for that country (e.g. "-12", "PG-13",
// "ACB MA 15+"). Agency prefixes ("ACB", "BBFC", "MPAA", "FSK", "CNC", "CSA",
// "DJCTQ", "ICAA", …) are stripped before lookup so callers don't have to
// pre-normalize, and an empty country falls back to the agency's home country.
func MinAgeFor(country, rating string) int {
	c := normalize(country)
	r := normalize(rating)
	if r == "" {
		return -1
	}
	if cleaned, agencyCountry := stripAgencyPrefix(r); cleaned != r {
		r = cleaned
		if c == "" {
			c = agencyCountry
		}
	}
	return minAgeForKey(c, r)
}

// minAgeForKey is MinAgeFor's table lookup on an already normalized country
// and rating key: the country's table, then the US one, else -1.
func minAgeForKey(c, r string) int {
	if c == "" || r == "" {
		return -1
	}
	if table, ok := byCountry[c]; ok {
		if age, ok := table[r]; ok {
			return age
		}
	}
	// Fallback: try US MPAA if the country-specific lookup missed (TMDB
	// sometimes mirrors US rating into other locales' officialRating field).
	if c != "US" {
		if age, ok := byCountry["US"][r]; ok {
			return age
		}
	}
	return -1
}

// WorstCaseMinAge returns the highest minimum recommended age across the
// profile-region certification, the OfficialRating field, and every other
// country certification we can map. Returns -1 when none of them resolve to a
// known min-age. Callers enforcing a parental filter for kids should treat -1
// as "block" (fail closed): a title that exposes only, say, an "AU: MA 15+"
// rating must not slip past a profile region'd to FR just because there is no
// FR cert to match (the parental filter treats the -1 as block).
func WorstCaseMinAge(region, officialRating string, certByCountry map[string]string) int {
	worst := -1
	if region != "" && certByCountry != nil {
		if r, ok := certByCountry[region]; ok && r != "" {
			if age := MinAgeFor(region, r); age > worst {
				worst = age
			}
		}
	}
	if officialRating != "" {
		if age := MinAgeFor(region, officialRating); age > worst {
			worst = age
		}
	}
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

// AllowedRatingsForAge returns the union of all rating strings (across countries)
// whose minimum age is <= maxAge. Callers use this for SQL-level pre-filtering;
// the precise per-country check runs later via WorstCaseMinAge in the parental filter.
// Returns nil when maxAge < 0 (no filter).
func AllowedRatingsForAge(maxAge int) []string {
	if maxAge < 0 {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, table := range byCountry {
		for rating, age := range table {
			if age >= 0 && age <= maxAge && !seen[rating] {
				seen[rating] = true
				out = append(out, rating)
			}
		}
	}
	return out
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

func normalize(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// byCountry maps an ISO-2 country to its rating table. Values are the minimum
// recommended age for the rating. Sources: official classification boards; some
// systems overlap (FSK/USK in DE, OFLC/ACB in AU) - we collapse onto one table
// per country for the most common TMDB values.
var byCountry = map[string]map[string]int{
	// France - CSA (ARCOM) for TV + CNC visas for cinema. TMDB returns both.
	"FR": {
		"TP": 0, "TOUS PUBLICS": 0, "U": 0,
		"-10": 10, "10": 10,
		"-12": 12, "12": 12,
		"-16": 16, "16": 16,
		"-18": 18, "18": 18, "X": 18,
	},
	// USA - MPAA (films) + TV Parental Guidelines.
	"US": {
		"G": 0, "TV-Y": 0, "TV-G": 0, "TV-Y7": 7, "TV-Y7-FV": 7,
		"PG": 10, "TV-PG": 10,
		"PG-13": 13, "TV-14": 14,
		"R": 17, "TV-MA": 17,
		"NC-17": 17, "NR": -1, "UR": -1,
	},
	// UK - BBFC.
	"GB": {
		"U": 0, "UC": 0, "PG": 8, "12": 12, "12A": 12, "15": 15, "18": 18, "R18": 18,
	},
	// Ireland - IFCO (close to BBFC).
	"IE": {
		"G": 0, "PG": 8, "12": 12, "12A": 12, "15": 15, "15A": 15, "16": 16, "18": 18,
	},
	// Germany - FSK (films + TV).
	"DE": {
		"0": 0, "6": 6, "12": 12, "16": 16, "18": 18,
	},
	// Austria / Switzerland often use FSK too.
	"AT": {"0": 0, "6": 6, "12": 12, "16": 16, "18": 18},
	"CH": {"0": 0, "6": 6, "12": 12, "14": 14, "16": 16, "18": 18},
	// Spain - ICAA.
	"ES": {
		"APTA": 0, "TP": 0, "A": 0,
		"7": 7, "12": 12, "13": 13, "16": 16, "18": 18, "X": 18,
	},
	// Italy - CNC (no age 0-12, 12-14, 14-18 bands).
	"IT": {
		"T": 0, "V.M. 6": 6, "V.M.6": 6, "6+": 6,
		"12": 12, "V.M. 12": 12, "V.M.12": 12,
		"14": 14, "14+": 14,
		"V.M. 18": 18, "V.M.18": 18, "18": 18,
	},
	// Netherlands - Kijkwijzer.
	"NL": {
		"AL": 0, "6": 6, "9": 9, "12": 12, "14": 14, "16": 16, "18": 18,
	},
	// Belgium - NICAM-compatible (uses same labels as NL + national film board).
	"BE": {"AL": 0, "KT": 0, "KNT": 0, "6": 6, "9": 9, "12": 12, "14": 14, "16": 16, "18": 18},
	// Australia - ACB.
	"AU": {
		"G": 0, "PG": 8, "M": 15, "MA 15+": 15, "MA15+": 15, "R 18+": 18, "R18+": 18, "X 18+": 18, "X18+": 18,
	},
	// New Zealand - OFLC.
	"NZ": {
		"G": 0, "PG": 8, "M": 13, "R13": 13, "R15": 15, "R16": 16, "R18": 18, "R": 18,
	},
	// Canada - provincial boards converge on similar labels.
	"CA": {
		"G": 0, "PG": 10, "14A": 14, "14+": 14, "16+": 16, "18A": 18, "18+": 18, "R": 18,
	},
	// Japan - EIRIN.
	"JP": {
		"G": 0, "PG12": 12, "R15+": 15, "R18+": 18,
	},
	// South Korea - KMRB.
	"KR": {"ALL": 0, "12": 12, "15": 15, "18": 18, "19": 18},
	// India - CBFC.
	"IN": {
		"U": 0, "U/A": 12, "U/A 7+": 7, "U/A 13+": 13, "U/A 16+": 16, "A": 18, "S": -1,
	},
	// Brazil - ClassInd / DJCTQ.
	"BR": {
		"L": 0, "LIVRE": 0, "10": 10, "12": 12, "14": 14, "16": 16, "18": 18,
	},
	// Denmark - Medierådet.
	"DK": {"A": 0, "7": 7, "11": 11, "15": 15},
	// Finland - KAVI.
	"FI": {"S": 0, "7": 7, "12": 12, "16": 16, "18": 18, "K-7": 7, "K-12": 12, "K-16": 16, "K-18": 18},
	// Sweden - SMRC.
	"SE": {"BTL": 0, "7": 7, "11": 11, "15": 15},
	// Norway - Medietilsynet.
	"NO": {"A": 0, "6": 6, "9": 9, "12": 12, "15": 15, "18": 18},
	// Poland - films generally labeled by age.
	"PL": {"AL": 0, "7": 7, "12": 12, "15": 15, "16": 16, "18": 18},
	// Portugal - CCE.
	"PT": {"M/3": 0, "M/6": 6, "M/12": 12, "M/14": 14, "M/16": 16, "M/18": 18},
	// Russia - age-labelled directly.
	"RU": {"0+": 0, "6+": 6, "12+": 12, "14+": 14, "16+": 16, "18+": 18},
	// Mexico - RTC.
	"MX": {"AA": 0, "A": 0, "B": 12, "B-15": 15, "B15": 15, "C": 18, "D": 18},
}
