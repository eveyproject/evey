package fakes

import (
	"context"
)

type FakeWatchHistoryAnalyzer struct {
	Genres    []string
	Watched   bool
	Watching  bool
	Relevance int
}

func NewFakeWatchHistoryAnalyzer() *FakeWatchHistoryAnalyzer {
	return &FakeWatchHistoryAnalyzer{}
}

func (f *FakeWatchHistoryAnalyzer) GetFavoriteGenres(_ context.Context, _ string, _ int) ([]string, error) {
	return f.Genres, nil
}

func (f *FakeWatchHistoryAnalyzer) HasWatchedSeries(_ context.Context, _, _ string) (bool, error) {
	return f.Watched, nil
}

func (f *FakeWatchHistoryAnalyzer) IsCurrentlyWatchingSeries(_ context.Context, _, _ string) (bool, error) {
	return f.Watching, nil
}

func (f *FakeWatchHistoryAnalyzer) CalculateRelevanceScore(_ context.Context, _ string, _ []string) (int, error) {
	return f.Relevance, nil
}
