package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

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

// FetchPersonTranslations searches TMDB for a person by name, then fetches biography in each locale.
func (f *TMDBPersonFetcher) FetchPersonTranslations(ctx context.Context, name string, locales []string) (map[string]string, error) {
	searchURL := fmt.Sprintf("%s/search/person?query=%s&language=%s", f.tmdb.baseURL, url.QueryEscape(name), f.tmdb.language)
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
	return f.tmdb.FetchPersonTranslations(ctx, searchResp.Results[0].ID, locales)
}
