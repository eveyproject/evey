package mkv

import "context"

type ContainerReader interface {
	Open(ctx context.Context, path string) (*Container, error)
	ReadTracks(ctx context.Context, path string) ([]Track, error)
	ReadChapters(ctx context.Context, path string) ([]Chapter, error)
	ReadAttachments(ctx context.Context, path string) ([]Attachment, error)
	ReadTags(ctx context.Context, path string) ([]Tag, error)
}
