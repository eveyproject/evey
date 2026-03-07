package mkv

import "time"

type ContainerParsed struct {
	Path       string
	TrackCount int
	DurationMs int64
	OccurredAt time.Time
}

func (e ContainerParsed) EventName() string { return "mkv.container.parsed" }

type TrackExtracted struct {
	SourcePath string
	TrackID    uint64
	OutputPath string
	OccurredAt time.Time
}

func (e TrackExtracted) EventName() string { return "mkv.track.extracted" }

type ContainerMuxed struct {
	OutputPath string
	TrackCount int
	OccurredAt time.Time
}

func (e ContainerMuxed) EventName() string { return "mkv.container.muxed" }
