package fakes

import (
	"context"
	"strings"

	"github.com/eveyproject/evey/internal/domain/library"
)

type FakeCollectionSmartQuerier struct {
	Items      []*library.MediaItem
	Err        error
	LastSortBy string
}

func NewFakeCollectionSmartQuerier() *FakeCollectionSmartQuerier {
	return &FakeCollectionSmartQuerier{}
}

func (q *FakeCollectionSmartQuerier) QueryByRules(_ context.Context, rules []library.CollectionRule, sortBy, _sortOrder string, limit, offset int) ([]*library.MediaItem, error) {
	q.LastSortBy = sortBy
	if q.Err != nil {
		return nil, q.Err
	}
	var result []*library.MediaItem
	for _, item := range q.Items {
		if matchesCollectionRules(item, rules) {
			result = append(result, item)
		}
	}
	if offset > 0 {
		if offset >= len(result) {
			return nil, nil
		}
		result = result[offset:]
	}
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (q *FakeCollectionSmartQuerier) CountByRules(_ context.Context, rules []library.CollectionRule) (int, error) {
	if q.Err != nil {
		return 0, q.Err
	}
	count := 0
	for _, item := range q.Items {
		if matchesCollectionRules(item, rules) {
			count++
		}
	}
	return count, nil
}

func matchesCollectionRules(item *library.MediaItem, rules []library.CollectionRule) bool {
	for _, rule := range rules {
		if !matchCollectionRule(item, rule) {
			return false
		}
	}
	return true
}

func matchCollectionRule(item *library.MediaItem, rule library.CollectionRule) bool {
	switch rule.Field {
	case "genre":
		if item.Catalog == nil {
			return false
		}
		for _, g := range item.Catalog.Genres {
			if strings.EqualFold(g, rule.Value) {
				return true
			}
		}
		return false
	case "type":
		return string(item.Type) == rule.Value
	default:
		// Unknown field: deny. Returning true here silently matched every
		// item against unsupported rule fields, which in tests hid real
		// typos (e.g. "genres" instead of "genre") behind passing queries.
		return false
	}
}
