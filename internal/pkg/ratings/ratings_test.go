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
	}
	for _, tt := range tests {
		got := MinAgeFor(tt.country, tt.rating)
		if got != tt.want {
			t.Errorf("MinAgeFor(%q, %q) = %d, want %d", tt.country, tt.rating, got, tt.want)
		}
	}
}

func TestIsAllowedByAge(t *testing.T) {
	t.Parallel()
	tests := []struct {
		country, rating string
		maxAge          int
		want            bool
	}{
		// FR CSA
		{"FR", "-12", 12, true},
		{"FR", "-16", 12, false},
		{"FR", "-12", 18, true},
		{"FR", "TP", 0, true},
		// US MPAA
		{"US", "PG-13", 13, true},
		{"US", "R", 13, false},
		{"US", "R", 17, true},
		// Unknown rating: permissive (don't block on missing data)
		{"FR", "UNKNOWN", 10, true},
		{"ZZ", "XYZ", 10, true},
		// maxAge < 0 disables filter
		{"US", "R", -1, true},
	}
	for _, tt := range tests {
		got := IsAllowedByAge(tt.country, tt.rating, tt.maxAge)
		if got != tt.want {
			t.Errorf("IsAllowedByAge(%q, %q, %d) = %v, want %v", tt.country, tt.rating, tt.maxAge, got, tt.want)
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
