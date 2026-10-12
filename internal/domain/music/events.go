package music

import (
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

type TrackScanned struct {
	LibraryID  shared.LibraryID
	TrackID    string
	Title      string
	Path       string
	OccurredAt time.Time
}

func (e TrackScanned) EventName() string { return "music.track.scanned" }

type TrackDeleted struct {
	LibraryID  shared.LibraryID
	TrackID    string
	OccurredAt time.Time
}

func (e TrackDeleted) EventName() string { return "music.track.deleted" }

type TrackAdded struct {
	LibraryID  shared.LibraryID
	TrackID    string
	Title      string
	Artist     string
	OccurredAt time.Time
}

func (e TrackAdded) EventName() string { return "music.track.added" }

type TrackRemoved struct {
	LibraryID  shared.LibraryID
	TrackID    string
	OccurredAt time.Time
}

func (e TrackRemoved) EventName() string { return "music.track.removed" }

type AlbumCreated struct {
	LibraryID  shared.LibraryID
	AlbumName  string
	Artist     string
	OccurredAt time.Time
}

func (e AlbumCreated) EventName() string { return "music.album.created" }
