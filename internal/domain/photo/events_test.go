package photo_test

import (
	"testing"

	"github.com/gravity-zero/evey/internal/domain/photo"
)

func TestPhotoEventNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{photo.PhotoImported{}, "photo.imported"},
		{photo.PhotoDeleted{}, "photo.deleted"},
	}

	for _, c := range cases {
		got := c.event.EventName()
		if got != c.want {
			t.Errorf("EventName() = %q, want %q", got, c.want)
		}
	}
}
