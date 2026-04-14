package subtitle_test

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/subtitle"
)

func TestEventNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{subtitle.SubtitleDownloaded{}, "subtitle.downloaded"},
		{subtitle.SubtitleDeleted{}, "subtitle.deleted"},
	}

	for _, c := range cases {
		got := c.event.EventName()
		if got != c.want {
			t.Errorf("EventName() = %q, want %q", got, c.want)
		}
	}
}
