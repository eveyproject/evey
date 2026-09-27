package ratings

import "testing"

func TestMinAgeFor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		country, rating string
		want            int
	}{
		// France CSA
		{"FR", "-12", 12},
		{"FR", "12", 12},
		{"FR", "-16", 16},
		{"FR", "TP", 0},
		{"fr", "-10", 10}, // case-insensitive country
		// USA MPAA
		{"US", "G", 0},
		{"US", "PG", 10},
		{"US", "PG-13", 13},
		{"US", "R", 17},
		{"US", "NC-17", 17},
		{"US", "TV-MA", 17},
		{"US", "TV-Y7", 7},
		// Not rated / unrated map to -1: the parental filter reads it as "block" for a kids profile.
		{"US", "NR", -1},
		{"US", "UR", -1},
		{"IN", "S", -1},
		// UK BBFC
		{"GB", "U", 0},
		{"GB", "12A", 12},
		{"GB", "15", 15},
		{"GB", "R18", 18},
		// Germany FSK
		{"DE", "0", 0},
		{"DE", "12", 12},
		{"DE", "18", 18},
		// Australia ACB
		{"AU", "MA 15+", 15},
		{"AU", "R18+", 18},
		// Netherlands Kijkwijzer
		{"NL", "AL", 0},
		{"NL", "16", 16},
		// Brazil ClassInd
		{"BR", "LIVRE", 0},
		{"BR", "14", 14},
		// Russia age-labelled
		{"RU", "18+", 18},
		// Fallback: unknown country, but US rating still works
		{"SN", "PG-13", 13},
		// Truly unknown → -1
		{"FR", "XYZ", -1},
		{"ZZ", "FOO", -1},
		{"", "PG", -1},
		{"US", "", -1},
		// Agency prefix stripping (TMDB sometimes glues the agency name in)
		{"AU", "ACB MA 15+", 15},
		{"", "ACB MA 15+", 15}, // implied country from prefix
		{"", "BBFC 15", 15},
		{"", "MPAA R", 17},
		{"", "MPA PG-13", 13},
		{"", "FSK 16", 16},
		{"", "CSA -12", 12},
		{"AU", "ACB-MA 15+", 15}, // dash separator
		// Bad/no separator: prefix attached without space stays as-is
		{"AU", "ACBMA 15+", -1},
	}
	for _, tt := range tests {
		got := MinAgeFor(tt.country, tt.rating)
		if got != tt.want {
			t.Errorf("MinAgeFor(%q, %q) = %d, want %d", tt.country, tt.rating, got, tt.want)
		}
	}
}

func TestHasBlockedTag(t *testing.T) {
	t.Parallel()
	tests := []struct {
		tags    []string
		blocked []string
		want    bool
	}{
		{nil, nil, false},
		{[]string{"action"}, nil, false},
		{[]string{"action", "horror"}, []string{"Horror"}, true},
		{[]string{"action", "comedy"}, []string{"horror"}, false},
		{[]string{"Action"}, []string{"action"}, true},
		{[]string{" Horror "}, []string{"horror"}, true},
	}
	for _, tt := range tests {
		got := HasBlockedTag(tt.tags, tt.blocked)
		if got != tt.want {
			t.Errorf("HasBlockedTag(%v, %v) = %v, want %v", tt.tags, tt.blocked, got, tt.want)
		}
	}
}

func TestWorstCaseMinAge(t *testing.T) {
	tests := []struct {
		name           string
		region         string
		officialRating string
		certByCountry  map[string]string
		want           int
	}{
		{"empty everything", "FR", "", nil, -1},
		{"official rating only", "US", "R", nil, 17},
		{"region cert preferred but worst-case wins", "FR", "PG", map[string]string{"FR": "-12", "AU": "MA 15+"}, 15},
		{"foreign cert raises the floor", "FR", "", map[string]string{"AU": "R 18+"}, 18},
		{"unknown labels resolve to -1", "FR", "ZZ-bogus", map[string]string{"XX": "weird"}, -1},
		{"all-ages stays 0", "FR", "TP", nil, 0},
		{"an unrated label does not mask a known cert", "US", "NR", map[string]string{"GB": "15"}, 15},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WorstCaseMinAge(tt.region, tt.officialRating, tt.certByCountry); got != tt.want {
				t.Errorf("WorstCaseMinAge(%q,%q,%v) = %d, want %d", tt.region, tt.officialRating, tt.certByCountry, got, tt.want)
			}
		})
	}
}
