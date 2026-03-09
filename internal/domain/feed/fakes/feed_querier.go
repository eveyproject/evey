package fakes

import (
	"context"

	"github.com/gravity-zero/evey/internal/domain/feed"
)

type FakeFeedQuerier struct {
	ContinueWatchingItems []*feed.FeedItem
	RecentlyAddedItems    []*feed.FeedItem
	RecommendedItems      []*feed.FeedItem
	SimilarItemsResult    []*feed.FeedItem
}

func NewFakeFeedQuerier() *FakeFeedQuerier {
	return &FakeFeedQuerier{}
}

func (q *FakeFeedQuerier) ContinueWatching(_ context.Context, _ string, _ int) ([]*feed.FeedItem, error) {
	return q.ContinueWatchingItems, nil
}

func (q *FakeFeedQuerier) RecentlyAdded(_ context.Context, _, _ int) ([]*feed.FeedItem, error) {
	return q.RecentlyAddedItems, nil
}

func (q *FakeFeedQuerier) Recommended(_ context.Context, _ string, _ []string, _ int) ([]*feed.FeedItem, error) {
	return q.RecommendedItems, nil
}

func (q *FakeFeedQuerier) SimilarItems(_ context.Context, _, _ string, _ int) ([]*feed.FeedItem, error) {
	return q.SimilarItemsResult, nil
}
