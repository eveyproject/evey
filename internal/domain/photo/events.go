package photo

import "time"

type PhotoAdded struct {
	LibraryID  string
	PhotoID    string
	Path       string
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

type PhotoDeleted struct {
	LibraryID  string
	PhotoID    string
	OccurredAt time.Time
}

func (e PhotoDeleted) EventName() string { return "photo.deleted" }

type AlbumCreated struct {
	LibraryID  string
	AlbumName  string
	OccurredAt time.Time
}

func (e AlbumCreated) EventName() string { return "photo.album.created" }
