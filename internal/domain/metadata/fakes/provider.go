package fakes

import (
	"context"
	"errors"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/metadata"
)

var ErrNotFound = errors.New("not found")

type FakeMetadataProvider struct {
	mu            sync.RWMutex
	ProviderName  string
	SearchResults map[string][]metadata.MetadataResult
	ByProviderID  map[string]*metadata.MetadataResult
}

func NewFakeMetadataProvider(name string) *FakeMetadataProvider {
	return &FakeMetadataProvider{
		ProviderName:  name,
		SearchResults: make(map[string][]metadata.MetadataResult),
		ByProviderID:  make(map[string]*metadata.MetadataResult),
	}
}

func (p *FakeMetadataProvider) Name() string { return p.ProviderName }

func (p *FakeMetadataProvider) Search(_ context.Context, query string, _ string) ([]metadata.MetadataResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.SearchResults[query], nil
}

func (p *FakeMetadataProvider) GetByProviderID(_ context.Context, providerID string, _ string) (*metadata.MetadataResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	r, ok := p.ByProviderID[providerID]
	if !ok {
		return nil, ErrNotFound
	}
	return r, nil
}
