package library

import (
	"context"
	"log/slog"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// CachingPersonRepo wraps a PersonRepository with a read-through cache that
// lazy-fills biography/details from TMDB on GetByID when missing, and persists
// the result. Writes are best-effort: failures are logged, not returned.
type CachingPersonRepo struct {
	library.PersonRepository
	fetcher            library.PersonDetailsFetcher
	transFetcher       library.PersonTranslationFetcher
	personTranslations library.PersonTranslationRepository
	locales            []string
}

func NewCachingPersonRepo(
	inner library.PersonRepository,
	fetcher library.PersonDetailsFetcher,
	transFetcher library.PersonTranslationFetcher,
	personTranslations library.PersonTranslationRepository,
	locales []string,
) *CachingPersonRepo {
	return &CachingPersonRepo{
		PersonRepository:   inner,
		fetcher:            fetcher,
		transFetcher:       transFetcher,
		personTranslations: personTranslations,
		locales:            locales,
	}
}

func (r *CachingPersonRepo) GetByID(ctx context.Context, id string) (*library.Person, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.CachingPersonRepo.GetByID")
	defer span.End()
	person, err := r.PersonRepository.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if person.Biography != "" || r.fetcher == nil {
		return person, nil
	}
	details, fetchErr := r.fetcher.GetPersonDetails(ctx, person.Name)
	if fetchErr != nil || details == nil || details.Biography == "" {
		return person, nil
	}
	person.Biography = details.Biography
	person.Birthday = details.Birthday
	person.Deathday = details.Deathday
	person.PlaceOfBirth = details.PlaceOfBirth
	person.KnownFor = details.KnownFor
	if details.ThumbURL != "" && person.ThumbPath == nil {
		person.ThumbPath = &details.ThumbURL
	}
	if updateErr := r.Update(ctx, person); updateErr != nil {
		slog.WarnContext(ctx, "cache person bio failed", slog.String("personID", person.ID), slog.Any("err", updateErr))
	}
	if r.transFetcher != nil && r.personTranslations != nil && len(r.locales) > 0 {
		if bios, bioErr := r.transFetcher.FetchPersonTranslations(ctx, person.Name, r.locales); bioErr == nil {
			for locale, bio := range bios {
				if uErr := r.personTranslations.Upsert(ctx, &library.PersonTranslation{
					PersonID: person.ID, Locale: locale, Biography: bio,
				}); uErr != nil {
					slog.WarnContext(ctx, "upsert person translation failed", slog.String("personID", person.ID), slog.String("locale", locale), slog.Any("err", uErr))
				}
			}
		}
	}
	return person, nil
}
