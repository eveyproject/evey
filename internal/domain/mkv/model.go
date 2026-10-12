package mkv

import (
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

type TrackType string

const (
	VideoTrack    TrackType = "video"
	AudioTrack    TrackType = "audio"
	SubtitleTrack TrackType = "subtitle"
)

type Container struct {
	Path        string
	Info        SegmentInfo
	Tracks      []Track
	Chapters    []Chapter
	Attachments []Attachment
	Tags        []Tag
	// Duration is the total container length parsed from the segment.
	// Stored as shared.Duration (non-negative ms) so callers can't mix
	// units; conversions happen at the mkvgo adapter and gRPC boundaries.
	Duration shared.Duration
}

type SegmentInfo struct {
	Title         string
	MuxingApp     string
	WritingApp    string
	Duration      float64
	TimecodeScale int64
	DateUTC       *time.Time
}

type Track struct {
	ID         uint64
	Type       TrackType
	Codec      string
	Language   string
	Name       string
	IsDefault  bool
	IsForced   bool
	Width      *uint32
	Height     *uint32
	Channels   *uint8
	SampleRate *float64
	BitDepth   *uint8
}

type Chapter struct {
	ID      uint64
	Title   string
	StartMs int64
	EndMs   int64
}

type Attachment struct {
	ID       uint64
	Name     string
	MIMEType string
	Size     int64
	Data     []byte
}

type Tag struct {
	TargetType string
	TargetID   uint64
	SimpleTag  []SimpleTag
}

type SimpleTag struct {
	Name     string
	Value    string
	Language string
}
