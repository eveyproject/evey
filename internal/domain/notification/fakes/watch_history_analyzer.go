package fakes

import (
	"context"
)

type FakeWatchHistoryAnalyzer struct {
	Genres    []string
	Watched   bool
	Watching  bool
	Relevance int
	Err       error
}

func NewFakeWatchHistoryAnalyzer() *FakeWatchHistoryAnalyzer {
	return &FakeWatchHistoryAnalyzer{}
}

func (f *FakeWatchHistoryAnalyzer) GetFavoriteGenres(_ context.Context, _ string, _ int) ([]string, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Genres, nil
}

func (f *FakeWatchHistoryAnalyzer) GetFavoriteGenresBatch(_ context.Context, profileIDs []string, _ int) (map[string][]string, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	result := make(map[string][]string, len(profileIDs))
	for _, pid := range profileIDs {
		result[pid] = f.Genres
	}
	return result, nil
}

func (f *FakeWatchHistoryAnalyzer) HasWatchedSeries(_ context.Context, _, _ string) (bool, error) {
	if f.Err != nil {
		return false, f.Err
	}
	return f.Watched, nil
}

func (f *FakeWatchHistoryAnalyzer) IsCurrentlyWatchingSeries(_ context.Context, _, _ string) (bool, error) {
	if f.Err != nil {
		return false, f.Err
	}
	return f.Watching, nil
}

func (f *FakeWatchHistoryAnalyzer) CalculateRelevanceScore(_ context.Context, _ string, _ []string) (int, error) {
	if f.Err != nil {
		return 0, f.Err
	}
	return f.Relevance, nil
}
