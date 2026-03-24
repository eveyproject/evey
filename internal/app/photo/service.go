package photo

import (
	"context"

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/photo"
	"github.com/gravity-zero/evey/internal/domain/shared"
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

// Service wraps CommandService + QueryService for backward compatibility.
type Service struct {
	cmd   *CommandService
	query *QueryService
}

func NewService(repo photo.Repository, bus shared.EventBus, libRepo library.LibraryQueryRepo) *Service {
	cmd := NewCommandService(repo, bus, libRepo)
	q := NewQueryService(repo)
	return &Service{cmd: cmd, query: q}
}

func (s *Service) CommandService() *CommandService { return s.cmd }
func (s *Service) QueryService() *QueryService     { return s.query }

func (s *Service) ScanDirectory(ctx context.Context, libraryID, dir string) (int, error) {
	return s.cmd.ScanDirectory(ctx, libraryID, dir)
}

func (s *Service) DeletePhoto(ctx context.Context, photoID string) error {
	return s.cmd.DeletePhoto(ctx, photoID)
}

func (s *Service) ListPhotos(ctx context.Context, libraryID string, limit, offset int) ([]photo.Photo, int, error) {
	return s.query.ListPhotos(ctx, libraryID, limit, offset)
}

func (s *Service) GetAlbums(ctx context.Context, libraryID string) ([]photo.Album, error) {
	return s.query.GetAlbums(ctx, libraryID)
}

func (s *Service) GetPhoto(ctx context.Context, id string) (*photo.Photo, error) {
	return s.query.GetPhoto(ctx, id)
}

func (s *Service) Search(ctx context.Context, libraryID, query string, limit int) ([]photo.Photo, error) {
	return s.query.Search(ctx, libraryID, query, limit)
}
