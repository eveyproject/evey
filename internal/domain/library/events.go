package library

import "time"

type LibraryCreated struct {
	LibraryID string
	Name      string
	Type      string
	OccurredAt time.Time
}

func (e LibraryCreated) EventName() string { return "library.created" }

type LibraryDeleted struct {
	LibraryID  string
	OccurredAt time.Time
}

func (e LibraryDeleted) EventName() string { return "library.deleted" }

type LibraryScanStarted struct {
	LibraryID  string
	OccurredAt time.Time
}

func (e LibraryScanStarted) EventName() string { return "library.scan.started" }

type LibraryScanCompleted struct {
	LibraryID    string
	ItemsFound   int
	ItemsAdded   int
	ItemsUpdated int
	ItemsRemoved int
	OccurredAt   time.Time
}

func (e LibraryScanCompleted) EventName() string { return "library.scan.completed" }

type MediaItemAdded struct {
	MediaItemID string
	LibraryID   string
	Title       string
	Type        string
	Genres      []string
	OccurredAt  time.Time
}

func (e MediaItemAdded) EventName() string { return "media.item.added" }

type MediaItemUpdated struct {
	MediaItemID string
	OccurredAt  time.Time
}

func (e MediaItemUpdated) EventName() string { return "media.item.updated" }

type MediaItemRemoved struct {
	MediaItemID string
	LibraryID   string
	OccurredAt  time.Time
}

func (e MediaItemRemoved) EventName() string { return "media.item.removed" }
