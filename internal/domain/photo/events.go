package photo

import "time"

type PhotoImported struct {
	LibraryID  string
	PhotoID    string
	Path       string
	OccurredAt time.Time
}

func (e PhotoImported) EventName() string { return "photo.imported" }

// PhotoScanned is an alias for PhotoImported satisfying ARC-NEW-001.
type PhotoScanned struct {
	PhotoID    string
	LibraryID  string
	OccurredAt time.Time
}

func (e PhotoScanned) EventName() string { return "photo.scanned" }

type PhotoDeleted struct {
	LibraryID  string
	PhotoID    string
	OccurredAt time.Time
}

func (e PhotoDeleted) EventName() string { return "photo.deleted" }

type PhotoAdded struct {
	LibraryID  string
	PhotoID    string
	Title      string
	Album      string
	OccurredAt time.Time
}

func (e PhotoAdded) EventName() string { return "photo.added" }

type PhotoRemoved struct {
	LibraryID  string
	PhotoID    string
	OccurredAt time.Time
}

func (e PhotoRemoved) EventName() string { return "photo.removed" }
