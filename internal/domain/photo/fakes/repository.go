package fakes

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/gravity-zero/evey/internal/domain/photo"
)

type FakeRepository struct {
	mu     sync.RWMutex
	photos map[string]photo.Photo
	Err    error
}

func NewFakeRepository() *FakeRepository {
	return &FakeRepository{photos: make(map[string]photo.Photo)}
}

func (r *FakeRepository) Upsert(_ context.Context, p photo.Photo) error {
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
		return nil, nil
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

func (r *FakeRepository) ListByLibrary(_ context.Context, libraryID string, limit, offset int) ([]photo.Photo, int, error) {
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

func (r *FakeRepository) ListByAlbum(_ context.Context, libraryID, album string, limit, offset int) ([]photo.Photo, int, error) {
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

func (r *FakeRepository) GetAlbums(_ context.Context, libraryID string) ([]photo.Album, error) {
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

func (r *FakeRepository) Search(_ context.Context, libraryID, query string, limit int) ([]photo.Photo, error) {
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

func (r *FakeRepository) ListAllByLibrary(_ context.Context, libraryID string) ([]photo.Photo, error) {
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

func (r *FakeRepository) UpdateFilePath(_ context.Context, id, newPath string, modTime time.Time) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.photos[id]; ok {
		p.FilePath = newPath
		p.ModTime = modTime
		r.photos[id] = p
	}
	return nil
}

func (r *FakeRepository) CountByLibrary(_ context.Context, libraryID string) (int, error) {
	if r.Err != nil {
		return 0, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, p := range r.photos {
		if p.LibraryID == libraryID {
			count++
		}
	}
	return count, nil
}
