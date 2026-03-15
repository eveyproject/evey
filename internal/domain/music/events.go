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
