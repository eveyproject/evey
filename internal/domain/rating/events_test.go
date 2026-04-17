package rating_test

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/rating"
)

func TestRatingEventNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{rating.RatingSet{}, "rating.set"},
		{rating.RatingDeleted{}, "rating.deleted"},
		{rating.RatingRemoved{}, "rating.removed"},
	}

	for _, c := range cases {
		got := c.event.EventName()
		if got != c.want {
			t.Errorf("EventName() = %q, want %q", got, c.want)
		}
	}
}
