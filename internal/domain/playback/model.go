package playback

import (
	"errors"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

type PlayMethod string

const (
	DirectPlay   PlayMethod = "direct_play"
	DirectStream PlayMethod = "direct_stream"
	Transcode    PlayMethod = "transcode"
)

type WatchProgress struct {
	ID           string
	ProfileID    shared.ProfileID
	MediaItemID  shared.MediaItemID
	PositionMs   int64
	Completed    bool
	PlayCount    int
	LastPlayedAt time.Time
}

type PlaySession struct {
	ID           string
	ProfileID    shared.ProfileID
	MediaItemID  shared.MediaItemID
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
	IsPaused     bool
}

func (s *PlaySession) Validate() error {
	if s.ProfileID == "" {
		return errors.New("play session profile ID is required")
	}
	if s.MediaItemID == "" {
		return errors.New("play session media item ID is required")
	}
	return nil
}

type ActiveSessionInfo struct {
	SessionID     string
	ProfileID     shared.ProfileID
	ProfileName   string
	MediaItemID   shared.MediaItemID
	MediaTitle    string
	MediaType     string
	DurationMs    int64
	PositionMs    int64
	StartedAt     time.Time
	DeviceName    string
	DeviceType    string
	ClientIP      string
	PlayMethod    string
	VideoCodec    string
	AudioCodec    string
	Resolution    string
	BandwidthBps  int64
	TranscodeInfo *string
	SeriesTitle   string
	SeasonNumber  int
	EpisodeNumber int
	IsPaused      bool
}

func (a *ActiveSessionInfo) ProgressPercent() float64 {
	if a.DurationMs <= 0 {
		return 0
	}
	p := float64(a.PositionMs) / float64(a.DurationMs) * 100
	if p > 100 {
		return 100
	}
	return p
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

func (wp *WatchProgress) IsInProgress() bool {
	return !wp.Completed && wp.PositionMs > 0
}

func (s *PlaySession) IsActive() bool {
	return s.EndedAt == nil
}
