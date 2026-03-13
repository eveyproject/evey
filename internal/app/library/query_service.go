package library

import (
	"context"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type QueryService struct {
	libraries   library.LibraryQueryRepo
	mediaItems  library.MediaItemQueryRepo
	streams     library.MediaStreamRepository
	search      library.SearchIndex
}

func NewQueryService(
	libraries library.LibraryQueryRepo,
	mediaItems library.MediaItemQueryRepo,
	streams library.MediaStreamRepository,
	search library.SearchIndex,
) *QueryService {
	return &QueryService{
		libraries:  libraries,
		mediaItems: mediaItems,
		streams:    streams,
		search:     search,
	}
}

func (s *QueryService) GetLibrary(ctx context.Context, id string) (*library.Library, error) {
	return s.libraries.GetByID(ctx, id)
}

func (s *QueryService) ListLibraries(ctx context.Context) ([]*library.Library, error) {
	return s.libraries.List(ctx)
}

func (s *QueryService) GetMediaItem(ctx context.Context, id string) (*library.MediaItem, error) {
	return s.mediaItems.GetByID(ctx, id)
}

func (s *QueryService) ListMediaItems(ctx context.Context, filter library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	return s.mediaItems.List(ctx, filter)
}

func (s *QueryService) CountMediaItems(ctx context.Context, filter library.ListMediaItemsFilter) (int, error) {
	return s.mediaItems.Count(ctx, filter)
}

func (s *QueryService) GetCast(ctx context.Context, mediaItemID string) ([]library.CastMember, error) {
	return s.mediaItems.GetCastByMediaItem(ctx, mediaItemID)
}

func (s *QueryService) GetCastForItems(ctx context.Context, mediaItemIDs []string) (map[string][]library.CastMember, error) {
	return s.mediaItems.GetCastForItems(ctx, mediaItemIDs)
}

func (s *QueryService) GetMediaStreams(ctx context.Context, mediaItemID string) ([]*library.MediaStream, error) {
	return s.streams.GetByMediaItemID(ctx, mediaItemID)
}

func (s *QueryService) SearchMedia(ctx context.Context, query string, filters map[string]string, limit int64) ([]library.SearchHit, int64, error) {
	return s.search.Search(ctx, query, filters, limit)
}
