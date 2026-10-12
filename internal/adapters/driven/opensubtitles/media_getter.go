package opensubtitles

import (
	"context"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/subtitle"
	"github.com/eveyproject/evey/internal/pkg/tracing"
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
	ctx, span := tracing.StartSpan(ctx, "opensubtitles.mediagetter.GetMediaItem")
	defer span.End()

	item, err := g.q.GetMediaItem(ctx, id)
	if err != nil {
		return nil, err
	}
	info := &subtitle.MediaItemInfo{
		ID:    string(item.ID),
		Title: item.Title,
	}
	if item.File != nil {
		info.FilePath = item.File.Path
	}
	return info, nil
}
