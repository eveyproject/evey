package fakes

import (
	"context"
	"sort"
	"strings"

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/playlist"
)

type FakeSmartPlaylistQuerier struct {
	Items []*library.MediaItem
	Err   error
}

func NewFakeSmartPlaylistQuerier() *FakeSmartPlaylistQuerier {
	return &FakeSmartPlaylistQuerier{}
}

func (q *FakeSmartPlaylistQuerier) QueryByRules(_ context.Context, rules []playlist.Rule, sortBy, sortOrder string, limit int) ([]*library.MediaItem, error) {
	if q.Err != nil {
		return nil, q.Err
	}
	var result []*library.MediaItem
	for _, item := range q.Items {
		if matchesRules(item, rules) {
			result = append(result, item)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		switch sortBy {
		case "title":
			if sortOrder == "desc" {
				return result[i].Title > result[j].Title
			}
			return result[i].Title < result[j].Title
		default:
			return result[i].AddedAt.Before(result[j].AddedAt)
		}
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func matchesRules(item *library.MediaItem, rules []playlist.Rule) bool {
	for _, rule := range rules {
		if !matchRule(item, rule) {
			return false
		}
	}
	return true
}

func matchRule(item *library.MediaItem, rule playlist.Rule) bool {
	switch rule.Field {
	case "genre":
		for _, g := range item.Genres {
			if strings.EqualFold(g, rule.Value) {
				return true
			}
		}
		return false
	case "type":
		return item.Type == rule.Value
	case "year":
		if item.Year == nil {
			return false
		}
		return strings.Contains(rule.Value, string(rune('0'+*item.Year%10)))
	default:
		return true
	}
}
