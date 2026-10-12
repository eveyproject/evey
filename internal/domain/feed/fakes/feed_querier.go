package fakes

import (
	"context"

	"github.com/eveyproject/evey/internal/domain/feed"
)

type FakeFeedQuerier struct {
	ContinueWatchingItems []*feed.FeedItem
	ResumePlaybackItems   []*feed.FeedItem
	NextEpisodeItems      []*feed.FeedItem
	RecentlyAddedItems    []*feed.FeedItem
	RecommendedItems      []*feed.FeedItem
	SimilarItemsResult    []*feed.FeedItem
	Err                   error
}

func NewFakeFeedQuerier() *FakeFeedQuerier {
	return &FakeFeedQuerier{}
}

func (q *FakeFeedQuerier) ContinueWatching(_ context.Context, _ string, _ int) ([]*feed.FeedItem, error) {
	if q.Err != nil {
		return nil, q.Err
	}
	return q.ContinueWatchingItems, nil
}

func (q *FakeFeedQuerier) ResumePlayback(_ context.Context, _ string, _ int) ([]*feed.FeedItem, error) {
	if q.Err != nil {
		return nil, q.Err
	}
	return q.ResumePlaybackItems, nil
}

func (q *FakeFeedQuerier) NextEpisode(_ context.Context, _ string, _ int) ([]*feed.FeedItem, error) {
	if q.Err != nil {
		return nil, q.Err
	}
	return q.NextEpisodeItems, nil
}

func (q *FakeFeedQuerier) RecentlyAdded(_ context.Context, _ string, _, _ int) ([]*feed.FeedItem, error) {
	if q.Err != nil {
		return nil, q.Err
	}
	return q.RecentlyAddedItems, nil
}

func (q *FakeFeedQuerier) Recommended(_ context.Context, _ string, _ []string, _ int) ([]*feed.FeedItem, error) {
	if q.Err != nil {
		return nil, q.Err
	}
	return q.RecommendedItems, nil
}

func (q *FakeFeedQuerier) SimilarItems(_ context.Context, _, _ string, _ int) ([]*feed.FeedItem, error) {
	if q.Err != nil {
		return nil, q.Err
	}
	return q.SimilarItemsResult, nil
}
