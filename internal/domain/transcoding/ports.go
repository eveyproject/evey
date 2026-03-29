package transcoding

import (
	"context"

	// library.MediaStream is used in HLSProcessOpts. Duplicating it here would
	// be worse: the struct has 20+ fields, is referenced in 24 files, and
	// transcoding inherently depends on stream metadata (codec, resolution, HDR).
	"github.com/gravity-zero/evey/internal/domain/library"
)

type Transcoder interface {
	Start(ctx context.Context, job *TranscodeJob) (string, error)
	Stop(ctx context.Context, jobID string) error
	Progress(ctx context.Context, jobID string) (*TranscodeProgress, error)
}

type HWDetector interface {
	DetectDevices(ctx context.Context) ([]HWDevice, error)
	SupportsCodec(ctx context.Context, deviceType, codec string) (bool, error)
}

type TranscodeJobRepository interface {
	Create(ctx context.Context, job *TranscodeJob) error
	GetByID(ctx context.Context, id string) (*TranscodeJob, error)
	Update(ctx context.Context, job *TranscodeJob) error
	ListActive(ctx context.Context) ([]*TranscodeJob, error)
	ListByMediaItem(ctx context.Context, mediaItemID string) ([]*TranscodeJob, error)
}

type TranscodeProfileRepository interface {
	GetByID(ctx context.Context, id string) (*TranscodeProfile, error)
	List(ctx context.Context) ([]*TranscodeProfile, error)
	Create(ctx context.Context, profile *TranscodeProfile) error
	Update(ctx context.Context, profile *TranscodeProfile) error
	Delete(ctx context.Context, id string) error
}

type CacheStore interface {
	GetCacheInfo(ctx context.Context) (*CacheInfo, error)
	ClearCache(ctx context.Context) error
	DeleteFile(ctx context.Context, filename string) error
}

// HLSProcess spawns and manages an ffmpeg HLS transcode process.
type HLSProcess interface {
	Start(ctx context.Context, opts HLSProcessOpts) (HLSProcessHandle, error)
}

// HLSProcessHandle controls a running HLS transcode.
type HLSProcessHandle interface {
	WaitReady(ctx context.Context) error
	Stop() error
	Done() <-chan struct{}
	Err() error
}

type HLSProcessOpts struct {
	OutputDir          string
	FilePath           string
	Profiles           []HLSProfile
	VideoDecision      string // "copy", "transcode", or "" (multi-variant)
	AudioDecision      string // "copy" or "transcode"
	AudioTrackIndex    int
	SubtitleTrackIndex int    // -1 = no subtitle
	SubtitleCodec      string
	HDRSupported       bool
	AudioNormalize     bool
	StartSec           float64
	Streams            []*library.MediaStream
}
