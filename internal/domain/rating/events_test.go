package rating_test

import (
	"testing"

	"github.com/gravity-zero/evey/internal/domain/rating"
)

func TestRatingEventNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{rating.MediaRated{}, "rating.media.rated"},
		{rating.MediaRatingDeleted{}, "rating.media.deleted"},
	}

	for _, c := range cases {
		got := c.event.EventName()
		if got != c.want {
			t.Errorf("EventName() = %q, want %q", got, c.want)
		}
	}
}
