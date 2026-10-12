package fakes

import (
	"context"
	"fmt"
	"sync"

	"github.com/eveyproject/evey/internal/domain/subtitle"
)

type FakeProvider struct {
	mu      sync.RWMutex
	Results []subtitle.SearchResult
	Err     error
	// DownloadPath returned by Download; empty triggers default
	DownloadPath string
}

func NewFakeProvider() *FakeProvider {
	return &FakeProvider{}
}

func (p *FakeProvider) Search(_ context.Context, _, _, _ string) ([]subtitle.SearchResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.Err != nil {
		return nil, p.Err
	}
	return p.Results, nil
}

func (p *FakeProvider) Download(_ context.Context, fileID int, destPath string) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.Err != nil {
		return "", p.Err
	}
	if p.DownloadPath != "" {
		return p.DownloadPath, nil
	}
	return fmt.Sprintf("%s/%d.srt", destPath, fileID), nil
}
