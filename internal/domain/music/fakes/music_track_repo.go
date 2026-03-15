package fakes

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gravity-zero/evey/internal/domain/music"
)

type FakeMusicTrackRepository struct {
	mu        sync.RWMutex
	tracks    map[string]music.Track
	Err       error
	UpsertErr error
}

func NewFakeMusicTrackRepository() *FakeMusicTrackRepository {
	return &FakeMusicTrackRepository{tracks: make(map[string]music.Track)}
}

func (r *FakeMusicTrackRepository) Upsert(_ context.Context, t music.Track) error {
	if r.UpsertErr != nil {
		return r.UpsertErr
	}
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	// upsert by file_path
	for id, existing := range r.tracks {
		if existing.FilePath == t.FilePath {
			t.ID = id
			r.tracks[id] = t
			return nil
		}
	}
	r.tracks[t.ID] = t
	return nil
}

func (r *FakeMusicTrackRepository) GetByID(_ context.Context, id string) (*music.Track, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tracks[id]
	if !ok {
		return nil, nil
	}
	return &t, nil
}

func (r *FakeMusicTrackRepository) Delete(_ context.Context, id string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tracks, id)
	return nil
}

func (r *FakeMusicTrackRepository) ListByLibrary(_ context.Context, libraryID string, limit, offset int, search string) ([]music.Track, int, error) {
	if r.Err != nil {
		return nil, 0, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	q := strings.ToLower(search)
	var out []music.Track
	for _, t := range r.tracks {
		if t.LibraryID != libraryID {
			continue
		}
		if q != "" &&
			!strings.Contains(strings.ToLower(t.Title), q) &&
			!strings.Contains(strings.ToLower(t.Artist), q) &&
			!strings.Contains(strings.ToLower(t.Album), q) {
			continue
		}
		out = append(out, t)
	}
	total := len(out)
	if offset < len(out) {
		out = out[offset:]
	} else {
		out = nil
	}
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, total, nil
}

func (r *FakeMusicTrackRepository) ListByAlbum(_ context.Context, album, artist string) ([]music.Track, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []music.Track
	for _, t := range r.tracks {
		if t.Album == album && t.AlbumArtist == artist {
			out = append(out, t)
		}
	}
	return out, nil
}

func (r *FakeMusicTrackRepository) ListByArtist(_ context.Context, artist string, limit, offset int) ([]music.Track, int, error) {
	if r.Err != nil {
		return nil, 0, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []music.Track
	for _, t := range r.tracks {
		if t.AlbumArtist == artist || t.Artist == artist {
			out = append(out, t)
		}
	}
	total := len(out)
	if offset < len(out) {
		out = out[offset:]
	} else {
		out = nil
	}
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, total, nil
}

func (r *FakeMusicTrackRepository) Search(_ context.Context, query string, limit int) ([]music.Track, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	q := strings.ToLower(query)
	var out []music.Track
	for _, t := range r.tracks {
		if strings.Contains(strings.ToLower(t.Title), q) ||
			strings.Contains(strings.ToLower(t.Artist), q) ||
			strings.Contains(strings.ToLower(t.Album), q) {
			out = append(out, t)
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *FakeMusicTrackRepository) GetAlbums(_ context.Context, libraryID string) ([]music.Album, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	type key struct{ album, artist string }
	m := make(map[key]*music.Album)
	for _, t := range r.tracks {
		if t.LibraryID != libraryID {
			continue
		}
		artist := t.AlbumArtist
		if artist == "" {
			artist = t.Artist
		}
		k := key{t.Album, artist}
		a, ok := m[k]
		if !ok {
			a = &music.Album{
				ID:          fmt.Sprintf("%s|%s", t.Album, artist),
				Title:       t.Album,
				Artist:      artist,
				Year:        t.Year,
				CoverArtURL: t.CoverArtURL,
			}
			m[k] = a
		}
		a.TrackCount++
		a.Duration += t.Duration
	}
	out := make([]music.Album, 0, len(m))
	for _, a := range m {
		out = append(out, *a)
	}
	return out, nil
}

func (r *FakeMusicTrackRepository) GetArtists(_ context.Context, libraryID string) ([]music.Artist, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	type stats struct {
		tracks int
		albums map[string]struct{}
	}
	m := make(map[string]*stats)
	for _, t := range r.tracks {
		if t.LibraryID != libraryID {
			continue
		}
		name := t.AlbumArtist
		if name == "" {
			name = t.Artist
		}
		s, ok := m[name]
		if !ok {
			s = &stats{albums: make(map[string]struct{})}
			m[name] = s
		}
		s.tracks++
		s.albums[t.Album] = struct{}{}
	}
	out := make([]music.Artist, 0, len(m))
	for name, s := range m {
		out = append(out, music.Artist{
			ID:         name,
			Name:       name,
			TrackCount: s.tracks,
			AlbumCount: len(s.albums),
		})
	}
	return out, nil
}

func (r *FakeMusicTrackRepository) ListAllByLibrary(_ context.Context, libraryID string) ([]music.Track, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []music.Track
	for _, t := range r.tracks {
		if t.LibraryID == libraryID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (r *FakeMusicTrackRepository) UpdateFilePath(_ context.Context, id, newPath string, modTime time.Time) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.tracks[id]; ok {
		t.FilePath = newPath
		t.ModTime = modTime
		r.tracks[id] = t
	}
	return nil
}

func (r *FakeMusicTrackRepository) CountByLibrary(_ context.Context, libraryID string) (int, error) {
	if r.Err != nil {
		return 0, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := 0
	for _, t := range r.tracks {
		if t.LibraryID == libraryID {
			n++
		}
	}
	return n, nil
}
