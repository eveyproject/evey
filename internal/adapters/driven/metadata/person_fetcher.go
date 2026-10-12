package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type TMDBPersonFetcher struct {
	tmdb *TMDBProvider
}

func NewTMDBPersonFetcher(tmdb *TMDBProvider) *TMDBPersonFetcher {
	return &TMDBPersonFetcher{tmdb: tmdb}
}

func (f *TMDBPersonFetcher) GetPersonDetails(ctx context.Context, name string) (*domainlibrary.PersonDetailsResult, error) {
	ctx, span := tracing.StartSpan(ctx, "metadata.person.GetPersonDetails")
	defer span.End()

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

// FetchPersonTranslations searches TMDB for a person by name, then fetches biography in each locale.
func (f *TMDBPersonFetcher) FetchPersonTranslations(ctx context.Context, name string, locales []string) (map[string]string, error) {
	ctx, span := tracing.StartSpan(ctx, "metadata.person.FetchPersonTranslations")
	defer span.End()

	searchURL := fmt.Sprintf("%s/search/person?query=%s&language=%s", f.tmdb.baseURL, url.QueryEscape(name), f.tmdb.language())
	body, err := f.tmdb.doRequestWithRetry(ctx, searchURL)
	if err != nil {
		return nil, err
	}
	var searchResp tmdbPersonSearchResult
	if err = json.Unmarshal(body, &searchResp); err != nil {
		return nil, err
	}
	if len(searchResp.Results) == 0 {
		return nil, nil
	}
	return f.tmdb.FetchPersonTranslations(ctx, strconv.Itoa(searchResp.Results[0].ID), locales)
}
