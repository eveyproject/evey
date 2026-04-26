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

func (m PlayMethod) IsValid() bool {
	switch m {
	case DirectPlay, DirectStream, Transcode:
		return true
	}
	return false
}

type WatchProgress struct {
	ID           string
	ProfileID    shared.ProfileID
	MediaItemID  shared.MediaItemID
	Position     shared.Duration
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
	// Position is the last reported playback position. nil when no progress
	// has been reported yet for this session.
	Position *shared.Duration
	IsPaused bool
}

func (s *PlaySession) Validate() error {
	if s.ID == "" {
		return errors.New("play session ID is required")
	}
	if s.ProfileID == "" {
		return errors.New("play session profile ID is required")
	}
	if s.MediaItemID == "" {
		return errors.New("play session media item ID is required")
	}
	if s.PlayMethod != "" && !PlayMethod(s.PlayMethod).IsValid() {
		return errors.New("play session: invalid play method")
	}
	return nil
}

type PlaySessionOption func(*PlaySession)

func WithPlaySessionDevice(name, deviceType string) PlaySessionOption {
	return func(s *PlaySession) { s.DeviceName, s.DeviceType = name, deviceType }
}

func WithPlaySessionClientIP(ip string) PlaySessionOption {
	return func(s *PlaySession) { s.ClientIP = ip }
}

func WithPlaySessionMethod(m PlayMethod) PlaySessionOption {
	return func(s *PlaySession) { s.PlayMethod = string(m) }
}

func WithPlaySessionCodecs(video, audio string) PlaySessionOption {
	return func(s *PlaySession) { s.VideoCodec, s.AudioCodec = video, audio }
}

func WithPlaySessionResolution(r string) PlaySessionOption {
	return func(s *PlaySession) { s.Resolution = r }
}

func WithPlaySessionBandwidth(bps int64) PlaySessionOption {
	return func(s *PlaySession) { s.BandwidthBps = bps }
}

// NewPlaySession is the canonical constructor. Stamps StartedAt and
// validates. Required identity fields (id, profile, media) surfaced
// as params so constructors never succeed with them missing.
func NewPlaySession(id string, profileID shared.ProfileID, mediaItemID shared.MediaItemID, opts ...PlaySessionOption) (*PlaySession, error) {
	s := &PlaySession{
		ID:          id,
		ProfileID:   profileID,
		MediaItemID: mediaItemID,
		StartedAt:   time.Now(),
	}
	for _, opt := range opts {
		opt(s)
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return s, nil
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
	return !wp.Completed && !wp.Position.IsZero()
}

func (s *PlaySession) IsActive() bool {
	return s.EndedAt == nil
}
