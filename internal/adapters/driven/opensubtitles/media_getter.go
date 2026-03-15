package opensubtitles

import (
	"context"

	appsubtitle "github.com/gravity-zero/evey/internal/app/subtitle"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
)

type libraryQuerySvc interface {
	GetMediaItem(ctx context.Context, id string) (*domainlibrary.MediaItem, error)
}

// LibraryMediaGetter adapts applibrary.QueryService to appsubtitle.MediaItemGetter.
type LibraryMediaGetter struct {
	q libraryQuerySvc
}

func NewLibraryMediaGetter(q libraryQuerySvc) appsubtitle.MediaItemGetter {
	return &LibraryMediaGetter{q: q}
}

func (g *LibraryMediaGetter) GetMediaItem(ctx context.Context, id string) (*appsubtitle.MediaItemInfo, error) {
	item, err := g.q.GetMediaItem(ctx, id)
	if err != nil {
		return nil, err
	}
	return &appsubtitle.MediaItemInfo{
		ID:       item.ID,
		FilePath: item.FilePath,
		Title:    item.Title,
	}, nil
}
