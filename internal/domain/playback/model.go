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
	ID          string
	ProfileID   shared.ProfileID
	MediaItemID shared.MediaItemID
	DeviceName  string
	DeviceType  string
	ClientIP    string
	PlayMethod  string
	VideoCodec  string
	AudioCodec  string
	Resolution  string
	// VideoDecision / AudioDecision are the per-track serving decisions
	// ("copy"|"transcode"), copied from the StreamInfo the player resolved.
	// They let the admin panel distinguish a copy rung from a real transcode.
	VideoDecision string
	AudioDecision string
	BandwidthBps  int64
	StartedAt     time.Time
	EndedAt       *time.Time
	Duration      *shared.Duration
	// Position is the last reported playback position. nil when no progress
	// has been reported yet for this session.
	Position *shared.Duration
	IsPaused bool
	// CastDeviceName is the remote device the media ACTUALLY plays on when the
	// session's client is only a relay (web sender driving a Chromecast) - the
	// reported Position/IsPaused then describe the remote playback. Empty =
	// playing on the session's own device. Updated on every progress report,
	// so a finished cast clears it.
	CastDeviceName string
	// Serving is what the server actually serves, recorded at master render;
	// zero until then (direct play, or a session older than the column).
	Serving ServingPath
}

// ServingPath names the machinery behind each track of a session: the
// client's start-time decision only says copy/transcode, this says by whom.
type ServingPath struct {
	VideoDecision string // "copy" | "transcode"
	VideoEngine   string // "mkvgo" | "ffmpeg"
	VideoBackend  string // transcode only: "software" | "qsv" | "vaapi" | "nvenc"
	AudioDecision string // "copy" | "transcode" | "wasm"
	AudioEngine   string // "mkvgo" | "ffmpeg" | "client"
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

func WithPlaySessionMethod(m PlayMethod) PlaySessionOption {
	return func(s *PlaySession) { s.PlayMethod = string(m) }
}

// WithPlaySessionCastDevice names the receiver a cast session plays on.
func WithPlaySessionCastDevice(name string) PlaySessionOption {
	return func(s *PlaySession) { s.CastDeviceName = name }
}

// WithPlaySessionPosition seeds the playhead. Used when a session is created
// from a beacon that already carries a position rather than from a start at 0.
func WithPlaySessionPosition(pos shared.Duration) PlaySessionOption {
	return func(s *PlaySession) { s.Position = &pos }
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
	SessionID   string
	ProfileID   shared.ProfileID
	ProfileName string
	// Owning account: two households can both call a profile "Papa", so the
	// admin panel needs the account behind the profile name.
	UserID        shared.UserID
	Username      string
	MediaItemID   shared.MediaItemID
	MediaTitle    string
	MediaType     string
	Duration      shared.Duration
	Position      shared.Duration
	StartedAt     time.Time
	DeviceName    string
	DeviceType    string
	ClientIP      string
	PlayMethod    string
	VideoCodec    string
	AudioCodec    string
	Resolution    string
	VideoDecision string
	AudioDecision string
	BandwidthBps  int64
	TranscodeInfo *string
	SeriesTitle   string
	SeasonNumber  int
	EpisodeNumber int
	IsPaused      bool
	// CastDeviceName: see PlaySession.CastDeviceName - the real playback
	// target when the session's client relays to a cast device.
	CastDeviceName string
	VideoEngine    string
	VideoBackend   string
	AudioEngine    string
}

func (a *ActiveSessionInfo) ProgressPercent() float64 {
	dms := a.Duration.Milliseconds()
	if dms <= 0 {
		return 0
	}
	p := float64(a.Position.Milliseconds()) / float64(dms) * 100
	if p > 100 {
		return 100
	}
	return p
}

type StreamInfo struct {
	DirectPlayURL *string
	TranscodeURL  *string
	PlayMethod    string
	MediaStreams  []StreamDetail
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
