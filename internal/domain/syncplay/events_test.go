package syncplay_test

import (
	"testing"

	"github.com/gravity-zero/evey/internal/domain/syncplay"
)

func TestEventNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{syncplay.RoomCreated{}, "syncplay.room.created"},
		{syncplay.RoomClosed{}, "syncplay.room.closed"},
		{syncplay.MemberJoined{}, "syncplay.member.joined"},
		{syncplay.MemberLeft{}, "syncplay.member.left"},
	}

	for _, c := range cases {
		got := c.event.EventName()
		if got != c.want {
			t.Errorf("EventName() = %q, want %q", got, c.want)
		}
	}
}
