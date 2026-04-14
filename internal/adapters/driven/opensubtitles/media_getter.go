package opensubtitles

import (
	"context"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/subtitle"
)

type libraryQuerySvc interface {
	GetMediaItem(ctx context.Context, id string) (*domainlibrary.MediaItem, error)
}

// LibraryMediaGetter adapts library query service to subtitle.MediaItemGetter.
type LibraryMediaGetter struct {
	q libraryQuerySvc
}

func NewLibraryMediaGetter(q libraryQuerySvc) subtitle.MediaItemGetter {
	return &LibraryMediaGetter{q: q}
}

func (g *LibraryMediaGetter) GetMediaItem(ctx context.Context, id string) (*subtitle.MediaItemInfo, error) {
	item, err := g.q.GetMediaItem(ctx, id)
	if err != nil {
		return nil, err
	}
	return &subtitle.MediaItemInfo{
		ID:       string(item.ID),
		FilePath: item.FilePath,
		Title:    item.Title,
	}, nil
}
