package ratings

import "testing"

func TestIsAllowed(t *testing.T) {
	tests := []struct {
		content, max string
		want         bool
	}{
		{"", "PG-13", true},
		{"R", "", true},
		{"", "", true},
		{"G", "PG-13", true},
		{"PG", "PG-13", true},
		{"PG-13", "PG-13", true},
		{"R", "PG-13", false},
		{"NC-17", "PG-13", false},
		{"NC-17", "R", false},
		{"R", "R", true},
		{"G", "R", true},
		// TV ratings
		{"TV-Y", "TV-14", true},
		{"TV-G", "TV-14", true},
		{"TV-PG", "TV-14", true},
		{"TV-14", "TV-14", true},
		{"TV-MA", "TV-14", false},
		// cross-system
		{"TV-MA", "PG-13", false},
		{"TV-MA", "R", true},
		{"R", "TV-14", false},
		{"PG", "TV-PG", true},
		// unknown ratings pass through
		{"UNKNOWN", "PG-13", true},
		{"R", "UNKNOWN", true},
		// case insensitive
		{"r", "pg-13", false},
		{"pg", "PG-13", true},
		{"tv-ma", "R", true},
	}
	for _, tt := range tests {
		got := IsAllowed(tt.content, tt.max)
		if got != tt.want {
			t.Errorf("IsAllowed(%q, %q) = %v, want %v", tt.content, tt.max, got, tt.want)
		}
	}
}

func TestHasBlockedTag(t *testing.T) {
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
