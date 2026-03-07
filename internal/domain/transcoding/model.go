package transcoding

import "time"

type HWAccelType string

const (
	VAAPI HWAccelType = "vaapi"
	NVENC HWAccelType = "nvenc"
	QSV   HWAccelType = "qsv"
	None  HWAccelType = "none"
)

type JobStatus string

const (
	Queued    JobStatus = "queued"
	Running   JobStatus = "running"
	Completed JobStatus = "completed"
	Failed    JobStatus = "failed"
	Cancelled JobStatus = "cancelled"
)

type TranscodeJob struct {
	ID            string
	MediaItemID   string
	InputPath     string
	InputCodec    string
	OutputCodec   string
	HWAccelType   string
	Preset        string
	ResolutionIn  string
	ResolutionOut string
	BitrateIn     int64
	BitrateOut    int64
	Speed         float64
	Status        string
	ErrorMessage  string
	StartedAt     time.Time
	EndedAt       *time.Time
	DurationMs    *int64
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
