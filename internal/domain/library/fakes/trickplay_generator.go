package fakes

import (
	"context"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type FakeTrickplayGenerator struct {
	Results       map[string]*library.TrickplayResult
	Err           error
	LastOutputDir string
}

func NewFakeTrickplayGenerator() *FakeTrickplayGenerator {
	return &FakeTrickplayGenerator{Results: make(map[string]*library.TrickplayResult)}
}

func (g *FakeTrickplayGenerator) Generate(_ context.Context, filePath, outputDir string, interval, width, tileWidth, tileHeight int) (*library.TrickplayResult, error) {
	g.LastOutputDir = outputDir
	if g.Err != nil {
		return nil, g.Err
	}
	r := g.Results[filePath]
	if r == nil {
		return &library.TrickplayResult{ImageDir: outputDir, ImageCount: 1}, nil
	}
	return r, nil
}
