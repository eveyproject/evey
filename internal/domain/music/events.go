package music

import "time"

type TrackScanned struct {
	LibraryID  string
	TrackID    string
	Title      string
	Path       string
	OccurredAt time.Time
}

func (e TrackScanned) EventName() string { return "music.track.scanned" }

type TrackDeleted struct {
	LibraryID  string
	TrackID    string
	OccurredAt time.Time
}

func (e TrackDeleted) EventName() string { return "music.track.deleted" }

type TrackAdded struct {
	LibraryID  string
	TrackID    string
	Title      string
	Artist     string
	OccurredAt time.Time
}

func (e TrackAdded) EventName() string { return "music.track.added" }

type TrackRemoved struct {
	LibraryID  string
	TrackID    string
	OccurredAt time.Time
}

func (e TrackRemoved) EventName() string { return "music.track.removed" }

type AlbumCreated struct {
	LibraryID  string
	AlbumName  string
	Artist     string
	OccurredAt time.Time
}

func (e AlbumCreated) EventName() string { return "music.album.created" }
