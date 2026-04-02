package library

import (
	"context"
	"log/slog"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type QueryServiceOption func(*QueryService)

func WithPeople(people library.PersonRepository) QueryServiceOption {
	return func(s *QueryService) { s.people = people }
}

func WithPersonFetcher(fetcher library.PersonDetailsFetcher) QueryServiceOption {
	return func(s *QueryService) { s.personFetcher = fetcher }
}

func WithTranslations(
	mediaTranslations library.MediaItemTranslationRepository,
	personTranslations library.PersonTranslationRepository,
	personTransFetcher library.PersonTranslationFetcher,
	locales []string,
) QueryServiceOption {
	return func(s *QueryService) {
		s.mediaTranslations = mediaTranslations
		s.personTranslations = personTranslations
		s.personTransFetcher = personTransFetcher
		s.translationLocales = locales
	}
}

type QueryService struct {
	libraries            library.LibraryQueryRepo
	mediaItems           library.MediaItemQueryRepo
	streams              library.MediaStreamRepository
	search               library.SearchIndex
	people               library.PersonRepository
	personFetcher        library.PersonDetailsFetcher
	mediaTranslations    library.MediaItemTranslationRepository
	personTranslations   library.PersonTranslationRepository
	personTransFetcher   library.PersonTranslationFetcher
	translationLocales   []string
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
			// Fetch and store person translations
			if s.personTransFetcher != nil && s.personTranslations != nil && len(s.translationLocales) > 0 {
				if bios, bioErr := s.personTransFetcher.FetchPersonTranslations(ctx, person.Name, s.translationLocales); bioErr == nil {
					for locale, bio := range bios {
						_ = s.personTranslations.Upsert(ctx, &library.PersonTranslation{
							PersonID: person.ID, Locale: locale, Biography: bio,
						})
					}
				}
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
