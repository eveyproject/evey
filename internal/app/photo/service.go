package photo

import (
	"context"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gravity-zero/evey/internal/domain/photo"
	"github.com/gravity-zero/evey/internal/pkg/id"
)

var imageExts = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".heic": "image/heic",
	".webp": "image/webp",
	".gif":  "image/gif",
	".tiff": "image/tiff",
	".tif":  "image/tiff",
	".bmp":  "image/bmp",
}

type Service struct {
	repo photo.Repository
}

func NewService(repo photo.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) ScanDirectory(ctx context.Context, libraryID, dir string) (int, error) {
	count := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			slog.Warn("photo walk error", slog.String("path", path), slog.Any("err", walkErr))
			return nil
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		mime, ok := imageExts[ext]
		if !ok {
			return nil
		}

		title := strings.TrimSuffix(info.Name(), filepath.Ext(info.Name()))
		album := filepath.Base(filepath.Dir(path))
		if album == "." || album == dir {
			album = ""
		}

		width, height := imageDimensions(path)

		now := time.Now()
		p := photo.Photo{
			ID:        id.New("photo-"),
			LibraryID: libraryID,
			Title:     title,
			FilePath:  path,
			FileSize:  info.Size(),
			MimeType:  mime,
			Width:     width,
			Height:    height,
			Album:     album,
			CreatedAt: now,
			UpdatedAt: now,
		}

		if err := s.repo.Upsert(ctx, p); err != nil {
			slog.Warn("photo upsert failed", slog.String("path", path), slog.Any("err", err))
			return nil
		}
		count++
		return nil
	})
	return count, err
}

func (s *Service) ListPhotos(ctx context.Context, libraryID string, limit, offset int) ([]photo.Photo, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	return s.repo.ListByLibrary(ctx, libraryID, limit, offset)
}

func (s *Service) ListByAlbum(ctx context.Context, libraryID, album string, limit, offset int) ([]photo.Photo, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	return s.repo.ListByAlbum(ctx, libraryID, album, limit, offset)
}

func (s *Service) GetAlbums(ctx context.Context, libraryID string) ([]photo.Album, error) {
	return s.repo.GetAlbums(ctx, libraryID)
}

func (s *Service) GetPhoto(ctx context.Context, id string) (*photo.Photo, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) ServePhoto(ctx context.Context, photoID string) (filePath, mimeType string, err error) {
	p, err := s.repo.GetByID(ctx, photoID)
	if err != nil {
		return "", "", err
	}
	if p == nil {
		return "", "", nil
	}
	return p.FilePath, p.MimeType, nil
}

func (s *Service) Search(ctx context.Context, libraryID, query string, limit int) ([]photo.Photo, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.Search(ctx, libraryID, query, limit)
}

func imageDimensions(path string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}
