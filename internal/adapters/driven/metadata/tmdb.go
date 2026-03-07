package metadata

import (
	"context"
	"errors"
	"net/http"

	"github.com/gravity-zero/evey/internal/domain/metadata"
)

type TMDBProvider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewTMDBProvider(apiKey, baseURL string) *TMDBProvider {
	return &TMDBProvider{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

func (p *TMDBProvider) Name() string { return "tmdb" }

func (p *TMDBProvider) Search(_ context.Context, _ string, _ string) ([]metadata.MetadataResult, error) {
	// TODO: call TMDB /search/movie or /search/tv API
	return nil, errors.New("TODO: implement TMDB search")
}

func (p *TMDBProvider) GetByProviderID(_ context.Context, _ string, _ string) (*metadata.MetadataResult, error) {
	// TODO: call TMDB /movie/{id} or /tv/{id} API
	return nil, errors.New("TODO: implement TMDB get by ID")
}
