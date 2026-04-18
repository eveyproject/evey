package fakes

import (
	"context"
	"strings"
	"sync"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

type FakeMediaItemQueryRepo struct {
	mu    sync.RWMutex
	items map[string]*library.MediaItem
	cast  map[string][]library.CastMember
	Err   error
}

func NewFakeMediaItemQueryRepo() *FakeMediaItemQueryRepo {
	return &FakeMediaItemQueryRepo{
		items: make(map[string]*library.MediaItem),
		cast:  make(map[string][]library.CastMember),
	}
}

func (r *FakeMediaItemQueryRepo) GetByID(_ context.Context, id shared.MediaItemID) (*library.MediaItem, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	item, ok := r.items[string(id)]
	if !ok {
		return nil, ErrNotFound
	}
	return item, nil
}

func (r *FakeMediaItemQueryRepo) GetByIDs(_ context.Context, ids []shared.MediaItemID) ([]*library.MediaItem, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*library.MediaItem
	for _, id := range ids {
		if item, ok := r.items[string(id)]; ok {
			result = append(result, item)
		}
	}
	return result, nil
}

func allowedRatingsSet(allowed []string) map[string]bool {
	if len(allowed) == 0 {
		return nil
	}
	m := make(map[string]bool, len(allowed))
	for _, r := range allowed {
		m[strings.ToUpper(strings.TrimSpace(r))] = true
	}
	return m
}

func (r *FakeMediaItemQueryRepo) List(_ context.Context, filter library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	allowedSet := allowedRatingsSet(filter.AllowedRatings)
	var result []*library.MediaItem
	libIDsSet := make(map[string]bool, len(filter.LibraryIDs))
	for _, id := range filter.LibraryIDs {
		libIDsSet[string(id)] = true
	}
	for _, item := range r.items {
		if filter.LibraryID != nil && item.LibraryID != *filter.LibraryID {
			continue
		}
		if len(libIDsSet) > 0 && !libIDsSet[string(item.LibraryID)] {
			continue
		}
		if filter.ParentID != nil && item.ParentID != *filter.ParentID {
			continue
		}
		if filter.Type != nil && string(item.Type) != *filter.Type {
			continue
		}
		if allowedSet != nil {
			official := ""
			if item.Classification != nil {
				official = item.Classification.OfficialRating
			}
			rating := strings.ToUpper(strings.TrimSpace(official))
			if rating != "" && !allowedSet[rating] {
				continue
			}
		}
		if filter.YearFrom != 0 && (item.Year == nil || *item.Year < filter.YearFrom) {
			continue
		}
		if filter.YearTo != 0 && (item.Year == nil || *item.Year > filter.YearTo) {
			continue
		}
		if filter.MinRating != 0 {
			cr := classificationCommunityRating(item)
			if cr == nil || *cr < filter.MinRating {
				continue
			}
		}
		result = append(result, item)
	}
	if filter.Offset > 0 && filter.Offset < len(result) {
		result = result[filter.Offset:]
	} else if filter.Offset >= len(result) {
		return nil, nil
	}
	if filter.Limit > 0 && filter.Limit < len(result) {
		result = result[:filter.Limit]
	}
	return result, nil
}

func (r *FakeMediaItemQueryRepo) Count(_ context.Context, filter library.ListMediaItemsFilter) (int, error) {
	if r.Err != nil {
		return 0, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return 0, r.Err
	}
	allowedSet := allowedRatingsSet(filter.AllowedRatings)
	countLibIDsSet := make(map[string]bool, len(filter.LibraryIDs))
	for _, id := range filter.LibraryIDs {
		countLibIDsSet[string(id)] = true
	}
	count := 0
	for _, item := range r.items {
		if filter.LibraryID != nil && item.LibraryID != *filter.LibraryID {
			continue
		}
		if len(countLibIDsSet) > 0 && !countLibIDsSet[string(item.LibraryID)] {
			continue
		}
		if filter.Type != nil && string(item.Type) != *filter.Type {
			continue
		}
		if filter.ParentID != nil && item.ParentID != *filter.ParentID {
			continue
		}
		if allowedSet != nil {
			official := ""
			if item.Classification != nil {
				official = item.Classification.OfficialRating
			}
			rating := strings.ToUpper(strings.TrimSpace(official))
			if rating != "" && !allowedSet[rating] {
				continue
			}
		}
		if filter.YearFrom != 0 && (item.Year == nil || *item.Year < filter.YearFrom) {
			continue
		}
		if filter.YearTo != 0 && (item.Year == nil || *item.Year > filter.YearTo) {
			continue
		}
		if filter.MinRating != 0 {
			cr := classificationCommunityRating(item)
			if cr == nil || *cr < filter.MinRating {
				continue
			}
		}
		count++
	}
	return count, nil
}

func (r *FakeMediaItemQueryRepo) GetCastByMediaItem(_ context.Context, mediaItemID shared.MediaItemID) ([]library.CastMember, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	return r.cast[string(mediaItemID)], nil
}

func (r *FakeMediaItemQueryRepo) GetCastForItems(_ context.Context, mediaItemIDs []shared.MediaItemID) (map[shared.MediaItemID][]library.CastMember, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	result := make(map[shared.MediaItemID][]library.CastMember)
	for _, id := range mediaItemIDs {
		if cast, ok := r.cast[string(id)]; ok {
			result[id] = cast
		}
	}
	return result, nil
}

func (r *FakeMediaItemQueryRepo) SeriesAggregate(_ context.Context, seriesID shared.MediaItemID) (library.SeriesAggregate, error) {
	if r.Err != nil {
		return library.SeriesAggregate{}, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return library.SeriesAggregate{}, r.Err
	}
	visited := map[string]bool{string(seriesID): true}
	queue := []string{string(seriesID)}
	var agg library.SeriesAggregate
	seasons := make(map[string]bool)
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		for _, item := range r.items {
			if item.ParentID != parent {
				continue
			}
			if visited[string(item.ID)] {
				continue
			}
			visited[string(item.ID)] = true
			queue = append(queue, string(item.ID))
			if string(item.Type) == "episode" {
				if item.File != nil && item.File.DurationMs > 0 {
					agg.TotalDurationMs += item.File.DurationMs
				}
				agg.EpisodeCount++
				if item.ParentID != "" {
					seasons[item.ParentID] = true
				}
			}
		}
	}
	agg.SeasonCount = len(seasons)
	return agg, nil
}

func classificationCommunityRating(m *library.MediaItem) *float64 {
	if m == nil || m.Classification == nil {
		return nil
	}
	return m.Classification.CommunityRating
}

// Seed adds items for testing.
func (r *FakeMediaItemQueryRepo) Seed(item *library.MediaItem) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[string(item.ID)] = item
}

// SeedCast sets cast for a media item.
func (r *FakeMediaItemQueryRepo) SeedCast(mediaItemID string, cast []library.CastMember) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cast[mediaItemID] = cast
}
