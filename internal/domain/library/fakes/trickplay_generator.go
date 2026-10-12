package fakes

import (
	"context"

	"github.com/eveyproject/evey/internal/domain/library"
)

type FakeTrickplayGenerator struct {
	Results       map[string]*library.TrickplayResult
	Err           error
	LastOutputDir string
	// LastMaxWorkers is the decoder thread count the service asked for.
	LastMaxWorkers int
}

func NewFakeTrickplayGenerator() *FakeTrickplayGenerator {
	return &FakeTrickplayGenerator{Results: make(map[string]*library.TrickplayResult)}
}

func (g *FakeTrickplayGenerator) Generate(_ context.Context, filePath, outputDir string, interval, width, tileWidth, tileHeight, maxWorkers int, _ int64) (*library.TrickplayResult, error) {
	g.LastOutputDir = outputDir
	g.LastMaxWorkers = maxWorkers
	if g.Err != nil {
		return nil, g.Err
	}
	r := g.Results[filePath]
	if r == nil {
		// Provide a minimal but valid result (1 sprite, 1 frame at 0 ms).
		return &library.TrickplayResult{ImageDir: outputDir, ImageCount: 1, FrameTimestampsMs: []int64{0}}, nil
	}
	if len(r.FrameTimestampsMs) == 0 {
		// Synthesize uniform timestamps so callers that need a sidecar still
		// get a populated result.
		total := r.ImageCount * tileWidth * tileHeight
		ms := make([]int64, total)
		for i := range total {
			ms[i] = int64(i*interval) * 1000
		}
		r.FrameTimestampsMs = ms
	}
	return r, nil
}
