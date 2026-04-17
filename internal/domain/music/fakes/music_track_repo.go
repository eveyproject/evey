package fakes

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/evey-project/evey/internal/domain/music"
	"github.com/evey-project/evey/internal/domain/shared"
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

func (r *FakeMusicTrackRepository) ListByLibrary(_ context.Context, libraryID shared.LibraryID, limit, offset int, search string) ([]music.Track, int, error) {
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

func (r *FakeMusicTrackRepository) ListByLibraryAndAlbum(_ context.Context, libraryID shared.LibraryID, album string) ([]music.Track, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []music.Track
	for _, t := range r.tracks {
		if t.LibraryID == libraryID && t.Album == album {
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

func (r *FakeMusicTrackRepository) GetAlbums(_ context.Context, libraryID shared.LibraryID) ([]music.Album, error) {
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

func (r *FakeMusicTrackRepository) GetArtists(_ context.Context, libraryID shared.LibraryID) ([]music.Artist, error) {
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

func (r *FakeMusicTrackRepository) ListAllByLibrary(_ context.Context, libraryID shared.LibraryID) ([]music.Track, error) {
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

func (r *FakeMusicTrackRepository) ListAlbumsMissingArt(_ context.Context, libraryID shared.LibraryID) ([]music.AlbumKey, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	type key struct{ album, artist string }
	hasArt := make(map[key]bool)
	keys := make(map[key]struct{})
	for _, t := range r.tracks {
		if t.LibraryID != libraryID || t.Album == "" {
			continue
		}
		artist := t.AlbumArtist
		if artist == "" {
			artist = t.Artist
		}
		if artist == "" {
			continue
		}
		k := key{t.Album, artist}
		keys[k] = struct{}{}
		if t.CoverArtURL != "" {
			hasArt[k] = true
		}
	}
	var out []music.AlbumKey
	for k := range keys {
		if !hasArt[k] {
			out = append(out, music.AlbumKey{Album: k.album, Artist: k.artist})
		}
	}
	return out, nil
}

func (r *FakeMusicTrackRepository) SetAlbumCoverArtURL(_ context.Context, libraryID shared.LibraryID, album, albumArtist, url string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, t := range r.tracks {
		if t.LibraryID != libraryID || t.Album != album {
			continue
		}
		a := t.AlbumArtist
		if a == "" {
			a = t.Artist
		}
		if a != albumArtist {
			continue
		}
		t.CoverArtURL = url
		r.tracks[id] = t
	}
	return nil
}

