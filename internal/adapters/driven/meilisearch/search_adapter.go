package meilisearch

import (
	"context"
	"errors"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type SearchAdapter struct {
	url    string
	apiKey string
}

func NewSearchAdapter(url, apiKey string) *SearchAdapter {
	return &SearchAdapter{url: url, apiKey: apiKey}
}

func (a *SearchAdapter) IndexMediaItem(_ context.Context, _ *library.MediaItem) error {
	return errors.New("TODO: implement Meilisearch index")
}

func (a *SearchAdapter) RemoveMediaItem(_ context.Context, _ string) error {
	return errors.New("TODO: implement Meilisearch delete")
}

func (a *SearchAdapter) Search(_ context.Context, _ string, _ map[string]string) ([]string, error) {
	return nil, errors.New("TODO: implement Meilisearch search")
}
