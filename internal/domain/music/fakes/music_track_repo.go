package fakes

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/music"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/google/uuid"
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

func (r *FakeMusicTrackRepository) Upsert(_ context.Context, t music.Track) (string, error) {
	if r.UpsertErr != nil {
		return "", r.UpsertErr
	}
	if r.Err != nil {
		return "", r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	// Mirror the postgres Upsert: derive album_folder from file_path when
	// caller didn't set it. Keeps the fake behaviorally consistent so
	// unit tests don't leak nil album_folder rows into the rest.
	if t.AlbumFolder == "" && t.FilePath != "" {
		t.AlbumFolder = filepath.Dir(t.FilePath)
	}
	// upsert by file_path: on conflict the existing row keeps its id, so we
	// return THAT id (mirrors the postgres RETURNING id), not the caller's
	// freshly-minted one.
	for id, existing := range r.tracks {
		if existing.FilePath == t.FilePath {
			t.ID = id
			r.tracks[id] = t
			return id, nil
		}
	}
	r.tracks[t.ID] = t
	return t.ID, nil
}

func (r *FakeMusicTrackRepository) GetByID(_ context.Context, id string) (*music.Track, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tracks[id]
	if !ok {
		return nil, shared.ErrNotFound
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

// ListByLibraryCursor mimics the real repo's keyset seek: sort by
// (album_artist, album, disk_number, track_number, id) ascending and return
// rows strictly after the cursor in that order.
func (r *FakeMusicTrackRepository) ListByLibraryCursor(_ context.Context, libraryID shared.LibraryID, search string, cursor *music.TrackCursor, limit int) ([]music.Track, error) {
	if r.Err != nil {
		return nil, r.Err
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
	sort.Slice(out, func(i, j int) bool { return trackCursorLess(out[i], out[j]) })
	if cursor != nil {
		filtered := out[:0:0]
		for _, t := range out {
			if trackCursorLess(music.Track{AlbumArtist: cursor.AlbumArtist, Album: cursor.Album, DiskNumber: cursor.DiskNumber, TrackNumber: cursor.TrackNumber, ID: cursor.ID}, t) {
				filtered = append(filtered, t)
			}
		}
		out = filtered
	}
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}

// trackCursorLess reports whether a sorts strictly before b in (album_artist,
// album, disk_number, track_number, id) ascending order - the tuple
// ListByLibraryCursor's ORDER BY sorts on.
func trackCursorLess(a, b music.Track) bool {
	if a.AlbumArtist != b.AlbumArtist {
		return a.AlbumArtist < b.AlbumArtist
	}
	if a.Album != b.Album {
		return a.Album < b.Album
	}
	if a.DiskNumber != b.DiskNumber {
		return a.DiskNumber < b.DiskNumber
	}
	if a.TrackNumber != b.TrackNumber {
		return a.TrackNumber < b.TrackNumber
	}
	return a.ID < b.ID
}

// artistTrackCursorLess reports whether a sorts strictly before b in (album,
// disk_number, track_number, id) ascending order - the tuple
// ListByArtistCursor's ORDER BY sorts on.
func artistTrackCursorLess(a, b music.Track) bool {
	if a.Album != b.Album {
		return a.Album < b.Album
	}
	if a.DiskNumber != b.DiskNumber {
		return a.DiskNumber < b.DiskNumber
	}
	if a.TrackNumber != b.TrackNumber {
		return a.TrackNumber < b.TrackNumber
	}
	return a.ID < b.ID
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

func (r *FakeMusicTrackRepository) CountByLibrary(_ context.Context, libraryID shared.LibraryID) (int, error) {
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

func (r *FakeMusicTrackRepository) ListByAlbumFolder(_ context.Context, libraryID shared.LibraryID, albumFolder string) ([]music.Track, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []music.Track
	for _, t := range r.tracks {
		if t.LibraryID == libraryID && t.AlbumFolder == albumFolder {
			out = append(out, t)
		}
	}
	return out, nil
}

func (r *FakeMusicTrackRepository) ListAlbumFolders(_ context.Context, libraryID shared.LibraryID) ([]string, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := map[string]bool{}
	var out []string
	for _, t := range r.tracks {
		if t.LibraryID == libraryID && t.AlbumFolder != "" && !seen[t.AlbumFolder] {
			seen[t.AlbumFolder] = true
			out = append(out, t.AlbumFolder)
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

// ListByArtistCursor is the keyset form of ListByArtist. See ListByLibraryCursor.
func (r *FakeMusicTrackRepository) ListByArtistCursor(_ context.Context, artist string, cursor *music.ArtistTrackCursor, limit int) ([]music.Track, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []music.Track
	for _, t := range r.tracks {
		if t.AlbumArtist == artist || t.Artist == artist {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return artistTrackCursorLess(out[i], out[j]) })
	if cursor != nil {
		filtered := out[:0:0]
		for _, t := range out {
			if artistTrackCursorLess(music.Track{Album: cursor.Album, DiskNumber: cursor.DiskNumber, TrackNumber: cursor.TrackNumber, ID: cursor.ID}, t) {
				filtered = append(filtered, t)
			}
		}
		out = filtered
	}
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}

func (r *FakeMusicTrackRepository) Search(_ context.Context, libraryID shared.LibraryID, query string, limit int) ([]music.Track, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	q := strings.ToLower(query)
	var out []music.Track
	for _, t := range r.tracks {
		// An empty libraryID searches across every library (mirrors the
		// postgres repo's cross-library fallback for the global search bar).
		if libraryID != "" && t.LibraryID != libraryID {
			continue
		}
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
	// Group by album_folder (authoritative identity), then pick the
	// most frequent display values among the folder's tracks.
	type folderAgg struct {
		folder   string
		albumCnt map[string]int
		artCnt   map[string]int
		minYear  int
		years    int
		trackCnt int
		duration float64
		cover    string
	}
	m := make(map[string]*folderAgg)
	for _, t := range r.tracks {
		if t.LibraryID != libraryID {
			continue
		}
		agg, ok := m[t.AlbumFolder]
		if !ok {
			agg = &folderAgg{
				folder:   t.AlbumFolder,
				albumCnt: map[string]int{},
				artCnt:   map[string]int{},
			}
			m[t.AlbumFolder] = agg
		}
		if t.Album != "" {
			agg.albumCnt[t.Album]++
		}
		// Prefer album_artist; fall back to artist so compilations with
		// empty album_artist still get a display string.
		a := t.AlbumArtist
		if a == "" {
			a = t.Artist
		}
		if a != "" {
			agg.artCnt[a]++
		}
		if t.Year > 0 && (agg.years == 0 || t.Year < agg.minYear) {
			agg.minYear = t.Year
		}
		if t.Year > 0 {
			agg.years++
		}
		agg.trackCnt++
		agg.duration += t.Duration
		// Même préférence que le repo postgres : la pochette EMBARQUÉE dans le
		// fichier bat celle récupérée en externe. Le fake prenait « la première
		// non vide », donc il ne pouvait pas voir le bug de préséance qui a fait
		// afficher la pochette de Mixed Signals sur deux albums Neon Harbor.
		if t.CoverArtURL != "" && (agg.cover == "" || preferredCoverArt(t.CoverArtURL, agg.cover)) {
			agg.cover = t.CoverArtURL
		}
	}
	out := make([]music.Album, 0, len(m))
	for _, agg := range m {
		out = append(out, music.Album{
			// Mirror the postgres repo: expose the short opaque album id, not
			// the raw folder path.
			ID:          music.AlbumID(agg.folder),
			Title:       modeString(agg.albumCnt),
			Artist:      modeString(agg.artCnt),
			Year:        agg.minYear,
			CoverArtURL: agg.cover,
			TrackCount:  agg.trackCnt,
			Duration:    agg.duration,
		})
	}
	return out, nil
}

// embeddedArtPrefix est le préfixe des URLs de pochette lues dans le fichier
// audio lui-même, par opposition à /api/v1/music/album-art/<hash> qui désigne
// une image récupérée auprès d'une source externe.
const embeddedArtPrefix = "/api/v1/music/tracks/"

// preferredCoverArt dit si candidate doit remplacer current : seule une pochette
// embarquée détrône une pochette externe. Entre deux du même genre on garde la
// plus petite lexicographiquement, pour que le résultat soit déterministe comme
// le MIN() du repo postgres.
func preferredCoverArt(candidate, current string) bool {
	candEmbedded := strings.HasPrefix(candidate, embeddedArtPrefix)
	curEmbedded := strings.HasPrefix(current, embeddedArtPrefix)
	if candEmbedded != curEmbedded {
		return candEmbedded
	}
	return candidate < current
}

// modeString returns the key with the highest count. Ties are broken
// lexicographically (stable) so tests don't see order flakiness.
func modeString(counts map[string]int) string {
	best := ""
	bestN := 0
	for k, n := range counts {
		if n > bestN || (n == bestN && k < best) {
			best = k
			bestN = n
		}
	}
	return best
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
		t.AlbumFolder = filepath.Dir(newPath)
		r.tracks[id] = t
	}
	return nil
}

func (r *FakeMusicTrackRepository) ListAllAlbums(_ context.Context, libraryID shared.LibraryID) ([]music.AlbumKey, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	// One entry per folder. cover_art_url state is NOT considered here; the
	// caller decides what to fetch by checking the on-disk file. Display
	// album/artist are the folder's majority.
	type folderAgg struct {
		folder      string
		albumCnt    map[string]int
		artCnt      map[string]int
		hasEmbedded bool
	}
	m := make(map[string]*folderAgg)
	for _, t := range r.tracks {
		if t.LibraryID != libraryID {
			continue
		}
		agg, ok := m[t.AlbumFolder]
		if !ok {
			agg = &folderAgg{folder: t.AlbumFolder, albumCnt: map[string]int{}, artCnt: map[string]int{}}
			m[t.AlbumFolder] = agg
		}
		if t.Album != "" {
			agg.albumCnt[t.Album]++
		}
		a := t.AlbumArtist
		if a == "" {
			a = t.Artist
		}
		if a != "" {
			agg.artCnt[a]++
		}
		if strings.HasPrefix(t.CoverArtURL, embeddedArtPrefix) {
			agg.hasEmbedded = true
		}
	}
	var out []music.AlbumKey
	for _, agg := range m {
		album := modeString(agg.albumCnt)
		artist := modeString(agg.artCnt)
		// Same filter as the postgres repo: skip folders we cannot
		// represent as (artist, album) for a downstream fetcher.
		if album == "" || artist == "" {
			continue
		}
		out = append(out, music.AlbumKey{
			AlbumFolder:    agg.folder,
			Album:          album,
			Artist:         artist,
			HasEmbeddedArt: agg.hasEmbedded,
		})
	}
	return out, nil
}

func (r *FakeMusicTrackRepository) SetTrackCoverArtURL(_ context.Context, trackID, url string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tracks[trackID]
	if !ok {
		return shared.ErrNotFound
	}
	t.CoverArtURL = url
	r.tracks[trackID] = t
	return nil
}

func (r *FakeMusicTrackRepository) SetAlbumCoverArtURL(_ context.Context, libraryID shared.LibraryID, albumFolder, url string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, t := range r.tracks {
		if t.LibraryID != libraryID || t.AlbumFolder != albumFolder {
			continue
		}
		// Comme le repo postgres : une pochette embarquée n'est jamais écrasée.
		if strings.HasPrefix(t.CoverArtURL, embeddedArtPrefix) {
			continue
		}
		t.CoverArtURL = url
		r.tracks[id] = t
	}
	return nil
}
