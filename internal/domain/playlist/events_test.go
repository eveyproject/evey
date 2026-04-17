package playlist_test

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/playlist"
)

func TestPlaylistEventNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{playlist.PlaylistCreated{}, "playlist.created"},
		{playlist.PlaylistUpdated{}, "playlist.updated"},
		{playlist.ItemAdded{}, "playlist.item.added"},
		{playlist.ItemRemoved{}, "playlist.item.removed"},
	}

	for _, c := range cases {
		got := c.event.EventName()
		if got != c.want {
			t.Errorf("EventName() = %q, want %q", got, c.want)
		}
	}
}
