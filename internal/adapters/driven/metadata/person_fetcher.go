package metadata

import (
	"context"

	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
)

type TMDBPersonFetcher struct {
	tmdb *TMDBProvider
}

func NewTMDBPersonFetcher(tmdb *TMDBProvider) *TMDBPersonFetcher {
	return &TMDBPersonFetcher{tmdb: tmdb}
}

func (f *TMDBPersonFetcher) GetPersonDetails(ctx context.Context, name string) (*domainlibrary.PersonDetailsResult, error) {
	details, err := f.tmdb.GetPersonDetails(ctx, name)
	if err != nil || details == nil {
		return nil, err
	}
	return &domainlibrary.PersonDetailsResult{
		Biography:    details.Biography,
		Birthday:     details.Birthday,
		Deathday:     details.Deathday,
		PlaceOfBirth: details.PlaceOfBirth,
		KnownFor:     details.KnownFor,
		ThumbURL:     details.ThumbURL,
	}, nil
}
