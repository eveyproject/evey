package mkv_test

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/mkv"
)

func TestEventNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{mkv.ContainerParsed{}, "mkv.container.parsed"},
		{mkv.TrackExtracted{}, "mkv.track.extracted"},
		{mkv.ContainerMuxed{}, "mkv.container.muxed"},
	}

	for _, c := range cases {
		got := c.event.EventName()
		if got != c.want {
			t.Errorf("EventName() = %q, want %q", got, c.want)
		}
	}
}
