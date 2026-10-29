package music

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/dhowden/tag"
	"github.com/eveyproject/evey/internal/domain/music"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type QueryService struct {
	tracks      music.TrackReader
	scanner     music.MusicScanner
	searchIndex music.MusicSearchIndex
}

func NewQueryService(tracks music.TrackReader, scanner music.MusicScanner) *QueryService {
	return &QueryService{tracks: tracks, scanner: scanner}
}

// WithSearchIndex wires the full-text music search index (typically the
// resilient Meili+SQL wrapper). When nil, Search falls back to the repo's SQL
// ILIKE directly.
func (s *QueryService) WithSearchIndex(idx music.MusicSearchIndex) *QueryService {
	s.searchIndex = idx
	return s
}

func (s *QueryService) GetTrack(ctx context.Context, trackID string) (*music.Track, error) {
	ctx, span := tracing.StartSpan(ctx, "app.music.GetTrack")
	defer span.End()
	t, err := s.tracks.GetByID(ctx, trackID)
	if errors.Is(err, shared.ErrNotFound) {
		return nil, nil
	}
	return t, err
}

// GetTracksByIDs returns the tracks among ids, skipping an id with no track.
func (s *QueryService) GetTracksByIDs(ctx context.Context, ids []string) ([]music.Track, error) {
	ctx, span := tracing.StartSpan(ctx, "app.music.GetTracksByIDs")
	defer span.End()
	return s.tracks.GetByIDs(ctx, ids)
}

func (s *QueryService) GetTrackArt(ctx context.Context, trackID string) (data []byte, mimeType string, err error) {
	ctx, span := tracing.StartSpan(ctx, "app.music.GetTrackArt")
	defer span.End()
	t, err := s.tracks.GetByID(ctx, trackID)
	if errors.Is(err, shared.ErrNotFound) {
		return nil, "", os.ErrNotExist
	}
	if err != nil {
		return nil, "", err
	}
	if t == nil {
		return nil, "", os.ErrNotExist
	}
	f, err := s.scanner.OpenFile(t.FilePath)
	if err != nil {
		return nil, "", os.ErrNotExist
	}
	defer f.Close()
	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil, "", os.ErrNotExist
	}
	pic := m.Picture()
	if pic == nil {
		return nil, "", os.ErrNotExist
	}
	mt := pic.MIMEType
	if mt == "" {
		mt = "image/jpeg"
	}
	return pic.Data, mt, nil
}

// CountTracks returns the library's track count without reading its rows.
func (s *QueryService) CountTracks(ctx context.Context, libraryID shared.LibraryID) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.music.CountTracks")
	defer span.End()
	return s.tracks.CountByLibrary(ctx, libraryID)
}

func (s *QueryService) ListTracks(ctx context.Context, libraryID shared.LibraryID, limit, offset int, search string) ([]music.Track, int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.music.ListTracks")
	defer span.End()
	return s.tracks.ListByLibrary(ctx, libraryID, limit, offset, search)
}

// ListTracksCursor is the keyset form of ListTracks: no total, page forward
// only via the cursor of the last item returned. nil cursor = first page.
func (s *QueryService) ListTracksCursor(ctx context.Context, libraryID shared.LibraryID, search string, cursor *music.TrackCursor, limit int) ([]music.Track, error) {
	ctx, span := tracing.StartSpan(ctx, "app.music.ListTracksCursor")
	defer span.End()
	return s.tracks.ListByLibraryCursor(ctx, libraryID, search, cursor, limit)
}

// ShuffleTracks draws up to limit random tracks from the whole library.
func (s *QueryService) ShuffleTracks(ctx context.Context, libraryID shared.LibraryID, limit int) ([]music.Track, error) {
	ctx, span := tracing.StartSpan(ctx, "app.music.ShuffleTracks")
	defer span.End()
	return s.tracks.SampleByLibrary(ctx, libraryID, limit)
}

func (s *QueryService) ListTracksByAlbum(ctx context.Context, libraryID shared.LibraryID, album string) ([]music.Track, error) {
	ctx, span := tracing.StartSpan(ctx, "app.music.ListTracksByAlbum")
	defer span.End()
	return s.tracks.ListByLibraryAndAlbum(ctx, libraryID, album)
}

// ListTracksByAlbumID returns every track of the album identified by the short
// opaque albumID (music.AlbumID). The id is one-way, so we resolve it by hashing
// each known album folder and matching - the folder set is small and bounded.
// Returns an empty slice (not an error) when no album matches.
func (s *QueryService) ListTracksByAlbumID(ctx context.Context, libraryID shared.LibraryID, albumID string) ([]music.Track, error) {
	ctx, span := tracing.StartSpan(ctx, "app.music.ListTracksByAlbumID")
	defer span.End()
	folders, err := s.tracks.ListAlbumFolders(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	for _, f := range folders {
		if music.AlbumID(f) == albumID {
			return s.tracks.ListByAlbumFolder(ctx, libraryID, f)
		}
	}
	return nil, nil
}

func (s *QueryService) GetAlbums(ctx context.Context, libraryID shared.LibraryID) ([]music.Album, error) {
	ctx, span := tracing.StartSpan(ctx, "app.music.GetAlbums")
	defer span.End()
	return s.tracks.GetAlbums(ctx, libraryID)
}

func (s *QueryService) GetArtists(ctx context.Context, libraryID shared.LibraryID) ([]music.Artist, error) {
	ctx, span := tracing.StartSpan(ctx, "app.music.GetArtists")
	defer span.End()
	return s.tracks.GetArtists(ctx, libraryID)
}

func (s *QueryService) Search(ctx context.Context, libraryID shared.LibraryID, query string, limit int) ([]music.Track, error) {
	ctx, span := tracing.StartSpan(ctx, "app.music.Search")
	defer span.End()
	// Prefer the full-text index (typo tolerance, relevance, cross-library);
	// it carries its own SQL fallback. Without it, query SQL directly.
	if s.searchIndex != nil {
		return s.searchIndex.Search(ctx, libraryID, query, limit)
	}
	return s.tracks.Search(ctx, libraryID, query, limit)
}

func (s *QueryService) StreamTrack(ctx context.Context, trackID string) (filePath string, mimeType string, err error) {
	ctx, span := tracing.StartSpan(ctx, "app.music.StreamTrack")
	defer span.End()
	t, err := s.tracks.GetByID(ctx, trackID)
	if errors.Is(err, shared.ErrNotFound) {
		return "", "", os.ErrNotExist
	}
	if err != nil {
		return "", "", err
	}
	if t == nil {
		return "", "", os.ErrNotExist
	}
	return t.FilePath, t.MimeType, nil
}

func (s *QueryService) StreamTrackURL(trackID string) string {
	return fmt.Sprintf("/api/v1/music/tracks/%s/stream", trackID)
}
