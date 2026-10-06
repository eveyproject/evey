package playback

import (
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

// SeriesEpisode is one episode of a series, in viewing order, with the
// profile's progress on it. LastPlayedAt is zero for an episode never played.
type SeriesEpisode struct {
	MediaItemID  shared.MediaItemID
	Position     shared.Duration
	Completed    bool
	LastPlayedAt time.Time
}

// SeriesStarted says whether the profile has played any episode of the series:
// what lets a client say "Resume" rather than "Play".
func SeriesStarted(episodes []SeriesEpisode) bool {
	for _, e := range episodes {
		if !e.LastPlayedAt.IsZero() {
			return true
		}
	}
	return false
}

// PickSeriesEpisode is the episode "Play" on a series starts: the last one
// played if it was left unfinished, else the first unfinished one after it
// (season boundaries included), else the first unfinished one anywhere, else
// the first episode. ok is false only for a series without episodes.
func PickSeriesEpisode(episodes []SeriesEpisode) (SeriesEpisode, bool) {
	if len(episodes) == 0 {
		return SeriesEpisode{}, false
	}
	last := -1
	for i, e := range episodes {
		if e.LastPlayedAt.IsZero() {
			continue
		}
		if last < 0 || e.LastPlayedAt.After(episodes[last].LastPlayedAt) {
			last = i
		}
	}
	if last < 0 {
		return episodes[0], true
	}
	if !episodes[last].Completed {
		return episodes[last], true
	}
	for _, e := range episodes[last+1:] {
		if !e.Completed {
			return e, true
		}
	}
	for _, e := range episodes {
		if !e.Completed {
			return e, true
		}
	}
	return episodes[0], true
}
