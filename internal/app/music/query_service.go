package music

import (
	"context"
	"fmt"
	"os"

	"github.com/dhowden/tag"
	"github.com/gravity-zero/evey/internal/domain/music"
)

type QueryService struct {
	tracks  music.TrackRepository
	scanner music.MusicScanner
}

func NewQueryService(tracks music.TrackRepository, scanner music.MusicScanner) *QueryService {
	return &QueryService{tracks: tracks, scanner: scanner}
}

func (s *QueryService) GetTrack(ctx context.Context, trackID string) (*music.Track, error) {
	return s.tracks.GetByID(ctx, trackID)
}

func (s *QueryService) GetTrackArt(ctx context.Context, trackID string) (data []byte, mimeType string, err error) {
	t, err := s.tracks.GetByID(ctx, trackID)
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

func (s *QueryService) ListTracks(ctx context.Context, libraryID string, limit, offset int, search string) ([]music.Track, int, error) {
	return s.tracks.ListByLibrary(ctx, libraryID, limit, offset, search)
}

func (s *QueryService) GetAlbums(ctx context.Context, libraryID string) ([]music.Album, error) {
	return s.tracks.GetAlbums(ctx, libraryID)
}

func (s *QueryService) GetArtists(ctx context.Context, libraryID string) ([]music.Artist, error) {
	return s.tracks.GetArtists(ctx, libraryID)
}

func (s *QueryService) Search(ctx context.Context, query string, limit int) ([]music.Track, error) {
	return s.tracks.Search(ctx, query, limit)
}

func (s *QueryService) StreamTrack(ctx context.Context, trackID string) (filePath string, mimeType string, err error) {
	t, err := s.tracks.GetByID(ctx, trackID)
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
