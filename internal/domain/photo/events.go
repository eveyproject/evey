package photo

import (
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

type PhotoAdded struct {
	LibraryID  shared.LibraryID
	PhotoID    string
	Path       string
	Title      string
	Album      string
	OccurredAt time.Time
}

func (e PhotoAdded) EventName() string { return "photo.added" }

type PhotoRemoved struct {
	LibraryID  shared.LibraryID
	PhotoID    string
	OccurredAt time.Time
}

func (e PhotoRemoved) EventName() string { return "photo.removed" }
