package fakes

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

type FakeMediaItemRepository struct {
	mu    sync.RWMutex
	items map[string]*library.MediaItem
	// Per-method error injection; zero value = no error (backward compatible)
	ErrCreate  error
	ErrGetByID error
	ErrList    error
	ErrUpdate  error
	// GetByFilePathCalls counts lookups so a test can assert that a scan pass
	// resolves from its preloaded index instead of one query per file.
	GetByFilePathCalls       atomic.Int64
	ErrDelete                error
	GetByFileFingerprintErr  error
	GetByLibraryTypeTitleErr error
}

func NewFakeMediaItemRepository() *FakeMediaItemRepository {
	return &FakeMediaItemRepository{items: make(map[string]*library.MediaItem)}
}

func (r *FakeMediaItemRepository) Create(_ context.Context, item *library.MediaItem) error {
	if r.ErrCreate != nil {
		return r.ErrCreate
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[string(item.ID)] = cloneMediaItem(item)
	return nil
}

func (r *FakeMediaItemRepository) GetByID(_ context.Context, id shared.MediaItemID) (*library.MediaItem, error) {
	if r.ErrGetByID != nil {
		return nil, r.ErrGetByID
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[string(id)]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneMediaItem(item), nil
}

func (r *FakeMediaItemRepository) List(_ context.Context, filter library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	if r.ErrList != nil {
		return nil, r.ErrList
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*library.MediaItem
	for _, item := range r.items {
		if filter.LibraryID != nil && item.LibraryID != *filter.LibraryID {
			continue
		}
		if filter.ParentID != nil && item.ParentID != *filter.ParentID {
			continue
		}
		if filter.Type != nil && string(item.Type) != *filter.Type {
			continue
		}
		if filter.MissingProviderIDs != nil {
			hasPIDs := item.Catalog != nil && len(item.Catalog.ProviderIDs) > 0
			if *filter.MissingProviderIDs && hasPIDs {
				continue
			}
			if !*filter.MissingProviderIDs && !hasPIDs {
				continue
			}
		}
		result = append(result, item)
	}
	// Items live in a map, so iteration order is randomised by the runtime.
	// Left as-is, paginating over this fake is not merely "a different order
	// from Postgres" - it is a DIFFERENT order on every call, so page 2 can
	// repeat or skip rows page 1 already returned, and any test that walks
	// pages is quietly meaningless. Sorting by id gives the stable ordering
	// that paging needs. (Postgres orders by sort_title unless asked
	// otherwise; the point here is determinism, not matching that exactly.)
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })

	if filter.Offset > 0 && filter.Offset < len(result) {
		result = result[filter.Offset:]
	} else if filter.Offset >= len(result) {
		return nil, nil
	}
	// Postgres does NOT return everything when the caller leaves Limit at zero:
	// it clamps to DefaultListLimit. A fake that hands back the whole set makes
	// an unpaginated sweep look correct in every unit test and truncated in
	// production - which is exactly what happened to RefreshAllTranslations,
	// green here and stopping at 500 items on a real server.
	//
	// The number is duplicated rather than imported: this is the domain layer
	// and postgres is an adapter. If it ever changes there, this comment and
	// the guard in postgres/media_item_repo.go are the two places to keep in
	// step.
	const defaultListLimit = 500 // mirrors postgres.DefaultListLimit
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

func (r *FakeMediaItemRepository) Update(_ context.Context, item *library.MediaItem) error {
	if r.ErrUpdate != nil {
		return r.ErrUpdate
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[string(item.ID)]; !ok {
		return ErrNotFound
	}
	r.items[string(item.ID)] = cloneMediaItem(item)
	return nil
}

func (r *FakeMediaItemRepository) Delete(_ context.Context, id shared.MediaItemID) error {
	if r.ErrDelete != nil {
		return r.ErrDelete
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[string(id)]; !ok {
		return ErrNotFound
	}
	delete(r.items, string(id))
	return nil
}

func (r *FakeMediaItemRepository) GetByFilePath(_ context.Context, path string) (*library.MediaItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, item := range r.items {
		if item.File != nil && item.File.Path == path {
			return cloneMediaItem(item), nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeMediaItemRepository) GetByFileFingerprint(_ context.Context, fingerprint string) (*library.MediaItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.GetByFileFingerprintErr != nil {
		return nil, r.GetByFileFingerprintErr
	}
	for _, item := range r.items {
		if item.File != nil && item.File.Fingerprint == fingerprint {
			return cloneMediaItem(item), nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakeMediaItemRepository) Count(_ context.Context, filter library.ListMediaItemsFilter) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, item := range r.items {
		if filter.LibraryID != nil && item.LibraryID != *filter.LibraryID {
			continue
		}
		if filter.Type != nil && string(item.Type) != *filter.Type {
			continue
		}
		if filter.ParentID != nil && item.ParentID != *filter.ParentID {
			continue
		}
		if filter.MissingProviderIDs != nil {
			hasPIDs := item.Catalog != nil && len(item.Catalog.ProviderIDs) > 0
			if *filter.MissingProviderIDs && hasPIDs {
				continue
			}
			if !*filter.MissingProviderIDs && !hasPIDs {
				continue
			}
		}
		count++
	}
	return count, nil
}

func (r *FakeMediaItemRepository) SeriesAggregate(_ context.Context, seriesID shared.MediaItemID) (library.SeriesAggregate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	// Collect descendants iteratively (BFS over parent_id links).
	want := map[string]bool{string(seriesID): true}
	for {
		grew := false
		for _, item := range r.items {
			if want[item.ParentID] && !want[string(item.ID)] {
				want[string(item.ID)] = true
				grew = true
			}
		}
		if !grew {
			break
		}
	}
	agg := library.SeriesAggregate{}
	seasonIDs := map[string]bool{}
	var totalMs int64
	for _, item := range r.items {
		if !want[string(item.ID)] {
			continue
		}
		if string(item.Type) != "episode" {
			continue
		}
		agg.EpisodeCount++
		if item.File != nil && !item.File.Duration.IsZero() {
			totalMs += item.File.Duration.Milliseconds()
		}
		if item.ParentID != "" {
			seasonIDs[item.ParentID] = true
		}
	}
	agg.SeasonCount = len(seasonIDs)
	dur, derr := shared.NewDurationMs(totalMs)
	if derr != nil {
		return library.SeriesAggregate{}, derr
	}
	agg.TotalDuration = dur
	return agg, nil
}

func (r *FakeMediaItemRepository) GetByLibraryTypeTitle(_ context.Context, libraryID shared.LibraryID, itemType, title, parentID string) (*library.MediaItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.GetByLibraryTypeTitleErr != nil {
		return nil, r.GetByLibraryTypeTitleErr
	}
	for _, item := range r.items {
		if item.LibraryID == libraryID && string(item.Type) == itemType && item.Title == title &&
			(parentID == "" || item.ParentID == parentID) {
			return cloneMediaItem(item), nil
		}
	}
	return nil, ErrNotFound
}

// cloneMediaItem detaches the stored record from the caller's pointer. A fake
// that hands back the same pointer it stores makes persistence untestable: a
// service that loads an item, mutates it and never calls Update is
// indistinguishable from one that persists. A real repository serialises, so
// the fake must too.
//
// Shallow at the top level plus the mutable sub-structs services actually
// reach into (File, Catalog) - deep enough to break the aliasing that hid the
// bug, without pretending to be a full deep copy.
func cloneMediaItem(item *library.MediaItem) *library.MediaItem {
	if item == nil {
		return nil
	}
	cp := *item
	if item.File != nil {
		f := *item.File
		cp.File = &f
	}
	if item.Catalog != nil {
		c := *item.Catalog
		cp.Catalog = &c
	}
	return &cp
}
