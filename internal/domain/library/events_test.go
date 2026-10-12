package library_test

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/library"
)

func TestEventNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{library.LibraryCreated{}, "library.created"},
		{library.LibraryDeleted{}, "library.deleted"},
		{library.LibraryScanStarted{}, "library.scan.started"},
		{library.LibraryScanCompleted{}, "library.scan.completed"},
		{library.MediaItemAdded{}, "media.item.added"},
		{library.MediaItemRemoved{}, "media.item.removed"},
	}

	for _, c := range cases {
		got := c.event.EventName()
		if got != c.want {
			t.Errorf("EventName() = %q, want %q", got, c.want)
		}
	}
}
