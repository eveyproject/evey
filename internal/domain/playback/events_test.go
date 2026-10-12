package playback_test

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/playback"
)

func TestEventNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{playback.PlaybackStarted{}, "playback.started"},
		{playback.PlaybackStopped{}, "playback.stopped"},
		{playback.MediaMarkedWatched{}, "media.marked.watched"},
	}

	for _, c := range cases {
		got := c.event.EventName()
		if got != c.want {
			t.Errorf("EventName() = %q, want %q", got, c.want)
		}
	}
}
