package transcoding

import "context"

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
	Delete(ctx context.Context, id string) error
}

type CacheStore interface {
	GetCacheInfo(ctx context.Context) (*CacheInfo, error)
	ClearCache(ctx context.Context) error
	DeleteFile(ctx context.Context, filename string) error
}
