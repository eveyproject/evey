package music_test

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/music"
)

func TestMusicEventNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{music.TrackScanned{}, "music.track.scanned"},
		{music.TrackDeleted{}, "music.track.deleted"},
		{music.TrackAdded{}, "music.track.added"},
		{music.TrackRemoved{}, "music.track.removed"},
		{music.AlbumCreated{}, "music.album.created"},
	}

	for _, c := range cases {
		got := c.event.EventName()
		if got != c.want {
			t.Errorf("EventName() = %q, want %q", got, c.want)
		}
	}
}
