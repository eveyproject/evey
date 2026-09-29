package fakes

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/photo"
	"github.com/evey-project/evey/internal/domain/shared"
)

type FakeRepository struct {
	mu        sync.RWMutex
	photos    map[string]photo.Photo
	Err       error
	UpsertErr error
}

func NewFakeRepository() *FakeRepository {
	return &FakeRepository{photos: make(map[string]photo.Photo)}
}

func (r *FakeRepository) Upsert(_ context.Context, p photo.Photo) error {
	if r.UpsertErr != nil {
		return r.UpsertErr
	}
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.photos[p.ID] = p
	return nil
}

func (r *FakeRepository) GetByID(_ context.Context, id string) (*photo.Photo, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.photos[id]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return &p, nil
}

func (r *FakeRepository) Delete(_ context.Context, id string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.photos, id)
	return nil
}

func (r *FakeRepository) ListByLibrary(_ context.Context, libraryID shared.LibraryID, limit, offset int) ([]photo.Photo, int, error) {
	if r.Err != nil {
		return nil, 0, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []photo.Photo
	for _, p := range r.photos {
		if p.LibraryID == libraryID {
			result = append(result, p)
		}
	}
	// Sort exactly like the postgres query (taken_at DESC NULLS LAST, created_at
	// DESC, id DESC). Without this the fake returned map iteration order, so it
	// was BOTH non-deterministic and unrelated to production order - the kind of
	// green test that proves nothing, and it made the offset->cursor handover
	// look broken when the handover was right and the fake was wrong.
	sort.Slice(result, func(i, j int) bool {
		return photoCursorLess(result[j], result[i])
	})
	total := len(result)
	if offset >= total {
		return nil, total, nil
	}
	result = result[offset:]
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result, total, nil
}

func (r *FakeRepository) CountByLibrary(_ context.Context, libraryID shared.LibraryID) (int, error) {
	if r.Err != nil {
		return 0, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := 0
	for _, p := range r.photos {
		if p.LibraryID == libraryID {
			n++
		}
	}
	return n, nil
}

func (r *FakeRepository) ListByAlbum(_ context.Context, libraryID shared.LibraryID, album string, limit, offset int) ([]photo.Photo, int, error) {
	if r.Err != nil {
		return nil, 0, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []photo.Photo
	for _, p := range r.photos {
		if p.LibraryID == libraryID && p.Album == album {
			result = append(result, p)
		}
	}
	total := len(result)
	if offset >= total {
		return nil, total, nil
	}
	result = result[offset:]
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result, total, nil
}

// ListByLibraryCursor mimics the real repo's keyset seek: sort by (taken_at,
// created_at, id) DESC (nil TakenAt sorts last, like NULLS LAST) and return
// rows strictly after the cursor in that order.
func (r *FakeRepository) ListByLibraryCursor(_ context.Context, libraryID shared.LibraryID, cursor *photo.PhotoCursor, limit int) ([]photo.Photo, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []photo.Photo
	for _, p := range r.photos {
		if p.LibraryID == libraryID {
			result = append(result, p)
		}
	}
	return pagePhotosByCursor(result, cursor, limit), nil
}

// ListByAlbumCursor is the keyset form of ListByAlbum. See ListByLibraryCursor.
func (r *FakeRepository) ListByAlbumCursor(_ context.Context, libraryID shared.LibraryID, album string, cursor *photo.PhotoCursor, limit int) ([]photo.Photo, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []photo.Photo
	for _, p := range r.photos {
		if p.LibraryID == libraryID && p.Album == album {
			result = append(result, p)
		}
	}
	return pagePhotosByCursor(result, cursor, limit), nil
}

// pagePhotosByCursor sorts by (taken_at DESC NULLS LAST, created_at DESC, id
// DESC) - the same tuple the postgres ORDER BY uses - then keeps rows
// strictly "after" the cursor and truncates to limit.
func pagePhotosByCursor(photos []photo.Photo, cursor *photo.PhotoCursor, limit int) []photo.Photo {
	sort.Slice(photos, func(i, j int) bool {
		return photoCursorLess(photos[j], photos[i])
	})
	if cursor != nil {
		filtered := photos[:0:0]
		for _, p := range photos {
			if photoCursorLess(p, photo.Photo{TakenAt: cursor.TakenAt, CreatedAt: cursor.CreatedAt, ID: cursor.ID}) {
				filtered = append(filtered, p)
			}
		}
		photos = filtered
	}
	if limit > 0 && limit < len(photos) {
		photos = photos[:limit]
	}
	return photos
}

// photoCursorLess reports whether a sorts strictly before b in (taken_at,
// created_at, id) ascending order (nil TakenAt is smallest, matching
// COALESCE(taken_at, '-infinity') in the real query).
func photoCursorLess(a, b photo.Photo) bool {
	at, bt := a.TakenAt, b.TakenAt
	switch {
	case at == nil && bt != nil:
		return true
	case at != nil && bt == nil:
		return false
	case at != nil && bt != nil && !at.Equal(*bt):
		return at.Before(*bt)
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.Before(b.CreatedAt)
	}
	return a.ID < b.ID
}

func (r *FakeRepository) GetAlbums(_ context.Context, libraryID shared.LibraryID) ([]photo.Album, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	albums := make(map[string]*photo.Album)
	for _, p := range r.photos {
		if p.LibraryID != libraryID || p.Album == "" {
			continue
		}
		a, ok := albums[p.Album]
		if !ok {
			albums[p.Album] = &photo.Album{
				Name:         p.Album,
				LibraryID:    libraryID,
				PhotoCount:   1,
				CoverPhotoID: p.ID,
			}
		} else {
			a.PhotoCount++
		}
	}
	result := make([]photo.Album, 0, len(albums))
	for _, a := range albums {
		result = append(result, *a)
	}
	return result, nil
}

func (r *FakeRepository) Search(_ context.Context, libraryID shared.LibraryID, query string, limit int) ([]photo.Photo, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	q := strings.ToLower(query)
	var result []photo.Photo
	for _, p := range r.photos {
		if p.LibraryID != libraryID {
			continue
		}
		if strings.Contains(strings.ToLower(p.Title), q) || strings.Contains(strings.ToLower(p.Album), q) {
			result = append(result, p)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (r *FakeRepository) ListAllByLibrary(_ context.Context, libraryID shared.LibraryID) ([]photo.Photo, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []photo.Photo
	for _, p := range r.photos {
		if p.LibraryID == libraryID {
			result = append(result, p)
		}
	}
	return result, nil
}

func (r *FakeRepository) UpdateFilePath(_ context.Context, id, newPath, album string, modTime time.Time) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.photos[id]; ok {
		p.FilePath = newPath
		p.Album = album
		p.ModTime = modTime
		r.photos[id] = p
	}
	return nil
}
