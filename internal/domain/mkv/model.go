package mkv

import "time"

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
	DurationMs  int64
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

type MuxOptions struct {
	OutputPath string
	Tracks     []TrackInput
	Chapters   []Chapter
	Tags       []Tag
}

type DemuxOptions struct {
	OutputDir string
	TrackIDs  []uint64
}

type TrackInput struct {
	SourcePath string
	TrackID    uint64
	Language   string
	Name       string
	IsDefault  bool
}
