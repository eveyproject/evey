package photo_test

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/photo"
)

func TestPhotoEventNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{photo.PhotoAdded{}, "photo.added"},
		{photo.PhotoRemoved{}, "photo.removed"},
		{photo.PhotoDeleted{}, "photo.deleted"},
		{photo.AlbumCreated{}, "photo.album.created"},
	}

	for _, c := range cases {
		got := c.event.EventName()
		if got != c.want {
			t.Errorf("EventName() = %q, want %q", got, c.want)
		}
	}
}
