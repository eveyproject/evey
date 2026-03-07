package playback

import "time"

type PlayMethod string

const (
	DirectPlay   PlayMethod = "direct_play"
	DirectStream PlayMethod = "direct_stream"
	Transcode    PlayMethod = "transcode"
)

type WatchProgress struct {
	ID           string
	ProfileID    string
	MediaItemID  string
	PositionMs   int64
	Completed    bool
	PlayCount    int
	LastPlayedAt time.Time
}

type PlaySession struct {
	ID           string
	ProfileID    string
	MediaItemID  string
	DeviceName   string
	DeviceType   string
	ClientIP     string
	PlayMethod   string
	VideoCodec   string
	AudioCodec   string
	Resolution   string
	BandwidthBps int64
	StartedAt    time.Time
	EndedAt      *time.Time
	DurationMs   *int64
	PositionMs   *int64
}

type StreamInfo struct {
	DirectPlayURL  *string
	TranscodeURL   *string
	PlayMethod     string
	MediaStreams    []StreamDetail
}

type StreamDetail struct {
	StreamIndex int
	StreamType  string
	Codec       string
	Language    string
}
