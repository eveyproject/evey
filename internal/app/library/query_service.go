package library

import (
	"context"
	"log/slog"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type PersonDetailsFetcher interface {
	GetPersonDetails(ctx context.Context, name string) (*PersonDetailsResult, error)
}

type PersonDetailsResult struct {
	Biography    string
	Birthday     string
	Deathday     string
	PlaceOfBirth string
	KnownFor     string
	ThumbURL     string
}

type QueryService struct {
	libraries     library.LibraryQueryRepo
	mediaItems    library.MediaItemQueryRepo
	streams       library.MediaStreamRepository
	search        library.SearchIndex
	people        library.PersonRepository
	personFetcher PersonDetailsFetcher
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

func (s *QueryService) WithPeople(people library.PersonRepository) *QueryService {
	s.people = people
	return s
}

func (s *QueryService) WithPersonFetcher(fetcher PersonDetailsFetcher) *QueryService {
	s.personFetcher = fetcher
	return s
}

func (s *QueryService) GetPerson(ctx context.Context, id string) (*library.Person, error) {
	if s.people == nil {
		return nil, nil
	}
	person, err := s.people.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// Lazy-fetch bio from TMDB if empty
	if person.Biography == "" && s.personFetcher != nil {
		details, fetchErr := s.personFetcher.GetPersonDetails(ctx, person.Name)
		if fetchErr == nil && details != nil && details.Biography != "" {
			person.Biography = details.Biography
			person.Birthday = details.Birthday
			person.Deathday = details.Deathday
			person.PlaceOfBirth = details.PlaceOfBirth
			person.KnownFor = details.KnownFor
			if details.ThumbURL != "" && person.ThumbPath == nil {
				person.ThumbPath = &details.ThumbURL
			}
			if updateErr := s.people.Update(ctx, person); updateErr != nil {
				slog.Warn("cache person bio failed", slog.String("personID", person.ID), slog.Any("err", updateErr))
			}
		}
	}
	return person, nil
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

func (s *QueryService) GetMediaItemsByIDs(ctx context.Context, ids []string) ([]*library.MediaItem, error) {
	return s.mediaItems.GetByIDs(ctx, ids)
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

func (s *QueryService) SearchMedia(ctx context.Context, query string, filters map[string]string, sort []string, limit int64) ([]library.SearchHit, int64, error) {
	return s.search.Search(ctx, query, filters, sort, limit)
}
