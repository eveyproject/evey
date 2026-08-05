package notification

import "testing"

// Seen on the TV on 2026-08-02: "Nouvelle série : Lioness (1 seasons, 1 episodes)".
// Two defects in one line - the English, which needs a payload change and is the
// user's decision, and the count, which was ungrammatical in English too.
func TestBatchedSeriesTitle_SingularCounts(t *testing.T) {
	cases := []struct {
		name                      string
		seasonCount, episodeCount int
		want                      string
	}{
		{"one of each", 1, 1, "Lioness (1 season, 1 episode)"},
		{"several of each", 2, 5, "Lioness (2 seasons, 5 episodes)"},
		{"one season only", 1, 0, "Lioness (1 season)"},
		{"one episode only", 0, 1, "Lioness (1 episode)"},
		{"no counts at all", 0, 0, "Lioness"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := batchedSeriesTitle("Lioness", c.seasonCount, c.episodeCount)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
