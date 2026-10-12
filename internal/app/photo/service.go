package photo

import (
	"context"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/photo"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// Service wraps CommandService + QueryService for backward compatibility.
type Service struct {
	cmd   *CommandService
	query *QueryService
}

func NewService(photos photo.Repository, bus shared.EventBus, libRepo library.LibraryQueryRepo, scanner photo.PhotoScanner) *Service {
	cmd := NewCommandService(photos, bus, libRepo, scanner)
	q := NewQueryService(photos)
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

func (s *Service) CountPhotos(ctx context.Context, libraryID shared.LibraryID) (int, error) {
	return s.query.CountPhotos(ctx, libraryID)
}

func (s *Service) ListPhotos(ctx context.Context, libraryID shared.LibraryID, limit, offset int) ([]photo.Photo, int, error) {
	return s.query.ListPhotos(ctx, libraryID, limit, offset)
}

func (s *Service) ListPhotosByAlbum(ctx context.Context, libraryID shared.LibraryID, album string, limit, offset int) ([]photo.Photo, int, error) {
	return s.query.ListPhotosByAlbum(ctx, libraryID, album, limit, offset)
}

func (s *Service) GetAlbums(ctx context.Context, libraryID shared.LibraryID) ([]photo.Album, error) {
	return s.query.GetAlbums(ctx, libraryID)
}

func (s *Service) GetPhoto(ctx context.Context, photoID string) (*photo.Photo, error) {
	return s.query.GetPhoto(ctx, photoID)
}

func (s *Service) Search(ctx context.Context, libraryID shared.LibraryID, query string, limit int) ([]photo.Photo, error) {
	return s.query.Search(ctx, libraryID, query, limit)
}
