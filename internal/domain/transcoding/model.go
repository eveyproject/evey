package transcoding

import (
	"errors"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

type HWAccelType string

const (
	VAAPI HWAccelType = "vaapi"
	NVENC HWAccelType = "nvenc"
	QSV   HWAccelType = "qsv"
	None  HWAccelType = "none"
)

func (h HWAccelType) IsValid() bool {
	switch h {
	case VAAPI, NVENC, QSV, None:
		return true
	}
	return false
}

// DefaultAudioCodec is AAC - the only universally supported audio codec for HLS/web playback
const DefaultAudioCodec = "aac"

// DefaultVideoCodec is H.264 - the universally decodable fallback for the
// transcode/HLS path (mirrors DefaultAudioCodec).
const DefaultVideoCodec = "h264"

type JobStatus string

const (
	Queued    JobStatus = "queued"
	Running   JobStatus = "running"
	Completed JobStatus = "completed"
	Failed    JobStatus = "failed"
	Cancelled JobStatus = "cancelled"
)

func (s JobStatus) IsValid() bool {
	switch s {
	case Queued, Running, Completed, Failed, Cancelled:
		return true
	}
	return false
}

type TranscodeJob struct {
	ID            string
	MediaItemID   shared.MediaItemID
	InputPath     string
	InputCodec    string
	OutputCodec   string
	AudioCodec    string
	HWAccelType   string
	Preset        string
	ResolutionIn  string
	ResolutionOut string
	BitrateIn     int64
	BitrateOut    int64
	Speed         float64
	Status        JobStatus
	ErrorMessage  string
	StartedAt     time.Time
	EndedAt       *time.Time
	// Duration is the elapsed transcode time once the job is terminal. nil
	// while queued/running. Stored as a pointer so the queued/running rows
	// distinguish "not yet measured" from a 0 ms duration on disk.
	Duration *shared.Duration
}

func (j *TranscodeJob) Validate() error {
	if j.ID == "" {
		return errors.New("transcode job ID is required")
	}
	if j.MediaItemID == "" {
		return errors.New("transcode job media item ID is required")
	}
	if j.InputPath == "" {
		return errors.New("transcode job input path is required")
	}
	if j.OutputCodec == "" {
		return errors.New("transcode job output codec is required")
	}
	if j.Status != "" && !j.Status.IsValid() {
		return errors.New("transcode job: invalid status")
	}
	if j.HWAccelType != "" && !HWAccelType(j.HWAccelType).IsValid() {
		return errors.New("transcode job: invalid hw accel type")
	}
	return nil
}

func (j *TranscodeJob) IsTerminal() bool {
	return j.Status == Completed || j.Status == Failed || j.Status == Cancelled
}

type TranscodeJobOption func(*TranscodeJob)

func WithJobAudioCodec(codec string) TranscodeJobOption {
	return func(j *TranscodeJob) { j.AudioCodec = codec }
}

func WithJobHWAccel(accel string) TranscodeJobOption {
	return func(j *TranscodeJob) { j.HWAccelType = accel }
}

func WithJobBitrate(in, out int64) TranscodeJobOption {
	return func(j *TranscodeJob) { j.BitrateIn, j.BitrateOut = in, out }
}

func WithJobResolution(in, out string) TranscodeJobOption {
	return func(j *TranscodeJob) { j.ResolutionIn, j.ResolutionOut = in, out }
}

// NewTranscodeJob is the canonical constructor. Validates. Required
// identity fields (id, mediaItem, inputPath, outputCodec) are surfaced
// as params so construction never succeeds with them missing. Status
// defaults to Queued; StartedAt is stamped when the worker picks it up.
func NewTranscodeJob(id string, mediaItemID shared.MediaItemID, inputPath, outputCodec string, opts ...TranscodeJobOption) (*TranscodeJob, error) {
	j := &TranscodeJob{
		ID:          id,
		MediaItemID: mediaItemID,
		InputPath:   inputPath,
		OutputCodec: outputCodec,
		Status:      Queued,
	}
	for _, opt := range opts {
		opt(j)
	}
	if err := j.Validate(); err != nil {
		return nil, err
	}
	return j, nil
}

type TranscodeProfile struct {
	ID           string
	Name         string
	VideoCodec   string
	AudioCodec   string
	Container    string
	MaxWidth     int
	MaxHeight    int
	VideoBitrate int64
	AudioBitrate int64
	Preset       string
}

func (p *TranscodeProfile) Validate() error {
	if p.Name == "" {
		return errors.New("transcode profile name is required")
	}
	if p.VideoCodec == "" {
		return errors.New("transcode profile video codec is required")
	}
	if p.Container == "" {
		return errors.New("transcode profile container is required")
	}
	return nil
}

type HWDevice struct {
	Type            string
	Name            string
	SupportedCodecs []string
}

type TranscodeProgress struct {
	JobID          string
	Percent        float64
	FPS            float64
	Speed          float64
	CurrentBitrate int64
	ETAMs          int64
}

type CacheFileInfo struct {
	Name         string
	Size         int64
	LastModified time.Time
}

type CacheInfo struct {
	TotalSize int64
	FileCount int
	Files     []CacheFileInfo
}

type HLSProfile struct {
	Name         string
	Width        int
	Height       int
	VideoBitrate int64 // kbps
	AudioBitrate int64 // kbps
	// Copy marks a video-COPY (remux) rung: the source H.264 stream is muxed
	// without re-encoding. Copy bytes differ from transcoded bytes at the SAME
	// WxH, so this MUST reach the cache fingerprint (ProfileFingerprint.Copy →
	// Hash) or copy and transcoded segments alias and corrupt the stream.
	Copy bool
	// CostModelled: VideoBitrate is the cost model's ceiling for this title
	// (margin included), so the encoder's distribution ceiling is the bitrate
	// itself rather than the class table's headroom above it. Different
	// bytes at the same nominal bitrate: fingerprinted and hashed.
	CostModelled bool
	// EfficiencyVersion is the CodecEfficiencyModel version that converted the
	// cost price to this route's encoder (0 = the reference price, or the
	// table). Hashed like CostModelled: a recalibration is new bytes.
	EfficiencyVersion int
}

// Must stay identical to LadderFilmNormal, which is the fallback ladder - the
// audio floors here follow the same measurements (see ladders.go).
var DefaultHLSProfiles = []HLSProfile{
	{Name: "360p", Width: 640, Height: 360, VideoBitrate: 800, AudioBitrate: 128},
	{Name: "480p", Width: 854, Height: 480, VideoBitrate: 1400, AudioBitrate: 192},
	{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 2800, AudioBitrate: 192},
	{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 5000, AudioBitrate: 192},
}
