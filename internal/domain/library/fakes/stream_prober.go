package fakes

import (
	"context"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type FakeStreamProber struct {
	Results map[string]*library.ProbeResult
	Err     error
}

func NewFakeStreamProber() *FakeStreamProber {
	return &FakeStreamProber{Results: make(map[string]*library.ProbeResult)}
}

func (p *FakeStreamProber) ProbeStreams(_ context.Context, filePath string) (*library.ProbeResult, error) {
	if p.Err != nil {
		return nil, p.Err
	}
	r := p.Results[filePath]
	if r == nil {
		return &library.ProbeResult{}, nil
	}
	return r, nil
}
