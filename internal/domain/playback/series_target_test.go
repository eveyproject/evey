package playback_test

import (
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/playback"
	"github.com/eveyproject/evey/internal/domain/shared"
)

func TestPickSeriesEpisode(t *testing.T) {
	t.Parallel()
	at := func(h int) time.Time { return time.Date(2026, 10, 6, h, 0, 0, 0, time.UTC) }
	ep := func(id string, completed bool, played time.Time) playback.SeriesEpisode {
		return playback.SeriesEpisode{MediaItemID: shared.MediaItemID(id), Completed: completed, LastPlayedAt: played}
	}
	cases := []struct {
		name     string
		episodes []playback.SeriesEpisode
		want     string
	}{
		{"nothing played starts at the first episode",
			[]playback.SeriesEpisode{ep("s1e1", false, time.Time{}), ep("s1e2", false, time.Time{})}, "s1e1"},
		{"an unfinished episode is resumed, even an earlier one",
			[]playback.SeriesEpisode{ep("s1e1", true, at(1)), ep("s2e1", true, at(2)), ep("s2e2", false, at(3)), ep("s2e3", false, time.Time{})}, "s2e2"},
		{"a finished season ending moves on to the next season",
			[]playback.SeriesEpisode{ep("s1e1", true, at(1)), ep("s1e2", true, at(2)), ep("s2e1", false, time.Time{})}, "s2e1"},
		{"the most recently played decides, not the furthest",
			[]playback.SeriesEpisode{ep("s1e1", true, at(1)), ep("s1e2", false, time.Time{}), ep("s2e1", true, at(5)), ep("s2e2", false, time.Time{})}, "s2e2"},
		{"episodes already seen after the last one are skipped",
			[]playback.SeriesEpisode{ep("e1", true, at(3)), ep("e2", true, at(1)), ep("e3", false, time.Time{})}, "e3"},
		{"past the end, the first unfinished episode anywhere",
			[]playback.SeriesEpisode{ep("e1", false, time.Time{}), ep("e2", true, at(1)), ep("e3", true, at(2))}, "e1"},
		{"everything seen starts over",
			[]playback.SeriesEpisode{ep("e1", true, at(1)), ep("e2", true, at(2))}, "e1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, ok := playback.PickSeriesEpisode(c.episodes)
			if !ok || string(got.MediaItemID) != c.want {
				t.Fatalf("got %q (ok=%v), want %q", got.MediaItemID, ok, c.want)
			}
		})
	}
	if _, ok := playback.PickSeriesEpisode(nil); ok {
		t.Fatal("a series without episodes has no target")
	}
}
