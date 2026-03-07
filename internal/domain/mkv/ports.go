package mkv

import "context"

type ContainerReader interface {
	Open(ctx context.Context, path string) (*Container, error)
	ReadTracks(ctx context.Context, path string) ([]Track, error)
	ReadChapters(ctx context.Context, path string) ([]Chapter, error)
	ReadAttachments(ctx context.Context, path string) ([]Attachment, error)
	ReadTags(ctx context.Context, path string) ([]Tag, error)
}

type ContainerWriter interface {
	Mux(ctx context.Context, opts MuxOptions) error
	Demux(ctx context.Context, path string, opts DemuxOptions) error
	AddTrack(ctx context.Context, containerPath string, input TrackInput) error
	RemoveTrack(ctx context.Context, containerPath string, trackID uint64) error
	ExtractAttachment(ctx context.Context, containerPath string, attachmentID uint64, destPath string) error
}
