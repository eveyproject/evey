package subtitle

import "context"

type Repository interface {
	Save(ctx context.Context, sub *SubtitleDownload) error
	GetByMediaItem(ctx context.Context, mediaItemID string) ([]*SubtitleDownload, error)
	Delete(ctx context.Context, id string) error
}

type Provider interface {
	Search(ctx context.Context, hash, title, language string) ([]SearchResult, error)
	Download(ctx context.Context, fileID int, destPath string) (string, error)
}

type MediaItemGetter interface {
	GetMediaItem(ctx context.Context, id string) (*MediaItemInfo, error)
}
