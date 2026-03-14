package music

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/dhowden/tag"
	"github.com/google/uuid"
	"github.com/gravity-zero/evey/internal/domain/music"
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

type Service struct {
	tracks music.TrackRepository
}

func NewService(tracks music.TrackRepository) *Service {
	return &Service{tracks: tracks}
}

func (s *Service) ScanDirectory(ctx context.Context, libraryID, dir string) (int, error) {
	count := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if info.IsDir() {
			return nil
		}
		mimeType, ok := audioExts[filepath.Ext(path)]
		if !ok {
			return nil
		}
		t, parseErr := parseTrack(path, libraryID, mimeType, info.Size())
		if parseErr != nil {
			slog.Warn("music scan: parse failed", slog.String("path", path), slog.Any("err", parseErr))
			// still add a minimal track entry
			t = &music.Track{
				ID:        uuid.New().String(),
				LibraryID: libraryID,
				Title:     filepath.Base(path),
				FilePath:  path,
				FileSize:  info.Size(),
				MimeType:  mimeType,
			}
		}
		if upsertErr := s.tracks.Upsert(ctx, *t); upsertErr != nil {
			slog.Warn("music scan: upsert failed", slog.String("path", path), slog.Any("err", upsertErr))
			return nil
		}
		count++
		return nil
	})
	return count, err
}

func parseTrack(path, libraryID, mimeType string, fileSize int64) (*music.Track, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil, err
	}

	trackNum, _ := m.Track()
	diskNum, _ := m.Disc()

	artist := m.Artist()
	albumArtist := m.AlbumArtist()
	if albumArtist == "" {
		albumArtist = artist
	}

	return &music.Track{
		ID:          uuid.New().String(),
		LibraryID:   libraryID,
		Title:       m.Title(),
		Artist:      artist,
		Album:       m.Album(),
		AlbumArtist: albumArtist,
		Year:        m.Year(),
		TrackNumber: trackNum,
		DiskNumber:  diskNum,
		Duration:    0, // tag lib doesn't expose duration directly
		FilePath:    path,
		FileSize:    fileSize,
		MimeType:    mimeType,
		Genre:       m.Genre(),
	}, nil
}

func (s *Service) ListTracks(ctx context.Context, libraryID string, limit, offset int) ([]music.Track, int, error) {
	return s.tracks.ListByLibrary(ctx, libraryID, limit, offset)
}

func (s *Service) GetAlbums(ctx context.Context, libraryID string) ([]music.Album, error) {
	return s.tracks.GetAlbums(ctx, libraryID)
}

func (s *Service) GetArtists(ctx context.Context, libraryID string) ([]music.Artist, error) {
	return s.tracks.GetArtists(ctx, libraryID)
}

func (s *Service) Search(ctx context.Context, query string, limit int) ([]music.Track, error) {
	return s.tracks.Search(ctx, query, limit)
}

func (s *Service) StreamTrack(ctx context.Context, trackID string) (filePath string, mimeType string, err error) {
	t, err := s.tracks.GetByID(ctx, trackID)
	if err != nil {
		return "", "", err
	}
	if t == nil {
		return "", "", os.ErrNotExist
	}
	return t.FilePath, t.MimeType, nil
}
