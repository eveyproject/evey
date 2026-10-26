package fakes

import (
	"context"
	"sort"
	"sync"

	"github.com/eveyproject/evey/internal/domain/playlist"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type FakePlaylistItemRepository struct {
	mu    sync.RWMutex
	items map[string][]*playlist.PlaylistItem // playlistID -> items
	Err   error
	purge int
}

func NewFakePlaylistItemRepository() *FakePlaylistItemRepository {
	return &FakePlaylistItemRepository{items: make(map[string][]*playlist.PlaylistItem)}
}

func (r *FakePlaylistItemRepository) Add(_ context.Context, item *playlist.PlaylistItem) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.items[item.PlaylistID] {
		if existing.MediaItemID == item.MediaItemID {
			return nil
		}
	}
	r.items[item.PlaylistID] = append(r.items[item.PlaylistID], item)
	return nil
}

func (r *FakePlaylistItemRepository) AddNextOrder(_ context.Context, item *playlist.PlaylistItem, maxItems int) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	next := 0
	for _, existing := range r.items[item.PlaylistID] {
		if existing.MediaItemID == item.MediaItemID {
			return nil // ON CONFLICT DO NOTHING
		}
		if existing.SortOrder >= next {
			next = existing.SortOrder + 1
		}
	}
	if maxItems > 0 && len(r.items[item.PlaylistID]) >= maxItems {
		return playlist.ErrMaxItemsReached
	}
	item.SortOrder = next
	r.items[item.PlaylistID] = append(r.items[item.PlaylistID], item)
	return nil
}

func (r *FakePlaylistItemRepository) Remove(_ context.Context, playlistID, mediaItemID string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	items := r.items[playlistID]
	for i, it := range items {
		if string(it.MediaItemID) == mediaItemID {
			r.items[playlistID] = append(items[:i], items[i+1:]...)
			return nil
		}
	}
	return shared.ErrNotFound
}

func (r *FakePlaylistItemRepository) GetByPlaylistID(_ context.Context, playlistID string) ([]*playlist.PlaylistItem, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := r.items[playlistID]
	result := make([]*playlist.PlaylistItem, len(items))
	copy(result, items)
	sort.Slice(result, func(i, j int) bool {
		return result[i].SortOrder < result[j].SortOrder
	})
	return result, nil
}

func (r *FakePlaylistItemRepository) GetByPlaylistIDs(_ context.Context, playlistIDs []string) (map[string][]*playlist.PlaylistItem, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string][]*playlist.PlaylistItem, len(playlistIDs))
	for _, playlistID := range playlistIDs {
		items := r.items[playlistID]
		if len(items) == 0 {
			continue
		}
		got := make([]*playlist.PlaylistItem, len(items))
		copy(got, items)
		sort.Slice(got, func(i, j int) bool {
			return got[i].SortOrder < got[j].SortOrder
		})
		result[playlistID] = got
	}
	return result, nil
}

func (r *FakePlaylistItemRepository) Reorder(_ context.Context, playlistID string, mediaItemIDs []string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	items := r.items[playlistID]
	byID := make(map[string]*playlist.PlaylistItem, len(items))
	for _, it := range items {
		byID[string(it.MediaItemID)] = it
	}
	reordered := make([]*playlist.PlaylistItem, 0, len(mediaItemIDs))
	matched := make(map[string]struct{}, len(mediaItemIDs))
	for i, mid := range mediaItemIDs {
		if it, ok := byID[mid]; ok {
			it.SortOrder = i
			reordered = append(reordered, it)
			matched[mid] = struct{}{}
		}
	}
	// Mirror the postgres adapter: fewer matched rows than distinct requested
	// ids = membership drift, refuse instead of half-applying.
	distinct := make(map[string]struct{}, len(mediaItemIDs))
	for _, mid := range mediaItemIDs {
		distinct[mid] = struct{}{}
	}
	if len(matched) != len(distinct) {
		return playlist.ErrReorderMembershipDrift
	}
	r.items[playlistID] = reordered
	return nil
}

func (r *FakePlaylistItemRepository) PurgeOrphans(_ context.Context) (int64, error) {
	if r.Err != nil {
		return 0, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.purge++
	return 0, nil
}

// PurgeOrphansCalls reports how many times PurgeOrphans ran.
func (r *FakePlaylistItemRepository) PurgeOrphansCalls() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.purge
}
