package notification

import "testing"

// Seen on the TV on 2026-08-02: "Nouvelle série : Some Series (1 seasons, 1 episodes)".
// Two defects in one line - the English, which needs a payload change and is the
// user's decision, and the count, which was ungrammatical in English too.
func TestBatchedSeriesTitle_SingularCounts(t *testing.T) {
	cases := []struct {
		name                      string
		seasonCount, episodeCount int
		want                      string
	}{
		{"one of each", 1, 1, "Some Series (1 season, 1 episode)"},
		{"several of each", 2, 5, "Some Series (2 seasons, 5 episodes)"},
		{"one season only", 1, 0, "Some Series (1 season)"},
		{"one episode only", 0, 1, "Some Series (1 episode)"},
		{"no counts at all", 0, 0, "Some Series"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := batchedSeriesTitle("Some Series", c.seasonCount, c.episodeCount)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestExtractSeriesName(t *testing.T) {
	for stored, want := range map[string]string{
		"new_episode:a series - Pilot":      "a series",
		"new_episode:a series (3 episodes)": "a series",
		"new_episode:a series":              "a series",
		"new_episode:Hyphen - Heavy - Title":   "Hyphen",
		"new_episode:(Parens) First - Ep":      "(Parens) First",
		"Untyped Title (2 episodes)":           "Untyped Title",
	} {
		if got := extractSeriesName(stored); got != want {
			t.Errorf("%q: got %q, want %q", stored, got, want)
		}
	}
}
