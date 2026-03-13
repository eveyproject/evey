package ratings

import "testing"

func TestIsAllowed(t *testing.T) {
	t.Parallel()
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

func TestAllowedRatingsFor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		max         string
		mustInclude []string
		mustExclude []string
	}{
		{"PG", []string{"G", "PG", "TV-G", "TV-PG", "TV-Y"}, []string{"PG-13", "R", "NC-17", "TV-14", "TV-MA"}},
		{"PG-13", []string{"G", "PG", "PG-13", "TV-14"}, []string{"R", "NC-17", "TV-MA"}},
		{"R", []string{"G", "PG", "PG-13", "R", "TV-MA"}, []string{"NC-17"}},
		{"", nil, nil},
		{"UNKNOWN", nil, nil},
	}
	for _, tt := range tests {
		got := AllowedRatingsFor(tt.max)
		if tt.mustInclude == nil {
			if got != nil {
				t.Errorf("AllowedRatingsFor(%q) = %v, want nil", tt.max, got)
			}
			continue
		}
		gotSet := make(map[string]bool, len(got))
		for _, r := range got {
			gotSet[r] = true
		}
		for _, r := range tt.mustInclude {
			if !gotSet[r] {
				t.Errorf("AllowedRatingsFor(%q) missing %q, got %v", tt.max, r, got)
			}
		}
		for _, r := range tt.mustExclude {
			if gotSet[r] {
				t.Errorf("AllowedRatingsFor(%q) should not contain %q, got %v", tt.max, r, got)
			}
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
