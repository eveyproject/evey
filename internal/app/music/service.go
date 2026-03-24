package music

import (
	"context"

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/music"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

var audioExts = map[string]string{
	".mp3":  "audio/mpeg",
	".flac": "audio/flac",
	".m4a":  "audio/mp4",
	".ogg":  "audio/ogg",
	".opus": "audio/ogg",
	".wav":  "audio/wav",
	".aac":  "audio/aac",
}

// Service wraps CommandService + QueryService for backward compatibility.
type Service struct {
	cmd   *CommandService
	query *QueryService
}

func NewService(tracks music.TrackRepository, bus shared.EventBus, libRepo library.LibraryQueryRepo) *Service {
	cmd := NewCommandService(tracks, bus, libRepo)
	q := NewQueryService(tracks)
	return &Service{cmd: cmd, query: q}
}

func (s *Service) CommandService() *CommandService { return s.cmd }
func (s *Service) QueryService() *QueryService     { return s.query }

func (s *Service) ScanDirectory(ctx context.Context, libraryID, dir string) (int, error) {
	return s.cmd.ScanDirectory(ctx, libraryID, dir)
}

func (s *Service) DeleteTrack(ctx context.Context, trackID string) error {
	return s.cmd.DeleteTrack(ctx, trackID)
}

func (s *Service) GetTrack(ctx context.Context, trackID string) (*music.Track, error) {
	return s.query.GetTrack(ctx, trackID)
}

func (s *Service) GetTrackArt(ctx context.Context, trackID string) (data []byte, mimeType string, err error) {
	return s.query.GetTrackArt(ctx, trackID)
}

func (s *Service) ListTracks(ctx context.Context, libraryID string, limit, offset int, search string) ([]music.Track, int, error) {
	return s.query.ListTracks(ctx, libraryID, limit, offset, search)
}

func (s *Service) GetAlbums(ctx context.Context, libraryID string) ([]music.Album, error) {
	return s.query.GetAlbums(ctx, libraryID)
}

func (s *Service) GetArtists(ctx context.Context, libraryID string) ([]music.Artist, error) {
	return s.query.GetArtists(ctx, libraryID)
}

func (s *Service) Search(ctx context.Context, query string, limit int) ([]music.Track, error) {
	return s.query.Search(ctx, query, limit)
}

func (s *Service) StreamTrack(ctx context.Context, trackID string) (filePath string, mimeType string, err error) {
	return s.query.StreamTrack(ctx, trackID)
}
