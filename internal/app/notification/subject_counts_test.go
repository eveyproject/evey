package notification

import "testing"

// The counts must travel as DATA, not baked into an English string: that is what
// let a French TV render "Nouvelle série : Lioness (1 seasons, 1 episodes)".
// subjectOf is the piece that decides WHAT the counts are about, and getting it
// wrong regrouped "The Wire" into "S01E07 (2 episodes)".
func TestSubjectOf(t *testing.T) {
	cases := []struct {
		name, title, seriesTitle, want string
	}{
		{"episode keeps the series", "S01E07", "The Wire", "The Wire"},
		{"movie is its own subject", "Heat", "", "Heat"},
		{"empty series falls back", "Season 2", "", "Season 2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := subjectOf(c.title, c.seriesTitle); got != c.want {
				t.Errorf("subjectOf(%q, %q) = %q, want %q", c.title, c.seriesTitle, got, c.want)
			}
		})
	}
}
