package library

import (
	"context"
	"errors"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

type QueryServiceOption func(*QueryService)

func WithPeople(people library.PersonRepository) QueryServiceOption {
	return func(s *QueryService) { s.people = people }
}

func WithChapters(chapters library.ChapterRepository) QueryServiceOption {
	return func(s *QueryService) { s.chapters = chapters }
}

func WithTrickplay(trickplay library.TrickplayRepository) QueryServiceOption {
	return func(s *QueryService) { s.trickplay = trickplay }
}

func WithTranslations(
	mediaTranslations library.MediaItemTranslationRepository,
	personTranslations library.PersonTranslationRepository,
) QueryServiceOption {
	return func(s *QueryService) {
		s.mediaTranslations = mediaTranslations
		s.personTranslations = personTranslations
	}
}

type QueryService struct {
	libraries          library.LibraryQueryRepo
	mediaItems         library.MediaItemQueryRepo
	streams            library.MediaStreamRepository
	search             library.SearchIndex
	people             library.PersonRepository
	mediaTranslations  library.MediaItemTranslationRepository
	personTranslations library.PersonTranslationRepository
	chapters           library.ChapterRepository
	trickplay          library.TrickplayRepository
}

func NewQueryService(
	libraries library.LibraryQueryRepo,
	mediaItems library.MediaItemQueryRepo,
	streams library.MediaStreamRepository,
	search library.SearchIndex,
	opts ...QueryServiceOption,
) *QueryService {
	s := &QueryService{
		libraries:  libraries,
		mediaItems: mediaItems,
		streams:    streams,
		search:     search,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *QueryService) GetMediaItemTranslation(ctx context.Context, mediaItemID, locale string) (*library.MediaItemTranslation, error) {
	if s.mediaTranslations == nil {
		return nil, nil
	}
	return s.mediaTranslations.GetByMediaItemAndLocale(ctx, mediaItemID, locale)
}

func (s *QueryService) GetPersonTranslation(ctx context.Context, personID, locale string) (*library.PersonTranslation, error) {
	if s.personTranslations == nil {
		return nil, nil
	}
	return s.personTranslations.GetByPersonAndLocale(ctx, personID, locale)
}

func (s *QueryService) GetPerson(ctx context.Context, id string) (*library.Person, error) {
	if s.people == nil {
		return nil, nil
	}
	return s.people.GetByID(ctx, id)
}

func (s *QueryService) GetLibrary(ctx context.Context, id string) (*library.Library, error) {
	return s.libraries.GetByID(ctx, shared.LibraryID(id))
}

func (s *QueryService) ListLibraries(ctx context.Context) ([]*library.Library, error) {
	return s.libraries.List(ctx)
}

func (s *QueryService) GetMediaItem(ctx context.Context, id string) (*library.MediaItem, error) {
	return s.mediaItems.GetByID(ctx, shared.MediaItemID(id))
}

func (s *QueryService) GetMediaItemsByIDs(ctx context.Context, ids []string) ([]*library.MediaItem, error) {
	typedIDs := make([]shared.MediaItemID, len(ids))
	for i, id := range ids {
		typedIDs[i] = shared.MediaItemID(id)
	}
	return s.mediaItems.GetByIDs(ctx, typedIDs)
}

func (s *QueryService) ListMediaItems(ctx context.Context, filter library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	return s.mediaItems.List(ctx, filter)
}

func (s *QueryService) CountMediaItems(ctx context.Context, filter library.ListMediaItemsFilter) (int, error) {
	return s.mediaItems.Count(ctx, filter)
}

func (s *QueryService) GetCast(ctx context.Context, mediaItemID string) ([]library.CastMember, error) {
	return s.mediaItems.GetCastByMediaItem(ctx, shared.MediaItemID(mediaItemID))
}

func (s *QueryService) GetCastForItems(ctx context.Context, mediaItemIDs []string) (map[string][]library.CastMember, error) {
	typedIDs := make([]shared.MediaItemID, len(mediaItemIDs))
	for i, id := range mediaItemIDs {
		typedIDs[i] = shared.MediaItemID(id)
	}
	typed, err := s.mediaItems.GetCastForItems(ctx, typedIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]library.CastMember, len(typed))
	for k, v := range typed {
		out[string(k)] = v
	}
	return out, nil
}

func (s *QueryService) GetSeriesAggregate(ctx context.Context, seriesID string) (library.SeriesAggregate, error) {
	return s.mediaItems.SeriesAggregate(ctx, shared.MediaItemID(seriesID))
}

func (s *QueryService) GetMediaStreams(ctx context.Context, mediaItemID string) ([]*library.MediaStream, error) {
	return s.streams.GetByMediaItemID(ctx, shared.MediaItemID(mediaItemID))
}

func (s *QueryService) GetChapters(ctx context.Context, mediaItemID string) ([]*library.Chapter, error) {
	if s.chapters == nil {
		return []*library.Chapter{}, nil
	}
	return s.chapters.GetByMediaItemID(ctx, shared.MediaItemID(mediaItemID))
}

func (s *QueryService) GetTrickplayManifest(ctx context.Context, mediaItemID string) (*library.TrickplayManifest, error) {
	if s.trickplay == nil {
		return nil, errors.New("trickplay not configured")
	}
	return s.trickplay.GetByMediaItemID(ctx, shared.MediaItemID(mediaItemID))
}

func (s *QueryService) SearchMedia(ctx context.Context, query string, filters map[string]string, sort []string, limit int64) ([]library.SearchHit, int64, error) {
	return s.search.Search(ctx, query, filters, sort, limit)
}
