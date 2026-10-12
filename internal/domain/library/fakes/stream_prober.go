package fakes

import (
	"context"
	"sync/atomic"

	"github.com/eveyproject/evey/internal/domain/library"
)

type FakeStreamProber struct {
	Results map[string]*library.ProbeResult
	Err     error
	Calls   atomic.Int64
}

func NewFakeStreamProber() *FakeStreamProber {
	return &FakeStreamProber{Results: make(map[string]*library.ProbeResult)}
}

func (p *FakeStreamProber) ProbeStreams(_ context.Context, filePath string) (*library.ProbeResult, error) {
	p.Calls.Add(1)
	if p.Err != nil {
		return nil, p.Err
	}
	r := p.Results[filePath]
	if r == nil {
		return &library.ProbeResult{}, nil
	}
	return r, nil
}
