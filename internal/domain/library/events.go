package library

import (
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

type LibraryCreated struct {
	LibraryID shared.LibraryID
	Name      string
	Type      string
	OccurredAt time.Time
}

func (e LibraryCreated) EventName() string { return "library.created" }

type LibraryDeleted struct {
	LibraryID  shared.LibraryID
	OccurredAt time.Time
}

func (e LibraryDeleted) EventName() string { return "library.deleted" }

type LibraryScanStarted struct {
	LibraryID  shared.LibraryID
	OccurredAt time.Time
}

func (e LibraryScanStarted) EventName() string { return "library.scan.started" }

type LibraryScanCompleted struct {
	LibraryID    shared.LibraryID
	ItemsFound   int
	ItemsAdded   int
	ItemsUpdated int
	ItemsRemoved int
	OccurredAt   time.Time
}

func (e LibraryScanCompleted) EventName() string { return "library.scan.completed" }

type MediaItemAdded struct {
	MediaItemID shared.MediaItemID
	LibraryID   shared.LibraryID
	Title       string
	SeriesTitle string // non-empty for episodes/seasons
	Type        string
	Genres      []string
	OccurredAt  time.Time
}

func (e MediaItemAdded) EventName() string { return "media.item.added" }

type MediaItemUpdated struct {
	MediaItemID shared.MediaItemID
	OccurredAt  time.Time
}

func (e MediaItemUpdated) EventName() string { return "media.item.updated" }

type MediaItemRemoved struct {
	MediaItemID shared.MediaItemID
	LibraryID   shared.LibraryID
	OccurredAt  time.Time
}

func (e MediaItemRemoved) EventName() string { return "media.item.removed" }
