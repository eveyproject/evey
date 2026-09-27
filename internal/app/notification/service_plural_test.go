package notification

import "testing"

// Seen on the TV on 2026-08-02: "Nouvelle série : Some Series (1 seasons, 1 episodes)".
// Two defects in one line - the English, which needs a payload change and is the
// user's decision, and the count, which was ungrammatical in English too.
func TestBatchedSeriesTitle_SingularCounts(t *testing.T) {
	cases := []struct {
		name                      string
		seasonCount, episodeCount int
		want, pushKey             string
	}{
		{"one of each", 1, 1, "Some Series (1 season, 1 episode)", "notifications.push.series_with_seasons_and_episodes.title"},
		{"several of each", 2, 5, "Some Series (2 seasons, 5 episodes)", "notifications.push.series_with_seasons_and_episodes.title"},
		{"one season only", 1, 0, "Some Series (1 season)", "notifications.push.series_with_seasons.title"},
		{"one episode only", 0, 1, "Some Series (1 episode)", "notifications.push.series_with_episodes.title"},
		{"no counts at all", 0, 0, "Some Series", "notifications.push.new_series.title"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, push := batchedSeriesTitle("Some Series", c.seasonCount, c.episodeCount)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
			// The push is translated by its key: the wrong key reads as the wrong sentence on the phone.
			if push.key != c.pushKey {
				t.Errorf("push key %q, want %q", push.key, c.pushKey)
			}
			if push.fallback != "New Series: "+c.want {
				t.Errorf("push fallback %q, want %q", push.fallback, "New Series: "+c.want)
			}
		})
	}
}

func TestExtractSeriesName(t *testing.T) {
	for stored, want := range map[string]string{
		"new_episode:Some Series - Pilot":      "Some Series",
		"new_episode:Some Series (3 episodes)": "Some Series",
		"new_episode:Some Series":              "Some Series",
		"new_episode:Hyphen - Heavy - Title":   "Hyphen",
		"new_episode:(Parens) First - Ep":      "(Parens) First",
		"Untyped Title (2 episodes)":           "Untyped Title",
	} {
		if got := extractSeriesName(stored); got != want {
			t.Errorf("%q: got %q, want %q", stored, got, want)
		}
	}
}
