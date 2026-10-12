package fakes

import (
	"context"
	"sync"

	"github.com/eveyproject/evey/internal/domain/metadata"
)

type FakeArtworkRepository struct {
	mu    sync.RWMutex
	items map[string]*metadata.ArtworkRecord
	Err   error

	// DeleteByMediaItemIDCalls / DeleteByMediaItemIDsCalls count invocations
	// of the single-item vs batch delete, so tests can assert the N+1 is gone.
	DeleteByMediaItemIDCalls  int
	DeleteByMediaItemIDsCalls int
	// CallLog, when non-nil, records a tag ("artwork") on every delete call.
	// Shared across the three fakes used by ResetItemMetadata so a test can
	// assert the overrides-before-artwork-before-persons ordering invariant.
	CallLog *[]string
	// ErrDeleteByMediaItemIDs, when set, fails only the batch delete (not the
	// single-item one), so a test can force a mid-level abort without also
	// failing the single top-item reset that runs before any batch call.
	ErrDeleteByMediaItemIDs error
}

func NewFakeArtworkRepository() *FakeArtworkRepository {
	return &FakeArtworkRepository{items: make(map[string]*metadata.ArtworkRecord)}
}

func (r *FakeArtworkRepository) Create(_ context.Context, a *metadata.ArtworkRecord) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items[a.ID] = a
	return nil
}

func (r *FakeArtworkRepository) CreateBatch(_ context.Context, records []*metadata.ArtworkRecord) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range records {
		r.items[a.ID] = a
	}
	return nil
}

func (r *FakeArtworkRepository) GetByMediaItem(_ context.Context, mediaItemID string) ([]*metadata.ArtworkRecord, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	var result []*metadata.ArtworkRecord
	for _, a := range r.items {
		if string(a.MediaItemID) == mediaItemID {
			result = append(result, a)
		}
	}
	return result, nil
}

func (r *FakeArtworkRepository) GetByMediaItems(_ context.Context, mediaItemIDs []string) (map[string][]*metadata.ArtworkRecord, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	idSet := make(map[string]bool, len(mediaItemIDs))
	for _, id := range mediaItemIDs {
		idSet[id] = true
	}
	result := make(map[string][]*metadata.ArtworkRecord)
	for _, a := range r.items {
		if idSet[string(a.MediaItemID)] {
			result[string(a.MediaItemID)] = append(result[string(a.MediaItemID)], a)
		}
	}
	return result, nil
}

func (r *FakeArtworkRepository) GetByPerson(_ context.Context, personID string) ([]*metadata.ArtworkRecord, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	var result []*metadata.ArtworkRecord
	for _, a := range r.items {
		if a.PersonID != nil && *a.PersonID == personID {
			result = append(result, a)
		}
	}
	return result, nil
}

func (r *FakeArtworkRepository) ListMissingLocalPath(_ context.Context) ([]*metadata.ArtworkRecord, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*metadata.ArtworkRecord
	for _, a := range r.items {
		if a.SourceURL != nil && (a.LocalPath == nil || *a.LocalPath == "") {
			result = append(result, a)
		}
	}
	return result, nil
}

func (r *FakeArtworkRepository) ListMissingBlurhash(_ context.Context) ([]*metadata.ArtworkRecord, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*metadata.ArtworkRecord
	for _, a := range r.items {
		if a.LocalPath != nil && *a.LocalPath != "" && a.Blurhash == nil {
			result = append(result, a)
		}
	}
	return result, nil
}

func (r *FakeArtworkRepository) LocalPathStats(_ context.Context) (metadata.ArtworkStats, error) {
	if r.Err != nil {
		return metadata.ArtworkStats{}, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var stats metadata.ArtworkStats
	for _, a := range r.items {
		stats.TotalRecords++
		if a.LocalPath != nil && *a.LocalPath != "" {
			stats.WithLocalPath++
		} else {
			stats.MissingLocalPath++
		}
	}
	return stats, nil
}

func (r *FakeArtworkRepository) UpdateLocalPath(_ context.Context, id string, localPath string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.items[id]
	if !ok {
		return ErrNotFound
	}
	a.LocalPath = &localPath
	return nil
}

func (r *FakeArtworkRepository) UpdateWebPPath(_ context.Context, id string, webpPath string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if a, ok := r.items[id]; ok {
		a.WebPPath = &webpPath
	}
	return nil
}

func (r *FakeArtworkRepository) UpdateBlurhash(_ context.Context, id string, blurhash string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.items[id]
	if !ok {
		return ErrNotFound
	}
	a.Blurhash = &blurhash
	return nil
}

func (r *FakeArtworkRepository) Delete(_ context.Context, id string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *FakeArtworkRepository) DeleteByMediaItemID(_ context.Context, mediaItemID string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.DeleteByMediaItemIDCalls++
	if r.CallLog != nil {
		*r.CallLog = append(*r.CallLog, "artwork")
	}
	if r.Err != nil {
		return r.Err
	}
	for id, a := range r.items {
		if string(a.MediaItemID) == mediaItemID {
			delete(r.items, id)
		}
	}
	return nil
}

// DeleteByMediaItemIDs is the batch form: one pass over the map instead of
// one call (and one map scan) per item.
func (r *FakeArtworkRepository) DeleteByMediaItemIDs(_ context.Context, mediaItemIDs []string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.DeleteByMediaItemIDsCalls++
	if r.CallLog != nil {
		*r.CallLog = append(*r.CallLog, "artwork")
	}
	if r.Err != nil {
		return r.Err
	}
	if r.ErrDeleteByMediaItemIDs != nil {
		return r.ErrDeleteByMediaItemIDs
	}
	wanted := make(map[string]bool, len(mediaItemIDs))
	for _, id := range mediaItemIDs {
		wanted[id] = true
	}
	for id, a := range r.items {
		if wanted[string(a.MediaItemID)] {
			delete(r.items, id)
		}
	}
	return nil
}

var (
	_ metadata.ArtworkCommandRepo = (*FakeArtworkRepository)(nil)
	_ metadata.ArtworkQueryRepo   = (*FakeArtworkRepository)(nil)
)
