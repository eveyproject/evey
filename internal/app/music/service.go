package music

import (
	"context"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/music"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// Service wraps CommandService + QueryService for backward compatibility.
type Service struct {
	cmd   *CommandService
	query *QueryService
	plays music.PlayRepository
}

func NewService(tracks music.TrackRepository, bus shared.EventBus, libRepo library.LibraryQueryRepo, scanner music.MusicScanner) *Service {
	cmd := NewCommandService(tracks, bus, libRepo, scanner)
	q := NewQueryService(tracks, scanner)
	return &Service{cmd: cmd, query: q}
}

// WithPlayRepository wires per-profile play-count tracking. Optional: when nil,
// RecordPlay is a no-op.
func (s *Service) WithPlayRepository(plays music.PlayRepository) *Service {
	s.plays = plays
	return s
}

// RecordPlay registers one listen of trackID by profileID, feeding the
// playlist generator's taste signal. No-op if play tracking isn't wired.
func (s *Service) RecordPlay(ctx context.Context, profileID, trackID string) error {
	if s.plays == nil {
		return nil
	}
	return s.plays.IncrementPlay(ctx, profileID, trackID)
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

func (s *Service) CountTracks(ctx context.Context, libraryID shared.LibraryID) (int, error) {
	return s.query.CountTracks(ctx, libraryID)
}

func (s *Service) ListTracks(ctx context.Context, libraryID shared.LibraryID, limit, offset int, search string) ([]music.Track, int, error) {
	return s.query.ListTracks(ctx, libraryID, limit, offset, search)
}

func (s *Service) ListTracksByAlbum(ctx context.Context, libraryID shared.LibraryID, album string) ([]music.Track, error) {
	return s.query.ListTracksByAlbum(ctx, libraryID, album)
}

func (s *Service) ListTracksByAlbumID(ctx context.Context, libraryID shared.LibraryID, albumID string) ([]music.Track, error) {
	return s.query.ListTracksByAlbumID(ctx, libraryID, albumID)
}

func (s *Service) GetAlbums(ctx context.Context, libraryID shared.LibraryID) ([]music.Album, error) {
	return s.query.GetAlbums(ctx, libraryID)
}

func (s *Service) GetArtists(ctx context.Context, libraryID shared.LibraryID) ([]music.Artist, error) {
	return s.query.GetArtists(ctx, libraryID)
}

func (s *Service) Search(ctx context.Context, libraryID shared.LibraryID, query string, limit int) ([]music.Track, error) {
	return s.query.Search(ctx, libraryID, query, limit)
}

func (s *Service) StreamTrack(ctx context.Context, trackID string) (filePath string, mimeType string, err error) {
	return s.query.StreamTrack(ctx, trackID)
}

func (s *Service) AlbumArtDir() string { return s.cmd.AlbumArtDir() }

func (s *Service) FetchMissingAlbumArtAllLibraries(ctx context.Context) (int, error) {
	return s.cmd.FetchMissingAlbumArtAllLibraries(ctx)
}

func (s *Service) ReindexAllTracks(ctx context.Context) (int, error) {
	return s.cmd.ReindexAllTracks(ctx)
}
