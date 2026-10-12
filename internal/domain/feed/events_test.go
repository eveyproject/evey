package feed_test

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/feed"
)

func TestFeedEventNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{feed.FeedConfigChanged{}, "feed.config.changed"},
		{feed.FeedConfigRemoved{}, "feed.config.removed"},
	}

	for _, c := range cases {
		got := c.event.EventName()
		if got != c.want {
			t.Errorf("EventName() = %q, want %q", got, c.want)
		}
	}
}
