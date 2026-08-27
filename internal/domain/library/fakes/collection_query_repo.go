package fakes

import (
	"context"
	"sort"
	"sync"

	"github.com/evey-project/evey/internal/domain/library"
)

type FakeCollectionQueryRepo struct {
	mu          sync.RWMutex
	collections map[string]*library.Collection
	items       map[string][]collectionItem
	mediaItems  *FakeMediaItemRepository
	Err         error
	// GetItemsErr fails ONLY GetItems, so a test can make the per-collection
	// member read fail while the collection listing itself succeeds - the
	// exact shape of a transient DB timeout mid-request.
	GetItemsErr error
	// GetItemsByCollectionIDsCallCount is incremented on every call to
	// GetItemsByCollectionIDs, allowing tests to assert batch behaviour.
	GetItemsByCollectionIDsCallCount int
	// GetItemsByCollectionIDsErr fails ONLY the batch member read (the twin of
	// GetItemsErr for the per-collection path), so a test can make the batched
	// member lookup fail while the collection listing itself succeeds.
	GetItemsByCollectionIDsErr error
}

func NewFakeCollectionQueryRepo(cmdRepo *FakeCollectionCommandRepo, mediaItems *FakeMediaItemRepository) *FakeCollectionQueryRepo {
	return &FakeCollectionQueryRepo{
		collections: cmdRepo.collections,
		items:       cmdRepo.items,
		mediaItems:  mediaItems,
	}
}

func (r *FakeCollectionQueryRepo) GetByID(_ context.Context, id string) (*library.Collection, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	col, ok := r.collections[id]
	if !ok {
		return nil, ErrNotFound
	}
	return col, nil
}

func (r *FakeCollectionQueryRepo) List(_ context.Context) ([]*library.Collection, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*library.Collection, 0, len(r.collections))
	for _, col := range r.collections {
		result = append(result, col)
	}
	return result, nil
}

func (r *FakeCollectionQueryRepo) GetItems(_ context.Context, collectionID string, limit, offset int) ([]*library.MediaItem, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	if r.GetItemsErr != nil {
		return nil, r.GetItemsErr
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	its := r.items[collectionID]
	var result []*library.MediaItem
	for _, it := range its {
		if item, ok := r.mediaItems.items[it.mediaItemID]; ok {
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

func (r *FakeCollectionQueryRepo) CountItems(_ context.Context, collectionID string) (int, error) {
	if r.Err != nil {
		return 0, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.items[collectionID]), nil
}

func (r *FakeCollectionQueryRepo) GetItemsByCollectionIDs(_ context.Context, collectionIDs []string) (map[string][]*library.MediaItem, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	if r.GetItemsByCollectionIDsErr != nil {
		return nil, r.GetItemsByCollectionIDsErr
	}
	r.mu.Lock()
	r.GetItemsByCollectionIDsCallCount++
	r.mu.Unlock()
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string][]*library.MediaItem, len(collectionIDs))
	for _, colID := range collectionIDs {
		its := r.items[colID]
		for _, it := range its {
			if item, ok := r.mediaItems.items[it.mediaItemID]; ok {
				result[colID] = append(result[colID], item)
			}
		}
	}
	return result, nil
}

// ListMoviesWithoutCollection mirrors the postgres query: movies carrying a
// tmdb provider id that appear in no collection, in id order and strictly
// after the cursor. The order is part of the contract - it is what lets a
// sweep advance instead of re-reading the same head of the catalogue.
func (r *FakeCollectionQueryRepo) ListMoviesWithoutCollection(_ context.Context, afterID string, limit int) ([]library.UnlinkedMovie, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	linked := map[string]bool{}
	for _, its := range r.items {
		for _, it := range its {
			linked[it.mediaItemID] = true
		}
	}
	var out []library.UnlinkedMovie
	for id, item := range r.mediaItems.items {
		if item.Type != library.MovieItem || linked[id] {
			continue
		}
		if item.Catalog == nil || item.Catalog.ProviderIDs["tmdb"] == "" {
			continue
		}
		if string(item.ID) <= afterID {
			continue
		}
		out = append(out, library.UnlinkedMovie{MediaItemID: item.ID, ProviderID: item.Catalog.ProviderIDs["tmdb"]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MediaItemID < out[j].MediaItemID })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *FakeCollectionQueryRepo) GetByMediaItemID(_ context.Context, mediaItemID string) ([]*library.Collection, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*library.Collection
	for colID, its := range r.items {
		for _, it := range its {
			if it.mediaItemID == mediaItemID {
				if col, ok := r.collections[colID]; ok {
					out = append(out, col)
				}
				break
			}
		}
	}
	return out, nil
}
