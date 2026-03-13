package fakes

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type FakeCollectionQueryRepo struct {
	mu          sync.RWMutex
	collections map[string]*library.Collection
	items       map[string][]string
	mediaItems  *FakeMediaItemRepository
}

func NewFakeCollectionQueryRepo(cmdRepo *FakeCollectionCommandRepo, mediaItems *FakeMediaItemRepository) *FakeCollectionQueryRepo {
	return &FakeCollectionQueryRepo{
		collections: cmdRepo.collections,
		items:       cmdRepo.items,
		mediaItems:  mediaItems,
	}
}

func (r *FakeCollectionQueryRepo) GetByID(_ context.Context, id string) (*library.Collection, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	col, ok := r.collections[id]
	if !ok {
		return nil, ErrNotFound
	}
	return col, nil
}

func (r *FakeCollectionQueryRepo) List(_ context.Context) ([]*library.Collection, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*library.Collection, 0, len(r.collections))
	for _, col := range r.collections {
		result = append(result, col)
	}
	return result, nil
}

func (r *FakeCollectionQueryRepo) GetManualItems(_ context.Context, collectionID string, limit, offset int) ([]*library.MediaItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := r.items[collectionID]
	var result []*library.MediaItem
	for _, id := range ids {
		if item, ok := r.mediaItems.items[id]; ok {
			result = append(result, item)
		}
	}
	if offset > 0 && offset < len(result) {
		result = result[offset:]
	} else if offset >= len(result) {
		return nil, nil
	}
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

func (r *FakeCollectionQueryRepo) CountManualItems(_ context.Context, collectionID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.items[collectionID]), nil
}

func (r *FakeCollectionQueryRepo) GetSmartItems(_ context.Context, col *library.Collection, limit, offset int) ([]*library.MediaItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*library.MediaItem
	for _, item := range r.mediaItems.items {
		if matchesRules(item, col.Rules) {
			result = append(result, item)
		}
	}
	if offset > 0 && offset < len(result) {
		result = result[offset:]
	} else if offset >= len(result) {
		return nil, nil
	}
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

func (r *FakeCollectionQueryRepo) CountSmartItems(_ context.Context, col *library.Collection) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, item := range r.mediaItems.items {
		if matchesRules(item, col.Rules) {
			count++
		}
	}
	return count, nil
}

func matchesRules(item *library.MediaItem, rules []library.CollectionRule) bool {
	for _, rule := range rules {
		if !matchesRule(item, rule) {
			return false
		}
	}
	return true
}

func matchesRule(item *library.MediaItem, rule library.CollectionRule) bool {
	switch rule.Field {
	case "genre":
		genresJSON, _ := json.Marshal(item.Genres)
		gs := string(genresJSON)
		switch rule.Operator {
		case library.OpContains:
			for _, g := range item.Genres {
				if strings.EqualFold(g, rule.Value) {
					return true
				}
			}
			return false
		case library.OpEq:
			return strings.Contains(gs, rule.Value)
		}
	case "year":
		if item.Year == nil {
			return false
		}
		v, _ := strconv.Atoi(rule.Value)
		switch rule.Operator {
		case library.OpEq:
			return *item.Year == v
		case library.OpGt:
			return *item.Year > v
		case library.OpLt:
			return *item.Year < v
		}
	case "type":
		switch rule.Operator {
		case library.OpEq:
			return item.Type == rule.Value
		case library.OpNeq:
			return item.Type != rule.Value
		}
	case "rating":
		if item.CommunityRating == nil {
			return false
		}
		v, _ := strconv.ParseFloat(rule.Value, 64)
		switch rule.Operator {
		case library.OpGt:
			return *item.CommunityRating > v
		case library.OpLt:
			return *item.CommunityRating < v
		case library.OpEq:
			return *item.CommunityRating == v
		}
	}
	return false
}
