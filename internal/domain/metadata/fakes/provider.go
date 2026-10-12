package fakes

import (
	"context"
	"fmt"
	"sync"

	"github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/domain/shared"
)

var ErrNotFound = shared.ErrNotFound

type FakeMetadataProvider struct {
	mu               sync.RWMutex
	ProviderName     string
	SearchResults    map[string][]metadata.MetadataResult
	ByProviderID     map[string]*metadata.MetadataResult
	EpisodeDetails   map[string]*metadata.MetadataResult // key: "seriesID:season:episode"
	SeasonDetails    map[string]*metadata.MetadataResult // key: "seriesID:season"
	ExternalLookups  map[string]*metadata.MetadataResult // key: "provider:id"
	AltTitlesResults map[string][]string                 // key: "mediaType:providerID"
	Err              error
}

func NewFakeMetadataProvider(name string) *FakeMetadataProvider {
	return &FakeMetadataProvider{
		ProviderName:     name,
		SearchResults:    make(map[string][]metadata.MetadataResult),
		ByProviderID:     make(map[string]*metadata.MetadataResult),
		EpisodeDetails:   make(map[string]*metadata.MetadataResult),
		SeasonDetails:    make(map[string]*metadata.MetadataResult),
		ExternalLookups:  make(map[string]*metadata.MetadataResult),
		AltTitlesResults: make(map[string][]string),
	}
}

func (p *FakeMetadataProvider) Name() string { return p.ProviderName }

func (p *FakeMetadataProvider) Search(_ context.Context, query string, _ *int, _ string) ([]metadata.MetadataResult, error) {
	if p.Err != nil {
		return nil, p.Err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.SearchResults[query], nil
}

func (p *FakeMetadataProvider) GetByProviderID(_ context.Context, providerID string, _ string) (*metadata.MetadataResult, error) {
	if p.Err != nil {
		return nil, p.Err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	r, ok := p.ByProviderID[providerID]
	if !ok {
		return nil, ErrNotFound
	}
	return r, nil
}

func (p *FakeMetadataProvider) GetEpisodeDetails(_ context.Context, seriesID string, seasonNum, episodeNum int) (*metadata.MetadataResult, error) {
	if p.Err != nil {
		return nil, p.Err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	key := seriesID + ":" + itoa(seasonNum) + ":" + itoa(episodeNum)
	r, ok := p.EpisodeDetails[key]
	if !ok {
		return nil, ErrNotFound
	}
	return r, nil
}

func (p *FakeMetadataProvider) GetSeasonDetails(_ context.Context, seriesID string, seasonNum int) (*metadata.MetadataResult, error) {
	if p.Err != nil {
		return nil, p.Err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	key := seriesID + ":" + itoa(seasonNum)
	r, ok := p.SeasonDetails[key]
	if !ok {
		return nil, ErrNotFound
	}
	return r, nil
}

func (p *FakeMetadataProvider) LookupByExternalID(_ context.Context, providerName, externalID string) (*metadata.MetadataResult, error) {
	if p.Err != nil {
		return nil, p.Err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	key := providerName + ":" + externalID
	r, ok := p.ExternalLookups[key]
	if !ok {
		return nil, ErrNotFound
	}
	return r, nil
}

func (p *FakeMetadataProvider) AlternativeTitles(_ context.Context, providerID string, mediaType string) ([]string, error) {
	if p.Err != nil {
		return nil, p.Err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.AltTitlesResults[mediaType+":"+providerID], nil
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}
