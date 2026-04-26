package music

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/dhowden/tag"
	"github.com/evey-project/evey/internal/domain/music"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type QueryService struct {
	tracks  music.TrackRepository
	scanner music.MusicScanner
}

func NewQueryService(tracks music.TrackRepository, scanner music.MusicScanner) *QueryService {
	return &QueryService{tracks: tracks, scanner: scanner}
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

func (s *QueryService) ListTracks(ctx context.Context, libraryID shared.LibraryID, limit, offset int, search string) ([]music.Track, int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.music.ListTracks")
	defer span.End()
	return s.tracks.ListByLibrary(ctx, libraryID, limit, offset, search)
}

func (s *QueryService) ListTracksByAlbum(ctx context.Context, libraryID shared.LibraryID, album string) ([]music.Track, error) {
	ctx, span := tracing.StartSpan(ctx, "app.music.ListTracksByAlbum")
	defer span.End()
	return s.tracks.ListByLibraryAndAlbum(ctx, libraryID, album)
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
